package alert

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// Cold-start (D-16) — anomaly evaluators (anomaly_p95, anomaly_iqr,
// anomaly_quiet_hour) must NOT fire on a metering point whose measurement
// history is too short for a statistical baseline to be meaningful. The
// 21-day threshold is hardcoded in v1; Phase 7 may promote it to a
// retention_config column once real-customer data shows whether the value
// should be longer/shorter.
//
// The gate is the single source of truth used by both the worker (skip
// evaluation if !eligible) AND the API endpoint (Plan 06-04) that powers the
// MP detail "Anomaly detection" card (warming_up | eligible_inactive |
// active states) and the Settings → Alerts warmup roster (Plan 06-10).

// AnomalyWarmupDays is the cold-start gate threshold (D-16). v1 constant;
// Phase 7 may promote to retention_config.
const AnomalyWarmupDays = 21

// IsMPEligibleForAnomaly returns true when the metering point has at least
// one measurement ≥ AnomalyWarmupDays days old. This is the D-16 cold-start
// gate the AnomalyWorker MUST check before every per-MP evaluation.
//
// Thin wrapper over the sqlc-generated query — kept as a named function so
// the worker code reads as a domain operation ("is this MP eligible?")
// rather than a query call.
func IsMPEligibleForAnomaly(ctx context.Context, q *sqlc.Queries, mpID uuid.UUID) (bool, error) {
	if q == nil {
		return false, fmt.Errorf("alert: nil queries handle")
	}
	eligible, err := q.IsMPEligibleForAnomaly(ctx, pgUUID(mpID))
	if err != nil {
		return false, fmt.Errorf("alert: is mp eligible for anomaly: %w", err)
	}
	return eligible, nil
}

// AnomalyEligibility is one row from the warmup roster. DaysUntilEligible=0
// means the MP is eligible for anomaly evaluation; positive values count
// days remaining before the 21-day window is satisfied (max 21 = no
// measurements yet).
type AnomalyEligibility struct {
	MeteringPointID    uuid.UUID `json:"metering_point_id"`
	MeteringPointLabel string    `json:"metering_point_label"`
	SiteLabel          string    `json:"site_label"`
	DaysUntilEligible  int32     `json:"days_until_eligible"` // 0 = eligible
}

// ListAnomalyWarmupRoster returns one row per active (non-archived) MP with
// its days_until_eligible value, ordered ASC (eligible first, longest-
// warmup last) so the UI can render the most-urgent-eligible bucket first.
// Used by the MP detail card AND the Settings → Alerts warmup section.
//
// Postgres-side sqlc types the CASE expression as interface{} because the
// THEN/ELSE branches mix an INT literal with EXTRACT casts; we coerce to
// int32 here so callers see a typed field. Out-of-range values are clamped
// to [0, 21] defensively even though the SQL already GREATEST/MIN-clamps.
func ListAnomalyWarmupRoster(ctx context.Context, q *sqlc.Queries) ([]AnomalyEligibility, error) {
	if q == nil {
		return nil, fmt.Errorf("alert: nil queries handle")
	}
	rows, err := q.ListAnomalyWarmupRoster(ctx)
	if err != nil {
		return nil, fmt.Errorf("alert: list anomaly warmup roster: %w", err)
	}
	out := make([]AnomalyEligibility, 0, len(rows))
	for _, r := range rows {
		days := int32(AnomalyWarmupDays)
		switch v := r.DaysUntilEligible.(type) {
		case int32:
			days = v
		case int64:
			days = int32(v)
		case int:
			days = int32(v)
		case float64:
			days = int32(v)
		}
		if days < 0 {
			days = 0
		}
		if days > AnomalyWarmupDays {
			days = AnomalyWarmupDays
		}
		out = append(out, AnomalyEligibility{
			MeteringPointID:    uuid.UUID(r.MeteringPointID.Bytes),
			MeteringPointLabel: r.MeteringPointLabel,
			SiteLabel:          r.SiteLabel,
			DaysUntilEligible:  days,
		})
	}
	return out, nil
}

