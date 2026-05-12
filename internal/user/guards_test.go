package user

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
	"github.com/stretchr/testify/require"
)

// TestRejectSelfAction covers the three branches:
// (a) actingID == targetID → ErrSelfAction
// (b) actingID != targetID → nil
// (c) any id empty (defensive) → nil
func TestRejectSelfAction(t *testing.T) {
	require.ErrorIs(t, RejectSelfAction("11111111-1111-1111-1111-111111111111",
		"11111111-1111-1111-1111-111111111111"), ErrSelfAction)
	require.NoError(t, RejectSelfAction("aaaa", "bbbb"))
	require.NoError(t, RejectSelfAction("", "bbbb"))
	require.NoError(t, RejectSelfAction("aaaa", ""))
}

func setupGuardsDB(t *testing.T) (*pgx.Conn, *auth.Store) {
	t.Helper()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool,
		slog.New(slog.NewTextHandler(os.Stderr, nil))))
	return nil, auth.NewStore(pool)
}

// TestRejectLastAdminDemote_BlocksLastAdmin — seed one admin; demoting
// returns ErrLastAdmin.
func TestRejectLastAdminDemote_BlocksLastAdmin(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	_, store := setupGuardsDB(t)
	pool := store.Pool()

	hash, err := auth.Hash("Ann-Strong-Pass-1!")
	require.NoError(t, err)
	adminID, err := store.InsertAdminUser(context.Background(), "ann@example.com", "Ann", hash)
	require.NoError(t, err)

	tx, err := pool.BeginTx(context.Background(), pgx.TxOptions{IsoLevel: pgx.Serializable})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })

	require.ErrorIs(t,
		RejectLastAdminDemote(context.Background(), tx, store, adminID, "viewer"),
		ErrLastAdmin)
}

// TestRejectLastAdminDemote_AllowsWhenMultipleAdmins — seed two admins;
// either demote returns nil.
func TestRejectLastAdminDemote_AllowsWhenMultipleAdmins(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	_, store := setupGuardsDB(t)
	pool := store.Pool()

	hash, err := auth.Hash("Pass-Strong-1234!")
	require.NoError(t, err)
	a1, err := store.InsertAdminUser(context.Background(), "a1@example.com", "A1", hash)
	require.NoError(t, err)
	a2, err := store.InsertAdminUser(context.Background(), "a2@example.com", "A2", hash)
	require.NoError(t, err)

	tx, err := pool.BeginTx(context.Background(), pgx.TxOptions{IsoLevel: pgx.Serializable})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })

	require.NoError(t, RejectLastAdminDemote(context.Background(), tx, store, a1, "viewer"))
	require.NoError(t, RejectLastAdminDemote(context.Background(), tx, store, a2, "viewer"))
}

// TestRejectLastAdminDemote_PromoteToAdminAllowed — newRole='admin' is always
// allowed; the function returns nil without even querying the DB.
func TestRejectLastAdminDemote_PromoteToAdminAllowed(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	_, store := setupGuardsDB(t)
	pool := store.Pool()

	tx, err := pool.BeginTx(context.Background(), pgx.TxOptions{IsoLevel: pgx.Serializable})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })

	require.NoError(t,
		RejectLastAdminDemote(context.Background(), tx, store,
			"00000000-0000-0000-0000-000000000000", "admin"))
}

// TestRejectLastAdminDemote_TOCTOU_SerializableTx — two parallel demotes
// targeting "the other admin" both observe one-other-admin and both pass
// the guard inside their own tx. Under READ COMMITTED both would commit and
// leave zero admins; under SERIALIZABLE one Commit returns a 40001
// serialization_failure and the other succeeds.
func TestRejectLastAdminDemote_TOCTOU_SerializableTx(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	_, store := setupGuardsDB(t)
	pool := store.Pool()

	hash, err := auth.Hash("Strong-pass-1234!")
	require.NoError(t, err)
	a1, err := store.InsertAdminUser(context.Background(), "a1@example.com", "A1", hash)
	require.NoError(t, err)
	a2, err := store.InsertAdminUser(context.Background(), "a2@example.com", "A2", hash)
	require.NoError(t, err)

	// Run two demotes in parallel. Each tx reads "is there at least one
	// OTHER admin?" — both observe the other and pass the guard, then both
	// try to write. Under SERIALIZABLE, one commit fails with 40001.
	var wg sync.WaitGroup
	wg.Add(2)
	results := make(chan error, 2)

	demote := func(actor, target string) {
		defer wg.Done()
		ctx := context.Background()
		tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			results <- err
			return
		}
		// SELECT FOR UPDATE the target row so each tx holds a different row
		// lock; the conflict surfaces on the index/predicate level.
		if _, err := store.GetByIDForUpdate(ctx, tx, target); err != nil {
			_ = tx.Rollback(ctx)
			results <- err
			return
		}
		if err := RejectLastAdminDemote(ctx, tx, store, target, "viewer"); err != nil {
			_ = tx.Rollback(ctx)
			results <- err
			return
		}
		if err := store.ChangeRoleTx(ctx, tx, target, "viewer"); err != nil {
			_ = tx.Rollback(ctx)
			results <- err
			return
		}
		results <- tx.Commit(ctx)
	}
	go demote(a1, a2)
	go demote(a2, a1)
	wg.Wait()
	close(results)

	successCount := 0
	serFailures := 0
	otherErrors := []error{}
	for err := range results {
		if err == nil {
			successCount++
			continue
		}
		var pgErr interface{ SQLState() string }
		if errors.As(err, &pgErr) && pgErr.SQLState() == "40001" {
			serFailures++
			continue
		}
		// ErrLastAdmin from inside the tx is also acceptable — one tx may
		// see the other's write before its own guard runs.
		if errors.Is(err, ErrLastAdmin) {
			serFailures++
			continue
		}
		otherErrors = append(otherErrors, err)
	}
	require.Empty(t, otherErrors, "unexpected errors: %v", otherErrors)
	require.Equal(t, 2, successCount+serFailures, "two outcomes expected")
	require.GreaterOrEqual(t, serFailures, 1,
		"at least one tx must fail to preserve last-admin invariant; got successes=%d failures=%d",
		successCount, serFailures)

	// Final invariant: at least one active admin remains.
	var activeAdmins int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM "user" WHERE role = 'admin' AND disabled_at IS NULL`).Scan(&activeAdmins))
	require.GreaterOrEqual(t, activeAdmins, 1, "last-admin invariant violated")
}
