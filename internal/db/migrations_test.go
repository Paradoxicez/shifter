package db

import (
	"context"
	"log/slog"
	"os"
	"strings"
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
		"measurement", "audit_log",
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
	// Bumped 17 → 20 in plan 03-02 (0018_gateway / 0019_import_job /
	// 0020_audit_log_vocabulary). Bumped 20 → 23 in plan 04-01
	// (0021_measurement_inserted_trigger / 0022_install_capabilities /
	// 0023_device_profile_expected_interval). Bumped 23 → 24 in plan 05-01
	// (0024_river_tables — River v0.36.0 job-queue schema). Bumped 24 → 28 in
	// plan 05-02 (0025_cagg_hourly / 0026_cagg_daily / 0027_cagg_monthly /
	// 0028_cagg_yearly — four-level CAGG hierarchy).
	var version int
	var dirty bool
	err = pool.QueryRow(ctx, `SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty)
	require.NoError(t, err)
	require.Equal(t, 28, version, "expected schema_migrations.version = 28 (latest after plan 05-02)")
	require.False(t, dirty, "expected schema_migrations.dirty = false")

	// 0025–0028: verify all four CAGGs exist.
	var caggCount int
	err = pool.QueryRow(ctx,
		`SELECT count(*) FROM timescaledb_information.continuous_aggregates`,
	).Scan(&caggCount)
	require.NoError(t, err)
	require.Equal(t, 4, caggCount, "0025–0028 must create exactly 4 CAGGs")

	// 0024 River tables: verify river_migration row exists with line='main', version=6.
	var riverVersion int64
	err = pool.QueryRow(ctx, `SELECT MAX(version) FROM river_migration WHERE line = 'main'`).Scan(&riverVersion)
	require.NoError(t, err)
	require.Equal(t, int64(6), riverVersion, "river_migration must have line='main' version=6 (v0.36 schema)")

	// 0024 River tables: river_job table must exist.
	var riverJobExists bool
	err = pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'river_job')`,
	).Scan(&riverJobExists)
	require.NoError(t, err)
	require.True(t, riverJobExists, "0024 must create river_job table")

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

	// 0016 must create audit_log as a REGULAR table (NOT a hypertable per D-06).
	var auditIsHypertable bool
	err = pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM timescaledb_information.hypertables WHERE hypertable_name = 'audit_log')`,
	).Scan(&auditIsHypertable)
	require.NoError(t, err)
	require.False(t, auditIsHypertable, "audit_log must be a regular table per D-06, not a hypertable")
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
	require.Equal(t, 28, version)
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

// TestAuditLog_RejectsUpdate — 0016 trigger `audit_log_no_update` must raise
// an exception on any UPDATE attempt. Mitigates T-02-04-02 (audit trail
// tampering): even a privileged actor cannot retroactively edit audit rows
// through the application path; tampering requires DBA-level direct DB access.
func TestAuditLog_RejectsUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, pool, log))

	// Seed a user (FK target) and an audit row.
	var userID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('admin@example.com', 'Admin', 'x', 'admin') RETURNING id`,
	).Scan(&userID))

	var rowID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO audit_log (user_id, action, entity_type, entity_id, after)
		 VALUES ($1, 'create', 'site', gen_random_uuid(), '{"name":"new-site"}'::jsonb)
		 RETURNING id`,
		userID,
	).Scan(&rowID))

	// Attempt UPDATE — must be rejected by trigger with the INSERT-ONLY message.
	_, err := pool.Exec(ctx, `UPDATE audit_log SET notes = 'tampered' WHERE id = $1`, rowID)
	require.Error(t, err, "UPDATE on audit_log must be rejected")
	require.Contains(t, err.Error(), "INSERT-ONLY",
		"error must mention INSERT-ONLY: got %q", err.Error())

	// The original row must still be intact (notes still NULL).
	var notes *string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT notes FROM audit_log WHERE id = $1`, rowID,
	).Scan(&notes))
	require.Nil(t, notes, "original notes must remain NULL after rejected UPDATE")
}

