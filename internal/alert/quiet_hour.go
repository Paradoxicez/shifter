package alert

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// EvalQuietHour evaluates the D-17 anomaly_quiet_hour rule for one MP.
//
// Returns (breach, value, atTime, err):
//   - breach=true  ⇒ there is a measurement in the last 24h whose timestamp
//                    (converted to install-local time via AT TIME ZONE) falls
//                    inside the rule's quiet window AND whose instant_value
//                    exceeds rule.FlowThreshold.
//   - breach=false ⇒ no qualifying measurement (the operator's "quiet" window
//                    is actually quiet).
//
// The underlying SQL handles cross-midnight windows (e.g. 22:00→06:00) via
// the canonical OR-form in 06-RESEARCH Pitfall 9. The install_tz parameter
// is the IANA name from install_identity.timezone — the worker reads it
// fresh each cycle so a Settings change takes effect at the next eval
// without restart.
//
// A rule with NULL quiet_window_start or quiet_window_end is silently
// no-op'd (returns false). This protects the worker from a half-configured
// rule that would otherwise emit spurious fires.
func EvalQuietHour(ctx context.Context, q *sqlc.Queries, mpID uuid.UUID, rule RuleRecord, installTZ string) (breach bool, value float64, atTime time.Time, err error) {
	if q == nil {
		return false, 0, time.Time{}, fmt.Errorf("alert: nil queries handle")
	}
	if rule.QuietWindowStart == nil || rule.QuietWindowEnd == nil {
		// Misconfigured rule — silently no-op rather than raise a fire.
		return false, 0, time.Time{}, nil
	}
	if installTZ == "" {
		installTZ = "UTC"
	}

	threshold := 0.0
	if rule.FlowThreshold != nil {
		threshold = *rule.FlowThreshold
	}

	startTime := pgtype.Time{
		Microseconds: timeToMicros(*rule.QuietWindowStart),
		Valid:        true,
	}
	endTime := pgtype.Time{
		Microseconds: timeToMicros(*rule.QuietWindowEnd),
		Valid:        true,
	}

	row, err := q.NonZeroFlowDuringQuietWindow(ctx, sqlc.NonZeroFlowDuringQuietWindowParams{
		MeteringPointID:  pgUUID(mpID),
		FlowThreshold:    threshold,
		QuietWindowStart: startTime,
		QuietWindowEnd:   endTime,
		InstallTz:        installTZ,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, 0, time.Time{}, nil
		}
		return false, 0, time.Time{}, fmt.Errorf("alert: non-zero flow during quiet window: %w", err)
	}

	val, ok := numericToFloat(row.InstantValue)
	if !ok {
		// Row exists but instant_value is NULL — not a breach.
		return false, 0, time.Time{}, nil
	}
	return true, val, row.Time.Time, nil
}

// timeToMicros converts a time.Time's clock component (hours/minutes/
// seconds/nanoseconds) into microseconds-since-midnight for pgtype.Time.
// PostgreSQL stores TIME as microseconds since 00:00:00; the date portion
// of the input is intentionally discarded.
//
// cross-midnight: the SQL handles wrap-around in the OR-form expression;
// this helper just gives Postgres the start/end of the window as a pure
// time-of-day.
func timeToMicros(t time.Time) int64 {
	hour := int64(t.Hour())
	minute := int64(t.Minute())
	second := int64(t.Second())
	nano := int64(t.Nanosecond())
	return ((hour*60+minute)*60+second)*1_000_000 + nano/1_000
}
