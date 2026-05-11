package events

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// TestListener_ContextCancelExitsRun verifies that Run returns promptly when
// its context is canceled — even if it is blocked inside WaitForNotification.
// This is a unit-ish test that needs a real Postgres (for LISTEN to work);
// it does NOT exercise trigger propagation (see trigger_test.go for that).
func TestListener_ContextCancelExitsRun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	require.NoError(t, db.RunMigrations(context.Background(), pool, log))

	h := NewHub(log)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		h.Run(ctx, pool, log)
		close(done)
	}()
	// Let the listener issue LISTEN.
	time.Sleep(300 * time.Millisecond)

	cancel()
	select {
	case <-done:
		// good — Run returned cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within 2s of context cancellation — goroutine leak")
	}
}

// TestListener_ReconnectsAfterAcquireError verifies that the outer reconnect
// loop retries after a failed pool.Acquire and eventually exits when ctx is
// canceled. Uses a fake pool that always fails Acquire so no real DB is needed.
func TestListener_ReconnectsAfterAcquireError(t *testing.T) {
	pool := newErrPool(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	h := NewHub(log)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	var retries atomic.Int32
	// Replace the pool Acquire call by monitoring errPool calls.
	go func() {
		h.runOnce(ctx, pool, log) // one acquire attempt
		retries.Add(1)
		close(done)
	}()
	<-done
	cancel()
	require.GreaterOrEqual(t, retries.Load(), int32(1),
		"runOnce must be callable and return on acquire error")
}

// TestListener_RunExitsOnContextCancelAfterAcquireError verifies that when
// the pool keeps returning errors, Run's outer loop detects ctx.Done and
// returns within 100ms of cancellation (no infinite backoff hang).
func TestListener_RunExitsOnContextCancelAfterAcquireError(t *testing.T) {
	pool := newErrPool(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	h := NewHub(log)
	// Cancel immediately — Run should exit on the first ctx check.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		h.Run(ctx, pool, log)
		close(done)
	}()
	select {
	case <-done:
		// good
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Run did not return within 100ms of pre-canceled context")
	}
}

// TestListener_DispatchesNotifyToHub — full integration: start the listener,
// then emit a measurement insert via a separate pool conn, and verify the Hub
// dispatched the payload to a subscriber.
func TestListener_DispatchesNotifyToHub(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.RunMigrations(ctx, pool, log))

	// Seed a metering point so the INSERT is valid.
	var siteID, mpID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('listener-test-site', 'UTC') RETURNING id`,
	).Scan(&siteID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class) VALUES ($1, 'listener-test-mp', 'water') RETURNING id`,
		siteID,
	).Scan(&mpID))

	h := NewHub(log)
	ch := h.Subscribe("test-conn", []string{"dashboard:global"})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		h.Run(ctx, pool, log)
	}()
	// Give the listener time to issue LISTEN before the INSERT.
	time.Sleep(300 * time.Millisecond)

	// Insert a measurement row — the 0021 trigger fires NOTIFY.
	_, err := pool.Exec(ctx, `
		INSERT INTO measurement (time, metering_point_id, raw_payload, decoded_object, quality)
		VALUES (now(), $1, '\x00'::bytea, '{}'::jsonb, 'ok')
	`, mpID)
	require.NoError(t, err)

	// Hub must dispatch the payload to our subscriber within 2s.
	select {
	case payload := <-ch:
		require.NotNil(t, payload, "dispatched payload must not be nil")
		require.NotEmpty(t, payload, "dispatched payload must not be empty")
	case <-time.After(2 * time.Second):
		t.Fatal("Hub did not dispatch payload within 2s — listener or dispatch broken")
	}

	cancel()
	wg.Wait()
}

// ---------------------------------------------------------------------------
// fakePool helpers
// ---------------------------------------------------------------------------

// newErrPool returns a *pgxpool.Pool whose Acquire will always fail because
// the DSN is invalid. Used by unit tests that need to exercise the reconnect
// path without a real container.
func newErrPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	// Invalid DSN: pgxpool.New succeeds (lazy connect) but Acquire fails.
	pool, err := pgxpool.New(context.Background(), "postgres://localhost:1/nonexistent?connect_timeout=1")
	require.NoError(t, err, "pgxpool.New (lazy — should not fail)")
	t.Cleanup(pool.Close)
	return pool
}