// TestAuditLog_RejectsDelete — 0016 trigger `audit_log_no_delete` must raise
// on any DELETE attempt. The audit trail is append-only by DB enforcement.
func TestAuditLog_RejectsDelete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, pool, log))

	var userID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('admin2@example.com', 'Admin Two', 'x', 'admin') RETURNING id`,
	).Scan(&userID))

	var rowID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO audit_log (user_id, action, entity_type, entity_id)
		 VALUES ($1, 'archive', 'metering_point', gen_random_uuid())
		 RETURNING id`,
		userID,
	).Scan(&rowID))

	// Attempt DELETE — must be rejected.
	_, err := pool.Exec(ctx, `DELETE FROM audit_log WHERE id = $1`, rowID)
	require.Error(t, err, "DELETE on audit_log must be rejected")
	require.Contains(t, err.Error(), "INSERT-ONLY",
		"error must mention INSERT-ONLY: got %q", err.Error())

	// Row must still be present.
	var stillThere bool
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM audit_log WHERE id = $1)`, rowID,
	).Scan(&stillThere))
	require.True(t, stillThere, "row must still exist after rejected DELETE")
}

// TestAuditLog_AcceptsInsertAndPersistsDiff — sanity test for the happy path:
// INSERT works, before/after JSONB persists round-trip, and FK to "user"
// is enforced. This is the AUDIT-01 schema-layer assertion (the same-txn
// behavior is tested at the audit package layer in Plan 02-08).
func TestAuditLog_AcceptsInsertAndPersistsDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, pool, log))

	var userID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('admin3@example.com', 'Admin Three', 'x', 'admin') RETURNING id`,
	).Scan(&userID))

	// Insert a diff row covering D-24 (changed-fields-only diff).
	var rowID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO audit_log (user_id, action, entity_type, entity_id, before, after, request_id)
		 VALUES ($1, 'update', 'site', gen_random_uuid(),
		         '{"name":"old-name"}'::jsonb,
		         '{"name":"new-name"}'::jsonb,
		         'req-abc123')
		 RETURNING id`,
		userID,
	).Scan(&rowID))

	// Round-trip the diff JSONB.
	var before, after string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT before::text, after::text FROM audit_log WHERE id = $1`, rowID,
	).Scan(&before, &after))
	require.Contains(t, before, "old-name")
	require.Contains(t, after, "new-name")

	// CHECK rejects unknown action vocabulary (T-02-04-01-style mitigation for
	// audit_log specifically — D-22's controlled action set).
	_, err := pool.Exec(ctx,
		`INSERT INTO audit_log (user_id, action, entity_type, entity_id)
		 VALUES ($1, 'frobnicate', 'site', gen_random_uuid())`,
		userID,
	)
	require.Error(t, err, "unknown action must be rejected by audit_log_action_valid CHECK")
	require.Contains(t, err.Error(), "audit_log_action_valid")

	// CHECK rejects unknown entity_type.
	_, err = pool.Exec(ctx,
		`INSERT INTO audit_log (user_id, action, entity_type, entity_id)
		 VALUES ($1, 'create', 'gizmo', gen_random_uuid())`,
		userID,
	)
	require.Error(t, err, "unknown entity_type must be rejected")
	require.Contains(t, err.Error(), "audit_log_entity_type_valid")
}

// =============================================================================
// Phase 3 migrations (skeleton — Wave 1 implements the SQL)
// =============================================================================
//
// Wave 0 leaves these as t.Skip placeholders. Wave 1's migration plan
// (03-02-PLAN.md / 03-05-PLAN.md) implements:
//
//   - 0018_gateway        — `gateway` table (id, gateway_id, name, region,
//                            archived_at, ...) + indexes
//   - 0019_import_jobs    — `import_job` (job_id, status, outcomes JSONB,
//                            created_at, expires_at) for D-11 1h TTL
//   - 0020_audit_log_vocabulary — extends audit_log CHECK constraints with
//                            the new actions/entity_types Phase 3 introduces
//                            (`device.bulk_import`, `device.reveal_secrets`,
//                            `gateway.create|update|archive|restore`,
//                            `import_job`).
//
// Each test below verifies one migration applies cleanly and (where
// applicable) the corresponding down migration rolls back to the prior
// schema_migrations version with no orphaned tables.

