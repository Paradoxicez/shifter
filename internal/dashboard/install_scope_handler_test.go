package dashboard

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// TestScopeHandler_WaterCapabilities verifies GET /api/dashboard/scope returns
// the correct capabilities string and zero onboarding counts on a fresh install.
func TestScopeHandler_WaterCapabilities(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	require.NoError(t, db.RunMigrations(context.Background(), pool, log))
	seedInstallIdentity(t, pool, "water")

	deps := newTestDashboardDeps(pool)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/scope", nil)
	w := httptest.NewRecorder()
	deps.handleScope(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp ScopeResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, "water", resp.Capabilities)
	// Fresh install — no gateways, no devices, no uplinks.
	assert.Equal(t, int64(0), resp.Onboarding.GatewayCount)
	assert.Equal(t, int64(0), resp.Onboarding.DeviceCount)
	assert.Equal(t, int64(0), resp.Onboarding.UplinkCount)
}

// TestScopeHandler_BothCapabilities verifies GET /api/dashboard/scope with "both".
func TestScopeHandler_BothCapabilities(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	require.NoError(t, db.RunMigrations(context.Background(), pool, log))
	seedInstallIdentity(t, pool, "both")

	deps := newTestDashboardDeps(pool)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/scope", nil)
	w := httptest.NewRecorder()
	deps.handleScope(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp ScopeResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "both", resp.Capabilities)
}

// TestScopeHandler_OnboardingCounts verifies D-21: gateway_count and device_count
// reflect the actual rows in the database.
func TestScopeHandler_OnboardingCounts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	require.NoError(t, db.RunMigrations(context.Background(), pool, log))
	seedInstallIdentity(t, pool, "both")

	ctx := context.Background()

	// Insert one active gateway.
	_, err := pool.Exec(ctx, `
		INSERT INTO gateway (gateway_id, name, region)
		VALUES ('aabbccddeeff0011', 'gw-1', 'EU868')
	`)
	require.NoError(t, err)

	// Insert one device (uses the seeded axioma_w1 profile).
	var profileID string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT id FROM device_profile WHERE slug = 'axioma_w1'`,
	).Scan(&profileID))
	_, err = pool.Exec(ctx, `
		INSERT INTO device (dev_eui, name, device_profile_id)
		VALUES ('aabbccddeeff0011', 'dev-1', $1)
	`, profileID)
	require.NoError(t, err)

	deps := newTestDashboardDeps(pool)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/scope", nil)
	w := httptest.NewRecorder()
	deps.handleScope(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp ScopeResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, int64(1), resp.Onboarding.GatewayCount)
	assert.Equal(t, int64(1), resp.Onboarding.DeviceCount)
	assert.Equal(t, int64(0), resp.Onboarding.UplinkCount)
}
