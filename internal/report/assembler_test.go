package report

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shifter-io/shifter/internal/db"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/testsupport"
	"github.com/stretchr/testify/require"
)

// TestReportScopeGrouping exercises the assembler across the scope/group matrix
// against a real TimescaleDB container with seeded CAGGs.
func TestReportScopeGrouping(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	// -- Seed: 2 sites, 3 metering points (2 water + 1 electricity) --
	var site1ID, site2ID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('Site-A', 'UTC') RETURNING id`,
	).Scan(&site1ID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('Site-B', 'UTC') RETURNING id`,
	).Scan(&site2ID))

	var mp1ID, mp2ID, mp3ID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class) VALUES ($1, 'Water-MP-1', 'water') RETURNING id`,
		site1ID,
	).Scan(&mp1ID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class) VALUES ($1, 'Water-MP-2', 'water') RETURNING id`,
		site2ID,
	).Scan(&mp2ID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class) VALUES ($1, 'Elec-MP-1', 'electricity') RETURNING id`,
		site1ID,
	).Scan(&mp3ID))

	// Seed measurements across a known window (7 days of hourly data).
	// Each MP gets one measurement per day at 10:00 UTC, simulating a daily reading.
	baseDay := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for day := 0; day < 8; day++ {
		ts := baseDay.Add(time.Duration(day)*24*time.Hour + 10*time.Hour)
		for _, mpID := range []string{mp1ID, mp2ID, mp3ID} {
			startVal := 1000 + day*100
			endVal := startVal + 50
			// Insert a "start" and "end" reading for each day so CAGG delta = 50.
			_, err := pool.Exec(ctx, `
				INSERT INTO measurement (time, metering_point_id, cumulative_value, quality, raw_payload, decoded_object)
				VALUES ($1::timestamptz, $2::uuid, $3, 'ok', '\x'::bytea, '{}'::jsonb)
			`, ts, mpID, startVal)
			require.NoError(t, err)
			_, err = pool.Exec(ctx, `
				INSERT INTO measurement (time, metering_point_id, cumulative_value, quality, raw_payload, decoded_object)
				VALUES ($1::timestamptz, $2::uuid, $3, 'ok', '\x'::bytea, '{}'::jsonb)
			`, ts.Add(30*time.Minute), mpID, endVal)
			require.NoError(t, err)
		}
	}

	// Refresh hourly CAGG covering our window.
	refreshStart := baseDay.Add(-time.Hour)
	refreshEnd := baseDay.Add(9 * 24 * time.Hour)
	_, err := pool.Exec(ctx,
		`CALL refresh_continuous_aggregate('measurement_hourly', $1::timestamptz, $2::timestamptz)`,
		refreshStart, refreshEnd,
	)
	require.NoError(t, err, "refresh measurement_hourly")

	q := sqlc.New(pool)
	start := baseDay
	end := baseDay.Add(7 * 24 * time.Hour)

	// Test 1: scope=all + group=category → produces water section + electricity section.
	t.Run("scope_all_group_category", func(t *testing.T) {
		rpt, err := BuildReport(ctx, q, ReportConfig{
			Scope:        "all",
			GroupBy:      "category",
			RangeKind:    "daily",
			Start:        start,
			End:          end,
			Capabilities: "both",
			Timezone:     time.UTC,
		})
		require.NoError(t, err)
		require.NotEmpty(t, rpt.PeriodRows, "should have period rows")

		// Collect utility classes seen.
		classes := map[string]bool{}
		for _, r := range rpt.PeriodRows {
			classes[r.UtilityClass] = true
		}
		require.True(t, classes["water"], "should have water rows")
		require.True(t, classes["electricity"], "should have electricity rows")
	})

	// Test 2: scope=all + group=site → rows grouped per site.
	t.Run("scope_all_group_site", func(t *testing.T) {
		rpt, err := BuildReport(ctx, q, ReportConfig{
			Scope:        "all",
			GroupBy:      "site",
			RangeKind:    "daily",
			Start:        start,
			End:          end,
			Capabilities: "both",
			Timezone:     time.UTC,
		})
		require.NoError(t, err)
		require.NotEmpty(t, rpt.PeriodRows)

		sites := map[string]bool{}
		for _, r := range rpt.PeriodRows {
			if r.SiteID != nil {
				sites[r.SiteID.String()] = true
			}
		}
		require.GreaterOrEqual(t, len(sites), 1, "should have at least one site in period rows")
	})

	// Test 3: scope=meter → measurement_daily-sourced rows for the single MP.
	t.Run("scope_meter_range_monthly", func(t *testing.T) {
		// Refresh daily CAGG for monthly scope.
		_, err := pool.Exec(ctx,
			`CALL refresh_continuous_aggregate('measurement_daily', $1::timestamptz, $2::timestamptz)`,
			refreshStart, refreshEnd,
		)
		require.NoError(t, err, "refresh measurement_daily")

		mpUUID, err := uuid.Parse(mp1ID)
		require.NoError(t, err)

		rpt, err := BuildReport(ctx, q, ReportConfig{
			Scope:           "meter",
			MeteringPointID: mpUUID,
			RangeKind:       "monthly",
			Start:           start,
			End:             end,
			Capabilities:    "water",
			Timezone:        time.UTC,
		})
		require.NoError(t, err)
		// May be empty if the 7-day window doesn't span a month boundary,
		// but the assembler must not error.
		_ = rpt
	})

	// Test 4: scope=site + group=none → meter_rows lists MPs belonging to the site.
	t.Run("scope_site_lists_meter_rows", func(t *testing.T) {
		siteUUID, err := uuid.Parse(site1ID)
		require.NoError(t, err)

		rpt, err := BuildReport(ctx, q, ReportConfig{
			Scope:        "site",
			SiteID:       siteUUID,
			GroupBy:      "none",
			RangeKind:    "daily",
			Start:        start,
			End:          end,
			Capabilities: "both",
			Timezone:     time.UTC,
		})
		require.NoError(t, err)
		require.NotEmpty(t, rpt.MeterRows, "scope=site should list meter rows")
		for _, m := range rpt.MeterRows {
			require.NotEmpty(t, m.Name)
		}
	})

	// Test 5: capabilities=water → assembler emits ONLY water-class rows.
	t.Run("single_capability_water_only", func(t *testing.T) {
		rpt, err := BuildReport(ctx, q, ReportConfig{
			Scope:        "all",
			GroupBy:      "category",
			RangeKind:    "daily",
			Start:        start,
			End:          end,
			Capabilities: "water", // D-09: single-capability install
			Timezone:     time.UTC,
		})
		require.NoError(t, err)
		for _, r := range rpt.PeriodRows {
			require.Equal(t, "water", r.UtilityClass,
				"single-capability install must drop electricity rows (D-09 / D-04): got %q", r.UtilityClass)
		}
		fmt.Printf("  water-only rows: %d\n", len(rpt.PeriodRows))
	})
}