// TestPhase3Migrations_0018_Gateway_Apply — applies all migrations through
// 0020 and verifies the `gateway` table exists with the required columns,
// CHECK constraints (lowercase hex16 gateway_id, lat/lng ranges, name
// non-empty), soft-delete columns, stats cache columns, partial indexes,
// and the touch_updated_at trigger. Negative cases exercise each CHECK.
func TestPhase3Migrations_0018_Gateway_Apply(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, pool, log))

	// All required columns must exist (Plan 03-02 Task 2 enumerates these).
	wantCols := []string{
		"id", "gateway_id", "name", "description", "region",
		"lat", "lng", "altitude", "tags", "cs_tenant_id",
		"stats_refreshed_at", "stats_rx_24h", "stats_tx_24h",
		"stats_tx_ok_24h", "stats_sparkline",
		"archived_at", "archived_reason", "archived_snapshot",
		"created_at", "updated_at",
	}
	for _, col := range wantCols {
		var exists bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM information_schema.columns
			               WHERE table_name = 'gateway' AND column_name = $1)`,
			col,
		).Scan(&exists)
		require.NoError(t, err)
		require.True(t, exists, "gateway.%s column must exist", col)
	}

	// Required CHECK constraints by name (defense — name them so error
	// messages on violation reference the constraint cleanly).
	wantConstraints := []string{
		"gateway_eui_lower", "gateway_eui_hex16",
		"gateway_name_not_empty",
		"gateway_lat_range", "gateway_lng_range",
	}
	for _, name := range wantConstraints {
		var exists bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conname = $1)`,
			name,
		).Scan(&exists)
		require.NoError(t, err)
		require.True(t, exists, "constraint %q must exist on gateway", name)
	}

	// Partial indexes (gated on archived_at IS NULL).
	for _, idx := range []string{"gateway_archived_idx", "gateway_region_idx"} {
		var exists bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM pg_indexes
			               WHERE tablename = 'gateway' AND indexname = $1)`,
			idx,
		).Scan(&exists)
		require.NoError(t, err)
		require.True(t, exists, "index %q must exist on gateway", idx)
	}

	// Happy path: valid lowercase hex16 EUI inserts, archived_snapshot JSONB.
	_, err := pool.Exec(ctx,
		`INSERT INTO gateway (gateway_id, name, region, lat, lng, archived_snapshot)
		 VALUES ('0123456789abcdef', 'gw-1', 'eu868', 12.34, -56.78,
		         '{"name":"gw-1"}'::jsonb)`,
	)
	require.NoError(t, err, "valid gateway row must insert")

	// Uppercase EUI → CHECK violation (either gateway_eui_lower OR
	// gateway_eui_hex16 — Postgres picks one of the violated CHECKs to
	// report; both correctly reject the row).
	_, err = pool.Exec(ctx,
		`INSERT INTO gateway (gateway_id, name, region)
		 VALUES ('0123456789ABCDEF', 'gw-bad-upper', 'eu868')`,
	)
	require.Error(t, err)
	require.True(t,
		strings.Contains(err.Error(), "gateway_eui_lower") ||
			strings.Contains(err.Error(), "gateway_eui_hex16"),
		"uppercase EUI must violate one of the EUI CHECKs, got: %v", err)

	// 15-char EUI → CHECK violation (gateway_eui_hex16).
	_, err = pool.Exec(ctx,
		`INSERT INTO gateway (gateway_id, name, region)
		 VALUES ('0123456789abcde', 'gw-bad-short', 'eu868')`,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "gateway_eui_hex16")

	// lat=91 → CHECK violation (gateway_lat_range).
	_, err = pool.Exec(ctx,
		`INSERT INTO gateway (gateway_id, name, region, lat)
		 VALUES ('1111111111111111', 'gw-bad-lat', 'eu868', 91.0)`,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "gateway_lat_range")

	// lng=181 → CHECK violation (gateway_lng_range).
	_, err = pool.Exec(ctx,
		`INSERT INTO gateway (gateway_id, name, region, lng)
		 VALUES ('2222222222222222', 'gw-bad-lng', 'eu868', 181.0)`,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "gateway_lng_range")

	// Empty name → CHECK violation (gateway_name_not_empty).
	_, err = pool.Exec(ctx,
		`INSERT INTO gateway (gateway_id, name, region)
		 VALUES ('3333333333333333', '', 'eu868')`,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "gateway_name_not_empty")

	// touch_updated_at trigger: UPDATE bumps updated_at.
	var beforeTS, afterTS string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT updated_at::text FROM gateway WHERE gateway_id = '0123456789abcdef'`,
	).Scan(&beforeTS))
	_, err = pool.Exec(ctx,
		`UPDATE gateway SET description = 'touched' WHERE gateway_id = '0123456789abcdef'`,
	)
	require.NoError(t, err)
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT updated_at::text FROM gateway WHERE gateway_id = '0123456789abcdef'`,
	).Scan(&afterTS))
	require.NotEqual(t, beforeTS, afterTS, "touch_updated_at trigger must bump updated_at")
}

// TestPhase3Migrations_0018_Down — applies all migrations then rolls back
// to before 0018 so the gateway table must disappear.
// Plan 05-02 bumped the chain to 28, so the rollback distance is 10 steps:
// 0028 → 0027 → 0026 → 0025 → 0024 → 0023 → 0022 → 0021 → 0020 → 0019 → 0018.
func TestPhase3Migrations_0018_Down(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, pool, log))

	// Roll back ten steps: 0028 → 0027 → 0026 → 0025 → 0024 → 0023 → 0022 → 0021 → 0020 → 0019 → 0018.
	require.NoError(t, runMigrateSteps(t, pool, -10))

	var exists bool
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'gateway')`,
	).Scan(&exists))
	require.False(t, exists, "gateway table must be dropped after 0018 down")

	// Re-up — schema should land cleanly on the latest version again.
	require.NoError(t, RunMigrations(ctx, pool, log))
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'gateway')`,
	).Scan(&exists))
	require.True(t, exists, "gateway table must reappear after re-up")
}

// TestPhase3Migrations_0019_ImportJob_Apply — applies 0019 and asserts the
// `import_job` + `import_job_row` tables exist with the right enums,
// file_format CHECK, partial preview-status index, and ON DELETE CASCADE.
func TestPhase3Migrations_0019_ImportJob_Apply(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, pool, log))

	// Enums.
	for _, typ := range []string{"import_job_status", "import_job_row_status"} {
		var exists bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM pg_type WHERE typname = $1)`,
			typ,
		).Scan(&exists)
		require.NoError(t, err)
		require.True(t, exists, "type %q must exist", typ)
	}

	// Tables.
	for _, table := range []string{"import_job", "import_job_row"} {
		var exists bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = $1)`,
			table,
		).Scan(&exists)
		require.NoError(t, err)
		require.True(t, exists, "table %q must exist", table)
	}

	// Seed a user (owner_id FK target).
	var userID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('importer@example.com', 'Importer', 'x', 'admin') RETURNING id`,
	).Scan(&userID))

	// Happy path: insert a preview job + 5 rows, then verify cascade delete.
	var jobID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO import_job (owner_id, file_name, file_format, total_rows, expires_at)
		 VALUES ($1, 'devices.xlsx', 'xlsx', 5, now() + interval '1 hour')
		 RETURNING id`,
		userID,
	).Scan(&jobID))

	for i := 0; i < 5; i++ {
		_, err := pool.Exec(ctx,
			`INSERT INTO import_job_row (import_job_id, row_index, raw_payload, status)
			 VALUES ($1, $2, '{"dev_eui":"aa"}'::jsonb, 'valid')`,
			jobID, i,
		)
		require.NoError(t, err, "import_job_row insert %d must succeed", i)
	}

	var rowCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM import_job_row WHERE import_job_id = $1`, jobID,
	).Scan(&rowCount))
	require.Equal(t, 5, rowCount)

	// Negative: invalid file_format rejected by CHECK.
	_, err := pool.Exec(ctx,
		`INSERT INTO import_job (owner_id, file_name, file_format)
		 VALUES ($1, 'bad.txt', 'txt')`,
		userID,
	)
	require.Error(t, err, "file_format outside ('xlsx','csv') must be rejected")

	// Negative: duplicate (import_job_id, row_index) — UNIQUE constraint.
	_, err = pool.Exec(ctx,
		`INSERT INTO import_job_row (import_job_id, row_index, raw_payload)
		 VALUES ($1, 0, '{"dup":true}'::jsonb)`,
		jobID,
	)
	require.Error(t, err, "duplicate (import_job_id, row_index) must be rejected")

	// ON DELETE CASCADE — deleting the parent removes child rows.
	_, err = pool.Exec(ctx, `DELETE FROM import_job WHERE id = $1`, jobID)
	require.NoError(t, err)
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM import_job_row WHERE import_job_id = $1`, jobID,
	).Scan(&rowCount))
	require.Equal(t, 0, rowCount, "ON DELETE CASCADE must remove child rows")

	// Partial preview-status index exists.
	var idxExists bool
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_indexes
		               WHERE tablename = 'import_job' AND indexname = 'import_job_status_idx')`,
	).Scan(&idxExists))
	require.True(t, idxExists)
}

