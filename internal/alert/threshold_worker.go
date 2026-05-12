package alert

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"github.com/shifter-io/shifter/internal/audit"
)

// ThresholdInstantaneousArgs is the River job-args type for the per-uplink
// threshold evaluator. RESEARCH §Decision C cadence: 1 minute.
type ThresholdInstantaneousArgs struct{}

// Kind returns the unique River job kind. Must match the
// alertWorkerKindFromJobKind switch in degraded.go.
func (ThresholdInstantaneousArgs) Kind() string { return "alert_threshold_instantaneous" }

// InsertOpts caps retries at 3 so a permanently-broken threshold worker hits
// JobStateDiscarded and trips the degraded flag (D-22).
func (ThresholdInstantaneousArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3}
}

// ThresholdHourlyArgs / ThresholdDailyArgs mirror Instantaneous with
// different cadences (15min / 1h) consumed from measurement_hourly /
// measurement_daily CAGGs respectively (D-02).
type ThresholdHourlyArgs struct{}

func (ThresholdHourlyArgs) Kind() string { return "alert_threshold_hourly" }
func (ThresholdHourlyArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3}
}

type ThresholdDailyArgs struct{}

func (ThresholdDailyArgs) Kind() string { return "alert_threshold_daily" }
func (ThresholdDailyArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3}
}

// ThresholdInstantaneousWorker pulls the most-recent measurement per MP and
// compares against the rule's high/low bound. Fires when value breaches,
// auto-clears when value returns inside bounds. Honors per-rule cooldown
// (D-05) and idempotent fire (partial unique index alert_firing_unique_idx).
type ThresholdInstantaneousWorker struct {
	river.WorkerDefaults[ThresholdInstantaneousArgs]

	Eng        EvaluateContext
	Rules      *RuleStore
	Alerts     *AlertStore
	WorkerStat *WorkerStateStore
}

// Work evaluates every active threshold_instantaneous rule once.
func (w *ThresholdInstantaneousWorker) Work(ctx context.Context, _ *river.Job[ThresholdInstantaneousArgs]) error {
	return runThresholdCycle(ctx, thresholdCycleDeps{
		Eng:        w.Eng,
		Rules:      w.Rules,
		Alerts:     w.Alerts,
		WorkerStat: w.WorkerStat,
		RuleKind:   "threshold_instantaneous",
		WorkerKind: "threshold_instantaneous",
		Latest: func(ctx context.Context, mpID uuid.UUID) (float64, time.Time, error) {
			row, err := w.Eng.Queries.GetLatestMeasurementForMP(ctx, pgUUID(mpID))
			if err != nil {
				return 0, time.Time{}, err
			}
			val, ok := numericToFloat(row.InstantValue)
			if !ok {
				return 0, time.Time{}, errInstantNotNumeric
			}
			return val, row.Time.Time, nil
		},
	})
}

// ThresholdHourlyWorker pulls the most-recent 1-hour bucket from
// measurement_hourly (CAGG) and compares avg_instant against the bound.
type ThresholdHourlyWorker struct {
	river.WorkerDefaults[ThresholdHourlyArgs]

	Eng        EvaluateContext
	Rules      *RuleStore
	Alerts     *AlertStore
	WorkerStat *WorkerStateStore
}

func (w *ThresholdHourlyWorker) Work(ctx context.Context, _ *river.Job[ThresholdHourlyArgs]) error {
	return runThresholdCycle(ctx, thresholdCycleDeps{
		Eng:        w.Eng,
		Rules:      w.Rules,
		Alerts:     w.Alerts,
		WorkerStat: w.WorkerStat,
		RuleKind:   "threshold_hourly",
		WorkerKind: "threshold_hourly",
		Latest: func(ctx context.Context, mpID uuid.UUID) (float64, time.Time, error) {
			row, err := w.Eng.Queries.GetLatestHourlyForMP(ctx, pgUUID(mpID))
			if err != nil {
				return 0, time.Time{}, err
			}
			// CAGG-returned aggregates land as float64 directly (no
			// pgtype.Numeric wrap) — sqlc reads the DOUBLE PRECISION
			// column straight into the Go scalar.
			return row.AvgInstant, bucketTime(row.Bucket), nil
		},
	})
}

// ThresholdDailyWorker pulls the most-recent 1-day bucket from
// measurement_daily and compares the day's sum_consumption (cumulative
// delta) against the bound — per D-02 daily checks the total flow.
type ThresholdDailyWorker struct {
	river.WorkerDefaults[ThresholdDailyArgs]

	Eng        EvaluateContext
	Rules      *RuleStore
	Alerts     *AlertStore
	WorkerStat *WorkerStateStore
}

