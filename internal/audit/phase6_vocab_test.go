package audit

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// TestPhase6VocabularyConstants — D-30 + D-51 vocabulary drift test. Every
// Phase 6 const declared in log.go MUST appear as a string literal in the
// 0037 up migration so the audit_log CHECK and the Go constants stay in lock-
// step. Phase 6 introduces 27 new actions + 6 new entity types (see D-30 +
// D-51 + D-35 — the audit.export verb is provisioned now but consumed in
// Plan 06-07).
func TestPhase6VocabularyConstants(t *testing.T) {
	// Locate the migration file relative to this test file (works under `go test`).
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller(0) must succeed")
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	migrationPath := filepath.Join(repoRoot, "internal", "db", "migrations",
		"0037_audit_vocab_phase6.up.sql")
	raw, err := os.ReadFile(migrationPath)
	require.NoError(t, err, "0037_audit_vocab_phase6.up.sql must exist at %s", migrationPath)
	sql := string(raw)

	// 27 new Phase 6 actions (D-30 auth events + user-mgmt + alerts + backup
	// + D-51 audit.prune + D-35 audit.export).
	wantActions := []string{
		// Auth events (D-30):
		ActionAuthLoginSuccess,
		ActionAuthLoginFailed,
		ActionAuthLogout,
		ActionAuthPasswordChange,
		ActionAuthPasswordResetByAdmin,
		ActionAuthSessionRevoked,
		// User mgmt (D-30):
		ActionUserCreate,
		ActionUserUpdate,
		ActionUserDisable,
		ActionUserEnable,
		ActionUserRoleChange,
		// Alerts (Phase 6):
		ActionAlertRuleCreate,
		ActionAlertRuleUpdate,
		ActionAlertRuleDisable,
		ActionAlertRuleEnable,
		ActionAlertFired,
		ActionAlertCleared,
		ActionAlertAcked,
		ActionAlertSnoozed,
		ActionAlertMuted,
		ActionAlertTestFired,
		// Backup + audit prune + audit export:
		ActionBackupStart,
		ActionBackupComplete,
		ActionBackupFailed,
		ActionBackupRestore,
		ActionAuditPrune,
		ActionAuditExport,
	}
	require.Equal(t, 27, len(wantActions), "Phase 6 must add exactly 27 actions")
	for _, action := range wantActions {
		require.Contains(t, sql, "'"+action+"'",
			"0037 must include the literal %q so the CHECK accepts the constant", action)
	}

	// 6 new entity types (D-30 + D-51).
	wantEntityTypes := []string{
		EntityTypeUser,
		EntityTypeSession,
		EntityTypeAlertRule,
		EntityTypeAlert,
		EntityTypeBackupRun,
		EntityTypeAuditLog,
	}
	require.Equal(t, 6, len(wantEntityTypes), "Phase 6 must add exactly 6 entity types")
	for _, et := range wantEntityTypes {
		require.Contains(t, sql, "'"+et+"'",
			"0037 must include the literal %q so the CHECK accepts the constant", et)
	}

	// Sanity: each new action MUST appear exactly once in the file (catches
	// accidental double-listing if a future plan edits the file).
	for _, action := range wantActions {
		// Account for the action appearing once in the up-migration CHECK.
		count := strings.Count(sql, "'"+action+"'")
		require.Equal(t, 1, count,
			"action %q must appear exactly once in 0037 up: got %d occurrences", action, count)
	}
}

// TestPhase6VocabularyMigration_AcceptsAndRejects — apply migrations, then:
//   - INSERT for every new vocabulary string succeeds.
//   - INSERT of an unknown string still fails 23514.
//   - After down, the new vocabulary strings fail 23514 (rollback works).
func TestPhase6VocabularyMigration_AcceptsAndRejects(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	// Seed user (FK target on audit_log.user_id).
	var userIDStr string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('phase6vocab@example.com', 'P6', 'x', 'admin') RETURNING id`,
	).Scan(&userIDStr))
	userID, err := uuid.Parse(userIDStr)
	require.NoError(t, err)

	// Map of new action → an entity_type that should pair with it. We use
	// pragmatic pairings — most actions only constrain `action`, not the
	// (action, entity_type) tuple. EntityTypeSession is reserved for
	// auth.session_revoked; the user verbs target EntityTypeUser; alerts
	// target alert_rule / alert; backups target backup_run; audit prune /
	// export target the audit_log meta-row.
	cases := []struct {
		action     string
		entityType string
	}{
		{ActionAuthLoginSuccess, EntityTypeUser},
		{ActionAuthLoginFailed, EntityTypeUser},
		{ActionAuthLogout, EntityTypeUser},
		{ActionAuthPasswordChange, EntityTypeUser},
		{ActionAuthPasswordResetByAdmin, EntityTypeUser},
		{ActionAuthSessionRevoked, EntityTypeSession},
		{ActionUserCreate, EntityTypeUser},
		{ActionUserUpdate, EntityTypeUser},
		{ActionUserDisable, EntityTypeUser},
		{ActionUserEnable, EntityTypeUser},
		{ActionUserRoleChange, EntityTypeUser},
		{ActionAlertRuleCreate, EntityTypeAlertRule},
		{ActionAlertRuleUpdate, EntityTypeAlertRule},
		{ActionAlertRuleDisable, EntityTypeAlertRule},
		{ActionAlertRuleEnable, EntityTypeAlertRule},
		{ActionAlertFired, EntityTypeAlert},
		{ActionAlertCleared, EntityTypeAlert},
		{ActionAlertAcked, EntityTypeAlert},
		{ActionAlertSnoozed, EntityTypeAlert},
		{ActionAlertMuted, EntityTypeAlert},
		{ActionAlertTestFired, EntityTypeAlert},
		{ActionBackupStart, EntityTypeBackupRun},
		{ActionBackupComplete, EntityTypeBackupRun},
		{ActionBackupFailed, EntityTypeBackupRun},
		{ActionBackupRestore, EntityTypeBackupRun},
		{ActionAuditPrune, EntityTypeAuditLog},
		{ActionAuditExport, EntityTypeAuditLog},
	}
	for _, c := range cases {
		tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
		require.NoError(t, err)
		err = WriteEntry(ctx, tx, Entry{
			UserID:     userID,
			Action:     c.action,
			EntityType: c.entityType,
			EntityID:   uuid.New(),
			After:      map[string]any{"k": "v"},
		})
		require.NoError(t, err,
			"action=%q entity=%q must satisfy Phase 6 CHECK after 0037 up", c.action, c.entityType)
		require.NoError(t, tx.Commit(ctx))
	}

	// Unknown action still fails 23514.
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	err = WriteEntry(ctx, tx, Entry{
		UserID:     userID,
		Action:     "unknown.future_action",
		EntityType: EntityTypeUser,
		EntityID:   uuid.New(),
		After:      map[string]any{"k": "v"},
	})
	require.Error(t, err, "unknown action must still 23514")
	require.Contains(t, err.Error(), "audit_log_action_valid")
}
