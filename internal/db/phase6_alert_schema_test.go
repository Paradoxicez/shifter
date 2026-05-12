package db

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/testsupport"
)

// TestAlertRuleSchema — 0038_alert_rule schema invariants (D-04 / D-05 / D-06
// / D-07 / D-17 — per RESEARCH §Decision C). Rule_kind/scope_kind/severity
// must accept the documented vocabulary and reject unknown values with 23514.
func TestAlertRuleSchema(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, pool, log))

	// Seed user (FK target).
	var userIDStr string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('alert_rule@example.com', 'AR', 'x', 'admin') RETURNING id`,
	).Scan(&userIDStr))

	// Seed a metering point so scope_kind='metering_point' / scope_id can
	// reference something real.
	var siteID, mpID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('alert-rule-site', 'UTC') RETURNING id`,
	).Scan(&siteID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class)
		 VALUES ($1, 'alert-rule-mp', 'water') RETURNING id`,
		siteID,
	).Scan(&mpID))

	// Happy-path insert: threshold_hourly + critical severity + scope_id set.
	var ruleID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO alert_rule
		   (rule_kind, scope_kind, scope_id, high_bound, comparison, unit, severity, cooldown_seconds, name, created_by)
		 VALUES ('threshold_hourly', 'metering_point', $1, 40.0, 'gt', 'm3/h', 'critical', 900, 'High flow', $2)
		 RETURNING id`,
		mpID, userIDStr,
	).Scan(&ruleID))
	require.NotEmpty(t, ruleID)

	// Default severity = critical (D-07).
	var defaultSev string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO alert_rule
		   (rule_kind, scope_kind, scope_id, high_bound, comparison, unit)
		 VALUES ('threshold_instantaneous', 'metering_point', $1, 100, 'gt', 'kWh')
		 RETURNING severity`,
		mpID,
	).Scan(&defaultSev))
	require.Equal(t, "critical", defaultSev, "default severity must be 'critical' (D-07)")

	// Default cooldown = 900 seconds (D-05).
	var defaultCD int
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO alert_rule
		   (rule_kind, scope_kind, scope_id, high_bound)
		 VALUES ('threshold_daily', 'metering_point', $1, 1000)
		 RETURNING cooldown_seconds`,
		mpID,
	).Scan(&defaultCD))
	require.Equal(t, 900, defaultCD, "default cooldown_seconds must be 900 (D-05)")

	// Quiet-window columns (D-17 anomaly_quiet_hour) are available.
	var qwStart, qwEnd pgconn.PgError
	_ = qwStart
	_ = qwEnd
	_, err := pool.Exec(ctx,
		`INSERT INTO alert_rule
		   (rule_kind, scope_kind, scope_id, quiet_window_start, quiet_window_end, flow_threshold, days_of_week)
		 VALUES ('anomaly_quiet_hour', 'metering_point', $1, TIME '01:00', TIME '05:00', 0.1, 1)`,
		mpID)
	require.NoError(t, err, "quiet-hour columns must accept time + flow + days_of_week")

	// Unknown rule_kind rejected with 23514.
	_, err = pool.Exec(ctx,
		`INSERT INTO alert_rule (rule_kind, scope_kind, scope_id)
		 VALUES ('unknown_rule', 'metering_point', $1)`, mpID)
	require.Error(t, err, "unknown rule_kind must be rejected")
	require.Contains(t, err.Error(), "23514")

	// Unknown severity rejected.
	_, err = pool.Exec(ctx,
		`INSERT INTO alert_rule (rule_kind, scope_kind, scope_id, severity)
		 VALUES ('threshold_hourly', 'metering_point', $1, 'catastrophic')`, mpID)
	require.Error(t, err, "unknown severity must be rejected")
	require.Contains(t, err.Error(), "23514")

	// scope_kind='global' allows NULL scope_id.
	_, err = pool.Exec(ctx,
		`INSERT INTO alert_rule (rule_kind, scope_kind, scope_id)
		 VALUES ('offline_gateway', 'global', NULL)`)
	require.NoError(t, err, "scope_kind='global' must allow NULL scope_id")

	// scope_kind='metering_point' with NULL scope_id violates the CHECK.
	_, err = pool.Exec(ctx,
		`INSERT INTO alert_rule (rule_kind, scope_kind, scope_id)
		 VALUES ('threshold_hourly', 'metering_point', NULL)`)
	require.Error(t, err, "non-global scope with NULL scope_id must be rejected")
	require.Contains(t, err.Error(), "23514")
}

