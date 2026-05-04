package db

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	pgxdb "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/shifter-io/shifter/internal/testsupport"
	"github.com/stretchr/testify/require"
)

// TestRunMigrations_RoundTrip — apply all migrations forward, roll all the
// way back down, then forward again. Catches non-idempotent down migrations
// and forward statements that fail when reapplied to a previously-migrated
// schema.
func TestRunMigrations_RoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()

	require.NoError(t, RunMigrations(ctx, pool, log), "first up")

	cfg := pool.Config()
	require.NotNil(t, cfg)
	dsn := cfg.ConnConfig.ConnString()
	sqlDB, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)

	src, err := iofs.New(migrationsFS, "migrations")
	require.NoError(t, err)
	drv, err := pgxdb.WithInstance(sqlDB, &pgxdb.Config{})
	require.NoError(t, err)
	m, err := migrate.NewWithInstance("iofs", src, "pgx", drv)
	require.NoError(t, err)

	// Roll all the way down. Some envs error with ErrNoChange when already at 0; treat as success.
	if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("migrate down: %v", err)
	}

	// Reapply. Should succeed back up to 12.
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("migrate up after down: %v", err)
	}
	v, dirty, err := m.Version()
	require.NoError(t, err)
	require.False(t, dirty)
	require.Equal(t, uint(12), v)

	// Verify seeds re-inserted after the round-trip.
	var n int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM device_profile WHERE slug IN ('axioma_w1','acrel_adl200','acrel_adw300')`,
	).Scan(&n))
	require.Equal(t, 3, n, "seeds present after round-trip")
}
