package alert

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/db"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// TestAuditPruneArgs_InsertOpts — the args carry the documented MaxAttempts=3
// cap so a permanently-broken prune eventually surfaces as discarded.
func TestAuditPruneArgs_InsertOpts(t *testing.T) {
	opts := AuditPruneArgs{}.InsertOpts()
	require.Equal(t, 3, opts.MaxAttempts)
}

// TestAuditPruneWorker_DeletesOldRows — seed 10 old audit rows, set
// audit_log_days=1825, run worker, observe 10 rows deleted + 1 audit.prune
// meta row inserted. Trigger remains active for direct DELETE.
func TestAuditPruneWorker_DeletesOldRows(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	// Seed retention_config (audit_log_days=1825 default).
	_, err := pool.Exec(ctx, `
		INSERT INTO retention_config (id, raw_days, hourly_days, daily_days, monthly_days, yearly_days)
		VALUES (1, 90, 365, 1825, 7300, NULL)`)
	require.NoError(t, err)

	// Seed user (FK target).
	var userID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('prune-worker@example.com', 'PW', 'x', 'admin') RETURNING id`,
	).Scan(&userID))

	// 10 audit rows with time = 2000d ago (older than 1825d cutoff).
	for i := 0; i < 10; i++ {
		_, err := pool.Exec(ctx,
			`INSERT INTO audit_log (time, user_id, action, entity_type, entity_id)
			 VALUES (now() - INTERVAL '2000 days', $1, 'create', 'site', gen_random_uuid())`,
			userID)
		require.NoError(t, err)
	}

	// Pre-condition: direct DELETE rejected.
	_, err = pool.Exec(ctx, `DELETE FROM audit_log WHERE action = 'create'`)
	require.Error(t, err, "direct DELETE must be rejected by INSERT-ONLY trigger before prune")
	require.Contains(t, err.Error(), "INSERT-ONLY")

	// Run worker.
	worker := &AuditPruneWorker{
		Pool:    pool,
		Queries: sqlc.New(pool),
		Log:     log,
	}
	err = worker.Work(ctx, &river.Job[AuditPruneArgs]{Args: AuditPruneArgs{}})
	require.NoError(t, err)

	// 10 old rows gone.
	var oldCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE action = 'create'`,
	).Scan(&oldCount))
	require.Equal(t, 0, oldCount, "all 10 old create rows must be pruned")

	// Exactly 1 audit.prune meta row inserted.
	var pruneCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE action = 'audit.prune'`,
	).Scan(&pruneCount))
	require.Equal(t, 1, pruneCount, "exactly one audit.prune meta row per worker cycle")

	// Trigger still active for direct DELETE.
	_, err = pool.Exec(ctx, `DELETE FROM audit_log WHERE action = 'audit.prune'`)
	require.Error(t, err, "trigger must still reject direct DELETE after worker exits")
	require.Contains(t, err.Error(), "INSERT-ONLY")
}

// TestAuditPruneWorker_NoOldRows — worker still writes a meta row even when
// deleted_count = 0 (audit trail of every prune, including zero-row prunes).
func TestAuditPruneWorker_NoOldRows(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	// Seed retention_config.
	_, err := pool.Exec(ctx, `
		INSERT INTO retention_config (id, raw_days, hourly_days, daily_days, monthly_days, yearly_days)
		VALUES (1, 90, 365, 1825, 7300, NULL)`)
	require.NoError(t, err)

	// No old rows to prune.
	worker := &AuditPruneWorker{
		Pool:    pool,
		Queries: sqlc.New(pool),
		Log:     log,
	}
	require.NoError(t, worker.Work(ctx, &river.Job[AuditPruneArgs]{Args: AuditPruneArgs{}}))

	var pruneCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE action = 'audit.prune'`,
	).Scan(&pruneCount))
	require.Equal(t, 1, pruneCount, "audit.prune meta row written even for zero-row prune")
}
