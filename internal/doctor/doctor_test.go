package doctor

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// makeDoctor is a test helper that starts a Postgres testcontainer, runs
// migrations, and returns a Doctor wired to the pool.
func makeDoctor(t *testing.T) (*Doctor, func()) {
	t.Helper()
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	// Seed retention_config row (required for /health/detailed CROSS JOIN).
	_, _ = pool.Exec(ctx, `
		INSERT INTO retention_config (id, raw_days, hourly_days, daily_days, monthly_days, yearly_days)
		VALUES (1, 90, 365, 1825, 7300, NULL)
		ON CONFLICT (id) DO NOTHING`)

	d := &Doctor{
		Pool: pool,
	}
	return d, func() { pool.Close() }
}

// TestDoctor_SnapshotBundleShape — SnapshotBundle returns a Bundle with all 9
// required top-level keys.
func TestDoctor_SnapshotBundleShape(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	d, cleanup := makeDoctor(t)
	defer cleanup()

	bundle, err := d.SnapshotBundle(context.Background())
	require.NoError(t, err)
	require.NotNil(t, bundle)

	// Validate shape by marshalling and unmarshalling to a map.
	raw, err := json.Marshal(bundle)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))

	for _, key := range []string{
		"generated_at",
		"shifter",
		"config_check",
		"health_detailed",
		"alert_workers",
		"last_backup",
		"chirpstack_grpc_ping",
		"recent_audit",
		"logs",
	} {
		_, ok := m[key]
		require.True(t, ok, "bundle must contain key %q", key)
	}

	// alert_workers must be an array.
	awRaw, _ := m["alert_workers"].([]any)
	require.NotNil(t, awRaw, "alert_workers must be a JSON array")

	// recent_audit must be an array.
	raRaw, _ := m["recent_audit"].([]any)
	require.NotNil(t, raRaw, "recent_audit must be a JSON array")

	// logs must be an array with the fallback note.
	logsRaw, _ := m["logs"].([]any)
	require.NotEmpty(t, logsRaw, "logs must be non-empty")
}

// TestDoctor_RecentAuditIsRedacted — when audit rows contain operator emails,
// the bundle's recent_audit entries have masked emails after MarshalRedacted.
func TestDoctor_RecentAuditIsRedacted(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	d, cleanup := makeDoctor(t)
	defer cleanup()
	ctx := context.Background()

	// Seed a user with a recognisable email and an audit row.
	var userID string
	require.NoError(t, d.Pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('operator@example.com', 'Op', 'x', 'admin') RETURNING id`,
	).Scan(&userID))
	_, err := d.Pool.Exec(ctx,
		`INSERT INTO audit_log (time, user_id, action, entity_type, entity_id)
		 VALUES (now(), $1, 'create', 'site', gen_random_uuid())`, userID)
	require.NoError(t, err)

	bundle, err := d.SnapshotBundle(ctx)
	require.NoError(t, err)

	redacted, err := bundle.MarshalRedacted()
	require.NoError(t, err)

	// The real email must not appear in the redacted output.
	require.NotContains(t, string(redacted), "operator@example.com",
		"MarshalRedacted must mask operator email in recent_audit")
	// The masked form must appear.
	require.Contains(t, string(redacted), "o***@example.com",
		"MarshalRedacted must contain the masked form")
}

// TestDoctor_LogsContainsFallbackNote — bundle.Logs contains the Docker socket
// fallback string (D-50 deferral documented in bundle).
func TestDoctor_LogsContainsFallbackNote(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	d, cleanup := makeDoctor(t)
	defer cleanup()

	bundle, err := d.SnapshotBundle(context.Background())
	require.NoError(t, err)

	require.Len(t, bundle.Logs, 1)
	require.Contains(t, bundle.Logs[0], "docker socket not mounted",
		"logs[0] must contain the Docker socket fallback note")
	require.Contains(t, bundle.Logs[0], "docker compose logs --tail=200 shifter",
		"logs[0] must contain the recommended fallback command")
}

// TestDoctor_MarshalRedacted_NoRawEmail — MarshalRedacted output never contains
// raw email-shaped tokens that were present in audit rows.
func TestDoctor_MarshalRedacted_NoRawEmail(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	d, cleanup := makeDoctor(t)
	defer cleanup()
	ctx := context.Background()

	// Seed 3 users with distinct emails.
	for _, email := range []string{"alice@a.io", "bob@b.io", "carol@c.io"} {
		var uid string
		require.NoError(t, d.Pool.QueryRow(ctx,
			`INSERT INTO "user" (email, name, password_hash, role)
			 VALUES ($1, $1, 'x', 'viewer') RETURNING id`, email,
		).Scan(&uid))
		_, _ = d.Pool.Exec(ctx,
			`INSERT INTO audit_log (time, user_id, action, entity_type, entity_id)
			 VALUES (now(), $1, 'create', 'site', gen_random_uuid())`, uid)
	}

	bundle, err := d.SnapshotBundle(ctx)
	require.NoError(t, err)
	redacted, err := bundle.MarshalRedacted()
	require.NoError(t, err)

	out := string(redacted)
	for _, raw := range []string{"alice@a.io", "bob@b.io", "carol@c.io"} {
		require.NotContains(t, out, raw, "raw email %q must not appear after redaction", raw)
	}
}
