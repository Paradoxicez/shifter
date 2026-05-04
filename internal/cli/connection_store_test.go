package cli

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/chirpstack"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/profile"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// TestConnectionStore_GetEmpty: a fresh install (Phase 1 wizard finished but
// CS-side bootstrap NOT yet run, cs_tenant_id + cs_application_id NULL) →
// GetCSConnection returns ("", "", nil). Bootstrap is allowed to short-circuit
// rather than seeing pgx.ErrNoRows propagate up.
func TestConnectionStore_GetEmpty(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test — requires testcontainers")
	}
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

	// Seed the singleton chirpstack_connection row with NULL cs_tenant_id +
	// cs_application_id (the post-Phase-1-wizard, pre-bootstrap state).
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO chirpstack_connection
		    (id, mode, grpc_url, api_token_ref, mqtt_url, mqtt_user, mqtt_password_ref, region_name, region_common_name)
		VALUES (1, 'bundled', 'chirpstack:8080', 'cs_api_token', 'tcp://mosquitto:1883', NULL, NULL, 'AS923_2', 'AS923-2')
		ON CONFLICT (id) DO NOTHING`)
	require.NoError(t, err)

	store := NewConnectionStore(pool)
	tID, aID, err := store.GetCSConnection(ctx)
	require.NoError(t, err)
	require.Equal(t, "", tID, "fresh install (NULL cs_tenant_id) must return empty string")
	require.Equal(t, "", aID, "fresh install (NULL cs_application_id) must return empty string")
}

// TestConnectionStore_RoundTrip: SetCSTenantApp + GetCSConnection round-trips
// the values verbatim.
func TestConnectionStore_RoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test — requires testcontainers")
	}
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO chirpstack_connection
		    (id, mode, grpc_url, api_token_ref, mqtt_url, mqtt_user, mqtt_password_ref, region_name, region_common_name)
		VALUES (1, 'bundled', 'chirpstack:8080', 'cs_api_token', 'tcp://mosquitto:1883', NULL, NULL, 'AS923_2', 'AS923-2')
		ON CONFLICT (id) DO NOTHING`)
	require.NoError(t, err)

	store := NewConnectionStore(pool)
	wantTenant := uuid.New().String()
	wantApp := uuid.New().String()
	require.NoError(t, store.SetCSTenantApp(ctx, wantTenant, wantApp))

	gotTenant, gotApp, err := store.GetCSConnection(ctx)
	require.NoError(t, err)
	require.Equal(t, wantTenant, gotTenant, "round-trip tenant id")
	require.Equal(t, wantApp, gotApp, "round-trip application id")
}

// TestConnectionStore_NoRow: if chirpstack_connection has zero rows
// (pre-install / pre-Phase-1-wizard), GetCSConnection returns ("", "", nil).
// pgx.ErrNoRows is mapped to empty strings, not propagated. Bootstrap is
// allowed to short-circuit; serve.go's wiring degrades cleanly.
func TestConnectionStore_NoRow(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test — requires testcontainers")
	}
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

	// Don't seed the row. GetCSConnection must return ("","",nil), not error.
	store := NewConnectionStore(pool)
	tID, aID, err := store.GetCSConnection(context.Background())
	require.NoError(t, err, "missing chirpstack_connection row must NOT error")
	require.Equal(t, "", tID)
	require.Equal(t, "", aID)
}

// TestConnectionStore_SatisfiesBothInterfaces is a compile-time assertion that
// a single *ConnectionStore satisfies BOTH chirpstack.ConnectionStore (read +
// write) and profile.ConnectionStore (read only). Compiles at package load
// time; the test body itself is just a no-op so `go test -run` reports it.
func TestConnectionStore_SatisfiesBothInterfaces(t *testing.T) {
	var _ chirpstack.ConnectionStore = (*ConnectionStore)(nil)
	var _ profile.ConnectionStore = (*ConnectionStore)(nil)
}
