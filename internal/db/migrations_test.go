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
	// Bumped from 16 to 17 in plan 02-07 Task 1 (added 0017_binding_changed_trigger).
	var version int
	var dirty bool
	err = pool.QueryRow(ctx, `SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty)
	require.NoError(t, err)
	require.Equal(t, 17, version, "expected schema_migrations.version = 17 (latest after plan 02-07 Task 1)")
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
	require.Equal(t, 17, version)
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

// TestPhase3Migrations_0018_Gateway_Apply — applies 0018 and asserts the
// `gateway` table exists with the expected columns + unique index on
// gateway_id (case-insensitive).
func TestPhase3Migrations_0018_Gateway_Apply(t *testing.T) {
	t.Skip("Wave 1: awaiting db/migrations/0018_gateway.up.sql (03-VALIDATION row migrations_test.TestPhase3Migrations_0018_Gateway_Apply)")
}

// TestPhase3Migrations_0018_Down — 0018 down drops the table cleanly with
// no dangling FK / index residue.
func TestPhase3Migrations_0018_Down(t *testing.T) {
	t.Skip("Wave 1: awaiting db/migrations/0018_gateway.down.sql (03-VALIDATION row migrations_test.TestPhase3Migrations_0018_Down)")
}

// TestPhase3Migrations_0019_ImportJob_Apply — applies 0019 and asserts the
// `import_job` table exists with status enum + expires_at default
// (now() + interval '1 hour').
func TestPhase3Migrations_0019_ImportJob_Apply(t *testing.T) {
	t.Skip("Wave 1: awaiting db/migrations/0019_import_jobs.up.sql (03-VALIDATION row migrations_test.TestPhase3Migrations_0019_ImportJob_Apply)")
}

// TestPhase3Migrations_0019_Down — 0019 down drops `import_job`.
func TestPhase3Migrations_0019_Down(t *testing.T) {
	t.Skip("Wave 1: awaiting db/migrations/0019_import_jobs.down.sql (03-VALIDATION row migrations_test.TestPhase3Migrations_0019_Down)")
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

	// Roll back 0020 only (one step). 0019/0018 stay applied.
	require.NoError(t, runMigrateSteps(t, pool, -1))

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

