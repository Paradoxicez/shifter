package alert

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// PayloadTarget mirrors the D-12 stable schema target sub-object.
type PayloadTarget struct {
	EntityType string    `json:"entity_type"`
	EntityID   uuid.UUID `json:"entity_id"`
	Label      string    `json:"label"`
}

// BuildPayloadInput is the value bundle the workers fill in before serializing.
// Keep the shape stable — the V2 webhook deliverer (V2-NOTIF-01) will
// deserialize this same shape, so any reshaping is a breaking change.
//
// D-12 top-level keys are exactly:
//
//	rule_id, rule_kind, severity, target, value, threshold, comparison, unit,
//	fired_at, install
//
// Offline-specific keys (added only when non-nil): last_uplink_at,
// expected_interval_s, suppresses_n_devices.
//
// Anomaly extensions (added only when non-nil; used by Plan 06-03):
// p95_baseline, baseline_window_days, time_of_day_bucket.
type BuildPayloadInput struct {
	RuleID     uuid.UUID
	RuleKind   string // "threshold_instantaneous"|...|"offline_gateway"
	Severity   string // "info"|"warning"|"critical"
	Target     PayloadTarget
	Value      float64
	Threshold  float64
	Comparison string // "gt"|"gte"|"lt"|"lte"|"eq"
	Unit       string
	FiredAt    time.Time
	// InstallName mirrors install_identity.display_name. Workers read it
	// once per cycle and inject so the wire payload doesn't require an
	// extra JOIN per fire.
	InstallName string
	// Offline-specific extensions (nil for non-offline rule kinds).
	LastUplinkAt      *time.Time
	ExpectedIntervalS *int32
	// Gateway-suppression extension (only on offline_gateway).
	SuppressesNDevices *int32
	// Anomaly extensions (Plan 06-03).
	P95Baseline        *float64
	BaselineWindowDays *int32
	TimeOfDayBucket    *string
}

// BuildPayload serializes BuildPayloadInput into the D-12 canonical
// JSON shape; returns the bytes ready for alert.payload JSONB column.
//
// Order matters only for readability — JSON object key order is unspecified
// per RFC 8259, but downstream tooling (jq pipelines in support runbooks)
// reads better when the canonical top-level keys appear first. The
// encoding/json package preserves insertion order for map iteration only
// since Go 1.12+ (sort.Strings under the hood), so callers that need a
// specific key order rely on the documented schema rather than wire byte
// order.
func BuildPayload(in BuildPayloadInput) ([]byte, error) {
	wire := map[string]any{
		"rule_id":   in.RuleID,
		"rule_kind": in.RuleKind,
		"severity":  in.Severity,
		"target": map[string]any{
			"entity_type": in.Target.EntityType,
			"entity_id":   in.Target.EntityID,
			"label":       in.Target.Label,
		},
		"value":      in.Value,
		"threshold":  in.Threshold,
		"comparison": in.Comparison,
		"unit":       in.Unit,
		"fired_at":   in.FiredAt.UTC().Format(time.RFC3339),
		"install":    map[string]any{"display_name": in.InstallName},
	}
	if in.LastUplinkAt != nil {
		wire["last_uplink_at"] = in.LastUplinkAt.UTC().Format(time.RFC3339)
	}
	if in.ExpectedIntervalS != nil {
		wire["expected_interval_s"] = *in.ExpectedIntervalS
	}
	if in.SuppressesNDevices != nil {
		wire["suppresses_n_devices"] = *in.SuppressesNDevices
	}
	if in.P95Baseline != nil {
		wire["p95_baseline"] = *in.P95Baseline
	}
	if in.BaselineWindowDays != nil {
		wire["baseline_window_days"] = *in.BaselineWindowDays
	}
	if in.TimeOfDayBucket != nil {
		wire["time_of_day_bucket"] = *in.TimeOfDayBucket
	}
	return json.Marshal(wire)
}
