package db

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/shifter-io/shifter/internal/testsupport"
	"github.com/stretchr/testify/require"
)

// TestRunMigrations_Clean — RunMigrations against an empty `shifter_test`
// database applies every migration and leaves schema_migrations on the
// expected latest version with dirty=false.
func TestRunMigrations_Clean(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, pool, log))

	// Each Phase 1 + Phase 2 table must exist after a clean run.
	for _, table := range []string{
		"user", "sessions", "install_state", "install_identity", "chirpstack_connection",
		"site", "metering_point", "device_profile",
		"device", "device_profile_mapping", "binding",
		"measurement",
	} {
		var exists bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = $1)`,
			table,
		).Scan(&exists)
		require.NoError(t, err, "querying for table %s", table)
		require.True(t, exists, "table %q should exist after migrations", table)
	}

	// TimescaleDB extension must be loaded by 0001_init (Phase 2 hypertables depend on it).
	var extExists bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'timescaledb')`,
	).Scan(&extExists)
	require.NoError(t, err)
	require.True(t, extExists, "timescaledb extension should be created by 0001_init")

	// 0010 seeds three vendor profiles (Axioma W1, Acrel ADL200, Acrel ADW300) — D-07 + DATA-10.
	var profileCount int
	err = pool.QueryRow(ctx,
		`SELECT count(*) FROM device_profile WHERE slug IN ('axioma_w1', 'acrel_adl200', 'acrel_adw300')`,
	).Scan(&profileCount)
	require.NoError(t, err)
	require.Equal(t, 3, profileCount, "0010 must seed all three D-07 vendor profiles")

	// 0011 adds cs_tenant_id + cs_application_id columns to chirpstack_connection (D-28).
	for _, col := range []string{"cs_tenant_id", "cs_application_id"} {
		var colExists bool
		err = pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM information_schema.columns
			               WHERE table_name = 'chirpstack_connection' AND column_name = $1)`,
			col,
		).Scan(&colExists)
		require.NoError(t, err, "querying for column %s", col)
		require.True(t, colExists, "0011 must add column chirpstack_connection.%s", col)
	}

	// schema_migrations must be at the highest migration version, not dirty.
	// Bumped from 14 to 15 in plan 02-04 Task 1 (added 0015_measurement).
	var version int
	var dirty bool
	err = pool.QueryRow(ctx, `SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty)
	require.NoError(t, err)
	require.Equal(t, 15, version, "expected schema_migrations.version = 15 (latest after plan 02-04 Task 1)")
	require.False(t, dirty, "expected schema_migrations.dirty = false")

	// 0014 enables btree_gist for the binding non-overlap EXCLUDE constraints.
	var btreeGistExists bool
	err = pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'btree_gist')`,
	).Scan(&btreeGistExists)
	require.NoError(t, err)
	require.True(t, btreeGistExists, "0014 must CREATE EXTENSION btree_gist")

	// 0015 must convert `measurement` to a TimescaleDB hypertable (D-06).
	// timescaledb_information.hypertables is the canonical source — the entry
	// only exists after create_hypertable() succeeds.
	var measurementIsHypertable bool
	err = pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM timescaledb_information.hypertables WHERE hypertable_name = 'measurement')`,
	).Scan(&measurementIsHypertable)
	require.NoError(t, err)
	require.True(t, measurementIsHypertable, "0015 must convert measurement to a hypertable")

	// 0015 chunk_time_interval must be 1 day (CONTEXT D-06). The dimension
	// `time_interval` is stored as a Postgres INTERVAL; cast to extract days.
	var chunkDays float64
	err = pool.QueryRow(ctx,
		`SELECT EXTRACT(EPOCH FROM time_interval) / 86400.0
		 FROM timescaledb_information.dimensions
		 WHERE hypertable_name = 'measurement' AND dimension_type = 'Time'`,
	).Scan(&chunkDays)
	require.NoError(t, err)
	require.InDelta(t, 1.0, chunkDays, 0.0001, "0015 chunk_time_interval must be 1 day")
}

// TestRunMigrations_Idempotent — Running RunMigrations twice in a row is a
// no-op on the second invocation (no spurious version rows, no errors).
func TestRunMigrations_Idempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, pool, log), "first run")
	require.NoError(t, RunMigrations(ctx, pool, log), "second run must be no-op")

	// Sanity: version is still at the latest after the no-op.
	var version int
	err := pool.QueryRow(ctx, `SELECT version FROM schema_migrations`).Scan(&version)
	require.NoError(t, err)
	require.Equal(t, 15, version)
}

// TestRunMigrations_DirtyState — When schema_migrations has dirty=true,
// RunMigrations refuses to advance and surfaces a typed error so operators
// can investigate manually rather than silently corrupting state.
func TestRunMigrations_DirtyState(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, pool, log))

	// Force the schema dirty as if a previous migration aborted mid-way.
	_, err := pool.Exec(ctx, `UPDATE schema_migrations SET dirty = TRUE`)
	require.NoError(t, err)

	err = RunMigrations(ctx, pool, log)
	require.Error(t, err, "RunMigrations must refuse to advance a dirty schema")
	require.Contains(t, err.Error(), "dirty",
		"error must mention 'dirty' so operators recognize it: got %q", err.Error())
}

