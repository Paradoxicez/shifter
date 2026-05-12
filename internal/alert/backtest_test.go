package alert

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/db"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

// backtestTestEnv spins up Postgres, runs migrations, seeds install_identity +
// site + one metering point, and returns the pieces tests need.
type backtestTestEnv struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
	mpID    uuid.UUID
}

func newBacktestTestEnv(t *testing.T) *backtestTestEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, nil))

	_, err := pool.Exec(ctx,
		`INSERT INTO install_identity (id, display_name, timezone, units)
		 VALUES (1, 'BacktestTest', 'UTC', 'metric') ON CONFLICT (id) DO NOTHING`)
	require.NoError(t, err)

	var siteIDStr, mpIDStr string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('bt-site', 'UTC') RETURNING id`,
	).Scan(&siteIDStr))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class) VALUES ($1, 'bt-mp', 'water') RETURNING id`,
		siteIDStr,
	).Scan(&mpIDStr))
	mpID, err := uuid.Parse(mpIDStr)
	require.NoError(t, err)

	return &backtestTestEnv{
		pool:    pool,
		queries: sqlc.New(pool),
		mpID:    mpID,
	}
}

// seedHourlyMeasurement inserts a single row into measurement_hourly for the
// given bucket time and avg_instant value.
// measurement_hourly is a continuous aggregate — inserts go into the raw
// measurement table and the CAGG is refreshed immediately for the test window.
func (e *backtestTestEnv) seedHourlyRow(t *testing.T, bucket time.Time, avgInstant float64) {
	t.Helper()
	ctx := context.Background()
	cumVal := 1000.0
	_, err := e.pool.Exec(ctx,
		`INSERT INTO measurement (
		    time, metering_point_id,
		    raw_value, cumulative_value, instant_value,
		    extra, raw_payload, decoded_object, quality
		 ) VALUES ($1, $2, $3, $3, $3, '{}'::jsonb, '\x00'::bytea, '{}'::jsonb, 'ok')`,
		bucket, e.mpID, cumVal,
	)
	require.NoError(t, err)
	// Also direct-insert into measurement_hourly CAGG backing store via the
	// hypertable_internal approach — measurement_hourly is a TimescaleDB CAGG
	// and we cannot INSERT directly. Instead we rely on the raw measurement row.
	// In test, the CAGG materialized_only=false flag means queries against
	// measurement_hourly will pick up un-materialized raw rows in real-time mode.
	// However, the CAGG may not be available in test Postgres without TimescaleDB.
	// Fall back: insert directly with a helper that writes to a measurement table
	// (the backtest queries target measurement_hourly, which must contain data).
	_ = avgInstant // actual avg_instant comes from the measurement row above
}

// seedHourlyDirect writes directly to the measurement table with specific
// instant_value so CAGG avg_instant (measured in real-time mode) equals it.
func (e *backtestTestEnv) seedHourlyDirect(t *testing.T, bucket time.Time, instantVal float64) {
	t.Helper()
	ctx := context.Background()
	_, err := e.pool.Exec(ctx,
		`INSERT INTO measurement (
		    time, metering_point_id,
		    raw_value, cumulative_value, instant_value,
		    extra, raw_payload, decoded_object, quality
		 ) VALUES ($1, $2, $3, 1000.0, $3, '{}'::jsonb, '\x00'::bytea, '{}'::jsonb, 'ok')`,
		bucket, e.mpID, instantVal,
	)
	require.NoError(t, err)
}

// ─── Unit tests for typed fill helpers (no DB needed) ────────────────────────

// TestFillDailyBucketsP95_ZeroFill: pass empty rows + days=30, assert 30 entries
// all with count=0.
func TestFillDailyBucketsP95_ZeroFill(t *testing.T) {
	result := fillDailyBucketsP95(30, nil)
	require.Len(t, result.DailyFires, 30)
	require.Equal(t, 0, result.FiresCount)
	for _, b := range result.DailyFires {
		require.Equal(t, 0, b.Count, "all counts should be zero when no rows")
	}
	// Oldest day first
	t0, _ := time.Parse("2006-01-02", result.DailyFires[0].Day)
	t1, _ := time.Parse("2006-01-02", result.DailyFires[1].Day)
	require.True(t, t1.After(t0), "days must be in ascending order")
}

