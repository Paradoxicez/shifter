package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// skipIfNotIntegration skips the test unless SHIFTER_INTEGRATION_TESTS is set.
// All tests in this file require a live TimescaleDB container and real NOTIFY
// round-trips; they are excluded from the normal short/unit suite.
func skipIfNotIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("SHIFTER_INTEGRATION_TESTS") == "" {
		t.Skip("skipping integration test: set SHIFTER_INTEGRATION_TESTS=1 to run")
	}
}

// TestMeasurementTrigger_PropagatesToChunks is the LOAD-BEARING assertion for
// Phase 4. It proves that the 0021 AFTER INSERT trigger on the `measurement`
// hypertable correctly propagates to the per-day TimescaleDB chunks created at
// INSERT time. Without chunk propagation the Phase 4 SSE pipeline would be
// silently dead — every uplink writes to a chunk, not the parent table.
//
// Test plan:
//  1. Spin up TimescaleDB testcontainer + apply all migrations.
//  2. Seed minimal MP fixture so measurement INSERTs don't fail FK-style.
//  3. Connection A: LISTEN measurement_inserted.
//  4. Connection B: INSERT a measurement with time=now() (creates/reuses today's chunk).
//  5. Connection A: WaitForNotification with 2s deadline — MUST succeed.
//  6. Payload: decode and assert all 7 D-02 keys present.
func TestMeasurementTrigger_PropagatesToChunks(t *testing.T) {
	skipIfNotIntegration(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	// Seed minimal fixture: site → metering_point.
	var siteID, mpID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('trigger-test-site', 'UTC') RETURNING id`,
	).Scan(&siteID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class)
		 VALUES ($1, 'trigger-test-mp', 'water') RETURNING id`,
		siteID,
	).Scan(&mpID))

	// Acquire a dedicated connection and issue LISTEN before the INSERT.
	// Postgres only queues notifications for connections that are already
	// listening at NOTIFY time — issue LISTEN before INSERT.
	listenConn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer listenConn.Release()

	_, err = listenConn.Exec(ctx, "LISTEN measurement_inserted")
	require.NoError(t, err)

	// Brief pause: ensure LISTEN is registered at the server before INSERT.
	// Postgres only queues notifications for connections that are already
	// listening at NOTIFY time — the pause eliminates the listen-vs-insert race.
	time.Sleep(200 * time.Millisecond)

	// Insert directly (not via goroutine): Postgres buffers the NOTIFY for the
	// listening connection even if the NOTIFY is emitted before WaitForNotification
	// is called — the notification is queued at the server until the client polls.
	_, err = pool.Exec(ctx, `
		INSERT INTO measurement (
			time, metering_point_id,
			cumulative_value, instant_value,
			battery_pct, rssi,
			raw_payload, decoded_object, quality
		) VALUES (
			now(), $1,
			123.45, 4.2,
			87, -65,
			'\x00'::bytea, '{}'::jsonb, 'ok'
		)`,
		mpID,
	)
	require.NoError(t, err, "measurement INSERT must succeed")

	// WaitForNotification with 2-second deadline. The notification was already
	// queued by the NOTIFY above; this call dequeues it.
	waitCtx, waitCancel := context.WithTimeout(ctx, 2*time.Second)
	defer waitCancel()
	notif, err := listenConn.Conn().WaitForNotification(waitCtx)
	require.NoError(t, err,
		"trigger did not fire — chunk propagation likely broken: TimescaleDB must propagate AFTER INSERT triggers from parent table to chunks")

	require.Equal(t, "measurement_inserted", notif.Channel)

	// Decode and assert all 7 D-02 keys are present.
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(notif.Payload), &decoded),
		"NOTIFY payload must be valid JSON: got %q", notif.Payload)

	for _, k := range []string{
		"metering_point_id", "time", "cumulative_value",
		"instant_value", "quality", "battery_pct", "rssi",
	} {
		_, ok := decoded[k]
		assert.True(t, ok, "payload missing D-02 key %q — full payload: %v", k, decoded)
	}

	// Spot-check: metering_point_id round-trips correctly.
	require.Equal(t, mpID, decoded["metering_point_id"],
		"metering_point_id must match the seeded MP")
	require.Equal(t, "ok", decoded["quality"],
		"quality literal must round-trip through json_build_object")

	// Payload size discipline: stay well under 8KB pg_notify cap.
	require.Less(t, len(notif.Payload), 1024,
		"NOTIFY payload must be compact: got %d bytes", len(notif.Payload))
}

