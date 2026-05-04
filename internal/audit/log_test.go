package audit

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// TestWriteEntry_RoundTrip — D-23 happy path. Open a tx, write an audit row
// with all 8 caller-provided columns populated, commit, and read it back via
// raw SQL to assert every field round-trips.
func TestWriteEntry_RoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	// Seed a user (FK target on audit_log.user_id).
	var userIDStr string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('admin@example.com', 'Admin', 'x', 'admin') RETURNING id`,
	).Scan(&userIDStr))
	userID, err := uuid.Parse(userIDStr)
	require.NoError(t, err)

	entityID := uuid.New()
	entry := Entry{
		UserID:     userID,
		Action:     ActionUpdate,
		EntityType: EntityTypeSite,
		EntityID:   entityID,
		Before:     map[string]any{"name": "old-name", "timezone": "UTC"},
		After:      map[string]any{"name": "new-name"},
		Notes:      "operator renamed site",
		RequestID:  "req-abc-123",
	}

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()

	require.NoError(t, WriteEntry(ctx, tx, entry))
	require.NoError(t, tx.Commit(ctx))

	// Read back the row by entity_id (single row expected).
	var (
		gotUserID                       pgtype.UUID
		gotAction, gotEntityType        string
		gotEntityID                     pgtype.UUID
		gotBeforeText, gotAfterText     string
		gotNotes, gotRequestID          *string
		gotTime                         pgtype.Timestamptz
	)
	err = pool.QueryRow(ctx, `
		SELECT user_id, action, entity_type, entity_id, before::text, after::text, notes, request_id, time
		FROM audit_log
		WHERE entity_id = $1
		LIMIT 1`,
		entityID,
	).Scan(&gotUserID, &gotAction, &gotEntityType, &gotEntityID, &gotBeforeText, &gotAfterText, &gotNotes, &gotRequestID, &gotTime)
	require.NoError(t, err)

	require.True(t, gotUserID.Valid)
	require.Equal(t, userID[:], gotUserID.Bytes[:])
	require.Equal(t, ActionUpdate, gotAction)
	require.Equal(t, EntityTypeSite, gotEntityType)
	require.True(t, gotEntityID.Valid)
	require.Equal(t, entityID[:], gotEntityID.Bytes[:])
	require.JSONEq(t, `{"name":"old-name","timezone":"UTC"}`, gotBeforeText)
	require.JSONEq(t, `{"name":"new-name"}`, gotAfterText)
	require.NotNil(t, gotNotes)
	require.Equal(t, "operator renamed site", *gotNotes)
	require.NotNil(t, gotRequestID)
	require.Equal(t, "req-abc-123", *gotRequestID)
	require.True(t, gotTime.Valid, "time defaults via now()")
	require.WithinDuration(t, time.Now(), gotTime.Time, 1*time.Minute)
}

// TestWriteEntry_NilBefore_OK — CREATE-shape: caller passes nil before map.
// Postgres must store SQL NULL (not the JSON literal "null", not "{}"). Phase
// 6 audit browse distinguishes "no diff captured" from "empty diff."
func TestWriteEntry_NilBefore_OK(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	var userIDStr string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('admin2@example.com', 'Admin Two', 'x', 'admin') RETURNING id`,
	).Scan(&userIDStr))
	userID, err := uuid.Parse(userIDStr)
	require.NoError(t, err)

	entityID := uuid.New()
	entry := Entry{
		UserID:     userID,
		Action:     ActionCreate,
		EntityType: EntityTypeSite,
		EntityID:   entityID,
		Before:     nil, // ← CREATE: no prior state
		After:      map[string]any{"name": "alpha"},
	}

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()

	require.NoError(t, WriteEntry(ctx, tx, entry))
	require.NoError(t, tx.Commit(ctx))

	var beforeIsNull bool
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT before IS NULL FROM audit_log WHERE entity_id = $1 LIMIT 1`,
		entityID,
	).Scan(&beforeIsNull))
	require.True(t, beforeIsNull, "nil before map must store SQL NULL, not '{}' or 'null' JSON literal")
}

// TestWriteEntry_RejectsBadAction — D-22 vocabulary enforcement: unknown
// action raises 23514 (audit_log_action_valid CHECK violation). The error
// surfaces in WriteEntry's return value with the constraint name in the
// message.
func TestWriteEntry_RejectsBadAction(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	var userIDStr string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('admin3@example.com', 'Admin Three', 'x', 'admin') RETURNING id`,
	).Scan(&userIDStr))
	userID, err := uuid.Parse(userIDStr)
	require.NoError(t, err)

	entry := Entry{
		UserID:     userID,
		Action:     "delete", // not in the allowed set — soft-delete is "archive"
		EntityType: EntityTypeSite,
		EntityID:   uuid.New(),
		After:      map[string]any{"name": "alpha"},
	}

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()

	err = WriteEntry(ctx, tx, entry)
	require.Error(t, err, "unknown action must be rejected")
	require.Contains(t, err.Error(), "audit_log_action_valid",
		"error must name the CHECK constraint: got %q", err.Error())
}

