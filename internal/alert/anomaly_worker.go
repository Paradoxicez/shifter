package alert

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/shifter-io/shifter/internal/audit"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// AnomalyArgs is the River job-args type for the statistical-anomaly
// evaluator. Runs at 1-hour cadence per 06-RESEARCH §Decision C.
//
// One worker dispatches all three anomaly rule kinds (p95, iqr,
// quiet_hour) per cycle so install_tz is loaded ONCE per cycle (D-17
// quiet-window correctness requires install-local time).
type AnomalyArgs struct{}

// Kind returns the unique River job kind. Mirrors the threshold/offline
// workers' pattern; alertWorkerKindFromJobKind in degraded.go will route
// retry-exhaustion to alert_worker_state.worker_kind='anomaly'.
func (AnomalyArgs) Kind() string { return "alert_anomaly" }

// InsertOpts caps retries at 3 (D-22) so a permanently-broken anomaly
// worker hits JobStateDiscarded and trips the degraded flag.
func (AnomalyArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3}
}

// AnomalyWorker evaluates the three D-17 statistical rule kinds with the
// D-16 cold-start gate enforced per-MP, per-cycle.
//
// One cycle:
//  1. Load install_identity.timezone (fresh — Settings change takes effect
//     at next cycle without restart).
//  2. For each of the three anomaly rule kinds: load active rules.
//  3. For each rule: expand scope to a list of target MPs.
//  4. For each target MP: check D-16 eligibility (skip if not eligible).
//  5. Evaluate the rule kind:
//       - anomaly_p95   → P95BaselineForMPAndHour + latest > p95
//       - anomaly_iqr   → IQRBaselineForMP + latest outside [Q1−1.5·IQR, Q3+1.5·IQR]
//       - anomaly_quiet_hour → EvalQuietHour (cross-midnight aware)
//  6. Fire (idempotent via partial unique) or clear (when state returns
//     inside bounds), with audit-in-tx (D-23).
type AnomalyWorker struct {
	river.WorkerDefaults[AnomalyArgs]

	Eng        EvaluateContext
	Rules      *RuleStore
	Alerts     *AlertStore
	WorkerStat *WorkerStateStore
}

// Work runs one anomaly evaluation cycle.
func (w *AnomalyWorker) Work(ctx context.Context, _ *river.Job[AnomalyArgs]) error {
	start := time.Now()
	state := &RunState{}

	defer func() {
		state.DurationMS = int32(time.Since(start).Milliseconds())
		if w.WorkerStat == nil {
			return
		}
		tx, err := w.Eng.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		if err != nil {
			return
		}
		_ = w.WorkerStat.UpsertWorkerState(ctx, tx, "anomaly", state)
		_ = tx.Commit(ctx)
	}()

	// Pull install_tz fresh each cycle. RESEARCH Open Question #4: read
	// per-cycle so a Settings change takes effect on the next eval, not
	// at process restart.
	installTZ := readInstallTimezone(ctx, w.Eng)
	installName := readInstallDisplayName(ctx, w.Eng)
	now := time.Now().UTC()

	for _, kind := range []string{"anomaly_p95", "anomaly_iqr", "anomaly_quiet_hour"} {
		rules, err := w.Rules.ListActiveRulesByKind(ctx, kind)
		if err != nil {
			state.RecordErr(err)
			continue
		}
		for _, rule := range rules {
			targets, err := expandScope(ctx, w.Eng, rule)
			if err != nil {
				state.RecordErr(err)
				continue
			}
			for _, target := range targets {
				// D-16 cold-start gate — skip MPs without ≥ 21 days of
				// history. Applies to ALL three anomaly rule kinds (the
				// statistical baseline isn't meaningful on fresh MPs).
				eligible, err := IsMPEligibleForAnomaly(ctx, w.Eng.Queries, target.MeteringPointID)
				if err != nil {
					state.RecordErr(err)
					continue
				}
				if !eligible {
					continue
				}
				state.RulesEvaluated++

				// Cool-down (D-05) gates RE-FIRE only — auto-clear runs
				// inside the cooldown window so a transient anomaly that
				// resolves quickly doesn't leave a stuck firing alert.
				inCooldown := !rule.CooledDown(now)

				var (
					breach    bool
					value     float64
					threshold float64
					extra     BuildPayloadInput
				)
				switch kind {
				case "anomaly_p95":
					breach, value, threshold, extra, err = w.evalP95(ctx, target.MeteringPointID)
				case "anomaly_iqr":
					breach, value, threshold, extra, err = w.evalIQR(ctx, target.MeteringPointID)
				case "anomaly_quiet_hour":
					var atT time.Time
					breach, value, atT, err = EvalQuietHour(ctx, w.Eng.Queries, target.MeteringPointID, rule, installTZ)
					threshold = derefFloat(rule.FlowThreshold, 0.0)
					_ = atT
				}
				if err != nil {
					state.RecordErr(err)
					continue
				}

				existing, _ := w.Alerts.ListFiringByRuleTarget(ctx, rule.ID, target.MeteringPointID)
				switch {
				case breach && existing == nil:
					if inCooldown {
						// D-05: suppress new fire while cooldown active.
						continue
					}
					if err := w.fireAnomaly(ctx, rule, target, kind, value, threshold, installName, extra); err != nil {
						if errors.Is(err, ErrDuplicateFire) {
							continue
						}
						state.RecordErr(err)
						continue
					}
					state.FiresEmitted++
				case !breach && existing != nil:
					if err := autoClearAlert(ctx, w.Eng, w.Alerts, *existing); err != nil {
						state.RecordErr(err)
						continue
					}
					state.Cleared++
				}
			}
		}
	}
	return nil
}

