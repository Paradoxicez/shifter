package db

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	pgxdb "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
)

// runMigrateSteps applies +N up steps or -N down steps from the current
// schema_migrations version. Test-only helper used by Phase 3 migration-down
// tests (no production code calls this — the runtime always uses Up()).
// Returns nil if golang-migrate returns ErrNoChange.
func runMigrateSteps(t *testing.T, pool *pgxpool.Pool, steps int) error {
	t.Helper()

	sqlDB, err := openMigrationDB(pool)
	if err != nil {
		return err
	}
	defer func() { _ = sqlDB.Close() }()
	if err := sqlDB.Ping(); err != nil {
		return err
	}

	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	drv, err := pgxdb.WithInstance(sqlDB, &pgxdb.Config{})
	if err != nil {
		return err
	}
	m, err := migrate.NewWithInstance("iofs", src, "pgx", drv)
	if err != nil {
		return err
	}
	if err := m.Steps(steps); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

// silence unused-import warning when this file compiles alone.
var _ = sql.ErrNoRows