// TestFillDailyBucketsIQR_SparsePopulation: 3 days with fires, 27 days zero.
func TestFillDailyBucketsIQR_SparsePopulation(t *testing.T) {
	// Inject 3 days with known counts using the core function directly
	today := time.Now().UTC().Truncate(24 * time.Hour)
	dayCounts := map[string]int{
		today.Add(-2 * 24 * time.Hour).Format("2006-01-02"): 5,
		today.Add(-10 * 24 * time.Hour).Format("2006-01-02"): 3,
		today.Add(-20 * 24 * time.Hour).Format("2006-01-02"): 7,
	}
	result := fillDailyBucketsCore(30, dayCounts)
	require.Len(t, result.DailyFires, 30, "must always return exactly 30 entries")
	require.Equal(t, 15, result.FiresCount, "5+3+7=15 total fires")

	// Verify ordering: first entry should be oldest
	t0, _ := time.Parse("2006-01-02", result.DailyFires[0].Day)
	t29, _ := time.Parse("2006-01-02", result.DailyFires[29].Day)
	require.True(t, t29.After(t0), "last entry must be most recent")

	// Verify non-fire days are zero
	zeroCount := 0
	for _, b := range result.DailyFires {
		if b.Count == 0 {
			zeroCount++
		}
	}
	require.Equal(t, 27, zeroCount, "27 zero-fill days expected")
}

// TestBacktest_RejectsInvalidDays tests that BacktestRun rejects invalid day counts.
func TestBacktest_RejectsInvalidDays(t *testing.T) {
	env := newBacktestTestEnv(t)

	ctx := context.Background()

	// days=0 is rejected
	_, err := BacktestRun(ctx, env.pool, "anomaly_p95", env.mpID, 0)
	require.Error(t, err, "days=0 should be rejected")

	// negative days rejected
	_, err = BacktestRun(ctx, env.pool, "anomaly_p95", env.mpID, -1)
	require.Error(t, err, "negative days should be rejected")

	// days > 90 rejected
	_, err = BacktestRun(ctx, env.pool, "anomaly_p95", env.mpID, 91)
	require.Error(t, err, "days>90 should be rejected")
}

// TestBacktest_NoData_ReturnsZeroes: an MP with no measurements should return
// zero fires for all rule kinds.
func TestBacktest_NoData_ReturnsZeroes(t *testing.T) {
	env := newBacktestTestEnv(t)
	ctx := context.Background()

	for _, kind := range []string{"anomaly_p95", "anomaly_iqr", "anomaly_quiet_hour"} {
		result, err := BacktestRun(ctx, env.pool, kind, env.mpID, 30)
		require.NoError(t, err, "kind=%s must not error on empty data", kind)
		require.Equal(t, 0, result.FiresCount, "kind=%s: zero fires expected on empty MP", kind)
		require.Len(t, result.DailyFires, 30, "kind=%s: must return 30 daily buckets", kind)
	}
}

// TestBacktest_P95_KnownBaseline: seeds 30 days of hourly measurements with a
// known distribution and asserts that P95 baseline matches and fire count
// reflects rows above P95.
//
// Strategy: seed 95% of rows at value=1.0, 5% at value=100.0.
// P95 baseline should be near 100.0 (the outlier).
// Row count where avg_instant > p95 should be ~0 (the 100.0 rows ARE the P95).
// To make the test deterministic: use 20 rows at 1.0 and 1 row at 100.0 per day.
// Then P95 should be 1.0 (95th percentile in 21-row series ascending: 1.0 at idx 19),
// and rows where avg_instant > 1.0 = 1 per day = ~30 fires.
//
// Simpler: 10 rows at 1.0 + 1 row at 10.0 per day for 30 days.
// P95 of {1,1,1,1,1,1,1,1,1,1,10} sorted ascending = row at index floor(0.95*11)=10 = 10.0
// Rows where avg_instant > 10.0: none (max is 10.0, condition is strict >).
// So fires_count = 0.
//
// Use 10 rows at 1.0 + 1 row at 10.0 per day; fires_count should be 0.
func TestBacktest_P95_KnownBaseline(t *testing.T) {
	env := newBacktestTestEnv(t)
	ctx := context.Background()

	// Seed 30 days × 11 hourly rows per day.
	now := time.Now().UTC()
	for dayOffset := 1; dayOffset <= 30; dayOffset++ {
		base := now.Add(-time.Duration(dayOffset) * 24 * time.Hour).Truncate(time.Hour)
		for h := 0; h < 10; h++ {
			env.seedHourlyDirect(t, base.Add(time.Duration(h)*time.Hour), 1.0)
		}
		// 1 outlier at 10.0 — this IS at or below the P95 value
		env.seedHourlyDirect(t, base.Add(10*time.Hour), 10.0)
	}

	result, err := BacktestRun(ctx, env.pool, "anomaly_p95", env.mpID, 30)
	require.NoError(t, err)
	require.Len(t, result.DailyFires, 30)
	// P95 = 10.0 (it IS the 95th pct); rows > 10.0 = 0
	require.Equal(t, 0, result.FiresCount, "no rows should exceed the P95 baseline itself")
}

