package alert

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/db"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// coldStartTestEnv is the shared fixture for Task 1 cold-start tests: spin up
// Postgres, run migrations, seed install_identity + site + MP, and return the
// pieces tests need to seed measurements at varying ages.
type coldStartTestEnv struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
	siteID  uuid.UUID
}

func newColdStartTestEnv(t *testing.T) *coldStartTestEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	_, err := pool.Exec(ctx,
		`INSERT INTO install_identity (id, display_name, timezone, units)
		 VALUES (1, 'ColdStart Test Install', 'UTC', 'metric')
		 ON CONFLICT (id) DO NOTHING`)
	require.NoError(t, err)

	var siteIDStr string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('cs-site', 'UTC') RETURNING id`,
	).Scan(&siteIDStr))
	siteID, err := uuid.Parse(siteIDStr)
	require.NoError(t, err)

	return &coldStartTestEnv{
		pool:    pool,
		queries: sqlc.New(pool),
		siteID:  siteID,
	}
}

// seedMP creates a metering_point row with the given label.
func (e *coldStartTestEnv) seedMP(t *testing.T, label string) uuid.UUID {
	t.Helper()
	var idStr string
	require.NoError(t, e.pool.QueryRow(context.Background(),
		`INSERT INTO metering_point (site_id, name, utility_class)
		 VALUES ($1, $2, 'water') RETURNING id`,
		e.siteID, label,
	).Scan(&idStr))
	id, err := uuid.Parse(idStr)
	require.NoError(t, err)
	return id
}

// seedMeasurement inserts a measurement row at the given time with a nominal
// instant_value of 1.0. Used to age MP history.
func (e *coldStartTestEnv) seedMeasurement(t *testing.T, mpID uuid.UUID, when time.Time) {
	t.Helper()
	_, err := e.pool.Exec(context.Background(),
		`INSERT INTO measurement (
		    time, metering_point_id,
		    raw_value, cumulative_value, instant_value,
		    extra, raw_payload, decoded_object, quality
		 ) VALUES ($1, $2, 1.0, 1.0, 1.0, '{}'::jsonb, '\x00'::bytea, '{}'::jsonb, 'ok')`,
		when, mpID,
	)
	require.NoError(t, err)
}

// TestColdStart_NewMPNotEligible: an MP with no measurements OR oldest
// measurement < 21 days ago is NOT eligible (D-16).
func TestColdStart_NewMPNotEligible(t *testing.T) {
	env := newColdStartTestEnv(t)
	ctx := context.Background()

	// Case 1: MP bound to 'full' profile but no measurements at all.
	noData := env.seedMPWithProfile(t, "cs-no-data", "full", 3600)
	eligible, err := IsMPEligibleForAnomaly(ctx, env.queries, noData, "anomaly_p95")
	require.NoError(t, err)
	require.False(t, eligible, "MP with no measurement history must not be eligible")

	// Case 2: MP bound to 'full' profile with oldest measurement only 5 days old.
	fresh := env.seedMPWithProfile(t, "cs-fresh", "full", 3600)
	env.seedMeasurement(t, fresh, time.Now().UTC().Add(-5*24*time.Hour))
	eligible, err = IsMPEligibleForAnomaly(ctx, env.queries, fresh, "anomaly_p95")
	require.NoError(t, err)
	require.False(t, eligible, "MP with < 21d history must not be eligible")
}

// TestColdStart_OldMPIsEligible: MP bound to 'full' profile with measurements
// ≥ 21 days ago is eligible.
func TestColdStart_OldMPIsEligible(t *testing.T) {
	env := newColdStartTestEnv(t)
	ctx := context.Background()

	mp := env.seedMPWithProfile(t, "cs-old", "full", 3600)
	// Seed at 22 days back, strictly older than 21 days.
	env.seedMeasurement(t, mp, time.Now().UTC().Add(-22*24*time.Hour))

	eligible, err := IsMPEligibleForAnomaly(ctx, env.queries, mp, "anomaly_p95")
	require.NoError(t, err)
	require.True(t, eligible, "MP with ≥ 21d history must be eligible")
}

// TestWarmupRoster_DaysUntilEligible: roster reports correct days_until_eligible
// for varying ages. Time-arithmetic-edge: we seed measurements at +12h offsets
// (5d12h instead of 5d) so EXTRACT(DAY FROM now() - m.time) lands stably on
// the integer floor (no jitter between 4 and 5).
func TestWarmupRoster_DaysUntilEligible(t *testing.T) {
	env := newColdStartTestEnv(t)
	ctx := context.Background()

	// MP with no measurements → 21 days until eligible.
	mpNoData := env.seedMP(t, "cs-noData")
	// MP with ~5.5 days of history → 21-5=16 days until eligible.
	mp5 := env.seedMP(t, "cs-5days")
	env.seedMeasurement(t, mp5, time.Now().UTC().Add(-(5*24+12)*time.Hour))
	// MP eligible already (≥ 21 days) → 0.
	mpEligible := env.seedMP(t, "cs-eligible")
	env.seedMeasurement(t, mpEligible, time.Now().UTC().Add(-(25*24+12)*time.Hour))

	roster, err := ListAnomalyWarmupRoster(ctx, env.queries)
	require.NoError(t, err)

	byID := map[uuid.UUID]AnomalyEligibility{}
	for _, row := range roster {
		byID[row.MeteringPointID] = row
	}

	require.Contains(t, byID, mpNoData)
	require.Equal(t, int32(21), byID[mpNoData].DaysUntilEligible, "no-data MP must report 21 days")

	require.Contains(t, byID, mp5)
	require.Equal(t, int32(16), byID[mp5].DaysUntilEligible, "5-day-old MP must report 16 days")

	require.Contains(t, byID, mpEligible)
	require.Equal(t, int32(0), byID[mpEligible].DaysUntilEligible, "eligible MP must report 0")
}

// ─────────────────────────────────────────────────────────────────────────────
// Phase 7 Plan 09b: profile-aware cold-start gate (Task 2)
// ─────────────────────────────────────────────────────────────────────────────

// seedMPWithProfile creates a metering_point bound to a specific device profile
// via a device + binding chain.
func (e *coldStartTestEnv) seedMPWithProfile(t *testing.T, label string, anomalyCompat string, intervalS int) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	// Create profile.
	var profileIDStr string
	slug := "test-compat-" + label
	err := e.pool.QueryRow(ctx, `SELECT id FROM device_profile WHERE slug = $1`, slug).Scan(&profileIDStr)
	if err != nil {
		require.NoError(t, e.pool.QueryRow(ctx,
			`INSERT INTO device_profile (slug, name, vendor, family, capabilities, codec_js,
			    expected_uplink_interval_seconds, offline_threshold_multiplier, anomaly_compatibility,
			    battery_curve)
			 VALUES ($1, $2, 'TestVendor', 'test-family', ARRAY['cumulative'], '// test', $3, 3.0, $4, 'linear_pct')
			 RETURNING id`,
			slug, "Test "+label, intervalS, anomalyCompat,
		).Scan(&profileIDStr))
	}
	profileID, err := uuid.Parse(profileIDStr)
	require.NoError(t, err)

	// Create device + binding.
	eui := fmt.Sprintf("%016x", fnvHash(label))
	var devIDStr string
	require.NoError(t, e.pool.QueryRow(ctx,
		`INSERT INTO device (dev_eui, name, device_profile_id) VALUES ($1, $2, $3) RETURNING id`,
		eui, "dev-"+label, profileID,
	).Scan(&devIDStr))
	devID, err := uuid.Parse(devIDStr)
	require.NoError(t, err)

	// Create MP.
	var mpIDStr string
	require.NoError(t, e.pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class) VALUES ($1, $2, 'water') RETURNING id`,
		e.siteID, label,
	).Scan(&mpIDStr))
	mpID, err := uuid.Parse(mpIDStr)
	require.NoError(t, err)

	// Create binding (valid_to IS NULL = active).
	require.NoError(t, e.pool.QueryRow(ctx,
		`INSERT INTO binding (device_id, metering_point_id, valid_from)
		 VALUES ($1, $2, now())
		 RETURNING id`,
		devID, mpID,
	).Scan(new(string)))

	return mpID
}