// TestAlertSchema — 0039_alert schema invariants. payload JSONB, state machine
// CHECK, partial unique index alert_firing_unique_idx for idempotent fire.
func TestAlertSchema(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, pool, log))

	// Seed: site + mp + rule.
	var userID, siteID, mpID, ruleID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('alert_schema@example.com', 'AS', 'x', 'admin') RETURNING id`,
	).Scan(&userID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('alert-schema-site', 'UTC') RETURNING id`,
	).Scan(&siteID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class)
		 VALUES ($1, 'alert-schema-mp', 'water') RETURNING id`,
		siteID,
	).Scan(&mpID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO alert_rule
		   (rule_kind, scope_kind, scope_id, high_bound, comparison, unit, severity)
		 VALUES ('threshold_hourly', 'metering_point', $1, 40.0, 'gt', 'm3/h', 'critical')
		 RETURNING id`, mpID,
	).Scan(&ruleID))

	payload := `{"rule_id":"` + ruleID + `","rule_kind":"threshold_hourly","severity":"critical","value":42.7,"threshold":40.0,"comparison":"gt","unit":"m3/h"}`

	// Happy path: insert firing alert.
	var alertID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO alert
		   (rule_id, rule_kind, severity, payload, target_entity_type, target_entity_id)
		 VALUES ($1, 'threshold_hourly', 'critical', $2::jsonb, 'metering_point', $3)
		 RETURNING id`,
		ruleID, payload, mpID,
	).Scan(&alertID))

	// Unknown state rejected.
	_, err := pool.Exec(ctx,
		`INSERT INTO alert
		   (rule_id, rule_kind, severity, state, payload, target_entity_type, target_entity_id)
		 VALUES ($1, 'threshold_hourly', 'critical', 'pending', $2::jsonb, 'metering_point', $3)`,
		ruleID, payload, mpID)
	require.Error(t, err, "unknown state must be rejected")
	require.Contains(t, err.Error(), "23514")

	// Duplicate firing alert for same (rule_id, target_entity_id) violates the
	// partial unique index — idempotent fire pattern.
	_, err = pool.Exec(ctx,
		`INSERT INTO alert
		   (rule_id, rule_kind, severity, payload, target_entity_type, target_entity_id)
		 VALUES ($1, 'threshold_hourly', 'critical', $2::jsonb, 'metering_point', $3)`,
		ruleID, payload, mpID)
	require.Error(t, err, "duplicate firing alert must violate partial unique")
	require.Contains(t, err.Error(), "alert_firing_unique_idx")

	// Clearing the first allows a second to be inserted.
	_, err = pool.Exec(ctx,
		`UPDATE alert SET state = 'cleared', cleared_at = now() WHERE id = $1`, alertID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO alert
		   (rule_id, rule_kind, severity, payload, target_entity_type, target_entity_id)
		 VALUES ($1, 'threshold_hourly', 'critical', $2::jsonb, 'metering_point', $3)`,
		ruleID, payload, mpID)
	require.NoError(t, err, "after first cleared, second firing for same (rule, target) must insert")

	// is_test default = FALSE.
	var isTest bool
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT is_test FROM alert WHERE id = $1`, alertID,
	).Scan(&isTest))
	require.False(t, isTest, "is_test default must be FALSE")
}

// TestAlertWorkerStateSeed — 0042 seeds exactly 5 worker_kind rows.
func TestAlertWorkerStateSeed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, pool, log))

	rows, err := pool.Query(ctx, `SELECT worker_kind FROM alert_worker_state ORDER BY worker_kind`)
	require.NoError(t, err)
	defer rows.Close()
	got := map[string]bool{}
	for rows.Next() {
		var k string
		require.NoError(t, rows.Scan(&k))
		got[k] = true
	}
	require.NoError(t, rows.Err())
	for _, want := range []string{"threshold_instantaneous", "threshold_hourly", "threshold_daily", "offline", "anomaly"} {
		require.True(t, got[want], "worker_kind %q must be seeded by 0042", want)
	}
	require.Len(t, got, 5, "exactly 5 worker_kind seeds, got %v", got)

	// Unknown worker_kind rejected.
	_, err = pool.Exec(ctx, `INSERT INTO alert_worker_state (worker_kind) VALUES ('bogus')`)
	require.Error(t, err, "unknown worker_kind must be rejected")
	require.Contains(t, err.Error(), "23514")
}

// TestRetentionConfigPhase6Columns — 0040 extends retention_config with
// alerts_days + audit_log_days. install.FinishSetup seeds the row but
// these new columns have NOT NULL DEFAULTs, so a freshly migrated DB
// without FinishSetup having run still has correct values when a row
// is inserted via INSERT (id, raw_days, ..., yearly_days) using DEFAULTs
// for the new columns.
func TestRetentionConfigPhase6Columns(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, pool, log))

	// Insert the singleton row using Phase 5 defaults — alerts_days and
	// audit_log_days take their NOT NULL DEFAULTs (365 / 1825).
	_, err := pool.Exec(ctx, `
		INSERT INTO retention_config (id, raw_days, hourly_days, daily_days, monthly_days, yearly_days)
		VALUES (1, 90, 365, 1825, 7300, NULL)`)
	require.NoError(t, err)

	var alertsDays, auditDays int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT alerts_days, audit_log_days FROM retention_config WHERE id = 1`,
	).Scan(&alertsDays, &auditDays))
	require.Equal(t, 365, alertsDays, "alerts_days default = 365 (D-13)")
	require.Equal(t, 1825, auditDays, "audit_log_days default = 1825 (D-38)")

	// CHECK bounds: alerts_days BETWEEN 30 AND 3650.
	_, err = pool.Exec(ctx, `UPDATE retention_config SET alerts_days = 10 WHERE id = 1`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "23514")
	_, err = pool.Exec(ctx, `UPDATE retention_config SET alerts_days = 10000 WHERE id = 1`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "23514")

	// CHECK bounds: audit_log_days BETWEEN 90 AND 18250.
	_, err = pool.Exec(ctx, `UPDATE retention_config SET audit_log_days = 30 WHERE id = 1`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "23514")
	_, err = pool.Exec(ctx, `UPDATE retention_config SET audit_log_days = 50000 WHERE id = 1`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "23514")
}

