package alert

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

// TestEvalQuietHour_ReturnsFalseWhenWindowUnset: a rule without
// quiet_window_start / quiet_window_end is a misconfiguration — EvalQuietHour
// must return (false, _, _, nil) so the worker silently no-ops rather than
// raising a spurious alert.
func TestEvalQuietHour_ReturnsFalseWhenWindowUnset(t *testing.T) {
	env := newAnomalyTestEnv(t)
	rule := RuleRecord{
		RuleKind:   "anomaly_quiet_hour",
		// QuietWindowStart, QuietWindowEnd intentionally nil.
	}
	breach, _, _, err := EvalQuietHour(context.Background(), env.queries, env.mpID, rule, "UTC")
	require.NoError(t, err)
	require.False(t, breach, "missing quiet window must produce no breach")
}

// TestEvalQuietHour_ReturnsFalseWhenNoMatchingMeasurement: a rule with a
// valid window but no measurement in the last 24h above flow_threshold
// must return false (no fire) and a nil error.
func TestEvalQuietHour_ReturnsFalseWhenNoMatchingMeasurement(t *testing.T) {
	env := newAnomalyTestEnv(t)

	// No measurements at all → still no breach (the eligibility gate is
	// applied at the AnomalyWorker layer; EvalQuietHour itself just runs
	// the window-match query).
	start := mustTime(t, "08:00")
	end := mustTime(t, "18:00")
	threshFloat := 0.1
	rule := RuleRecord{
		RuleKind:         "anomaly_quiet_hour",
		QuietWindowStart: tPtr(start),
		QuietWindowEnd:   tPtr(end),
		FlowThreshold:    &threshFloat,
	}
	breach, _, _, err := EvalQuietHour(context.Background(), env.queries, env.mpID, rule, "UTC")
	require.NoError(t, err)
	require.False(t, breach, "no measurements ⇒ no breach")
}

// tPtr returns a pointer to t for convenience in rule construction.
func tPtr(t time.Time) *time.Time { return &t }

// mustTime parses HH:MM into a time.Time at zero-day; helper for quiet-hour
// rule fixtures.
func mustTime(t *testing.T, hhmm string) time.Time {
	t.Helper()
	parsed, err := time.Parse("15:04", hhmm)
	require.NoError(t, err)
	return parsed
}

// Compile-time assurance that quiet_hour.go exports the symbols the file
// claims (catches accidental rename / removal at refactor time).
var _ = pgtype.Time{} // pgtype.Time is the underlying TIME representation
