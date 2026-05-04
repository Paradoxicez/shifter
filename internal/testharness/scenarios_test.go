package testharness_test

import (
	"context"
	"io"
	"log/slog"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/db"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/ingest"
	"github.com/shifter-io/shifter/internal/resolver"
	"github.com/shifter-io/shifter/internal/swap"
	"github.com/shifter-io/shifter/internal/testharness"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// silentLogger is a slog.Logger that discards output — keeps integration test
// stderr quiet by default. Tests that need to inspect log lines should
// substitute their own handler.
func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fixtureSeed packages the small set of values seedAxiomaW1Fixture +
// seedAcrelADW300Fixture share with their callers. Distinct from
// testharness.Fixture because the helpers want a *pgxpool.Pool-backed
// view of the seeded UUIDs in pgtype.UUID form for direct SQL.
type fixtureSeed struct {
	siteID    pgtype.UUID
	mpID      pgtype.UUID
	profileID pgtype.UUID
}

// seedAxiomaW1Fixture seeds: admin user, site, MP, looks up the axioma_w1
// device_profile (from migration 0010 seed), creates outgoing + incoming
// devices, opens the active binding on the OUTGOING device with the given
// initial offset + last_raw value, then seeds the 5 axioma_w1 mapping rows
// per the pinned table in 02-13-PLAN.md (B2 fix — codec result keys, not
// migration 0010).
//
// The seeded mapping rows use json_pointer values matching axioma_w1.js
// result.* EXACTLY:
//
//	/cumulative_l    → raw_value (numeric)
//	/battery_pct     → battery_pct (int)
//	/temperature_c   → temperature_c (numeric)
//	/leak            → leak_detected (bool)
//	/tamper          → tamper_detected (bool)
func seedAxiomaW1Fixture(t *testing.T, pool *pgxpool.Pool, suffix string, initialOffset, initialLastRaw *big.Float) testharness.Fixture {
	t.Helper()
	ctx := context.Background()
	seed := seedSiteMPProfile(t, pool, "axioma_w1", suffix)

	// Operator (audit_log.user_id FK target).
	var operatorIDStr string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ($1, 'Op', 'x', 'admin') RETURNING id`,
		"op-"+suffix+"@example.com",
	).Scan(&operatorIDStr))
	operatorID := uuid.MustParse(operatorIDStr)

	// Outgoing + incoming devices, lowercase 16-hex dev_euis.
	outDevEUI := padDevEUI("axioma" + suffix + "out")
	inDevEUI := padDevEUI("axioma" + suffix + "in")
	var outIDStr, inIDStr string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO device (dev_eui, name, device_profile_id) VALUES ($1, $2, $3) RETURNING id`,
		outDevEUI, "out-"+suffix, seed.profileID,
	).Scan(&outIDStr))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO device (dev_eui, name, device_profile_id) VALUES ($1, $2, $3) RETURNING id`,
		inDevEUI, "in-"+suffix, seed.profileID,
	).Scan(&inIDStr))
	outDeviceID := uuid.MustParse(outIDStr)
	inDeviceID := uuid.MustParse(inIDStr)

	// Open the active binding on the outgoing device.
	bindingStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	offsetStr := "0"
	if initialOffset != nil {
		offsetStr = initialOffset.Text('f', -1)
	}
	var bidStr string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO binding (metering_point_id, device_id, valid_from, reading_offset)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		seed.mpID, outIDStr, bindingStart, offsetStr,
	).Scan(&bidStr))
	bindingID := uuid.MustParse(bidStr)

	// Optionally pre-seed binding.last_raw_value (for rollover scenarios).
	if initialLastRaw != nil {
		_, err := pool.Exec(ctx,
			`UPDATE binding SET last_raw_value = $1 WHERE id = $2`,
			initialLastRaw.Text('f', -1), bidStr,
		)
		require.NoError(t, err, "seed initial last_raw_value")
	}

	// Seed the 5 mapping rows per the pinned axioma_w1 table.
	require.NoError(t, testharness.SeedAxiomaW1Mappings(ctx, pool, uuid.UUID(seed.profileID.Bytes)))

	initOff := initialOffset
	if initOff == nil {
		initOff = big.NewFloat(0)
	}
	return testharness.Fixture{
		SiteID:        uuid.UUID(seed.siteID.Bytes),
		MPID:          uuid.UUID(seed.mpID.Bytes),
		ProfileID:     uuid.UUID(seed.profileID.Bytes),
		OperatorID:    operatorID,
		OutDeviceID:   outDeviceID,
		OutDevEUI:     outDevEUI,
		InDeviceID:    inDeviceID,
		InDevEUI:      inDevEUI,
		BindingID:     bindingID,
		BindingStart:  bindingStart,
		InitialOffset: initOff,
		InitialRaw:    initialLastRaw,
	}
}

// seedSiteMPProfile creates a site, an MP, and looks up the seeded device_profile
// by slug ('axioma_w1' or 'acrel_adw300' from migration 0010).
func seedSiteMPProfile(t *testing.T, pool *pgxpool.Pool, profileSlug, suffix string) fixtureSeed {
	t.Helper()
	ctx := context.Background()
	var s fixtureSeed
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ($1, 'UTC') RETURNING id`,
		"site-"+profileSlug+"-"+suffix,
	).Scan(&s.siteID))

	utility := "water"
	if profileSlug == "acrel_adw300" || profileSlug == "acrel_adl200" {
		utility = "electricity"
	}
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class) VALUES ($1, $2, $3) RETURNING id`,
		s.siteID, "mp-"+suffix, utility,
	).Scan(&s.mpID))

	require.NoError(t, pool.QueryRow(ctx,
		`SELECT id FROM device_profile WHERE slug = $1`, profileSlug,
	).Scan(&s.profileID))
	return s
}

// padDevEUI maps a label to a stable lowercase 16-hex dev_eui via FNV-1a.
// Each distinct input yields a distinct output (collision-resistant for the
// small label space these tests use), avoiding the "all letters strip to the
// same hex" pitfall a naive hex-only filter has.
func padDevEUI(s string) string {
	// FNV-1a 64-bit, inlined to avoid pulling hash/fnv into the test file.
	var h uint64 = 0xcbf29ce484222325
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 0x100000001b3
	}
	out := make([]byte, 16)
	const hex = "0123456789abcdef"
	for i := 15; i >= 0; i-- {
		out[i] = hex[h&0xF]
		h >>= 4
	}
	return string(out)
}

// sqlcLoader satisfies resolver.Loader using the production
// sqlc.GetActiveBindingByDevEUI query. Used by the in-process scenario tests
// so the resolver hits the real DB and returns real Binding values.
type sqlcLoader struct {
	pool *pgxpool.Pool
}

func (l *sqlcLoader) LoadActive(ctx context.Context, devEUI string, at time.Time) (resolver.Binding, error) {
	q := sqlc.New(l.pool)
	row, err := q.GetActiveBindingByDevEUI(ctx, sqlc.GetActiveBindingByDevEUIParams{
		DevEui:    devEUI,
		ValidFrom: pgtype.Timestamptz{Time: at, Valid: true},
	})
	if err != nil {
		// pgx ErrNoRows → ErrNoActiveBinding
		if err.Error() == "no rows in result set" {
			return resolver.Binding{}, resolver.ErrNoActiveBinding
		}
		return resolver.Binding{}, err
	}
	b := resolver.Binding{
		BindingID:       uuid.UUID(row.ID.Bytes),
		MeteringPointID: uuid.UUID(row.MeteringPointID.Bytes),
		DeviceID:        uuid.UUID(row.DeviceID.Bytes),
		DeviceProfileID: uuid.UUID(row.DeviceProfileID.Bytes),
		CounterModulus:  row.CounterModulus,
	}
	if row.ValidFrom.Valid {
		b.ValidFrom = row.ValidFrom.Time
	}
	if row.ValidTo.Valid {
		b.ValidTo = row.ValidTo.Time
	}
	if f, err := numericToBigFloat(row.ReadingOffset); err == nil {
		b.ReadingOffset = f
	} else {
		b.ReadingOffset = big.NewFloat(0)
	}
	if row.LastRawValue.Valid {
		if f, err := numericToBigFloat(row.LastRawValue); err == nil {
			b.LastRawValue = f
		}
	}
	return b, nil
}

func numericToBigFloat(n pgtype.Numeric) (*big.Float, error) {
	if !n.Valid {
		return big.NewFloat(0), nil
	}
	b, err := n.MarshalJSON()
	if err != nil {
		return nil, err
	}
	s := string(b)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	f, _, err := big.ParseFloat(s, 10, 128, big.ToNearestEven)
	return f, err
}

// buildIngestDeps constructs the ingest.Deps a scenario needs to drive
// UplinkHandler in-process: real pgxpool, real resolver fed by a real sqlc
// loader, real SQLCMappingStore.
func buildIngestDeps(t *testing.T, pool *pgxpool.Pool) (ingest.Deps, *resolver.Resolver) {
	t.Helper()
	loader := &sqlcLoader{pool: pool}
	r := resolver.New(loader)
	mappings := &ingest.SQLCMappingStore{Pool: pool}
	return ingest.Deps{
		Pool:     pool,
		Resolver: r,
		Mappings: mappings,
		Log:      silentLogger(),
	}, r
}

// runMigrations applies all migrations against the testcontainer pool.
func runMigrations(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
}

// applicationID is the placeholder CS application UUID used by all test
// fixtures. The harness's vendor builders default to this value but every
// scenario re-stamps it via BuildAxiomaW1UplinkForDevEUI.
const applicationID = "00000000-0000-0000-0000-000000000000"

// =============================================================================
// Scenario tests — run in-process (Publisher == nil) against testcontainer DB.
// =============================================================================

func TestScenario_CleanSwap(t *testing.T) {
	if testing.Short() {
		t.Skip("integration")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	runMigrations(t, pool)

	fixture := seedAxiomaW1Fixture(t, pool, "clean", big.NewFloat(0), nil)
	deps, _ := buildIngestDeps(t, pool)

	result, err := testharness.CleanSwap(ctx, testharness.RunInput{
		Pool:          pool,
		IngestDeps:    deps,
		SwapDeps:      swap.Deps{Pool: pool, Log: silentLogger()},
		Fixture:       fixture,
		ApplicationID: applicationID,
		Log:           silentLogger(),
	})
	require.NoError(t, err)
	require.Equal(t, 2, result.MeasurementRows, "2 measurement rows (pre + post swap)")
	require.Equal(t, 1, result.AuditSwapRows, "1 audit row action='swap'")
	require.Equal(t, 0, result.AuditRolloverRows)

	// Final cumulative = 100 + 10500 = 10600.
	require.NotNil(t, result.FinalCumulative)
	expected := big.NewFloat(10600)
	require.Zero(t, result.FinalCumulative.Cmp(expected),
		"expected cumulative=%s got %s",
		expected.Text('f', 0), result.FinalCumulative.Text('f', 0))
}

func TestScenario_SwapWithInflightUplink(t *testing.T) {
	if testing.Short() {
		t.Skip("integration")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	runMigrations(t, pool)

	fixture := seedAxiomaW1Fixture(t, pool, "inflight", big.NewFloat(0), nil)
	deps, _ := buildIngestDeps(t, pool)

	result, err := testharness.SwapWithInflightUplink(ctx, testharness.RunInput{
		Pool:          pool,
		IngestDeps:    deps,
		SwapDeps:      swap.Deps{Pool: pool, Log: silentLogger()},
		Fixture:       fixture,
		ApplicationID: applicationID,
		Log:           silentLogger(),
	})
	require.NoError(t, err)
	require.Equal(t, 2, result.MeasurementRows, "2 measurement rows landed (no row dropped)")
	require.Equal(t, 1, result.AuditSwapRows)
	require.Equal(t, 0, result.AuditRolloverRows)

	// Final cumulative = post-swap raw 50 + offset 5000 = 5050.
	require.NotNil(t, result.FinalCumulative)
	require.Zero(t, result.FinalCumulative.Cmp(big.NewFloat(5050)),
		"final cumulative want 5050 got %s", result.FinalCumulative.Text('f', 0))
}

func TestScenario_Rollover(t *testing.T) {
	if testing.Short() {
		t.Skip("integration")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	runMigrations(t, pool)

	// Pre-seed binding.last_raw_value near 2^32 so the next uplink (raw=100)
	// triggers DetectRollover.
	prev := big.NewFloat(0).SetPrec(128).SetInt64(4294967200)
	fixture := seedAxiomaW1Fixture(t, pool, "rollover", big.NewFloat(0), prev)
	deps, _ := buildIngestDeps(t, pool)

	result, err := testharness.Rollover(ctx, testharness.RunInput{
		Pool:          pool,
		IngestDeps:    deps,
		SwapDeps:      swap.Deps{Pool: pool, Log: silentLogger()},
		Fixture:       fixture,
		ApplicationID: applicationID,
		Log:           silentLogger(),
	})
	require.NoError(t, err)
	require.Equal(t, 1, result.MeasurementRows)
	require.Equal(t, 0, result.AuditSwapRows)
	require.Equal(t, 1, result.AuditRolloverRows, "rollover_detected audit row")

	// Final cumulative = raw 100 + (offset 0 + 2^32 = 4294967296) = 4294967396.
	require.NotNil(t, result.FinalCumulative)
	expected := new(big.Float).SetPrec(128).SetInt64(4294967396)
	require.Zero(t, result.FinalCumulative.Cmp(expected),
		"final cumulative want 4294967396 got %s", result.FinalCumulative.Text('f', 0))
}

func TestScenario_SwapAndRollover(t *testing.T) {
	if testing.Short() {
		t.Skip("integration")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	runMigrations(t, pool)

	prev := new(big.Float).SetPrec(128).SetInt64(4294967200) // pre-seed near 2^32
	fixture := seedAxiomaW1Fixture(t, pool, "swaproll", big.NewFloat(0), prev)
	deps, _ := buildIngestDeps(t, pool)

	result, err := testharness.SwapAndRollover(ctx, testharness.RunInput{
		Pool:          pool,
		IngestDeps:    deps,
		SwapDeps:      swap.Deps{Pool: pool, Log: silentLogger()},
		Fixture:       fixture,
		ApplicationID: applicationID,
		Log:           silentLogger(),
	})
	require.NoError(t, err)
	require.Equal(t, 2, result.MeasurementRows, "rollover row + post-swap row")
	require.Equal(t, 1, result.AuditSwapRows)
	require.Equal(t, 1, result.AuditRolloverRows)

	// After rollover the outgoing's offset advanced by 2^32; measurement.cum at
	// the rollover row = 200 + 4294967296 = 4294967496. The swap then uses
	// R = 4294967496 → new binding offset = 4294967496. Post-swap raw=10 →
	// final cumulative = 10 + 4294967496 = 4294967506.
	require.NotNil(t, result.FinalCumulative)
	expected := new(big.Float).SetPrec(128).SetInt64(4294967506)
	require.Zero(t, result.FinalCumulative.Cmp(expected),
		"final cumulative want 4294967506 got %s", result.FinalCumulative.Text('f', 0))
}

func TestScenario_OverlappingUplinks(t *testing.T) {
	if testing.Short() {
		t.Skip("integration")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	runMigrations(t, pool)

	fixture := seedAxiomaW1Fixture(t, pool, "overlap", big.NewFloat(0), nil)
	deps, _ := buildIngestDeps(t, pool)

	result, err := testharness.OverlappingUplinks(ctx, testharness.RunInput{
		Pool:          pool,
		IngestDeps:    deps,
		SwapDeps:      swap.Deps{Pool: pool, Log: silentLogger()},
		Fixture:       fixture,
		ApplicationID: applicationID,
		Log:           silentLogger(),
	})
	require.NoError(t, err)
	require.Equal(t, 5, result.MeasurementRows, "5 uplinks all landed")
	require.Equal(t, 1, result.AuditSwapRows, "exactly 1 swap audit row")
	require.Equal(t, 0, result.AuditRolloverRows)

	// Final cumulative = last incoming raw 60 + new offset (R=1100 - N=0) = 1160.
	require.NotNil(t, result.FinalCumulative)
	require.Zero(t, result.FinalCumulative.Cmp(big.NewFloat(1160)),
		"final cumulative want 1160 got %s", result.FinalCumulative.Text('f', 0))
}

// TestScenario_AxiomaW1_E2E — DATA-10 proof. A single Axioma W1 synthetic
// uplink → ingest pipeline → measurement row with raw_payload + decoded_object
// preserved + cumulative_value populated.
func TestScenario_AxiomaW1_E2E(t *testing.T) {
	if testing.Short() {
		t.Skip("integration")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	runMigrations(t, pool)

	fixture := seedAxiomaW1Fixture(t, pool, "e2e", big.NewFloat(0), nil)
	deps, _ := buildIngestDeps(t, pool)

	// Synthesize a single Axioma W1 event with cumulative_l=12345.
	event, err := testharness.BuildAxiomaW1UplinkForDevEUI(
		fixture.OutDevEUI, applicationID,
		12345, 87, 23, false, false, 1,
		fixture.BindingStart.Add(time.Hour),
	)
	require.NoError(t, err)

	// Drive ingest in-process (Publisher == nil).
	in := testharness.RunInput{
		Pool:          pool,
		IngestDeps:    deps,
		SwapDeps:      swap.Deps{Pool: pool, Log: silentLogger()},
		Fixture:       fixture,
		ApplicationID: applicationID,
		Log:           silentLogger(),
	}
	// Use the package-internal publish helper via a CleanSwap-style loop —
	// but we want only ONE uplink, so call ingest.UplinkHandler directly.
	topic := "application/" + applicationID + "/device/" + fixture.OutDevEUI + "/event/up"
	ingest.UplinkHandler(deps)(topic, event)
	_ = in

	// Assert measurement row.
	q := sqlc.New(pool)
	count, err := q.CountMeasurementsByMP(ctx, pgtype.UUID{Bytes: fixture.MPID, Valid: true})
	require.NoError(t, err)
	require.Equal(t, int64(1), count, "exactly 1 measurement row from the single uplink")

	row, err := q.GetLatestMeasurement(ctx, pgtype.UUID{Bytes: fixture.MPID, Valid: true})
	require.NoError(t, err)

	// raw_value = 12345 (mapped from /cumulative_l).
	require.True(t, row.RawValue.Valid, "raw_value populated")
	rv, err := numericToBigFloat(row.RawValue)
	require.NoError(t, err)
	require.Zero(t, rv.Cmp(big.NewFloat(12345)),
		"raw_value want 12345 got %s", rv.Text('f', 0))

	// cumulative_value = 12345 (offset 0).
	require.True(t, row.CumulativeValue.Valid, "cumulative_value populated")
	cv, err := numericToBigFloat(row.CumulativeValue)
	require.NoError(t, err)
	require.Zero(t, cv.Cmp(big.NewFloat(12345)),
		"cumulative_value want 12345 got %s", cv.Text('f', 0))

	require.NotNil(t, row.BatteryPct, "battery_pct populated")
	require.Equal(t, int16(87), *row.BatteryPct)
	require.NotNil(t, row.TemperatureC, "temperature_c populated")
	require.InDelta(t, 23.0, float64(*row.TemperatureC), 0.001)

	require.NotEmpty(t, row.RawPayload, "raw_payload preserved (DATA-07)")
	require.NotEmpty(t, row.DecodedObject, "decoded_object preserved (DATA-07)")
	require.NotEqual(t, "{}", string(row.DecodedObject), "decoded_object non-empty JSON")

	require.Equal(t, ingest.QualityOK, row.Quality)
}
