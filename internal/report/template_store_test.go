package report

// Tests for the report template store (Plan 07-11a).
// UX-POWER Surface 6: saved report templates.
//
// These tests require a real Postgres testcontainer with all migrations applied
// (0053_report_template.up.sql + 0054_audit_vocab_report_template.up.sql).
// They are skipped with -short.

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// TestTemplateStore_SaveListGetUpdateDelete — integration test covering all
// 5 store operations with a real Postgres container (migrations applied).
func TestTemplateStore_SaveListGetUpdateDelete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	store := NewTemplateStore(pool)
	actorID := uuid.New()

	// --- Save ---
	state := json.RawMessage(`{"scope":"all","range":"monthly"}`)
	tpl, err := store.Save(ctx, "Monthly All", "desc", state, actorID)
	require.NoError(t, err)
	require.Equal(t, "Monthly All", tpl.Name)
	require.Equal(t, "desc", tpl.Description)
	require.NotEqual(t, uuid.Nil, tpl.ID)

	// --- List ---
	list, err := store.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "Monthly All", list[0].Name)

	// --- Get ---
	got, err := store.Get(ctx, tpl.ID)
	require.NoError(t, err)
	require.Equal(t, tpl.ID, got.ID)
	require.Equal(t, "Monthly All", got.Name)

	// --- Update ---
	newState := json.RawMessage(`{"scope":"site","range":"daily"}`)
	err = store.Update(ctx, tpl.ID, "Daily Site", "new desc", newState, actorID)
	require.NoError(t, err)

	updated, err := store.Get(ctx, tpl.ID)
	require.NoError(t, err)
	require.Equal(t, "Daily Site", updated.Name)
	require.Equal(t, "new desc", updated.Description)

	// --- Audit rows written ---
	var auditCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE entity_id = $1`,
		tpl.ID,
	).Scan(&auditCount))
	// Save writes 1 row + Update writes 1 row = 2 total.
	require.Equal(t, 2, auditCount, "Save + Update must each write an audit row")

	// --- Delete ---
	err = store.Delete(ctx, tpl.ID, actorID)
	require.NoError(t, err)

	_, err = store.Get(ctx, tpl.ID)
	require.Error(t, err, "Get after Delete must return an error")

	// Audit row for delete also written.
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE entity_id = $1`,
		tpl.ID,
	).Scan(&auditCount))
	require.Equal(t, 3, auditCount, "Save + Update + Delete must each write an audit row")
}