// TestPhase3Migrations_0019_Down — roll back to before 0019; import_job and
// import_job_row + both enums must be dropped.
// Plan 05-02 bumped the chain to 28, so the rollback distance is 9 steps:
// 0028 → 0027 → 0026 → 0025 → 0024 → 0023 → 0022 → 0021 → 0020 → 0019. 0018 stays applied.
func TestPhase3Migrations_0019_Down(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, pool, log))

	// Roll back nine steps: 0028 → ... → 0019. 0018 (gateway) stays.
	require.NoError(t, runMigrateSteps(t, pool, -9))

	for _, table := range []string{"import_job", "import_job_row"} {
		var exists bool
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = $1)`,
			table,
		).Scan(&exists))
		require.False(t, exists, "table %q must be dropped after 0019 down", table)
	}
	for _, typ := range []string{"import_job_status", "import_job_row_status"} {
		var exists bool
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM pg_type WHERE typname = $1)`, typ,
		).Scan(&exists))
		require.False(t, exists, "type %q must be dropped after 0019 down", typ)
	}

	// Gateway table from 0018 should still exist.
	var gwExists bool
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'gateway')`,
	).Scan(&gwExists))
	require.True(t, gwExists, "gateway must remain after rolling back only 0020+0019")
}

// TestPhase3Migrations_0020_AuditLogVocabulary_Apply — applies 0020 and
// asserts the audit_log_action_valid + audit_log_entity_type_valid CHECK
// constraints accept the new Phase 3 values. The constraint names must remain
// `audit_log_action_valid` and `audit_log_entity_type_valid` (DROP + re-ADD
// with same name) so dependent diagnostics/observability continue to work.
func TestPhase3Migrations_0020_AuditLogVocabulary_Apply(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, pool, log))

	// Constraint names preserved per Plan 03-02 Task 1.
	for _, name := range []string{"audit_log_action_valid", "audit_log_entity_type_valid"} {
		var exists bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conname = $1)`,
			name,
		).Scan(&exists)
		require.NoError(t, err)
		require.True(t, exists, "constraint %q must exist after 0020 (DROP+re-ADD preserved name)", name)
	}
}

