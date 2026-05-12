package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	pgxdb "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/shifter-io/shifter/internal/testsupport"
	"github.com/stretchr/testify/require"
)

// TestRunMigrations_RoundTrip — apply all migrations forward, roll all the
// way back down, then forward again. Catches non-idempotent down migrations
// and forward statements that fail when reapplied to a previously-migrated
// schema.
func TestRunMigrations_RoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()

	require.NoError(t, RunMigrations(ctx, pool, log), "first up")

	cfg := pool.Config()
	require.NotNil(t, cfg)
	dsn := cfg.ConnConfig.ConnString()
	sqlDB, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)

	src, err := iofs.New(migrationsFS, "migrations")
	require.NoError(t, err)
	drv, err := pgxdb.WithInstance(sqlDB, &pgxdb.Config{})
	require.NoError(t, err)
	m, err := migrate.NewWithInstance("iofs", src, "pgx", drv)
	require.NoError(t, err)

	// Roll all the way down. Some envs error with ErrNoChange when already at 0; treat as success.
	if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("migrate down: %v", err)
	}

	// Reapply. Should succeed back up to 43 (Plan 06-01 bumped from 37:
	// 0038_alert_rule / 0039_alert / 0040_retention_config_phase6 /
	// 0042_alert_worker_state / 0043_admin_prune_audit_rows — 0041 is a
	// deliberate gap so the SECURITY DEFINER prune function gets terminal
	// number 0043).
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("migrate up after down: %v", err)
	}
	v, dirty, err := m.Version()
	require.NoError(t, err)
	require.False(t, dirty)
	require.Equal(t, uint(43), v)

	// Verify seeds re-inserted after the round-trip.
	var n int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM device_profile WHERE slug IN ('axioma_w1','acrel_adl200','acrel_adw300')`,
	).Scan(&n))
	require.Equal(t, 3, n, "seeds present after round-trip")
}

// TestRunMigrations_MeasurementInsertedNotifyPropagatesToChunks — Plan 04-01
// Task 5 load-bearing assertion. The 0021_measurement_inserted_trigger
// migration registers an AFTER INSERT trigger on the `measurement` parent
// table; TimescaleDB must auto-propagate that trigger to the per-day chunk
// tables. Without chunk propagation Phase 4's SSE substrate (Plan 02 LISTENs
// on `measurement_inserted`) silently drops every uplink that lands in a
// chunk created after the trigger was installed.
//
// Test plan:
//  1. Apply all migrations (trigger landed by 0021).
//  2. On a dedicated pgx connection, LISTEN measurement_inserted; let LISTEN
//     issue before the INSERT (Postgres only queues notifications for already-
//     listening connections — same race the resolver listener test pins down).
//  3. INSERT a single measurement row with `time = now()` so TimescaleDB
//     creates a chunk and the row lands inside that chunk (not the parent).
//  4. WaitForNotification with 2s deadline. Notification MUST fire.
//  5. Decode the payload and confirm all 7 D-02 keys are present.
//
// Failure mode this guards against: a migration that drops chunk propagation
// (e.g. by re-creating the trigger via attach_table_trigger() or by adding
// an INSTEAD OF wrapper) would silently reduce dashboard freshness to zero.
func TestRunMigrations_MeasurementInsertedNotifyPropagatesToChunks(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	require.NoError(t, RunMigrations(ctx, pool, log))

	// Verify the trigger row exists in pg_trigger (defense-in-depth on top of
	// the chunk propagation observable assertion below).
	var triggerExists bool
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname = 'measurement_inserted_notify' AND NOT tgisinternal)`,
	).Scan(&triggerExists))
	require.True(t, triggerExists, "0021 must register trigger measurement_inserted_notify")

	// Seed a metering point — the inserted measurement references it (no FK
	// on the hypertable per 0015 docs, but using a real MP is realistic and
	// matches the UUID type at the column boundary).
	var siteID, mpID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('notify-roundtrip-site', 'UTC') RETURNING id`,
	).Scan(&siteID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class) VALUES ($1, 'notify-mp', 'water') RETURNING id`,
		siteID,
	).Scan(&mpID))

	// Acquire a dedicated connection and issue LISTEN. Releasing the conn
	// returns it to the pool with LISTEN state cleared.
	listenConn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer listenConn.Release()

	_, err = listenConn.Exec(ctx, "LISTEN measurement_inserted")
	require.NoError(t, err)

	// Wait briefly so the LISTEN definitely lands at the server before the
	// INSERT runs. Postgres only queues notifications for connections that
	// are already listening at NOTIFY time — race seen in resolver tests.
	time.Sleep(200 * time.Millisecond)

	// Insert via a separate goroutine so we can WaitForNotification on the
	// listen conn concurrently. time = now() forces TimescaleDB to either
	// reuse the current 1-day chunk or create one; the row is written to the
	// chunk table, NOT the parent — so the trigger MUST be propagated for
	// the NOTIFY to fire.
	var wg sync.WaitGroup
	wg.Add(1)
	insertErr := make(chan error, 1)
	go func() {
		defer wg.Done()
		_, err := pool.Exec(ctx,
			`INSERT INTO measurement (
				time, metering_point_id,
				cumulative_value, instant_value,
				battery_pct, rssi,
				raw_payload, decoded_object, quality
			) VALUES (
				now(), $1,
				123.456, 1.0,
				87, -85,
				'\x00'::bytea, '{}'::jsonb, 'ok'
			)`,
			mpID,
		)
		insertErr <- err
	}()

	// 2-second deadline matches the planner's <2s requirement.
	notifyCtx, notifyCancel := context.WithTimeout(ctx, 2*time.Second)
	defer notifyCancel()
	notification, err := listenConn.Conn().WaitForNotification(notifyCtx)
	require.NoError(t, err,
		"chunk-propagated trigger MUST fire NOTIFY within 2s — TimescaleDB chunk propagation broken if this fails")
	require.Equal(t, "measurement_inserted", notification.Channel)

	wg.Wait()
	require.NoError(t, <-insertErr, "INSERT must succeed")

	// Decode the payload and confirm all 7 D-02 keys are present and well-typed.
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(notification.Payload), &payload),
		"NOTIFY payload must be valid JSON: got %q", notification.Payload)

	for _, key := range []string{
		"metering_point_id", "time", "cumulative_value",
		"instant_value", "quality", "battery_pct", "rssi",
	} {
		_, ok := payload[key]
		require.True(t, ok, "D-02 key %q must be present in NOTIFY payload: %v", key, payload)
	}

	// Spot-check critical values round-trip cleanly through json_build_object.
	require.Equal(t, mpID, payload["metering_point_id"], "metering_point_id must match insert")
	require.Equal(t, "ok", payload["quality"], "quality literal must round-trip")

	// Payload size discipline (D-02 ≤200B headroom; the 8KB cap is a hard
	// pg_notify ceiling, but staying near 200B keeps the substrate cheap).
	require.Less(t, len(notification.Payload), 1024,
		"NOTIFY payload must stay well under the 8KB pg_notify cap: got %d bytes", len(notification.Payload))
}