// TestMeasurementTrigger_AcrossChunks proves that the trigger propagates not
// just to the current day's chunk, but also to historical chunks. This matters
// because TimescaleDB creates the historical chunk lazily (or reuses an
// existing one) and must propagate the trigger definition to it as well.
//
// Two inserts land in different chunks (now() and now() - 2 days). Both must
// trigger NOTIFY.
func TestMeasurementTrigger_AcrossChunks(t *testing.T) {
	skipIfNotIntegration(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	// Seed fixture.
	var siteID, mpID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('chunk-cross-site', 'UTC') RETURNING id`,
	).Scan(&siteID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class)
		 VALUES ($1, 'chunk-cross-mp', 'water') RETURNING id`,
		siteID,
	).Scan(&mpID))

	// Acquire LISTEN connection.
	listenConn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer listenConn.Release()

	_, err = listenConn.Exec(ctx, "LISTEN measurement_inserted")
	require.NoError(t, err)
	time.Sleep(200 * time.Millisecond) // ensure LISTEN is active at server

	// Insert row 1: current chunk (now()).
	_, err = pool.Exec(ctx, `
		INSERT INTO measurement (
			time, metering_point_id,
			raw_payload, decoded_object, quality
		) VALUES (
			now(), $1,
			'\x01'::bytea, '{}'::jsonb, 'ok'
		)`,
		mpID,
	)
	require.NoError(t, err, "current-chunk INSERT must succeed")

	// Receive notification 1.
	waitCtx1, cancelWait1 := context.WithTimeout(ctx, 2*time.Second)
	defer cancelWait1()
	notif1, err := listenConn.Conn().WaitForNotification(waitCtx1)
	require.NoError(t, err,
		"trigger did not fire for current-chunk INSERT — chunk propagation likely broken")
	require.Equal(t, "measurement_inserted", notif1.Channel)

	// Insert row 2: historical chunk (now() - 2 days → different 1-day chunk).
	_, err = pool.Exec(ctx, `
		INSERT INTO measurement (
			time, metering_point_id,
			raw_payload, decoded_object, quality
		) VALUES (
			now() - INTERVAL '2 days', $1,
			'\x02'::bytea, '{}'::jsonb, 'ok'
		)`,
		mpID,
	)
	require.NoError(t, err, "historical-chunk INSERT must succeed")

	// Receive notification 2.
	waitCtx2, cancelWait2 := context.WithTimeout(ctx, 2*time.Second)
	defer cancelWait2()
	notif2, err := listenConn.Conn().WaitForNotification(waitCtx2)
	require.NoError(t, err,
		"trigger did not fire for historical-chunk INSERT — TimescaleDB trigger chunk propagation broken for pre-existing chunks")
	require.Equal(t, "measurement_inserted", notif2.Channel)

	// Both payloads must be valid JSON with all 7 D-02 keys.
	for i, notif := range []*struct{ Payload string }{
		{notif1.Payload},
		{notif2.Payload},
	} {
		var decoded map[string]any
		require.NoError(t, json.Unmarshal([]byte(notif.Payload), &decoded),
			"notification %d payload must be valid JSON", i+1)
		for _, k := range []string{
			"metering_point_id", "time", "cumulative_value",
			"instant_value", "quality", "battery_pct", "rssi",
		} {
			_, ok := decoded[k]
			assert.True(t, ok, "notification %d missing D-02 key %q", i+1, k)
		}
	}
}