// TestWriteEntry_NoUpdate_NoDelete — 0016 INSERT-ONLY trigger. Even an audit
// row written through this package's WriteEntry cannot subsequently be
// modified by an UPDATE / DELETE. Combined with the API-level "tx-only"
// guarantee, audit rows are append-only end to end.
func TestWriteEntry_NoUpdate_NoDelete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	var userIDStr string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('admin4@example.com', 'Admin Four', 'x', 'admin') RETURNING id`,
	).Scan(&userIDStr))
	userID, err := uuid.Parse(userIDStr)
	require.NoError(t, err)

	entityID := uuid.New()

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	require.NoError(t, WriteEntry(ctx, tx, Entry{
		UserID:     userID,
		Action:     ActionCreate,
		EntityType: EntityTypeSite,
		EntityID:   entityID,
		After:      map[string]any{"name": "alpha"},
	}))
	require.NoError(t, tx.Commit(ctx))

	// Attempt UPDATE — must fail.
	_, err = pool.Exec(ctx, `UPDATE audit_log SET notes = 'tampered' WHERE entity_id = $1`, entityID)
	require.Error(t, err, "UPDATE must be rejected by 0016 trigger")
	require.Contains(t, err.Error(), "INSERT-ONLY", "got %q", err.Error())

	// Attempt DELETE — must fail.
	_, err = pool.Exec(ctx, `DELETE FROM audit_log WHERE entity_id = $1`, entityID)
	require.Error(t, err, "DELETE must be rejected by 0016 trigger")
	require.Contains(t, err.Error(), "INSERT-ONLY", "got %q", err.Error())
}

// TestWriteEntry_AtomicWithRollback — D-23 mechanical guarantee: if the
// caller's tx is rolled back, the audit row vanishes too. The "audit row
// cannot exist without its domain row" invariant.
func TestWriteEntry_AtomicWithRollback(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	var userIDStr string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('admin5@example.com', 'Admin Five', 'x', 'admin') RETURNING id`,
	).Scan(&userIDStr))
	userID, err := uuid.Parse(userIDStr)
	require.NoError(t, err)

	entityID := uuid.New()

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)

	require.NoError(t, WriteEntry(ctx, tx, Entry{
		UserID:     userID,
		Action:     ActionCreate,
		EntityType: EntityTypeSite,
		EntityID:   entityID,
		After:      map[string]any{"name": "to-be-rolled-back"},
	}))

	// Caller decides to roll back (simulates: domain mutation failed).
	require.NoError(t, tx.Rollback(ctx))

	// The audit row must NOT be visible — by Postgres atomicity.
	var present bool
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM audit_log WHERE entity_id = $1)`,
		entityID,
	).Scan(&present))
	require.False(t, present, "rolled-back audit row must not be visible")
}