// TestPhase3Migrations_0020_AcceptsNewActionsAndEntityTypes — INSERT rows
// using each new action + entity_type combination (`device.bulk_import`,
// `device.reveal_secrets`, `gateway.create|update|archive|restore`,
// `import_job`) and verify CHECK constraints pass. Regression check: at least
// one Phase 2 vocabulary value still inserts cleanly.
func TestPhase3Migrations_0020_AcceptsNewActionsAndEntityTypes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, pool, log))

	var userID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('vocab@example.com', 'Vocab', 'x', 'admin') RETURNING id`,
	).Scan(&userID))

	// Each new Phase 3 action — paired with a sensible entity_type so the
	// row also exercises the entity_type CHECK.
	newActions := []struct {
		action     string
		entityType string
	}{
		{"gateway.create", "gateway"},
		{"gateway.update", "gateway"},
		{"gateway.archive", "gateway"},
		{"gateway.restore", "gateway"},
		{"device.bulk_import", "import_job"},
		{"device.reveal_secrets", "device"},
	}
	for _, c := range newActions {
		_, err := pool.Exec(ctx,
			`INSERT INTO audit_log (user_id, action, entity_type, entity_id)
			 VALUES ($1, $2, $3, gen_random_uuid())`,
			userID, c.action, c.entityType,
		)
		require.NoError(t, err,
			"INSERT with action=%q entity_type=%q must pass 0020 CHECKs", c.action, c.entityType)
	}

	// New entity types paired with a Phase 2 action ('create') so the action
	// CHECK is satisfied while we exercise the entity_type CHECK in isolation.
	for _, et := range []string{"gateway", "import_job"} {
		_, err := pool.Exec(ctx,
			`INSERT INTO audit_log (user_id, action, entity_type, entity_id)
			 VALUES ($1, 'create', $2, gen_random_uuid())`,
			userID, et,
		)
		require.NoError(t, err, "entity_type %q must be admitted by 0020", et)
	}

	// Regression: a Phase 2 vocab pair (`create` + `site`) STILL inserts.
	_, err := pool.Exec(ctx,
		`INSERT INTO audit_log (user_id, action, entity_type, entity_id)
		 VALUES ($1, 'create', 'site', gen_random_uuid())`,
		userID,
	)
	require.NoError(t, err, "Phase 2 vocab must still insert after 0020 (regression)")

	// Negative case: a still-unknown action remains rejected.
	_, err = pool.Exec(ctx,
		`INSERT INTO audit_log (user_id, action, entity_type, entity_id)
		 VALUES ($1, 'frobnicate', 'gateway', gen_random_uuid())`,
		userID,
	)
	require.Error(t, err, "unknown action must still be rejected post-0020")
	require.Contains(t, err.Error(), "audit_log_action_valid")
}

// TestPhase3Migrations_0020_Down — applies 0020 up + down and verifies the
// CHECK vocab has reverted to the Phase 2 set. A row using Phase 3 vocab
// is rejected post-down; a Phase 2 row still inserts.
func TestPhase3Migrations_0020_Down(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, pool, log))

	// Roll back to before 0020. Plan 05-02 bumped the chain to 28, so the
	// rollback distance is 8 steps: 0028 → 0027 → 0026 → 0025 → 0024 → 0023 → 0022 → 0021 → 0020.
	// 0019/0018 stay applied.
	require.NoError(t, runMigrateSteps(t, pool, -8))

	var userID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('vocab_down@example.com', 'VD', 'x', 'admin') RETURNING id`,
	).Scan(&userID))

	// Phase 3 action no longer admitted after down.
	_, err := pool.Exec(ctx,
		`INSERT INTO audit_log (user_id, action, entity_type, entity_id)
		 VALUES ($1, 'gateway.create', 'site', gen_random_uuid())`,
		userID,
	)
	require.Error(t, err, "Phase 3 action must be rejected after 0020 down")
	require.Contains(t, err.Error(), "audit_log_action_valid")

	// Phase 3 entity_type no longer admitted after down.
	_, err = pool.Exec(ctx,
		`INSERT INTO audit_log (user_id, action, entity_type, entity_id)
		 VALUES ($1, 'create', 'gateway', gen_random_uuid())`,
		userID,
	)
	require.Error(t, err, "Phase 3 entity_type must be rejected after 0020 down")
	require.Contains(t, err.Error(), "audit_log_entity_type_valid")

	// Phase 2 vocab still works after down.
	_, err = pool.Exec(ctx,
		`INSERT INTO audit_log (user_id, action, entity_type, entity_id)
		 VALUES ($1, 'create', 'site', gen_random_uuid())`,
		userID,
	)
	require.NoError(t, err, "Phase 2 vocab must still insert after 0020 down")
}

