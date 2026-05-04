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

	// Each Phase 1 + early Phase 2 table must exist after a clean run.
	for _, table := range []string{
		"user", "sessions", "install_state", "install_identity", "chirpstack_connection",
		"site", "metering_point", "device_profile",
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
	// Bumped from 9 to 11 in plan 02-02 Task 3 (added 0010_seed_profiles, 0011_chirpstack_connection_cs_ids).
	var version int
	var dirty bool
	err = pool.QueryRow(ctx, `SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty)
	require.NoError(t, err)
	require.Equal(t, 11, version, "expected schema_migrations.version = 11 (latest after plan 02-02 Task 3)")
	require.False(t, dirty, "expected schema_migrations.dirty = false")
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
	require.Equal(t, 11, version)
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