// evalP95 computes the trailing-30d P95 of instant_value for this MP at
// the current hour-of-day; reports a breach when the latest measurement
// strictly exceeds it. Returns (false, 0, ...) on no-data / NULL baseline
// so the worker silently skips MPs without enough samples in this hour
// bucket (cold-start already ensures 21d of history overall).
func (w *AnomalyWorker) evalP95(ctx context.Context, mpID uuid.UUID) (breach bool, value, threshold float64, extra BuildPayloadInput, err error) {
	// Use UTC hour-of-day to match Postgres EXTRACT(HOUR FROM timestamptz)
	// which returns the hour in the session timezone (the DB sessions used
	// by sqlc default to UTC). Using time.Now().Hour() (local) would
	// query the wrong hour bucket on installs whose deploy host is not
	// in UTC (e.g. +07 dev machine; +08 production servers).
	hour := time.Now().UTC().Hour()
	p95, perr := w.Eng.Queries.P95BaselineForMPAndHour(ctx, sqlc.P95BaselineForMPAndHourParams{
		MeteringPointID: pgUUID(mpID),
		HourOfDay:       int32(hour),
	})
	if perr != nil {
		// percentile_cont on zero rows produces NULL → scan fails.
		// Treat as "no baseline yet" — silent skip.
		if w.Eng.Log != nil {
			w.Eng.Log.Debug("anomaly_p95: baseline query failed", "mp", mpID, "hour", hour, "err", perr)
		}
		return false, 0, 0, BuildPayloadInput{}, nil
	}

	latest, lerr := w.Eng.Queries.LatestInstantValueForMPAnomaly(ctx, pgUUID(mpID))
	if lerr != nil {
		if errors.Is(lerr, pgx.ErrNoRows) {
			return false, 0, 0, BuildPayloadInput{}, nil
		}
		return false, 0, 0, BuildPayloadInput{}, lerr
	}
	v, ok := numericToFloat(latest.InstantValue)
	if !ok {
		return false, 0, 0, BuildPayloadInput{}, nil
	}

	days := int32(30)
	bucket := fmt.Sprintf("%02d:00", hour)
	extra = BuildPayloadInput{
		P95Baseline:        &p95,
		BaselineWindowDays: &days,
		TimeOfDayBucket:    &bucket,
	}
	return v > p95, v, p95, extra, nil
}

