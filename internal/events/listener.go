package events

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// reconnectBackoff is the wait between LISTEN reconnect attempts when the
// dedicated connection drops (network blip, Postgres restart, etc.).
// PITFALL §12: too aggressive = thrashes a recovering DB; too slow = missed
// events queue up. 2s is the value pinned by Pattern 7 (mirrors resolver).
const reconnectBackoff = 2 * time.Second

// Run is the Hub's LISTEN measurement_inserted loop. It must be invoked once
// at process boot (typically as a goroutine inside `serve`) and runs until
// ctx is canceled.
//
// Outer reconnect loop (Pitfall 12 mitigation): if the underlying connection
// dies, runOnce returns an error; Run waits reconnectBackoff and retries.
// The defer conn.Release() inside runOnce returns the connection to the
// pool — pgxpool replaces dead connections on next Acquire.
//
// Run does NOT swallow ctx cancellation: when ctx.Err() != nil the loop
// returns cleanly so the caller's wait group / errgroup can join.
func (h *Hub) Run(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) {
	if log == nil {
		log = slog.Default()
	}
	for {
		if ctx.Err() != nil {
			log.Info("events listener stopping (context canceled)")
			return
		}
		err := h.runOnce(ctx, pool, log)
		if ctx.Err() != nil {
			log.Info("events listener stopping (context canceled)")
			return
		}
		log.Warn("events listener disconnected, reconnecting", "err", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(reconnectBackoff):
		}
	}
}

// runOnce acquires a dedicated pgx conn, issues LISTEN measurement_inserted,
// and loops on WaitForNotification. Returns the first error (or ctx
// cancellation) so the outer Run loop can decide to reconnect.
func (h *Hub) runOnce(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN measurement_inserted"); err != nil {
		return fmt.Errorf("LISTEN: %w", err)
	}
	log.Info("events listener listening on measurement_inserted")

	for {
		notif, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			return fmt.Errorf("WaitForNotification: %w", err)
		}
		h.dispatch([]byte(notif.Payload))
	}
}