// TestAdminPruneAuditRows_OwnedByAuditAdminRole — D-51: the SECURITY DEFINER
// function MUST be owned by `shifter_audit_admin` (a restricted role with
// only DELETE+INSERT on audit_log), NOT by the app role `shifter`.
func TestAdminPruneAuditRows_OwnedByAuditAdminRole(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, pool, log))

	var owner string
	err := pool.QueryRow(ctx, `
		SELECT pg_get_userbyid(proowner)
		FROM pg_proc
		WHERE proname = 'admin_prune_audit_rows'`,
	).Scan(&owner)
	require.NoError(t, err)
	require.Equal(t, "shifter_audit_admin", owner,
		"admin_prune_audit_rows must be owned by shifter_audit_admin (D-51)")
}

// TestAdminPruneAuditRows_BypassesTrigger — call the function, observe rows
// deleted + meta row inserted + trigger re-asserted for direct DELETE.
func TestAdminPruneAuditRows_BypassesTrigger(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, pool, log))

	// Seed user + an old audit row.
	var userID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('prune@example.com', 'P', 'x', 'admin') RETURNING id`,
	).Scan(&userID))
	_, err := pool.Exec(ctx,
		`INSERT INTO audit_log (time, user_id, action, entity_type, entity_id)
		 VALUES (now() - INTERVAL '2 years', $1, 'create', 'site', gen_random_uuid())`,
		userID)
	require.NoError(t, err)

	// Pre-condition: direct DELETE rejected by trigger.
	_, err = pool.Exec(ctx, `DELETE FROM audit_log WHERE action = 'create'`)
	require.Error(t, err, "direct DELETE must be rejected by INSERT-ONLY trigger")
	require.Contains(t, err.Error(), "INSERT-ONLY")

	// Function call MUST succeed and return number of rows deleted.
	var deleted int
	require.NoError(t, pool.QueryRow(ctx, `SELECT admin_prune_audit_rows(0)`).Scan(&deleted))
	require.GreaterOrEqual(t, deleted, 1, "must delete the 2-year-old row")

	// Meta audit.prune row must exist.
	var pruneRows int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE action = 'audit.prune'`,
	).Scan(&pruneRows))
	require.Equal(t, 1, pruneRows, "exactly one audit.prune meta row must be written per call")

	// Trigger remains active: a fresh direct DELETE is still rejected.
	_, err = pool.Exec(ctx, `DELETE FROM audit_log WHERE action = 'audit.prune'`)
	require.Error(t, err, "trigger must still reject direct DELETE after function exit")
	require.Contains(t, err.Error(), "INSERT-ONLY")
}

// TestAdminPruneAuditRows_RejectsNegativeCutoff — function defends against
// negative cutoff_days input.
func TestAdminPruneAuditRows_RejectsNegativeCutoff(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, pool, log))

	var deleted int
	err := pool.QueryRow(ctx, `SELECT admin_prune_audit_rows(-1)`).Scan(&deleted)
	require.Error(t, err, "negative cutoff_days must raise")
	require.Contains(t, err.Error(), "cutoff_days must be >= 0")
}

// silence unused-import warning for time when no test currently uses it.
var _ = time.Now
var _ = uuid.New
var _ = pgx.Identifier{}