func (w *ThresholdDailyWorker) Work(ctx context.Context, _ *river.Job[ThresholdDailyArgs]) error {
	return runThresholdCycle(ctx, thresholdCycleDeps{
		Eng:        w.Eng,
		Rules:      w.Rules,
		Alerts:     w.Alerts,
		WorkerStat: w.WorkerStat,
		RuleKind:   "threshold_daily",
		WorkerKind: "threshold_daily",
		Latest: func(ctx context.Context, mpID uuid.UUID) (float64, time.Time, error) {
			row, err := w.Eng.Queries.GetLatestDailyForMP(ctx, pgUUID(mpID))
			if err != nil {
				return 0, time.Time{}, err
			}
			// D-02 daily threshold compares the day's TOTAL consumption
			// (cumulative_delta sum) against the bound — operators set
			// rules like "alert if daily kWh > 1000". int64 → float64 is
			// safe within the realistic kWh ranges.
			return float64(row.SumConsumption), bucketTime(row.Bucket), nil
		},
	})
}

// errInstantNotNumeric signals a measurement row with NULL instant_value or
// otherwise non-numeric. Workers RecordErr-and-continue rather than fail the
// whole cycle — one bad MP shouldn't stop alerting for the rest.
var errInstantNotNumeric = errors.New("alert: latest measurement has no numeric value")

// thresholdCycleDeps bundles the per-worker variation points so
// runThresholdCycle is a single function with three call sites.
type thresholdCycleDeps struct {
	Eng        EvaluateContext
	Rules      *RuleStore
	Alerts     *AlertStore
	WorkerStat *WorkerStateStore
	RuleKind   string
	WorkerKind string
	Latest     func(ctx context.Context, mpID uuid.UUID) (float64, time.Time, error)
}

// runThresholdCycle is the shared cycle body. Reads all active rules of
// RuleKind, expands scope (metering_point|site|global → list of MP IDs),
// pulls the latest value for each target, evaluates the comparison, and
// fires/clears as needed. Per-cycle stats UPSERT into alert_worker_state at
// the end (D-21).
func runThresholdCycle(ctx context.Context, d thresholdCycleDeps) error {
	start := time.Now()
	state := &RunState{}

	defer func() {
		state.DurationMS = int32(time.Since(start).Milliseconds())
		// Per-cycle observability UPSERT runs in its own tx (not tied to
		// the fire txns) so a worker that successfully fired but failed
		// the final UPSERT still records the fire — and a worker that
		// failed every fire still surfaces non-zero error counts via
		// RecordErr. The UPSERT clears degraded on a clean run.
		if d.WorkerStat == nil {
			return
		}
		tx, err := d.Eng.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		if err != nil {
			return
		}
		_ = d.WorkerStat.UpsertWorkerState(ctx, tx, d.WorkerKind, state)
		_ = tx.Commit(ctx)
	}()

	rules, err := d.Rules.ListActiveRulesByKind(ctx, d.RuleKind)
	if err != nil {
		state.RecordErr(err)
		return err
	}

	installName := readInstallDisplayName(ctx, d.Eng)

	now := time.Now().UTC()
	for _, rule := range rules {
		// Cooldown gate (D-05) gates RE-FIRE only — auto-clear MUST still
		// run inside the cooldown window so a transient breach that
		// resolves quickly doesn't leave a stuck firing alert.
		inCooldown := !rule.CooledDown(now)

		targets, err := expandScope(ctx, d.Eng, rule)
		if err != nil {
			state.RecordErr(err)
			continue
		}
		for _, target := range targets {
			value, _, err := d.Latest(ctx, target.MeteringPointID)
			if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, errInstantNotNumeric) {
				// No data yet for this MP, or null instant_value — skip.
				state.RulesEvaluated++
				continue
			}
			if err != nil {
				state.RecordErr(err)
				continue
			}
			state.RulesEvaluated++

			cmp := derefStr(rule.Comparison, "gt")
			breached := compareWithOperator(value, rule.HighBound, rule.LowBound, cmp)

			existing, _ := d.Alerts.ListFiringByRuleTarget(ctx, rule.ID, target.MeteringPointID)
			switch {
			case breached && existing == nil:
				if inCooldown {
					// D-05: suppress new fire while last_fired_at + cooldown_seconds > now.
					continue
				}
				thr := pickThreshold(rule)
				if err := fireThresholdAlert(ctx, d.Eng, d.Alerts, d.Rules, rule, target, value, thr, installName); err != nil {
					if errors.Is(err, ErrDuplicateFire) {
						// idempotent — another worker won the race
						continue
					}
					state.RecordErr(err)
					continue
				}
				state.FiresEmitted++
			case !breached && existing != nil:
				if err := autoClearAlert(ctx, d.Eng, d.Alerts, *existing); err != nil {
					state.RecordErr(err)
					continue
				}
				state.Cleared++
			}
		}
	}
	return nil
}

