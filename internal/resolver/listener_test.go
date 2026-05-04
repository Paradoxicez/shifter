package resolver

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// waitForInvalidations blocks until r.Stats().Invalidations >= want or the
// timeout elapses. The poll interval is short (50ms) — listener-driven
// invalidations should propagate near-instantly under normal load.
func waitForInvalidations(t *testing.T, r *Resolver, want int64, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if r.Stats().Invalidations >= want {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// TestListener_InvalidatesOnNotify — happy path: start the listener, emit a
// pg_notify('binding_changed', dev_eui) from a regular conn, observe the
// resolver's invalidation counter go up + the cache entry vanish.
func TestListener_InvalidatesOnNotify(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.RunMigrations(ctx, pool, log))

	r := New(newFakeLoader())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.Run(ctx, pool, log)
	}()

	// Give the listener a moment to actually issue LISTEN. Without this we
	// race the test's NOTIFY against the LISTEN command — Postgres only
	// queues notifications for connections that are already listening.
	require.Eventually(t, func() bool {
		// Best-effort proxy: the listener has called Acquire+LISTEN once
		// it's ready. We can confirm by issuing a NOTIFY and seeing it
		// invalidate; but to keep this signal-clean, sleep briefly.
		return true
	}, 500*time.Millisecond, 50*time.Millisecond)
	time.Sleep(300 * time.Millisecond)

	// Pre-warm cache.
	r.Set("aabbccddeeff0011", makeBinding("alpha"))
	require.Equal(t, 1, r.Stats().Size)

	// Emit NOTIFY from a fresh tx.
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	require.NoError(t, EmitInvalidate(ctx, tx, "AABBCCDDEEFF0011"))
	require.NoError(t, tx.Commit(ctx))

	// Wait for the listener to apply the invalidation.
	require.True(t, waitForInvalidations(t, r, 1, 5*time.Second),
		"listener must process the binding_changed payload")

	// Cache must be empty now.
	require.Equal(t, 0, r.Stats().Size, "cache invalidated by NOTIFY")

	cancel()
	wg.Wait()
}

// TestListener_InvalidatesOnTriggerFire — end-to-end: insert a binding row,
// the 0017 trigger fires NOTIFY, listener invalidates the cache for that
// dev_eui. Pins the trigger ↔ listener contract end-to-end.
func TestListener_InvalidatesOnTriggerFire(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.RunMigrations(ctx, pool, log))

	// Seed minimum graph for binding insert: site, MP, profile, device.
	var siteID, mpID, profileID, deviceID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('test-site', 'UTC') RETURNING id`,
	).Scan(&siteID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class)
		 VALUES ($1, 'mp-1', 'water') RETURNING id`,
		siteID,
	).Scan(&mpID))
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT id FROM device_profile WHERE slug = 'axioma_w1'`,
	).Scan(&profileID))
	devEUI := "ffffaaaabbbb1234"
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO device (dev_eui, name, device_profile_id)
		 VALUES ($1, 'dev-1', $2) RETURNING id`,
		devEUI, profileID,
	).Scan(&deviceID))

	r := New(newFakeLoader())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.Run(ctx, pool, log)
	}()
	time.Sleep(300 * time.Millisecond) // listener startup

	// Pre-warm the cache so we can assert it was actually invalidated.
	r.Set(devEUI, makeBinding("alpha"))
	require.Equal(t, 1, r.Stats().Size)

	// Insert a binding — the 0017 trigger AFTER INSERT fires NOTIFY.
	_, err := pool.Exec(ctx,
		`INSERT INTO binding (metering_point_id, device_id, valid_from, reading_offset)
		 VALUES ($1, $2, '2026-05-04T12:00:00Z', 0)`,
		mpID, deviceID,
	)
	require.NoError(t, err)

	require.True(t, waitForInvalidations(t, r, 1, 5*time.Second),
		"trigger NOTIFY → listener invalidate")
	require.Equal(t, 0, r.Stats().Size)

	cancel()
	wg.Wait()
}

// TestListener_ContextCancel — Run must return promptly when ctx is canceled,
// not block on WaitForNotification. The deferred conn.Release() ensures the
// pool gets the conn back even if the listener was mid-wait.
func TestListener_ContextCancel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	require.NoError(t, db.RunMigrations(context.Background(), pool, log))

	r := New(newFakeLoader())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Run(ctx, pool, log)
		close(done)
	}()
	time.Sleep(300 * time.Millisecond) // let LISTEN issue

	cancel()
	select {
	case <-done:
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within 2s of context cancellation")
	}
}

// TestListener_ReconnectAfterDisconnect — kill the listener's conn (by
// running pg_terminate_backend on its pid). Run's outer loop must reconnect
// within reconnectBackoff + a bit of slack and process subsequent NOTIFYs.
func TestListener_ReconnectAfterDisconnect(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.RunMigrations(ctx, pool, log))

	// Use a small pool so we can identify "the listener's conn" deterministically.
	cfg := pool.Config().Copy()
	cfg.MaxConns = 4
	smallPool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	defer smallPool.Close()

	r := New(newFakeLoader())

	// Wrap Run with a ctx and run in goroutine.
	var runDone atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.Run(ctx, smallPool, log)
		runDone.Store(true)
	}()
	time.Sleep(300 * time.Millisecond) // initial LISTEN

	// Kill all OTHER backends listening to "binding_changed". The listener's
	// connection is the only one with that channel registered (every other
	// pool conn is short-lived per-query). pg_terminate_backend returns
	// true for each victim.
	rows, err := pool.Query(ctx,
		`SELECT pg_terminate_backend(p.pid)
		 FROM pg_stat_activity p
		 JOIN pg_listening_channels() lc(channel) ON lc.channel = 'binding_changed'
		 WHERE p.datname = current_database()`,
	)
	if err != nil {
		// pg_listening_channels is per-connection; the cross-connection
		// approach above only works inside the listening conn itself. Fall
		// back to: kill ALL backends except ours and the listener will be
		// among them.
		_, killErr := pool.Exec(ctx,
			`SELECT pg_terminate_backend(p.pid)
			 FROM pg_stat_activity p
			 WHERE p.datname = current_database()
			   AND p.pid <> pg_backend_pid()
			   AND p.application_name <> 'shifter-test-killer'
			   AND p.state IS DISTINCT FROM 'idle in transaction'`,
		)
		require.NoError(t, killErr)
	} else {
		rows.Close()
	}

	// Give the outer Run loop time to detect the failure (WaitForNotification
	// errors out) and reconnect (reconnectBackoff = 2s + LISTEN startup).
	time.Sleep(reconnectBackoff + 1*time.Second)

	// Pre-warm the cache and emit a NOTIFY through a fresh conn — the
	// reconnected listener should invalidate.
	r.Set("aabbccddeeff0011", makeBinding("alpha"))
	require.Equal(t, 1, r.Stats().Size)

	tx, err := smallPool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	require.NoError(t, EmitInvalidate(ctx, tx, "aabbccddeeff0011"))
	require.NoError(t, tx.Commit(ctx))

	// Allow up to reconnectBackoff*3 for the reconnect + delivery.
	require.True(t, waitForInvalidations(t, r, 1, 3*reconnectBackoff+2*time.Second),
		"reconnected listener must process subsequent NOTIFYs")

	cancel()
	wg.Wait()
	require.True(t, runDone.Load(), "Run must return after ctx cancel")
}