// MPAnomalyRuleState describes one anomaly rule that targets a metering
// point — used by the Plan 06-04 MP detail "Anomaly detection" card to
// render per-rule toggle state. RuleID is nil when no rule of that kind
// targets the MP (the UI then shows an "enable" affordance that POSTs a new
// rule with scope_kind='metering_point', scope_id=<mp>).
type MPAnomalyRuleState struct {
	RuleKind         string         `json:"rule_kind"` // anomaly_p95 | anomaly_iqr | anomaly_quiet_hour
	RuleID           *uuid.UUID     `json:"rule_id,omitempty"`
	Severity         string         `json:"severity,omitempty"`
	Enabled          bool           `json:"enabled"`
	QuietWindowStart *pgtype.Time   `json:"-"`
	QuietWindowEnd   *pgtype.Time   `json:"-"`
	FlowThreshold    *float64       `json:"flow_threshold,omitempty"`
	DaysOfWeek       *int32         `json:"days_of_week,omitempty"`
}

// GetMPAnomalyState returns the per-rule-kind enable state for the three
// anomaly rule kinds at this MP. Includes both metering_point-scoped rules
// (scope_id=mpID) AND global-scoped rules (which fire across all MPs and
// therefore implicitly cover this one).
//
// The three rule kinds always appear in the returned slice — entries without
// a backing rule have RuleID=nil + Enabled=false so the MP detail card can
// render the "Off (click to enable)" state without an extra "does a rule
// exist?" check on the frontend.
func GetMPAnomalyState(ctx context.Context, q *sqlc.Queries, mpID uuid.UUID) ([]MPAnomalyRuleState, error) {
	if q == nil {
		return nil, fmt.Errorf("alert: nil queries handle")
	}
	rows, err := q.GetMPAnomalyRules(ctx, pgUUID(mpID))
	if err != nil {
		return nil, fmt.Errorf("alert: get mp anomaly rules: %w", err)
	}

	// Seed result with a row per kind so the UI always sees three entries.
	state := map[string]MPAnomalyRuleState{
		"anomaly_p95":         {RuleKind: "anomaly_p95"},
		"anomaly_iqr":         {RuleKind: "anomaly_iqr"},
		"anomaly_quiet_hour":  {RuleKind: "anomaly_quiet_hour"},
	}

	for _, r := range rows {
		ruleID := uuid.UUID(r.ID.Bytes)
		enabled := false
		if b, ok := r.Enabled.(bool); ok {
			enabled = b
		}
		qs := r.QuietWindowStart
		qe := r.QuietWindowEnd
		entry := MPAnomalyRuleState{
			RuleKind: r.RuleKind,
			RuleID:   &ruleID,
			Severity: r.Severity,
			Enabled:  enabled,
			QuietWindowStart: func() *pgtype.Time {
				if qs.Valid {
					return &qs
				}
				return nil
			}(),
			QuietWindowEnd: func() *pgtype.Time {
				if qe.Valid {
					return &qe
				}
				return nil
			}(),
			FlowThreshold: r.FlowThreshold,
			DaysOfWeek:    r.DaysOfWeek,
		}
		// If multiple rules of the same kind exist (e.g. a global rule AND a
		// per-MP rule), prefer the more-specific one — the per-MP rule
		// already has higher precedence in the worker's scope-matching
		// logic. We approximate by keeping the LAST one written; sqlc orders
		// the result set arbitrarily so callers needing strict precedence
		// should query alert_rule directly.
		state[r.RuleKind] = entry
	}

	// Return in stable order: p95 → iqr → quiet_hour.
	return []MPAnomalyRuleState{
		state["anomaly_p95"],
		state["anomaly_iqr"],
		state["anomaly_quiet_hour"],
	}, nil
}
