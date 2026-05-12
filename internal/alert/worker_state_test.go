package alert

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// TestAlertWorkerStateUpserter — UpsertWorkerState writes per-cycle stats,
// clears degraded, leaves last_error empty when FirstErr is empty.
func TestAlertWorkerStateUpserter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	store := NewWorkerStateStore(pool)

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	rs := &RunState{
		RulesEvaluated: 3,
		FiresEmitted:   1,
		Cleared:        0,
		DurationMS:     42,
	}
	require.NoError(t, store.UpsertWorkerState(ctx, tx, "threshold_hourly", rs))
	require.NoError(t, tx.Commit(ctx))

	got, err := store.GetWorkerState(ctx, "threshold_hourly")
	require.NoError(t, err)
	require.Equal(t, int32(3), got.RulesEvaluated)
	require.Equal(t, int32(1), got.FiresEmitted)
	require.Equal(t, int32(42), got.DurationMS)
	require.False(t, got.Degraded)
	require.Nil(t, got.LastError)
}

// TestRunState_RecordErr_OnlyFirst — RecordErr stores only the first error.
func TestRunState_RecordErr_OnlyFirst(t *testing.T) {
	r := &RunState{}
	r.RecordErr(errors.New("first"))
	r.RecordErr(errors.New("second"))
	require.Equal(t, "first", r.FirstErr)
}

// TestMarkWorkerDegraded — flips degraded=true + writes last_error.
func TestMarkWorkerDegraded(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	store := NewWorkerStateStore(pool)
	require.NoError(t, store.MarkWorkerDegraded(ctx, "offline", "test failure"))

	got, err := store.GetWorkerState(ctx, "offline")
	require.NoError(t, err)
	require.True(t, got.Degraded)
	require.NotNil(t, got.LastError)
	require.Equal(t, "test failure", *got.LastError)
}