// TestRunMigrations_RiverDownUpClean — 0024_river_tables round-trip test.
// Applies all migrations (up to 28), rolls back 5 steps (0028→0027→0026→0025→0024),
// asserts river_job does NOT exist, then re-applies 5 steps, asserts it does.
// This pins the Pitfall #8 mitigation: River schema is managed by golang-migrate
// so the install script does NOT need a separate `river migrate-up` step.
func TestRunMigrations_RiverDownUpClean(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()

	// Apply all migrations up to 28.
	require.NoError(t, RunMigrations(ctx, pool, log))

	// Verify river_job exists before rollback.
	var existsBefore bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM to_regclass('river_job') WHERE to_regclass IS NOT NULL)`,
	).Scan(&existsBefore)
	require.NoError(t, err)
	require.True(t, existsBefore, "river_job must exist after 0024 up")

	// Roll back 5 steps: 0028 → 0027 → 0026 → 0025 → 0024.
	// This takes us from v28 (CAGG yearly) back before River (v23).
	require.NoError(t, runMigrateSteps(t, pool, -5))

	// river_job must NOT exist after 0024 down.
	var existsAfterDown bool
	err = pool.QueryRow(ctx,
		`SELECT to_regclass('river_job') IS NOT NULL`,
	).Scan(&existsAfterDown)
	require.NoError(t, err)
	require.False(t, existsAfterDown, "river_job must not exist after 0024 down")

	// Re-apply 5 steps (0024 + 0025-0028).
	require.NoError(t, runMigrateSteps(t, pool, 5))

	// river_job must exist again after re-applying 0024.
	var existsAfterUp bool
	err = pool.QueryRow(ctx,
		`SELECT to_regclass('river_job') IS NOT NULL`,
	).Scan(&existsAfterUp)
	require.NoError(t, err)
	require.True(t, existsAfterUp, "river_job must exist after 0024 re-applied")

	// river_migration must also be re-populated with version=6.
	var riverVersion int64
	err = pool.QueryRow(ctx, `SELECT MAX(version) FROM river_migration WHERE line = 'main'`).Scan(&riverVersion)
	require.NoError(t, err)
	require.Equal(t, int64(6), riverVersion, "river_migration must have version=6 after re-apply")
}

// TestRunMigrations_CAGGsDropClean — 0025–0028 CAGG round-trip test.
// Applies all migrations (up to 28), rolls back 4 steps (down 0028 → 0025),
// asserts continuous_aggregates count = 0, then re-applies (up 4 steps) and
// asserts continuous_aggregates count = 4.
//
// This pins the WITH NO DATA / Pitfall #1 mitigation: each CAGG migration
// file must succeed in golang-migrate's transaction wrapper both up and down.
func TestRunMigrations_CAGGsDropClean(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()

	// Apply all migrations up to 28.
	require.NoError(t, RunMigrations(ctx, pool, log))

	// Verify 4 CAGGs exist before rollback.
	var countBefore int
	err := pool.QueryRow(ctx,
		`SELECT count(*) FROM timescaledb_information.continuous_aggregates`,
	).Scan(&countBefore)
	require.NoError(t, err)
	require.Equal(t, 4, countBefore, "4 CAGGs must exist after migrations up to 28")

	// Roll back 4 steps: 0028 → 0027 → 0026 → 0025.
	require.NoError(t, runMigrateSteps(t, pool, -4))

	// All 4 CAGGs must be gone after rolling back 0025–0028.
	var countAfterDown int
	err = pool.QueryRow(ctx,
		`SELECT count(*) FROM timescaledb_information.continuous_aggregates`,
	).Scan(&countAfterDown)
	require.NoError(t, err)
	require.Equal(t, 0, countAfterDown, "continuous_aggregates must be empty after 0025–0028 down")

	// Re-apply 0025–0028 (4 steps up).
	require.NoError(t, runMigrateSteps(t, pool, 4))

	// All 4 CAGGs must reappear.
	var countAfterUp int
	err = pool.QueryRow(ctx,
		`SELECT count(*) FROM timescaledb_information.continuous_aggregates`,
	).Scan(&countAfterUp)
	require.NoError(t, err)
	require.Equal(t, 4, countAfterUp, "4 CAGGs must reappear after re-applying 0025–0028")

	// Schema version must be 28 after re-apply.
	var version int
	err = pool.QueryRow(ctx, `SELECT version FROM schema_migrations`).Scan(&version)
	require.NoError(t, err)
	require.Equal(t, 28, version, "schema_migrations.version must be 28 after re-apply")
}
