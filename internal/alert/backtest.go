// Package alert — Plan 07-10 backtest engine.
//
// BacktestRun is a read-only function that replays the three anomaly rule
// kinds against measurement_hourly CAGG data for a given metering point and
// time window. It uses the same evaluator math as the live AnomalyWorker but
// never writes to the alert table (T-07-10-04 mitigation).
//
// Two-pass pattern (Plan 07-10 RESEARCH §"Pattern 5", Pitfall §4):
//
//  - anomaly_p95:  pass 1 = percentile_cont P95; pass 2 = count rows > P95
//  - anomaly_iqr:  pass 1 = Q1/Q3; pass 2 = count rows outside Tukey fences
//  - anomaly_quiet_hour: single pass (no baseline needed)
//
// Checker M-1: three CONCRETE typed fill helpers (no `any` erasure).
package alert

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// BacktestResult holds the summary of a 30-day (or N-day) backtest run.
type BacktestResult struct {
	FiresCount int              `json:"fires_count"`
	DailyFires []DailyFireBucket `json:"daily_fires"`
}

// DailyFireBucket is one day's fire count in the backtest result.
type DailyFireBucket struct {
	Day   string `json:"day"`   // YYYY-MM-DD
	Count int    `json:"count"`
}

const (
	backtestMaxDays           = 90
	quietHourStart            = 0   // 00:00 UTC
	quietHourEnd              = 5   // 05:59 UTC
	quietHourFlowThreshold    = 0.001 // m³/h or kW — any positive flow qualifies
)

// BacktestRun executes a read-only backtest for the given rule kind and metering
// point over the requested number of days (capped at 90). It never writes to
// the alert table.
func BacktestRun(ctx context.Context, pool *pgxpool.Pool, kind string, mpID uuid.UUID, days int) (BacktestResult, error) {
	if days <= 0 || days > backtestMaxDays {
		return BacktestResult{}, errors.New("days must be 1..90")
	}

	q := sqlc.New(pool)
	interval := pgtype.Interval{
		Microseconds: int64(time.Duration(days) * 24 * time.Hour / time.Microsecond),
		Valid:        true,
	}
	pgID := pgtype.UUID{Bytes: mpID, Valid: true}

	switch kind {
	case "anomaly_p95":
		return backtestP95(ctx, q, pgID, interval, days)
	case "anomaly_iqr":
		return backtestIQR(ctx, q, pgID, interval, days)
	case "anomaly_quiet_hour":
		return backtestQuietHour(ctx, q, pgID, interval, days)
	default:
		return BacktestResult{}, errors.New("unsupported rule kind for backtest")
	}
}

// backtestP95 performs a two-pass P95 backtest.
func backtestP95(ctx context.Context, q *sqlc.Queries, pgID pgtype.UUID, interval pgtype.Interval, days int) (BacktestResult, error) {
	p95, err := q.BacktestP95Pass1(ctx, sqlc.BacktestP95Pass1Params{
		MeteringPointID: pgID,
		Column2:         interval,
	})
	if err != nil {
		// percentile_cont on zero rows produces NULL → scan fails.
		// Treat as "no data" — return zero-filled result.
		return fillDailyBucketsP95(days, nil), nil
	}

	rows, err := q.BacktestP95Pass2(ctx, sqlc.BacktestP95Pass2Params{
		MeteringPointID: pgID,
		Column2:         interval,
		AvgInstant:      p95,
	})
	if err != nil {
		return BacktestResult{}, err
	}
	return fillDailyBucketsP95(days, rows), nil
}

// backtestIQR performs a two-pass IQR (Tukey fences) backtest.
func backtestIQR(ctx context.Context, q *sqlc.Queries, pgID pgtype.UUID, interval pgtype.Interval, days int) (BacktestResult, error) {
	iqrRow, err := q.BacktestIQRPass1(ctx, sqlc.BacktestIQRPass1Params{
		MeteringPointID: pgID,
		Column2:         interval,
	})
	if err != nil {
		// Zero rows → NULL scan fails. Treat as no data.
		return fillDailyBucketsIQR(days, nil), nil
	}

	iqr := iqrRow.Q3 - iqrRow.Q1
	low := iqrRow.Q1 - 1.5*iqr
	high := iqrRow.Q3 + 1.5*iqr

	rows, err := q.BacktestIQRPass2(ctx, sqlc.BacktestIQRPass2Params{
		MeteringPointID: pgID,
		Column2:         interval,
		AvgInstant:      low,
		AvgInstant_2:    high,
	})
	if err != nil {
		return BacktestResult{}, err
	}
	return fillDailyBucketsIQR(days, rows), nil
}

// backtestQuietHour performs a single-pass quiet-hour flow backtest.
func backtestQuietHour(ctx context.Context, q *sqlc.Queries, pgID pgtype.UUID, interval pgtype.Interval, days int) (BacktestResult, error) {
	rows, err := q.BacktestQuietHourCount(ctx, sqlc.BacktestQuietHourCountParams{
		MeteringPointID: pgID,
		Column2:         interval,
		Column3:         quietHourStart,
		Column4:         quietHourEnd,
		AvgInstant:      quietHourFlowThreshold,
	})
	if err != nil {
		return BacktestResult{}, err
	}
	return fillDailyBucketsQuietHour(days, rows), nil
}

// ─── Typed fill helpers (Checker M-1: no `any` type erasure) ──────────────────
//
// Each helper extracts (Day, Fires) from the sqlc-generated row type for
// its respective query, then delegates to fillDailyBucketsCore.

// fillDailyBucketsP95 builds a BacktestResult from BacktestP95Pass2 rows.
func fillDailyBucketsP95(days int, rows []sqlc.BacktestP95Pass2Row) BacktestResult {
	m := make(map[string]int, len(rows))
	for _, r := range rows {
		if r.Day.Valid {
			m[r.Day.Time.Format("2006-01-02")] = int(r.Fires)
		}
	}
	return fillDailyBucketsCore(days, m)
}

// fillDailyBucketsIQR builds a BacktestResult from BacktestIQRPass2 rows.
func fillDailyBucketsIQR(days int, rows []sqlc.BacktestIQRPass2Row) BacktestResult {
	m := make(map[string]int, len(rows))
	for _, r := range rows {
		if r.Day.Valid {
			m[r.Day.Time.Format("2006-01-02")] = int(r.Fires)
		}
	}
	return fillDailyBucketsCore(days, m)
}

// fillDailyBucketsQuietHour builds a BacktestResult from BacktestQuietHourCount rows.
func fillDailyBucketsQuietHour(days int, rows []sqlc.BacktestQuietHourCountRow) BacktestResult {
	m := make(map[string]int, len(rows))
	for _, r := range rows {
		if r.Day.Valid {
			m[r.Day.Time.Format("2006-01-02")] = int(r.Fires)
		}
	}
	return fillDailyBucketsCore(days, m)
}

// fillDailyBucketsCore builds exactly `days` DailyFireBucket entries, ordered
// oldest-first, zero-filling days with no fires. Shared across all 3 typed
// fill helpers.
func fillDailyBucketsCore(days int, dayCounts map[string]int) BacktestResult {
	result := BacktestResult{DailyFires: make([]DailyFireBucket, 0, days)}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	for i := days - 1; i >= 0; i-- {
		d := today.Add(-time.Duration(i) * 24 * time.Hour).Format("2006-01-02")
		c := dayCounts[d]
		result.DailyFires = append(result.DailyFires, DailyFireBucket{Day: d, Count: c})
		result.FiresCount += c
	}
	return result
}
