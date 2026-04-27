package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	pgxdb "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" sql.Driver
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// RunMigrations applies all pending up migrations against the database backing pool.
//
// Idempotent: returns nil with no work if the schema is already at the latest version
// (golang-migrate's ErrNoChange is treated as success). Surfaces a clear error if the
// schema is dirty (a previous migration aborted mid-way) so the operator can run
// `shifter migrate force <prev>` and re-run rather than silently corrupting state.
//
// Internally opens a short-lived *sql.DB from the pool's DSN. We avoid
// stdlib.OpenDBFromPool because the migrate pgx/v5 driver leaves connection
// state on close that wedges the underlying puddle.Pool — *sql.DB.Close()
// must NOT also need to drain pool connections. The dedicated *sql.DB owns
// its own pgx connection that we can close cleanly without the test/process
// pool hanging at shutdown.
//
// D-13 (auto-migrate on serve), D-16 (golang-migrate as a library, not the CLI),
// D-17 (integer-prefix migration names).
func RunMigrations(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	sqlDB, err := openMigrationDB(pool)
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping migration db: %w", err)
	}

	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("iofs: %w", err)
	}

	drv, err := pgxdb.WithInstance(sqlDB, &pgxdb.Config{})
	if err != nil {
		return fmt.Errorf("pgx driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "pgx", drv)
	if err != nil {
		return fmt.Errorf("migrate.NewWithInstance: %w", err)
	}

	versionBefore, dirtyBefore, vErr := m.Version()
	if vErr != nil && !errors.Is(vErr, migrate.ErrNilVersion) {
		return fmt.Errorf("migrate version: %w", vErr)
	}
	if dirtyBefore {
		return fmt.Errorf("schema is dirty at version %d — run `shifter migrate force <prev>` and re-run", versionBefore)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}

	versionAfter, _, vErr := m.Version()
	if vErr != nil && !errors.Is(vErr, migrate.ErrNilVersion) {
		return fmt.Errorf("migrate version (after): %w", vErr)
	}
	if log != nil {
		log.Info("migrations applied", "from", versionBefore, "to", versionAfter)
	}
	return nil
}

// ForceVersion is the recovery escape hatch for a dirty schema. Operators run
// `shifter migrate force <N>` (Plan 05 wires the CLI subcommand) which calls
// this helper to mark schema_migrations clean at version N. After forcing, the
// next RunMigrations call attempts the next pending up migration normally.
func ForceVersion(ctx context.Context, pool *pgxpool.Pool, version int) error {
	sqlDB, err := openMigrationDB(pool)
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping migration db: %w", err)
	}

	drv, err := pgxdb.WithInstance(sqlDB, &pgxdb.Config{})
	if err != nil {
		return fmt.Errorf("pgx driver: %w", err)
	}
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("iofs: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "pgx", drv)
	if err != nil {
		return fmt.Errorf("migrate.NewWithInstance: %w", err)
	}
	return m.Force(version)
}

// openMigrationDB derives a fresh *sql.DB from pool's DSN. The returned DB is
// independent of the pgxpool: closing it does not affect the pool, and its
// connection lifecycle is fully owned by the caller (typically deferred Close
// in RunMigrations / ForceVersion).
func openMigrationDB(pool *pgxpool.Pool) (*sql.DB, error) {
	cfg := pool.Config()
	if cfg == nil || cfg.ConnConfig == nil {
		return nil, errors.New("pool has no config")
	}
	dsn := cfg.ConnConfig.ConnString()
	if dsn == "" {
		return nil, errors.New("pool config has empty connection string")
	}
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open(pgx): %w", err)
	}
	// Conservative limits — we only need one connection at a time for the
	// migrate driver, and we close the DB right after.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	return sqlDB, nil
}