// Target carries the resolved metering_point id + display label for one
// rule evaluation iteration.
type Target struct {
	MeteringPointID uuid.UUID
	Label           string
}

// expandScope resolves the rule's scope_kind / scope_id into the list of
// metering_point ids the worker iterates over. Soft-deleted MPs (archived_at
// not null) are filtered out at the query layer.
//
// scope_kind values per 0038_alert_rule CHECK:
//   - metering_point: rule.ScopeID is the MP id.
//   - site:           rule.ScopeID is the site id → list all MPs in the site.
//   - global:         rule.ScopeID is NULL → list all active MPs.
//   - device:         rule.ScopeID is the device id; threshold rules on a
//                     device resolve via the active binding to its MP.
//   - gateway:        rule.ScopeID is the gateway id; threshold rules on a
//                     gateway have no direct MP — they are skipped (the
//                     offline worker is the gateway-targeted evaluator).
func expandScope(ctx context.Context, eng EvaluateContext, rule RuleRecord) ([]Target, error) {
	switch rule.ScopeKind {
	case "metering_point":
		if rule.ScopeID == nil {
			return nil, nil
		}
		row, err := eng.Queries.GetMeteringPointLabel(ctx, pgUUID(*rule.ScopeID))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return []Target{{MeteringPointID: uuid.UUID(row.ID.Bytes), Label: row.Label}}, nil
	case "site":
		if rule.ScopeID == nil {
			return nil, nil
		}
		rows, err := eng.Queries.ListMeteringPointsBySite(ctx, pgUUID(*rule.ScopeID))
		if err != nil {
			return nil, err
		}
		out := make([]Target, 0, len(rows))
		for _, r := range rows {
			out = append(out, Target{MeteringPointID: uuid.UUID(r.ID.Bytes), Label: r.Label})
		}
		return out, nil
	case "global":
		rows, err := eng.Queries.ListAllActiveMeteringPoints(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]Target, 0, len(rows))
		for _, r := range rows {
			out = append(out, Target{MeteringPointID: uuid.UUID(r.ID.Bytes), Label: r.Label})
		}
		return out, nil
	case "device":
		// Device-scoped threshold rules resolve via the currently active
		// binding. Returning empty when no binding covers the rule's device
		// (e.g. just decommissioned) is the correct silent skip — the
		// device-offline rule kind covers the disconnection signal.
		// V1: device-scoped threshold rules are accepted by the schema
		// (alert_rule CHECK admits scope_kind='device') but the evaluator
		// silently no-ops them — the device-offline rule kind covers the
		// disconnection signal, and per-MP rules are the operator's path
		// for per-device thresholds. V2 may resolve via the active
		// binding when there is a real product need.
		return nil, nil
	case "gateway":
		// Gateway-scoped threshold rules are not yet defined; offline_gateway
		// is the only kind that resolves to a gateway target.
		return nil, nil
	default:
		return nil, fmt.Errorf("alert: unknown scope_kind %q", rule.ScopeKind)
	}
}

// compareWithOperator implements the rule's comparison operator. The default
// "gt" / "lt" mirror engine.CompareBound (strict >/<); "gte" and "lte" are
// inclusive at the boundary; "eq" means exact equality against the high bound
// when set, low bound otherwise.
func compareWithOperator(value float64, high, low *float64, op string) bool {
	switch op {
	case "gte":
		if high != nil && value >= *high {
			return true
		}
		if low != nil && value <= *low {
			return true
		}
		return false
	case "lte":
		if high != nil && value <= *high {
			return true
		}
		if low != nil && value >= *low {
			return true
		}
		return false
	case "lt":
		if low != nil && value < *low {
			return true
		}
		if high != nil && value < *high {
			return true
		}
		return false
	case "eq":
		if high != nil && value == *high {
			return true
		}
		if low != nil && value == *low {
			return true
		}
		return false
	default: // "gt" or unrecognized → strict >
		return CompareBound(value, high, low)
	}
}

// pickThreshold returns whichever bound is set; high preferred over low when
// both are populated (typical "rate too high" rule). Returns 0 if neither
// bound is set — the rule's evaluation should already have been skipped.
func pickThreshold(rule RuleRecord) float64 {
	if rule.HighBound != nil {
		return *rule.HighBound
	}
	if rule.LowBound != nil {
		return *rule.LowBound
	}
	return 0
}

