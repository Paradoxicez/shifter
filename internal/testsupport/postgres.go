package testsupport

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// StartPostgres starts TimescaleDB 2.26 in a container and returns a
// *pgxpool.Pool connected to a fresh `shifter_test` database. The container
// (and pool) are torn down via t.Cleanup.
//
// Migrations are NOT applied here — Plan 03 implements db.RunMigrations and
// callers invoke it explicitly. This separation lets migration tests start
// from a known-empty schema.
//
// Pinned tag (T-02-01): timescale/timescaledb:2.26.0-pg16.
func StartPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"timescale/timescaledb:2.26.0-pg16",
		postgres.WithDatabase("shifter_test"),
		postgres.WithUsername("shifter"),
		postgres.WithPassword("shifter"),
		tc.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("postgres container: %v", err)
	}
	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("postgres dsn: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool: %v", err)
	}
	t.Cleanup(pool.Close)

	return pool
}