// TestBinding_NoOverlapPerMP — 0014's btree_gist EXCLUDE constraint
// `binding_no_overlap_per_mp` must reject a second active binding on the same
// metering_point. Two opens windows on the same MP would corrupt the resolver
// dev_eui→MP query which assumes ≤1 active row per MP.
//
// Pitfall 10 + T-02-03-02 regression test. If the EXCLUDE were ever omitted
// or the bound semantics changed (e.g. `[]` instead of `[)`), this test catches
// it before the swap commit logic in Plan 02-07 silently produces overlapping
// windows.
func TestBinding_NoOverlapPerMP(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, pool, log))

	// Seed minimum graph: site → metering_point + device_profile → 2 devices.
	var siteID, mpID, profileID, devAID, devBID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('test-site', 'UTC') RETURNING id`,
	).Scan(&siteID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class) VALUES ($1, 'mp-1', 'water') RETURNING id`,
		siteID,
	).Scan(&mpID))
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT id FROM device_profile WHERE slug = 'axioma_w1'`,
	).Scan(&profileID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO device (dev_eui, name, device_profile_id) VALUES ('aaaaaaaaaaaaaaa1', 'dev-A', $1) RETURNING id`,
		profileID,
	).Scan(&devAID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO device (dev_eui, name, device_profile_id) VALUES ('aaaaaaaaaaaaaaa2', 'dev-B', $1) RETURNING id`,
		profileID,
	).Scan(&devBID))

	// First binding: open window starting at t0 (valid_to NULL = active).
	_, err := pool.Exec(ctx,
		`INSERT INTO binding (metering_point_id, device_id, valid_from)
		 VALUES ($1, $2, '2026-01-01T00:00:00Z')`,
		mpID, devAID,
	)
	require.NoError(t, err, "first active binding should insert successfully")

	// Second binding: SAME mp, different device, overlapping window.
	// The per-MP EXCLUDE must reject this — at most one active binding per MP.
	_, err = pool.Exec(ctx,
		`INSERT INTO binding (metering_point_id, device_id, valid_from)
		 VALUES ($1, $2, '2026-02-01T00:00:00Z')`,
		mpID, devBID,
	)
	require.Error(t, err, "second overlapping binding on same MP must be rejected")
	require.Contains(t, err.Error(), "binding_no_overlap_per_mp",
		"error must name the EXCLUDE constraint: got %q", err.Error())
}

// TestBinding_NoOverlapPerDevice — 0014's btree_gist EXCLUDE constraint
// `binding_no_overlap_per_device` must reject a second active binding on the
// same device. Two simultaneous bindings on one device would make the
// resolver dev_eui→MP lookup ambiguous (return >1 row); silent telemetry
// corruption that's hard to detect after the fact.
func TestBinding_NoOverlapPerDevice(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, pool, log))

	var siteID, mpAID, mpBID, profileID, devID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('test-site', 'UTC') RETURNING id`,
	).Scan(&siteID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class) VALUES ($1, 'mp-A', 'water') RETURNING id`,
		siteID,
	).Scan(&mpAID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class) VALUES ($1, 'mp-B', 'water') RETURNING id`,
		siteID,
	).Scan(&mpBID))
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT id FROM device_profile WHERE slug = 'axioma_w1'`,
	).Scan(&profileID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO device (dev_eui, name, device_profile_id) VALUES ('bbbbbbbbbbbbbbb1', 'dev-1', $1) RETURNING id`,
		profileID,
	).Scan(&devID))

	// First binding: device on mpA, open-ended.
	_, err := pool.Exec(ctx,
		`INSERT INTO binding (metering_point_id, device_id, valid_from)
		 VALUES ($1, $2, '2026-01-01T00:00:00Z')`,
		mpAID, devID,
	)
	require.NoError(t, err, "first active binding should insert successfully")

	// Second binding: SAME device, different MP, overlapping window.
	// Per-device EXCLUDE must reject — a device can only be at one MP at a time.
	_, err = pool.Exec(ctx,
		`INSERT INTO binding (metering_point_id, device_id, valid_from)
		 VALUES ($1, $2, '2026-02-01T00:00:00Z')`,
		mpBID, devID,
	)
	require.Error(t, err, "second overlapping binding on same device must be rejected")
	require.Contains(t, err.Error(), "binding_no_overlap_per_device",
		"error must name the EXCLUDE constraint: got %q", err.Error())
}

// TestBinding_HalfOpenInterval — Open Q #1 resolution: a binding with
// valid_to = T must NOT collide with a binding starting at exactly T (because
// the range bound is `[)`, not `[]`). This is the "swap at the exact second"
// determinism test.
func TestBinding_HalfOpenInterval(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, pool, log))

	var siteID, mpID, profileID, devAID, devBID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('test-site', 'UTC') RETURNING id`,
	).Scan(&siteID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class) VALUES ($1, 'mp-1', 'water') RETURNING id`,
		siteID,
	).Scan(&mpID))
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT id FROM device_profile WHERE slug = 'axioma_w1'`,
	).Scan(&profileID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO device (dev_eui, name, device_profile_id) VALUES ('cccccccccccccc01', 'dev-A', $1) RETURNING id`,
		profileID,
	).Scan(&devAID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO device (dev_eui, name, device_profile_id) VALUES ('cccccccccccccc02', 'dev-B', $1) RETURNING id`,
		profileID,
	).Scan(&devBID))

	swap := "2026-03-15T12:00:00Z"

	// Closed window for dev-A: [t0, swap). Then open window for dev-B starting at swap.
	_, err := pool.Exec(ctx,
		`INSERT INTO binding (metering_point_id, device_id, valid_from, valid_to)
		 VALUES ($1, $2, '2026-01-01T00:00:00Z', $3)`,
		mpID, devAID, swap,
	)
	require.NoError(t, err, "closed-window first binding should insert")

	// Adjacent (not overlapping) — half-open `[)` makes valid_to=swap exclusive.
	_, err = pool.Exec(ctx,
		`INSERT INTO binding (metering_point_id, device_id, valid_from)
		 VALUES ($1, $2, $3)`,
		mpID, devBID, swap,
	)
	require.NoError(t, err, "adjacent binding starting at exact valid_to must be allowed (half-open)")
}
