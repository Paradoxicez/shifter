package dashboard

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// seedInstallIdentity inserts (or updates) the install_identity singleton row
// so GetInstallIdentity and GetCapabilities always succeed in tests.
func seedInstallIdentity(t *testing.T, pool *pgxpool.Pool, capabilities string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO install_identity (id, display_name, timezone, units, capabilities)
		VALUES (1, 'Test Install', 'UTC', 'metric', $1)
		ON CONFLICT (id) DO UPDATE SET capabilities = $1
	`, capabilities)
	require.NoError(t, err, "seed install_identity")
}

// newTestDashboardDeps creates a Deps for handler tests using a real pool.
func newTestDashboardDeps(pool *pgxpool.Pool) Deps {
	return Deps{
		Pool:   pool,
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}
}

// TestSnapshotHandler_WaterOnly verifies that when capabilities='water', the
// snapshot response only contains a "water" key in kpis (D-09: no electricity
// key present, not even as null).
func TestSnapshotHandler_WaterOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	require.NoError(t, db.RunMigrations(context.Background(), pool, log))
	seedInstallIdentity(t, pool, "water")

	deps := newTestDashboardDeps(pool)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/snapshot", nil)
	w := httptest.NewRecorder()
	deps.handleSnapshot(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var snap Snapshot
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &snap))

	assert.Equal(t, "water", snap.Capabilities)
	assert.Contains(t, snap.KPIs, "water", "kpis must have water key")
	assert.NotContains(t, snap.KPIs, "electricity", "kpis must NOT have electricity key when capabilities=water")
	assert.NotNil(t, snap.LatestReadings, "latest_readings must not be nil (empty ok)")
	assert.NotZero(t, snap.GeneratedAt)
}

// TestSnapshotHandler_ElectricityOnly verifies D-09: only electricity KPI key.
func TestSnapshotHandler_ElectricityOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	require.NoError(t, db.RunMigrations(context.Background(), pool, log))
	seedInstallIdentity(t, pool, "electricity")

	deps := newTestDashboardDeps(pool)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/snapshot", nil)
	w := httptest.NewRecorder()
	deps.handleSnapshot(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var snap Snapshot
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &snap))

	assert.Equal(t, "electricity", snap.Capabilities)
	assert.Contains(t, snap.KPIs, "electricity", "kpis must have electricity key")
	assert.NotContains(t, snap.KPIs, "water", "kpis must NOT have water key when capabilities=electricity")
}

// TestSnapshotHandler_Both verifies D-09: both utility KPI keys present.
func TestSnapshotHandler_Both(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	require.NoError(t, db.RunMigrations(context.Background(), pool, log))
	seedInstallIdentity(t, pool, "both")

	deps := newTestDashboardDeps(pool)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/snapshot", nil)
	w := httptest.NewRecorder()
	deps.handleSnapshot(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var snap Snapshot
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &snap))

	assert.Equal(t, "both", snap.Capabilities)
	assert.Contains(t, snap.KPIs, "water")
	assert.Contains(t, snap.KPIs, "electricity")
}

// TestSnapshotHandler_EmptyInstall verifies that an empty measurement table
// returns zero values cleanly (no crashes, no nil panics).
func TestSnapshotHandler_EmptyInstall(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	require.NoError(t, db.RunMigrations(context.Background(), pool, log))
	seedInstallIdentity(t, pool, "both")

	deps := newTestDashboardDeps(pool)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/snapshot", nil)
	w := httptest.NewRecorder()
	deps.handleSnapshot(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var snap Snapshot
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &snap))

	// Empty install: all KPI values should be zero; period delta should be nil.
	waterKPI := snap.KPIs["water"]
	assert.Equal(t, float64(0), waterKPI.TodayConsumption)
	assert.Equal(t, float64(0), waterKPI.InstantTotal)
	assert.Nil(t, waterKPI.PeriodDeltaAbs, "period_delta_abs must be nil when no data")
	assert.Nil(t, waterKPI.PeriodDeltaPct, "period_delta_pct must be nil when no data")
	assert.Equal(t, int64(0), waterKPI.OnlineCount)
	assert.Equal(t, "m³", waterKPI.TodayUnit)
	assert.Equal(t, "L/min", waterKPI.InstantUnit)
}

// TestSnapshotHandler_UnitLabels verifies the unit strings match the Plan 07
// contract exactly: water → m³/L/min, electricity → kWh/W.
func TestSnapshotHandler_UnitLabels(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	require.NoError(t, db.RunMigrations(context.Background(), pool, log))
	seedInstallIdentity(t, pool, "both")

	deps := newTestDashboardDeps(pool)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/snapshot", nil)
	w := httptest.NewRecorder()
	deps.handleSnapshot(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var snap Snapshot
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &snap))

	assert.Equal(t, "m³", snap.KPIs["water"].TodayUnit)
	assert.Equal(t, "L/min", snap.KPIs["water"].InstantUnit)
	assert.Equal(t, "kWh", snap.KPIs["electricity"].TodayUnit)
	assert.Equal(t, "W", snap.KPIs["electricity"].InstantUnit)
}