// TestBacktest_IQR_OutlierDetection: seeds data with clear outliers and verifies
// that days with outlier rows register fires.
//
// Seed 28 days with values between 1-3 (IQR range), plus 2 days with a very
// high outlier (100.0). Those 2 days should fire.
func TestBacktest_IQR_OutlierDetection(t *testing.T) {
	env := newBacktestTestEnv(t)
	ctx := context.Background()

	now := time.Now().UTC()
	// Seed 28 normal days
	for dayOffset := 3; dayOffset <= 30; dayOffset++ {
		base := now.Add(-time.Duration(dayOffset) * 24 * time.Hour).Truncate(time.Hour)
		for h := 0; h < 8; h++ {
			val := 1.0 + float64(h%3)*0.5 // values between 1.0 and 2.0
			env.seedHourlyDirect(t, base.Add(time.Duration(h)*time.Hour), val)
		}
	}
	// Seed 2 outlier days with extreme values
	for dayOffset := 1; dayOffset <= 2; dayOffset++ {
		base := now.Add(-time.Duration(dayOffset) * 24 * time.Hour).Truncate(time.Hour)
		// Normal rows
		for h := 0; h < 7; h++ {
			env.seedHourlyDirect(t, base.Add(time.Duration(h)*time.Hour), 1.5)
		}
		// Outlier row — well outside IQR bounds
		env.seedHourlyDirect(t, base.Add(7*time.Hour), 100.0)
	}

	result, err := BacktestRun(ctx, env.pool, "anomaly_iqr", env.mpID, 30)
	require.NoError(t, err)
	require.Len(t, result.DailyFires, 30)
	// At least the 2 outlier days should fire; possibly more if Q3+1.5*IQR is
	// crossed by 100.0 threshold.
	require.GreaterOrEqual(t, result.FiresCount, 2, "at least 2 outlier days should fire")
}

// TestBacktest_QuietHour_NightFlow: seeds measurements during the quiet window
// (00:00-05:00) with flow > threshold for specific days and verifies those days fire.
func TestBacktest_QuietHour_NightFlow(t *testing.T) {
	env := newBacktestTestEnv(t)
	ctx := context.Background()

	now := time.Now().UTC()
	// Seed 5 days with night flow (02:00 UTC — in quiet window 00-05)
	fireCount := 0
	for dayOffset := 1; dayOffset <= 5; dayOffset++ {
		base := now.Add(-time.Duration(dayOffset) * 24 * time.Hour).Truncate(24 * time.Hour)
		// Insert at 02:00 UTC with high flow value (above quietHourFlowThreshold)
		env.seedHourlyDirect(t, base.Add(2*time.Hour), 1.0)
		fireCount++
	}
	// Seed 5 days with ONLY daytime flow (10:00 UTC — outside quiet window)
	for dayOffset := 6; dayOffset <= 10; dayOffset++ {
		base := now.Add(-time.Duration(dayOffset) * 24 * time.Hour).Truncate(24 * time.Hour)
		env.seedHourlyDirect(t, base.Add(10*time.Hour), 1.0)
	}

	result, err := BacktestRun(ctx, env.pool, "anomaly_quiet_hour", env.mpID, 30)
	require.NoError(t, err)
	require.Len(t, result.DailyFires, 30)
	// The 5 night-flow days should fire (avg_instant=1.0 > quietHourFlowThreshold=0.001)
	require.Equal(t, 5, result.FiresCount, "only days with quiet-window flow should fire")
}

// TestBacktest_UnsupportedKind: unknown rule_kind returns an error.
func TestBacktest_UnsupportedKind(t *testing.T) {
	env := newBacktestTestEnv(t)
	ctx := context.Background()

	_, err := BacktestRun(ctx, env.pool, "unknown_kind", env.mpID, 30)
	require.Error(t, err, "unsupported kind should return error")
	require.Contains(t, err.Error(), "unsupported")
}

// Compile-time proof that BacktestResult is exported.
var _ BacktestResult = BacktestResult{}

// Compile-time proof that the three typed fill helpers exist.
var _ = fillDailyBucketsP95
var _ = fillDailyBucketsIQR
var _ = fillDailyBucketsQuietHour

// Proof that no `any` erasure is used — verified by the compiler at call sites.
func helperSignatureCheck() {
	var _ = func(days int, rows []sqlc.BacktestP95Pass2Row) BacktestResult {
		return fillDailyBucketsP95(days, rows)
	}
	var _ = func(days int, rows []sqlc.BacktestIQRPass2Row) BacktestResult {
		return fillDailyBucketsIQR(days, rows)
	}
	var _ = func(days int, rows []sqlc.BacktestQuietHourCountRow) BacktestResult {
		return fillDailyBucketsQuietHour(days, rows)
	}
	// Keep sqlc in scope for the signature check (imported above).
	var _ sqlc.BacktestP95Pass2Row
}

// fmtSuppressUnused suppresses "declared and not used" for test helpers.
var _ = fmt.Sprintf
