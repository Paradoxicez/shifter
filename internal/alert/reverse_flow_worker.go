package alert

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/shifter-io/shifter/internal/audit"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// ReverseFlowIncreaseArgs is the River job-args type for the reverse-flow
// increase evaluator. Runs at 6-hour cadence (D-46).
type ReverseFlowIncreaseArgs struct{}

func (ReverseFlowIncreaseArgs) Kind() string { return "alert_reverse_flow" }

func (ReverseFlowIncreaseArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3}
}

// ReverseFlowIncreaseWorker fires an alert when a metering point's
// measurement.extra->>'reverse_flow_m3' increases by more than
// rule.FlowThreshold m³ over the window defined by rule.DaysOfWeek (repurposed
// as window_days for this rule kind — int32 days).
//
// T-07-09b-01 (threat register): window_days is clamped to [1, 365] and
// threshold must be positive to prevent operator-supplied param abuse.
//
// T-07-09b-02: query only runs for MPs with a reverse_flow_increase rule armed;
// scans are bounded by the metering_point_id index on measurement.
type ReverseFlowIncreaseWorker struct {
	river.WorkerDefaults[ReverseFlowIncreaseArgs]

	Eng    EvaluateContext
	Rules  *RuleStore
	Alerts *AlertStore
}

// Work runs one reverse-flow evaluation cycle (River worker interface).
func (w *ReverseFlowIncreaseWorker) Work(ctx context.Context, _ *river.Job[ReverseFlowIncreaseArgs]) error {
	return w.Run(ctx)
}

// Run is the testable core of the evaluation cycle. Called by Work and by tests.
func (w *ReverseFlowIncreaseWorker) Run(ctx context.Context) error {
	rules, err := w.Rules.ListActiveRulesByKind(ctx, "reverse_flow_increase")
	if err != nil {
		return fmt.Errorf("reverse_flow_worker: list rules: %w", err)
	}

	installName := readInstallDisplayName(ctx, w.Eng)
	now := time.Now().UTC()
	q := sqlc.New(w.Eng.Pool)

	for _, rule := range rules {
		// Only metering_point-scoped rules are supported in D-46 v1.
		// Global scope would require a full MP scan.
		if rule.ScopeKind != "metering_point" || rule.ScopeID == nil {
			continue
		}
		mpID := *rule.ScopeID

		// T-07-09b-01: clamp window_days to [1, 365].
		windowDays := int32(7)
		if rule.DaysOfWeek != nil {
			windowDays = *rule.DaysOfWeek
		}
		if windowDays < 1 {
			windowDays = 1
		}
		if windowDays > 365 {
			windowDays = 365
		}

		// T-07-09b-01: threshold must be positive.
		thresholdM3 := derefFloat(rule.FlowThreshold, 1.0)
		if thresholdM3 <= 0 {
			thresholdM3 = 1.0
		}

		windowSecs := float64(windowDays) * 86400.0

		delta, err := q.GetReverseFlowDelta(ctx, sqlc.GetReverseFlowDeltaParams{
			MeteringPointID: pgUUID(mpID),
			WindowSecs:      windowSecs,
		})
		if err != nil {
			if w.Eng.Log != nil {
				w.Eng.Log.Warn("reverse_flow_worker: get delta failed", "mp", mpID, "err", err)
			}
			continue
		}

		nowVal := toFloat64(delta.NowValue)
		pastVal := toFloat64(delta.PastValue)
		increase := nowVal - pastVal

		existing, _ := w.Alerts.ListFiringByRuleTarget(ctx, rule.ID, mpID)

		if increase <= thresholdM3 {
			// No breach — auto-clear any previously-firing alert.
			if existing != nil {
				_ = autoClearAlert(ctx, w.Eng, w.Alerts, *existing)
			}
			continue
		}

		// D-05 cooldown gate on re-fire.
		if !rule.CooledDown(now) {
			continue
		}

		// Already firing — idempotent, don't double-fire.
		if existing != nil {
			continue
		}

		if err := w.fireReverseFlow(ctx, q, rule, mpID, increase, thresholdM3, installName); err != nil {
			if w.Eng.Log != nil {
				w.Eng.Log.Error("reverse_flow_worker: fire failed",
					"rule", rule.ID, "mp", mpID, "err", err)
			}
		}
	}
	return nil
}

func (w *ReverseFlowIncreaseWorker) fireReverseFlow(
	ctx context.Context,
	q *sqlc.Queries,
	rule RuleRecord,
	mpID uuid.UUID,
	deltaM3, thresholdM3 float64,
	installName string,
) error {
	label := ""
	if row, err := q.GetMeteringPointLabel(ctx, pgUUID(mpID)); err == nil {
		label = row.Label
	}

	tx, err := w.Eng.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	in := BuildPayloadInput{
		RuleID:      rule.ID,
		RuleKind:    "reverse_flow_increase",
		Severity:    rule.Severity,
		Target:      PayloadTarget{EntityType: "metering_point", EntityID: mpID, Label: label},
		Value:       deltaM3,
		Threshold:   thresholdM3,
		Comparison:  "gt",
		FiredAt:     time.Now().UTC(),
		InstallName: installName,
	}
	payload, err := BuildPayload(in)
	if err != nil {
		return fmt.Errorf("reverse_flow_worker: build payload: %w", err)
	}

	row, err := w.Alerts.InsertAlert(ctx, tx, InsertAlertParams{
		RuleID:           rule.ID,
		RuleKind:         "reverse_flow_increase",
		Severity:         rule.Severity,
		Payload:          payload,
		TargetEntityType: "metering_point",
		TargetEntityID:   mpID,
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
		return fmt.Errorf("reverse_flow_worker: audit fire: %w", err)
	}
	if err := w.Rules.TouchLastFiredAt(ctx, tx, rule.ID); err != nil {
		return fmt.Errorf("reverse_flow_worker: touch rule: %w", err)
	}
	return tx.Commit(ctx)
}

// toFloat64 coerces interface{} DB values to float64, returning 0 on
// unrecognized types. Used for GetReverseFlowDelta rows where COALESCE
// over DOUBLE PRECISION expressions may be scanned as interface{}.
func toFloat64(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int64:
		return float64(x)
	case int32:
		return float64(x)
	case int:
		return float64(x)
	}
	return 0
}