// fireThresholdAlert opens a tx, inserts the alert row, writes the audit
// row in the SAME tx (D-23), touches last_fired_at to start the cooldown
// (D-05), and commits. Returns ErrDuplicateFire if another worker already
// raised an alert for this (rule, target) — caller treats as no-op.
func fireThresholdAlert(ctx context.Context, eng EvaluateContext, alerts *AlertStore, rules *RuleStore,
	rule RuleRecord, target Target, value, threshold float64, installName string) error {
	tx, err := eng.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	payload, err := BuildPayload(BuildPayloadInput{
		RuleID:      rule.ID,
		RuleKind:    rule.RuleKind,
		Severity:    rule.Severity,
		Target:      PayloadTarget{EntityType: "metering_point", EntityID: target.MeteringPointID, Label: target.Label},
		Value:       value,
		Threshold:   threshold,
		Comparison:  derefStr(rule.Comparison, "gt"),
		Unit:        derefStr(rule.Unit, ""),
		FiredAt:     time.Now().UTC(),
		InstallName: installName,
	})
	if err != nil {
		return fmt.Errorf("alert: build payload: %w", err)
	}

	row, err := alerts.InsertAlert(ctx, tx, InsertAlertParams{
		RuleID:           rule.ID,
		RuleKind:         rule.RuleKind,
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
		return fmt.Errorf("alert: audit fire: %w", err)
	}

	if err := rules.TouchLastFiredAt(ctx, tx, rule.ID); err != nil {
		return fmt.Errorf("alert: touch last_fired_at: %w", err)
	}

	return tx.Commit(ctx)
}

// autoClearAlert transitions a firing alert to cleared and writes the
// audit.cleared row in the same tx (D-23).
func autoClearAlert(ctx context.Context, eng EvaluateContext, alerts *AlertStore, existing AlertRecord) error {
	tx, err := eng.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := alerts.ClearAlert(ctx, tx, existing.ID); err != nil {
		return err
	}
	afterMap, _ := jsonbToMap(existing.Payload)
	if err := audit.WriteEntry(ctx, tx, audit.Entry{
		Action:     audit.ActionAlertCleared,
		EntityType: audit.EntityTypeAlert,
		EntityID:   existing.ID,
		After:      afterMap,
	}); err != nil {
		return fmt.Errorf("alert: audit clear: %w", err)
	}
	return tx.Commit(ctx)
}

// readInstallDisplayName loads install_identity.display_name once per cycle.
// Falls back to an empty string on first-boot (pre-install) so the payload
// install.display_name is never NULL-encoded (downstream consumers see "" not
// a missing key).
func readInstallDisplayName(ctx context.Context, eng EvaluateContext) string {
	if eng.Queries == nil {
		return ""
	}
	name, err := eng.Queries.GetInstallDisplayName(ctx)
	if err != nil {
		return ""
	}
	return name
}

// derefStr returns *p or fallback if p is nil.
func derefStr(p *string, fallback string) string {
	if p == nil {
		return fallback
	}
	return *p
}

// bucketTime extracts a time.Time from sqlc's CAGG-bucket scan target. sqlc
// types the time_bucket(...) result as `interface{}` because PostgreSQL
// reports the underlying expression type without dimensional metadata; in
// practice the driver delivers a time.Time (or pgtype.Timestamptz). This
// helper accepts both and falls back to time.Time{} so callers can still
// short-circuit on "no data yet".
func bucketTime(v any) time.Time {
	switch t := v.(type) {
	case time.Time:
		return t
	case pgtype.Timestamptz:
		if t.Valid {
			return t.Time
		}
	}
	return time.Time{}
}

// numericToFloat converts a pgtype.Numeric to float64. Returns ok=false when
// the numeric is NULL or otherwise not convertible. Used by the latest-value
// readers — a NULL instant_value means "no usable reading this cycle".
func numericToFloat(n pgtype.Numeric) (float64, bool) {
	if !n.Valid {
		return 0, false
	}
	bf, err := n.Float64Value()
	if err != nil || !bf.Valid {
		// Fallback to math/big for very large/small magnitudes pgtype
		// can't fit into a native float64Value() — extract via string.
		s, sErr := n.Value()
		if sErr != nil {
			return 0, false
		}
		str, ok := s.(string)
		if !ok {
			return 0, false
		}
		f, _, err := big.NewFloat(0).Parse(str, 10)
		if err != nil {
			return 0, false
		}
		fv, _ := f.Float64()
		return fv, true
	}
	return bf.Float64, true
}

// pgUUID is the local helper for converting uuid.UUID to pgtype.UUID (the
// sqlc-generated param type). Mirrors the pattern used elsewhere in the
// codebase (e.g. ingest/persist.go) so reviewers don't have to context-switch.
func pgUUID(u uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: u, Valid: true}
}

// jsonbToMap decodes a payload JSONB blob into a map for the audit-log diff
// JSONB column. The audit row's After field stores the same payload shape so
// browse + export can render "what fired" without joining alert.payload.
func jsonbToMap(b []byte) (map[string]any, error) {
	if len(b) == 0 {
		return nil, nil
	}
	out := map[string]any{}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}
