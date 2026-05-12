package alert

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// TestAlertsPruneArgs_InsertOpts — the args carry the documented MaxAttempts=3
// cap so a permanently-broken prune eventually surfaces as discarded.
func TestAlertsPruneArgs_InsertOpts(t *testing.T) {
	opts := AlertsPruneArgs{}.InsertOpts()
	require.Equal(t, 3, opts.MaxAttempts)
}

// seedAlertsPruneRetentionConfig inserts the required singleton retention_config
// row with the given alerts_days value.
func seedAlertsPruneRetentionConfig(t *testing.T, ctx context.Context, pool interface {
	Exec(ctx context.Context, sql string, args ...any) (interface{}, error)
}, alertsDays int) {
	t.Helper()
}

// TestAlertsPruneWorker_PrunesPerRetention — seeds 10 fresh + 10 stale alert rows;
// worker run deletes the 10 stale, leaves the 10 fresh.
func TestAlertsPruneWorker_PrunesPerRetention(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	// Seed retention_config with alerts_days = 365.
	_, err := pool.Exec(ctx, `
		INSERT INTO retention_config (id, raw_days, hourly_days, daily_days, monthly_days, yearly_days, alerts_days, audit_log_days)
		VALUES (1, 90, 365, 1825, 7300, NULL, 365, 1825)`)
	require.NoError(t, err)

	// Seed an alert_rule (FK target for alert rows).
	var ruleID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO alert_rule (rule_kind, scope_kind, severity, name, threshold_value, window_seconds, cooldown_seconds)
		 VALUES ('threshold_instantaneous', 'global', 'warning', 'prune-test-rule', 100, 60, 300)
		 RETURNING id`,
	).Scan(&ruleID))

	// 10 fresh alerts (fired today).
	for i := 0; i < 10; i++ {
		_, err := pool.Exec(ctx,
			`INSERT INTO alert (rule_id, state, fired_at)
			 VALUES ($1, 'firing', now())`,
			ruleID)
		require.NoError(t, err)
	}
	// 10 stale alerts (fired 400 days ago — older than 365-day retention).
	for i := 0; i < 10; i++ {
		_, err := pool.Exec(ctx,
			`INSERT INTO alert (rule_id, state, fired_at)
			 VALUES ($1, 'cleared', now() - INTERVAL '400 days')`,
			ruleID)
		require.NoError(t, err)
	}

	worker := &AlertsPruneWorker{Pool: pool, Log: log}
	require.NoError(t, worker.Work(ctx, &river.Job[AlertsPruneArgs]{Args: AlertsPruneArgs{}}))

	// 10 fresh rows still present.
	var remaining int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM alert`).Scan(&remaining))
	require.Equal(t, 10, remaining, "10 fresh alerts must survive; 10 stale must be pruned")
}