// fnvHash is a simple deterministic hash for test EUI generation.
func fnvHash(s string) uint64 {
	h := uint64(14695981039346656037)
	for _, c := range s {
		h ^= uint64(c)
		h *= 1099511628211
	}
	return h
}

// TestColdStart_LimitedProfile_60Days: Itron-like profile (anomaly_compatibility='limited').
// With 30d of history → not eligible. With 70d → eligible for p95/iqr, NOT for quiet_hour.
func TestColdStart_LimitedProfile_60Days(t *testing.T) {
	env := newColdStartTestEnv(t)
	ctx := context.Background()

	// MP bound to 'limited' profile.
	mpID := env.seedMPWithProfile(t, "cs-limited", "limited", 86400)

	// Case 1: 30d history → not eligible (needs 60d for limited).
	env.seedMeasurement(t, mpID, time.Now().UTC().Add(-30*24*time.Hour))
	eligibleP95, err := IsMPEligibleForAnomaly(ctx, env.queries, mpID, "anomaly_p95")
	require.NoError(t, err)
	require.False(t, eligibleP95, "limited profile with 30d history must not be eligible (needs 60d)")

	eligibleQH, err := IsMPEligibleForAnomaly(ctx, env.queries, mpID, "anomaly_quiet_hour")
	require.NoError(t, err)
	require.False(t, eligibleQH, "limited profile: quiet_hour always ineligible")

	// Case 2: add a measurement 70d old → p95/iqr become eligible; quiet_hour still not.
	env.seedMeasurement(t, mpID, time.Now().UTC().Add(-70*24*time.Hour))

	eligibleP95After, err := IsMPEligibleForAnomaly(ctx, env.queries, mpID, "anomaly_p95")
	require.NoError(t, err)
	require.True(t, eligibleP95After, "limited profile with 70d history must be eligible for p95")

	eligibleIQRAfter, err := IsMPEligibleForAnomaly(ctx, env.queries, mpID, "anomaly_iqr")
	require.NoError(t, err)
	require.True(t, eligibleIQRAfter, "limited profile with 70d history must be eligible for iqr")

	eligibleQHAfter, err := IsMPEligibleForAnomaly(ctx, env.queries, mpID, "anomaly_quiet_hour")
	require.NoError(t, err)
	require.False(t, eligibleQHAfter, "limited profile: quiet_hour must always be ineligible")
}

