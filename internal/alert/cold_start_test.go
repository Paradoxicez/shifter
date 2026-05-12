package alert

import (
	"context"
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

	// Case 1: MP with no measurements at all.
	noData := env.seedMP(t, "cs-no-data")
	eligible, err := IsMPEligibleForAnomaly(ctx, env.queries, noData)
	require.NoError(t, err)
	require.False(t, eligible, "MP with no measurement history must not be eligible")

	// Case 2: MP with oldest measurement only 5 days old.
	fresh := env.seedMP(t, "cs-fresh")
	env.seedMeasurement(t, fresh, time.Now().UTC().Add(-5*24*time.Hour))
	eligible, err = IsMPEligibleForAnomaly(ctx, env.queries, fresh)
	require.NoError(t, err)
	require.False(t, eligible, "MP with < 21d history must not be eligible")
}

// TestColdStart_OldMPIsEligible: MP with measurements ≥ 21 days ago is
// eligible.
func TestColdStart_OldMPIsEligible(t *testing.T) {
	env := newColdStartTestEnv(t)
	ctx := context.Background()

	mp := env.seedMP(t, "cs-old")
	// Seed at 22 days back, strictly older than 21 days.
	env.seedMeasurement(t, mp, time.Now().UTC().Add(-22*24*time.Hour))

	eligible, err := IsMPEligibleForAnomaly(ctx, env.queries, mp)
	require.NoError(t, err)
	require.True(t, eligible, "MP with ≥ 21d history must be eligible")
}

// TestWarmupRoster_DaysUntilEligible: roster reports correct days_until_eligible
// for varying ages.
func TestWarmupRoster_DaysUntilEligible(t *testing.T) {
	env := newColdStartTestEnv(t)
	ctx := context.Background()

	// MP with no measurements → 21 days until eligible.
	mpNoData := env.seedMP(t, "cs-noData")
	// MP with 5 days of history → 16 days until eligible.
	mp5 := env.seedMP(t, "cs-5days")
	env.seedMeasurement(t, mp5, time.Now().UTC().Add(-5*24*time.Hour))
	// MP eligible already (≥ 21 days) → 0.
	mpEligible := env.seedMP(t, "cs-eligible")
	env.seedMeasurement(t, mpEligible, time.Now().UTC().Add(-25*24*time.Hour))

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

// TestWarmupRoster_OrderingByDaysUntilEligible: roster returns rows ASC by
// days_until_eligible (eligible MPs first; longest-warmup last).
func TestWarmupRoster_OrderingByDaysUntilEligible(t *testing.T) {
	env := newColdStartTestEnv(t)
	ctx := context.Background()

	// Seed three MPs: 0 days (eligible), 16 days (5d old), 21 days (no data).
	mpNoData := env.seedMP(t, "cs-order-nodata")
	mp5 := env.seedMP(t, "cs-order-5days")
	env.seedMeasurement(t, mp5, time.Now().UTC().Add(-5*24*time.Hour))
	mpEligible := env.seedMP(t, "cs-order-eligible")
	env.seedMeasurement(t, mpEligible, time.Now().UTC().Add(-25*24*time.Hour))

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