// evalIQR computes Q1 / Q3 over the trailing 30 days and fires when the
// latest instant_value falls outside [Q1 − 1.5·IQR, Q3 + 1.5·IQR].
// threshold returned is whichever bound was crossed (upper if value above
// upper; lower if below lower).
func (w *AnomalyWorker) evalIQR(ctx context.Context, mpID uuid.UUID) (breach bool, value, threshold float64, extra BuildPayloadInput, err error) {
	row, qerr := w.Eng.Queries.IQRBaselineForMP(ctx, pgUUID(mpID))
	if qerr != nil {
		// Zero rows → NULL → scan fails. Silent skip.
		return false, 0, 0, BuildPayloadInput{}, nil
	}

	iqr := row.Q3 - row.Q1
	lower := row.Q1 - 1.5*iqr
	upper := row.Q3 + 1.5*iqr

	latest, lerr := w.Eng.Queries.LatestInstantValueForMPAnomaly(ctx, pgUUID(mpID))
	if lerr != nil {
		if errors.Is(lerr, pgx.ErrNoRows) {
			return false, 0, 0, BuildPayloadInput{}, nil
		}
		return false, 0, 0, BuildPayloadInput{}, lerr
	}
	v, ok := numericToFloat(latest.InstantValue)
	if !ok {
		return false, 0, 0, BuildPayloadInput{}, nil
	}

	days := int32(30)
	extra = BuildPayloadInput{BaselineWindowDays: &days}

	switch {
	case v > upper:
		return true, v, upper, extra, nil
	case v < lower:
		return true, v, lower, extra, nil
	default:
		return false, v, upper, extra, nil
	}
}

// fireAnomaly opens a tx, inserts the alert + audit row + TouchLastFiredAt
// in the same tx (D-23). RuleKind drives the payload's anomaly-specific
// fields (p95_baseline + baseline_window_days + time_of_day_bucket for
// p95, baseline_window_days only for iqr, none for quiet_hour).
func (w *AnomalyWorker) fireAnomaly(ctx context.Context, rule RuleRecord, target Target,
	kind string, value, threshold float64, installName string, extra BuildPayloadInput) error {
	tx, err := w.Eng.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	in := BuildPayloadInput{
		RuleID:             rule.ID,
		RuleKind:           kind,
		Severity:           rule.Severity,
		Target:             PayloadTarget{EntityType: "metering_point", EntityID: target.MeteringPointID, Label: target.Label},
		Value:              value,
		Threshold:          threshold,
		Comparison:         derefStr(rule.Comparison, "gt"),
		Unit:               derefStr(rule.Unit, ""),
		FiredAt:            time.Now().UTC(),
		InstallName:        installName,
		P95Baseline:        extra.P95Baseline,
		BaselineWindowDays: extra.BaselineWindowDays,
		TimeOfDayBucket:    extra.TimeOfDayBucket,
	}
	payload, err := BuildPayload(in)
	if err != nil {
		return fmt.Errorf("alert: build anomaly payload: %w", err)
	}

	row, err := w.Alerts.InsertAlert(ctx, tx, InsertAlertParams{
		RuleID:           rule.ID,
		RuleKind:         kind,
		Severity:         rule.Severity,
		Payload:          payload,
		TargetEntityType: "metering_point",
		TargetEntityID:   target.MeteringPointID,
	})
	if err != nil {
		return err
	}

	afterMap, _ := jsonbToMap(payload)
	if err := audit.WriteEntry(ctx, tx, audit.Entry{
		Action:     audit.ActionAlertFired,
		EntityType: audit.EntityTypeAlert,
		EntityID:   row.ID,
		After:      afterMap,
	}); err != nil {
		return fmt.Errorf("alert: audit anomaly fire: %w", err)
	}
	if err := w.Rules.TouchLastFiredAt(ctx, tx, rule.ID); err != nil {
		return fmt.Errorf("alert: touch anomaly rule: %w", err)
	}
	return tx.Commit(ctx)
}

// readInstallTimezone returns install_identity.timezone (IANA name) or
// "UTC" if the row isn't seeded yet / read fails. The anomaly worker
// re-reads this every cycle so Settings → Timezone changes take effect
// at the next cycle without process restart.
func readInstallTimezone(ctx context.Context, eng EvaluateContext) string {
	if eng.Pool == nil {
		return "UTC"
	}
	var tz string
	err := eng.Pool.QueryRow(ctx, `SELECT timezone FROM install_identity WHERE id = 1`).Scan(&tz)
	if err != nil || tz == "" {
		return "UTC"
	}
	return tz
}

// derefFloat returns *p or fallback if p is nil.
func derefFloat(p *float64, fallback float64) float64 {
	if p == nil {
		return fallback
	}
	return *p
}