// TestColdStart_UnsupportedProfile_NeverEligible: profile with anomaly_compatibility='unsupported'
// → all three anomaly kinds always return false.
func TestColdStart_UnsupportedProfile_NeverEligible(t *testing.T) {
	env := newColdStartTestEnv(t)
	ctx := context.Background()

	mpID := env.seedMPWithProfile(t, "cs-unsupported", "unsupported", 3600)
	// Seed 90d of history — doesn't matter for unsupported.
	env.seedMeasurement(t, mpID, time.Now().UTC().Add(-90*24*time.Hour))

	for _, kind := range []string{"anomaly_p95", "anomaly_iqr", "anomaly_quiet_hour"} {
		eligible, err := IsMPEligibleForAnomaly(ctx, env.queries, mpID, kind)
		require.NoError(t, err)
		require.False(t, eligible, "unsupported profile must never be eligible for "+kind)
	}
}

// TestColdStart_FullProfile_21Days: profile with anomaly_compatibility='full'.
// With 30d history → eligible for all 3 kinds (21d threshold).
func TestColdStart_FullProfile_21Days(t *testing.T) {
	env := newColdStartTestEnv(t)
	ctx := context.Background()

	mpID := env.seedMPWithProfile(t, "cs-full", "full", 3600)
	// 30d of history — exceeds 21d warmup.
	env.seedMeasurement(t, mpID, time.Now().UTC().Add(-30*24*time.Hour))

	for _, kind := range []string{"anomaly_p95", "anomaly_iqr", "anomaly_quiet_hour"} {
		eligible, err := IsMPEligibleForAnomaly(ctx, env.queries, mpID, kind)
		require.NoError(t, err)
		require.True(t, eligible, "full profile with 30d history must be eligible for "+kind)
	}
}

// TestWarmupRoster_OrderingByDaysUntilEligible: roster returns rows ASC by
// days_until_eligible (eligible MPs first; longest-warmup last).
func TestWarmupRoster_OrderingByDaysUntilEligible(t *testing.T) {
	env := newColdStartTestEnv(t)
	ctx := context.Background()

	// Seed three MPs: 0 days (eligible), 16 days (5d old), 21 days (no data).
	// +12h offsets ensure stable EXTRACT(DAY FROM ...) floor.
	mpNoData := env.seedMP(t, "cs-order-nodata")
	mp5 := env.seedMP(t, "cs-order-5days")
	env.seedMeasurement(t, mp5, time.Now().UTC().Add(-(5*24+12)*time.Hour))
	mpEligible := env.seedMP(t, "cs-order-eligible")
	env.seedMeasurement(t, mpEligible, time.Now().UTC().Add(-(25*24+12)*time.Hour))

	roster, err := ListAnomalyWarmupRoster(ctx, env.queries)
	require.NoError(t, err)

	// Find positions of the three IDs we care about.
	pos := map[uuid.UUID]int{}
	for i, row := range roster {
		pos[row.MeteringPointID] = i
	}
	require.Contains(t, pos, mpEligible)
	require.Contains(t, pos, mp5)
	require.Contains(t, pos, mpNoData)
	require.Less(t, pos[mpEligible], pos[mp5], "eligible (0d) must come before 5-day-old (16d)")
	require.Less(t, pos[mp5], pos[mpNoData], "5-day-old (16d) must come before no-data (21d)")
}