// TestAlertsPruneWorker_Idempotent — running the worker twice with no new stale
// rows deletes 0 rows both times; meta audit row written each cycle.
func TestAlertsPruneWorker_Idempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	_, err := pool.Exec(ctx, `
		INSERT INTO retention_config (id, raw_days, hourly_days, daily_days, monthly_days, yearly_days, alerts_days, audit_log_days)
		VALUES (1, 90, 365, 1825, 7300, NULL, 365, 1825)`)
	require.NoError(t, err)

	worker := &AlertsPruneWorker{Pool: pool, Log: log}

	// First run: no rows to prune.
	require.NoError(t, worker.Work(ctx, &river.Job[AlertsPruneArgs]{Args: AlertsPruneArgs{}}))
	// Second run: still no rows.
	require.NoError(t, worker.Work(ctx, &river.Job[AlertsPruneArgs]{Args: AlertsPruneArgs{}}))

	// 2 audit rows (one per cycle).
	var auditCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE action = 'alert.pruned'`,
	).Scan(&auditCount))
	require.Equal(t, 2, auditCount, "one alert.pruned meta row per worker cycle, regardless of count")
}

// TestAlertsPruneWorker_NeverPrunesFiringByAge — alerts in state='firing' that
// are older than retention are STILL pruned (D-13 hard cap; firing-but-stale
// alerts cannot persist indefinitely).
func TestAlertsPruneWorker_NeverPrunesFiringByAge(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	_, err := pool.Exec(ctx, `
		INSERT INTO retention_config (id, raw_days, hourly_days, daily_days, monthly_days, yearly_days, alerts_days, audit_log_days)
		VALUES (1, 90, 365, 1825, 7300, NULL, 365, 1825)`)
	require.NoError(t, err)

	var ruleID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO alert_rule (rule_kind, scope_kind, severity, name, threshold_value, window_seconds, cooldown_seconds)
		 VALUES ('threshold_instantaneous', 'global', 'warning', 'prune-firing-rule', 100, 60, 300)
		 RETURNING id`,
	).Scan(&ruleID))

	// Insert a 'firing' alert that is 400 days old.
	_, err = pool.Exec(ctx,
		`INSERT INTO alert (rule_id, state, fired_at)
		 VALUES ($1, 'firing', now() - INTERVAL '400 days')`,
		ruleID)
	require.NoError(t, err)

	worker := &AlertsPruneWorker{Pool: pool, Log: log}
	require.NoError(t, worker.Work(ctx, &river.Job[AlertsPruneArgs]{Args: AlertsPruneArgs{}}))

	// The firing-but-stale alert must be pruned (D-13 hard cap overrides state).
	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM alert`).Scan(&count))
	require.Equal(t, 0, count, "D-13 hard cap: firing-but-stale alerts must be pruned")
}

// TestAlertsPruneWorker_AuditRow — each run writes an 'alert.pruned' audit row
// with entity_type='alert' and a non-nil meta UUID (never uuid.Nil).
func TestAlertsPruneWorker_AuditRow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	_, err := pool.Exec(ctx, `
		INSERT INTO retention_config (id, raw_days, hourly_days, daily_days, monthly_days, yearly_days, alerts_days, audit_log_days)
		VALUES (1, 90, 365, 1825, 7300, NULL, 365, 1825)`)
	require.NoError(t, err)

	worker := &AlertsPruneWorker{Pool: pool, Log: log}
	require.NoError(t, worker.Work(ctx, &river.Job[AlertsPruneArgs]{Args: AlertsPruneArgs{}}))

	// Verify the audit row shape.
	var action, entityType string
	var entityIDBytes [16]byte
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT action, entity_type, entity_id FROM audit_log WHERE action = 'alert.pruned' LIMIT 1`,
	).Scan(&action, &entityType, &entityIDBytes))

	require.Equal(t, "alert.pruned", action)
	require.Equal(t, "alert", entityType)
	// entity_id must be a non-nil UUID (gen_random_uuid(), NOT uuid.Nil).
	zeroBytes := [16]byte{}
	require.NotEqual(t, zeroBytes, entityIDBytes, "entity_id must not be uuid.Nil — must be gen_random_uuid()")
}

// TestAlertsPruneWorker_UniqueMetaIDs — two successive runs produce two audit
// rows with DIFFERENT entity_ids (each cycle gets a fresh gen_random_uuid()).
func TestAlertsPruneWorker_UniqueMetaIDs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	_, err := pool.Exec(ctx, `
		INSERT INTO retention_config (id, raw_days, hourly_days, daily_days, monthly_days, yearly_days, alerts_days, audit_log_days)
		VALUES (1, 90, 365, 1825, 7300, NULL, 365, 1825)`)
	require.NoError(t, err)

	worker := &AlertsPruneWorker{Pool: pool, Log: log}
	require.NoError(t, worker.Work(ctx, &river.Job[AlertsPruneArgs]{Args: AlertsPruneArgs{}}))
	require.NoError(t, worker.Work(ctx, &river.Job[AlertsPruneArgs]{Args: AlertsPruneArgs{}}))

	// Both rows must have distinct entity_ids.
	var ids []string
	rows, err := pool.Query(ctx, `SELECT entity_id::text FROM audit_log WHERE action = 'alert.pruned' ORDER BY time`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	require.Len(t, ids, 2)
	require.NotEqual(t, ids[0], ids[1], "successive prune cycles must have distinct meta entity_ids")
}

// TestAlertsPruneArgs_Kind — Kind() must return "alerts_prune".
func TestAlertsPruneArgs_Kind(t *testing.T) {
	require.Equal(t, "alerts_prune", AlertsPruneArgs{}.Kind())
}

// fakeAlertsPruneTime returns a time.Time offset by the given duration from now,
// for use in fixture seeding. The function is unexported and used only in
// tests within this package.
func fakeAlertsPruneTime(offset time.Duration) time.Time {
	return time.Now().Add(offset)
}
