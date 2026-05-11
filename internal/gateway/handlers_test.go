package gateway

// Phase 3 Plan 03-04 — gateway handler integration tests.
//
// Bench: testcontainer Postgres + httptest server + fakeCSGateway +
// fakeBootstrap. Mirrors the device handler test layout (Plan 02-10).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	api "github.com/chirpstack/chirpstack/api/go/v4/api"
	common "github.com/chirpstack/chirpstack/api/go/v4/common"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/chirpstack"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// fakeCSGateway records every CS Gateway call so tests can assert ordering
// and inject errors. Mirrors fakeCSDevice from internal/device/handlers_test.go.
type fakeCSGateway struct {
	mu sync.Mutex

	createCalls atomic.Int64
	updateCalls atomic.Int64
	deleteCalls atomic.Int64
	getCalls    atomic.Int64

	// Failure injection queues — FIFO per method. nil entries are pass-through.
	createErrs []error
	updateErrs []error
	deleteErrs []error
	getErrs    []error

	// In-memory CS Gateway proto store, keyed by gateway_id. Tests inspect
	// this map to verify "gateway absent after Delete" expectations.
	gateways map[string]*api.Gateway
}

func newFakeCSGateway() *fakeCSGateway {
	return &fakeCSGateway{gateways: map[string]*api.Gateway{}}
}

func (f *fakeCSGateway) CreateGateway(_ context.Context, in chirpstack.CreateGatewayInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls.Add(1)
	if len(f.createErrs) > 0 {
		err := f.createErrs[0]
		f.createErrs = f.createErrs[1:]
		if err != nil {
			return err
		}
	}
	f.gateways[in.GatewayID] = &api.Gateway{
		GatewayId:   in.GatewayID,
		Name:        in.Name,
		Description: in.Description,
		TenantId:    in.TenantID,
		Tags:        in.Tags,
		Location: &common.Location{
			Latitude: in.Lat, Longitude: in.Lng, Altitude: in.Altitude,
			Source: common.LocationSource_CONFIG,
		},
	}
	return nil
}

func (f *fakeCSGateway) CreateGatewayFromProto(_ context.Context, gw *api.Gateway) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls.Add(1)
	if len(f.createErrs) > 0 {
		err := f.createErrs[0]
		f.createErrs = f.createErrs[1:]
		if err != nil {
			return err
		}
	}
	f.gateways[gw.GatewayId] = gw
	return nil
}

func (f *fakeCSGateway) GetGatewayProto(_ context.Context, gatewayID string) (*api.Gateway, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getCalls.Add(1)
	if len(f.getErrs) > 0 {
		err := f.getErrs[0]
		f.getErrs = f.getErrs[1:]
		if err != nil {
			return nil, err
		}
	}
	gw, ok := f.gateways[gatewayID]
	if !ok {
		return nil, chirpstack.ErrNotFound
	}
	return gw, nil
}

func (f *fakeCSGateway) UpdateGateway(_ context.Context, in chirpstack.CreateGatewayInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updateCalls.Add(1)
	if len(f.updateErrs) > 0 {
		err := f.updateErrs[0]
		f.updateErrs = f.updateErrs[1:]
		if err != nil {
			return err
		}
	}
	if _, ok := f.gateways[in.GatewayID]; !ok {
		return chirpstack.ErrNotFound
	}
	f.gateways[in.GatewayID].Name = in.Name
	f.gateways[in.GatewayID].Description = in.Description
	return nil
}

func (f *fakeCSGateway) DeleteGateway(_ context.Context, gatewayID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteCalls.Add(1)
	if len(f.deleteErrs) > 0 {
		err := f.deleteErrs[0]
		f.deleteErrs = f.deleteErrs[1:]
		if err != nil {
			return err
		}
	}
	if _, ok := f.gateways[gatewayID]; !ok {
		return chirpstack.ErrNotFound
	}
	delete(f.gateways, gatewayID)
	return nil
}

func (f *fakeCSGateway) hasGateway(gatewayID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.gateways[gatewayID]
	return ok
}

// fakeBootstrapper satisfies CSBootstrapper.
type fakeBootstrapper struct {
	tenantID string
	appID    string
}

func (b *fakeBootstrapper) EnsureTenantAndApplication(_ context.Context) (string, string, error) {
	return b.tenantID, b.appID, nil
}

// fakeMetricsCache satisfies MetricsCacheAccessor and counts Get/Invalidate
// invocations so the list-page caching tests can assert "≤ N CS calls".
type fakeMetricsCache struct {
	mu sync.Mutex

	getCalls        atomic.Int64
	invalidateCalls atomic.Int64

	// Per-gateway response stored after first Get; subsequent Get returns the
	// same pointer (singleflight semantics — the production cache also
	// returns the same pointer for cached calls).
	cached map[string]*api.GetGatewayMetricsResponse
}

func newFakeMetricsCache() *fakeMetricsCache {
	return &fakeMetricsCache{cached: map[string]*api.GetGatewayMetricsResponse{}}
}

func (m *fakeMetricsCache) Get(_ context.Context, gatewayID string) (*api.GetGatewayMetricsResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getCalls.Add(1)
	if r, ok := m.cached[gatewayID]; ok {
		return r, nil
	}
	r := &api.GetGatewayMetricsResponse{}
	m.cached[gatewayID] = r
	return r, nil
}

func (m *fakeMetricsCache) Invalidate(gatewayID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invalidateCalls.Add(1)
	delete(m.cached, gatewayID)
}

// fakeInstallState always returns AS923_2 (Thailand default).
type fakeInstallState struct{ region string }

func (f *fakeInstallState) GetLoRaWANRegionDefault(_ context.Context) (string, error) {
	return f.region, nil
}

// ----- fixture ------------------------------------------------------------

type gatewayFixture struct {
	pool    *pgxpool.Pool
	server  *httptest.Server
	client  *http.Client
	cs      *fakeCSGateway
	cache   *fakeMetricsCache
	state   *fakeInstallState
	boot    *fakeBootstrapper
	deps    Deps
	adminID string
	viewerID string
}

func nopLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newGatewayFixture(t *testing.T) *gatewayFixture {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, nopLogger()))

	var adminID, viewerID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('admin-gw@example.com', 'Admin GW', 'x', 'admin') RETURNING id::text`,
	).Scan(&adminID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('viewer-gw@example.com', 'Viewer GW', 'x', 'viewer') RETURNING id::text`,
	).Scan(&viewerID))

	cs := newFakeCSGateway()
	cache := newFakeMetricsCache()
	state := &fakeInstallState{region: "as923_2"}
	boot := &fakeBootstrapper{tenantID: uuid.NewString(), appID: uuid.NewString()}

	sm := auth.NewSessionManager(pool, true /*dev*/, 8*time.Hour, 24*time.Hour)
	deps := Deps{
		Pool:         pool,
		SessionMgr:   sm,
		Log:          nopLogger(),
		CS:           cs,
		Bootstrap:    boot,
		MetricsCache: cache,
		InstallState: state,
	}

	r := chi.NewRouter()
	r.Post("/test/seed/{role}", func(w http.ResponseWriter, req *http.Request) {
		role := chi.URLParam(req, "role")
		var id string
		switch role {
		case "admin":
			id = adminID
		case "viewer":
			id = viewerID
		default:
			http.Error(w, "bad role", 400)
			return
		}
		if err := auth.PutUser(req.Context(), sm, auth.User{ID: id, Role: role}); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	RegisterRoutes(r, deps)

	srv := httptest.NewServer(sm.LoadAndSave(r))
	t.Cleanup(srv.Close)

	jar, _ := cookiejar.New(nil)
	cli := &http.Client{Jar: jar}

	return &gatewayFixture{
		pool: pool, server: srv, client: cli,
		cs: cs, cache: cache, state: state, boot: boot, deps: deps,
		adminID: adminID, viewerID: viewerID,
	}
}

func (f *gatewayFixture) seedRole(t *testing.T, role string) {
	t.Helper()
	res, err := f.client.Post(f.server.URL+"/test/seed/"+role, "", nil)
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusNoContent, res.StatusCode)
}

func (f *gatewayFixture) doJSON(t *testing.T, method, path string, body any) *http.Response {
	t.Helper()
	var buf io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, f.server.URL+path, buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	res, err := f.client.Do(req)
	require.NoError(t, err)
	return res
}

func validGatewayID(suffix string) string {
	base := "aabbccddeeff00"
	if len(suffix) > 2 {
		suffix = suffix[:2]
	}
	for len(suffix) < 2 {
		suffix = "0" + suffix
	}
	return base + suffix
}

// ----- create + region default --------------------------------------------

// TestCreateGatewayHandler_RegionDefault — D-03: when the request omits
// region, the install-config region (AS923_2 for Thailand) is applied.
func TestCreateGatewayHandler_RegionDefault(t *testing.T) {
	f := newGatewayFixture(t)
	f.seedRole(t, "admin")

	gwID := validGatewayID("01")
	res := f.doJSON(t, "POST", "/api/gateways", map[string]any{
		"gateway_id": gwID,
		"name":       "gw-east",
		// no region — handler must read install_state default
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Equal(t, "as923_2", body["region"])
	require.Equal(t, gwID, body["gateway_id"])

	// Audit row written with action=gateway.create.
	var action, entityType string
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT action, entity_type FROM audit_log WHERE entity_type = 'gateway' ORDER BY time DESC LIMIT 1`,
	).Scan(&action, &entityType))
	require.Equal(t, "gateway.create", action)
}

// TestCreateGatewayHandler_AtomicityCSRollback — CS Create succeeds, PG
// insert hits a duplicate-key error; handler best-effort DeleteGateway on
// CS and returns 409. PG has no row.
func TestCreateGatewayHandler_AtomicityCSRollback(t *testing.T) {
	f := newGatewayFixture(t)
	f.seedRole(t, "admin")

	gwID := validGatewayID("02")
	// Pre-insert a row with the same gateway_id so CreateGateway PG hits
	// the unique constraint.
	_, err := f.pool.Exec(context.Background(),
		`INSERT INTO gateway (gateway_id, name, region) VALUES ($1, 'preexisting', 'eu868')`, gwID,
	)
	require.NoError(t, err)

	res := f.doJSON(t, "POST", "/api/gateways", map[string]any{
		"gateway_id": gwID,
		"name":       "gw-conflict",
		"region":     "as923_2",
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusConflict, res.StatusCode)

	// CS Create was attempted; the handler cleaned up via DeleteGateway.
	require.Equal(t, int64(1), f.cs.createCalls.Load(), "CS Create must be invoked")
	require.Equal(t, int64(1), f.cs.deleteCalls.Load(), "CS DeleteGateway must be invoked on PG rollback")
	require.False(t, f.cs.hasGateway(gwID), "CS gateway must be cleaned up")

	// Only the pre-existing row remains in PG.
	var count int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM gateway WHERE gateway_id = $1`, gwID,
	).Scan(&count))
	require.Equal(t, 1, count)
}

// TestCreateGatewayHandler_IgnoresClientTenantId — T-3-31: handler MUST
// ignore any tenant_id in the request body and use the server-side value
// from EnsureTenantAndApplication.
func TestCreateGatewayHandler_IgnoresClientTenantId(t *testing.T) {
	f := newGatewayFixture(t)
	f.seedRole(t, "admin")

	gwID := validGatewayID("03")
	maliciousTenant := uuid.NewString()
	res := f.doJSON(t, "POST", "/api/gateways", map[string]any{
		"gateway_id": gwID,
		"name":       "gw-tenant-test",
		"region":     "as923_2",
		"tenant_id":  maliciousTenant, // ← MUST be ignored
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)

	// Persisted cs_tenant_id MUST be the server-side tenant.
	var csTenant *string
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT cs_tenant_id FROM gateway WHERE gateway_id = $1`, gwID,
	).Scan(&csTenant))
	require.NotNil(t, csTenant)
	require.Equal(t, f.boot.tenantID, *csTenant)
	require.NotEqual(t, maliciousTenant, *csTenant)
}

// TestCreateGatewayHandler_InvalidGatewayID — non-16-hex gateway_id → 400.
func TestCreateGatewayHandler_InvalidGatewayID(t *testing.T) {
	f := newGatewayFixture(t)
	f.seedRole(t, "admin")

	res := f.doJSON(t, "POST", "/api/gateways", map[string]any{
		"gateway_id": "AABBCCDDEE",
		"name":       "bad",
		"region":     "as923_2",
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
	require.Equal(t, int64(0), f.cs.createCalls.Load(),
		"CS must NOT be called on validation failure")
}

// ----- list ---------------------------------------------------------------

// TestListGatewaysHandler — admin GET; viewer GET; stats appear only when
// stats_refreshed_at is non-NULL.
func TestListGatewaysHandler(t *testing.T) {
	f := newGatewayFixture(t)
	f.seedRole(t, "admin")

	for i := 1; i <= 3; i++ {
		_, err := f.pool.Exec(context.Background(),
			`INSERT INTO gateway (gateway_id, name, region) VALUES ($1, $2, 'as923_2')`,
			validGatewayID(string(rune('0'+i))), "gw-list",
		)
		require.NoError(t, err)
	}

	res := f.doJSON(t, "GET", "/api/gateways", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	var body map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.EqualValues(t, 3, body["total"])
	items := body["items"].([]any)
	require.Len(t, items, 3)

	// Viewer also passes.
	f.seedRole(t, "viewer")
	res2 := f.doJSON(t, "GET", "/api/gateways", nil)
	defer res2.Body.Close()
	require.Equal(t, http.StatusOK, res2.StatusCode)
}

// ----- update -------------------------------------------------------------

// TestUpdateGatewayHandler — PATCH name+region; CS UpdateGateway called;
// audit row diff before/after.
func TestUpdateGatewayHandler(t *testing.T) {
	f := newGatewayFixture(t)
	f.seedRole(t, "admin")

	gwID := validGatewayID("aa")
	res := f.doJSON(t, "POST", "/api/gateways", map[string]any{
		"gateway_id": gwID,
		"name":       "original-name",
		"region":     "as923_2",
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)
	var created map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
	id := created["id"].(string)

	res2 := f.doJSON(t, "PATCH", "/api/gateways/"+id, map[string]any{
		"name":   "renamed",
		"region": "eu868",
	})
	defer res2.Body.Close()
	require.Equal(t, http.StatusOK, res2.StatusCode)

	require.Equal(t, int64(1), f.cs.updateCalls.Load(), "CS Update must be invoked")

	// Audit diff.
	var beforeJSON, afterJSON []byte
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT before, after FROM audit_log WHERE action = 'gateway.update' ORDER BY time DESC LIMIT 1`,
	).Scan(&beforeJSON, &afterJSON))
	var before, after map[string]any
	require.NoError(t, json.Unmarshal(beforeJSON, &before))
	require.NoError(t, json.Unmarshal(afterJSON, &after))
	require.Equal(t, "original-name", before["name"])
	require.Equal(t, "renamed", after["name"])
}

// ----- archive / decommission --------------------------------------------

// TestDecommissionAtomic — happy path: PG archived_at set + CS Delete
// called + audit row + cache invalidated; archived_snapshot populated.
func TestDecommissionAtomic(t *testing.T) {
	f := newGatewayFixture(t)
	f.seedRole(t, "admin")

	gwID := validGatewayID("bb")
	res := f.doJSON(t, "POST", "/api/gateways", map[string]any{
		"gateway_id": gwID,
		"name":       "to-decommission",
		"region":     "as923_2",
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)
	var created map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
	id := created["id"].(string)

	require.True(t, f.cs.hasGateway(gwID), "CS gateway exists pre-archive")

	res2 := f.doJSON(t, "POST", "/api/gateways/"+id+"/archive", map[string]any{
		"reason": "operator decommission",
	})
	defer res2.Body.Close()
	require.Equal(t, http.StatusOK, res2.StatusCode)

	// CS gateway gone.
	require.False(t, f.cs.hasGateway(gwID), "CS DeleteGateway must remove the gateway")
	require.GreaterOrEqual(t, f.cs.deleteCalls.Load(), int64(1))

	// PG archived_at populated, archived_snapshot non-null.
	var archivedAt *time.Time
	var snapshot []byte
	var reason *string
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT archived_at, archived_reason, archived_snapshot FROM gateway WHERE id = $1::uuid`, id,
	).Scan(&archivedAt, &reason, &snapshot))
	require.NotNil(t, archivedAt, "archived_at must be set")
	require.NotNil(t, reason)
	require.Equal(t, "operator decommission", *reason)
	require.NotEmpty(t, snapshot, "archived_snapshot must capture CS proto")
	require.NotEqual(t, "null", string(snapshot))

	// Audit row.
	var action string
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT action FROM audit_log WHERE entity_id = $1::uuid AND action = 'gateway.archive'`, id,
	).Scan(&action))
	require.Equal(t, "gateway.archive", action)

	// Cache invalidated.
	require.GreaterOrEqual(t, f.cache.invalidateCalls.Load(), int64(1))
}

// TestDecommission_CSDeleteFails_RollsBackPG — mock CS DeleteGateway
// returns Unavailable; PG row NOT archived; 502 returned.
func TestDecommission_CSDeleteFails_RollsBackPG(t *testing.T) {
	f := newGatewayFixture(t)
	f.seedRole(t, "admin")

	gwID := validGatewayID("cc")
	res := f.doJSON(t, "POST", "/api/gateways", map[string]any{
		"gateway_id": gwID,
		"name":       "cs-delete-fails",
		"region":     "as923_2",
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)
	var created map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
	id := created["id"].(string)

	// Inject a CS delete failure.
	f.cs.mu.Lock()
	f.cs.deleteErrs = append(f.cs.deleteErrs, errors.New("cs unavailable"))
	f.cs.mu.Unlock()

	res2 := f.doJSON(t, "POST", "/api/gateways/"+id+"/archive", nil)
	defer res2.Body.Close()
	require.Equal(t, http.StatusBadGateway, res2.StatusCode)

	// PG row must NOT be archived.
	var archivedAt *time.Time
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT archived_at FROM gateway WHERE id = $1::uuid`, id,
	).Scan(&archivedAt))
	require.Nil(t, archivedAt, "PG archive must be rolled back when CS DeleteGateway fails")

	// CS gateway still exists (delete was rejected).
	require.True(t, f.cs.hasGateway(gwID), "CS gateway must still exist after delete failure")
}

// ----- restore ------------------------------------------------------------

// TestRestoreGateway — archive then restore; CS CreateGateway re-creates
// from snapshot; PG archive columns cleared; audit row written.
func TestRestoreGateway(t *testing.T) {
	f := newGatewayFixture(t)
	f.seedRole(t, "admin")

	gwID := validGatewayID("dd")
	res := f.doJSON(t, "POST", "/api/gateways", map[string]any{
		"gateway_id": gwID,
		"name":       "to-restore",
		"region":     "as923_2",
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)
	var created map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
	id := created["id"].(string)

	// Archive first.
	res2 := f.doJSON(t, "POST", "/api/gateways/"+id+"/archive", nil)
	defer res2.Body.Close()
	require.Equal(t, http.StatusOK, res2.StatusCode)
	require.False(t, f.cs.hasGateway(gwID))

	createsBefore := f.cs.createCalls.Load()

	// Restore.
	res3 := f.doJSON(t, "POST", "/api/gateways/"+id+"/restore", nil)
	defer res3.Body.Close()
	require.Equal(t, http.StatusOK, res3.StatusCode)

	require.True(t, f.cs.hasGateway(gwID), "CS CreateGatewayFromProto must re-create the gateway")
	require.Equal(t, createsBefore+1, f.cs.createCalls.Load(), "exactly one CS recreate")

	// PG archive cleared.
	var archivedAt *time.Time
	var snapshot []byte
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT archived_at, archived_snapshot FROM gateway WHERE id = $1::uuid`, id,
	).Scan(&archivedAt, &snapshot))
	require.Nil(t, archivedAt)
	require.Empty(t, snapshot)

	// Audit row.
	var action string
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT action FROM audit_log WHERE entity_id = $1::uuid AND action = 'gateway.restore'`, id,
	).Scan(&action))
	require.Equal(t, "gateway.restore", action)
}

// TestRestoreGateway_CSCreateFails — CS CreateGateway rejects; PG remains
// archived; 502 returned.
func TestRestoreGateway_CSCreateFails(t *testing.T) {
	f := newGatewayFixture(t)
	f.seedRole(t, "admin")

	gwID := validGatewayID("ee")
	res := f.doJSON(t, "POST", "/api/gateways", map[string]any{
		"gateway_id": gwID,
		"name":       "restore-failure",
		"region":     "as923_2",
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)
	var created map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
	id := created["id"].(string)

	res2 := f.doJSON(t, "POST", "/api/gateways/"+id+"/archive", nil)
	defer res2.Body.Close()
	require.Equal(t, http.StatusOK, res2.StatusCode)

	// Inject CS create failure for the restore attempt.
	f.cs.mu.Lock()
	f.cs.createErrs = append(f.cs.createErrs, errors.New("cs unavailable"))
	f.cs.mu.Unlock()

	res3 := f.doJSON(t, "POST", "/api/gateways/"+id+"/restore", nil)
	defer res3.Body.Close()
	require.Equal(t, http.StatusBadGateway, res3.StatusCode)

	// PG still archived.
	var archivedAt *time.Time
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT archived_at FROM gateway WHERE id = $1::uuid`, id,
	).Scan(&archivedAt))
	require.NotNil(t, archivedAt, "PG must remain archived when CS recreate fails")
}

// TestArchiveGateway_AlreadyArchived — second archive on same id → 409.
func TestArchiveGateway_AlreadyArchived(t *testing.T) {
	f := newGatewayFixture(t)
	f.seedRole(t, "admin")

	gwID := validGatewayID("ff")
	res := f.doJSON(t, "POST", "/api/gateways", map[string]any{
		"gateway_id": gwID,
		"name":       "double-archive",
		"region":     "as923_2",
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)
	var created map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
	id := created["id"].(string)

	res2 := f.doJSON(t, "POST", "/api/gateways/"+id+"/archive", nil)
	defer res2.Body.Close()
	require.Equal(t, http.StatusOK, res2.StatusCode)

	res3 := f.doJSON(t, "POST", "/api/gateways/"+id+"/archive", nil)
	defer res3.Body.Close()
	require.Equal(t, http.StatusConflict, res3.StatusCode)
}

// TestRestoreGateway_NotArchived — restore on a non-archived row → 409.
func TestRestoreGateway_NotArchived(t *testing.T) {
	f := newGatewayFixture(t)
	f.seedRole(t, "admin")

	gwID := validGatewayID("11")
	res := f.doJSON(t, "POST", "/api/gateways", map[string]any{
		"gateway_id": gwID,
		"name":       "active",
		"region":     "as923_2",
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)
	var created map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
	id := created["id"].(string)

	res2 := f.doJSON(t, "POST", "/api/gateways/"+id+"/restore", nil)
	defer res2.Body.Close()
	require.Equal(t, http.StatusConflict, res2.StatusCode)
}

// TestGatewayMutations_Viewer403 — viewer fails Create/Update/Archive/
// Restore all with 403; viewer SUCCEEDS on GET.
func TestGatewayMutations_Viewer403(t *testing.T) {
	f := newGatewayFixture(t)

	// Admin creates one row so the viewer can read.
	f.seedRole(t, "admin")
	gwID := validGatewayID("22")
	res := f.doJSON(t, "POST", "/api/gateways", map[string]any{
		"gateway_id": gwID,
		"name":       "for-viewer",
		"region":     "as923_2",
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)
	var created map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
	id := created["id"].(string)

	// Switch to viewer.
	f.seedRole(t, "viewer")

	// GET OK.
	resGet := f.doJSON(t, "GET", "/api/gateways", nil)
	defer resGet.Body.Close()
	require.Equal(t, http.StatusOK, resGet.StatusCode)

	// POST → 403.
	resPost := f.doJSON(t, "POST", "/api/gateways", map[string]any{
		"gateway_id": validGatewayID("33"),
		"name":       "viewer-create",
		"region":     "as923_2",
	})
	defer resPost.Body.Close()
	require.Equal(t, http.StatusForbidden, resPost.StatusCode)

	// PATCH → 403.
	resPatch := f.doJSON(t, "PATCH", "/api/gateways/"+id, map[string]any{
		"name":   "viewer-update",
		"region": "as923_2",
	})
	defer resPatch.Body.Close()
	require.Equal(t, http.StatusForbidden, resPatch.StatusCode)

	// Archive → 403.
	resArc := f.doJSON(t, "POST", "/api/gateways/"+id+"/archive", nil)
	defer resArc.Body.Close()
	require.Equal(t, http.StatusForbidden, resArc.StatusCode)

	// Restore → 403.
	resRes := f.doJSON(t, "POST", "/api/gateways/"+id+"/restore", nil)
	defer resRes.Body.Close()
	require.Equal(t, http.StatusForbidden, resRes.StatusCode)
}

// TestRouter_GatewayRoutesMounted — defensive future-proof test: GET on
// /api/gateways with NO session returns 401 (not 404), proving the route
// is mounted.
func TestRouter_GatewayRoutesMounted(t *testing.T) {
	f := newGatewayFixture(t)
	// Use a session-less client so RequireAction returns 401.
	bare := &http.Client{}
	req, err := http.NewRequest("GET", f.server.URL+"/api/gateways", nil)
	require.NoError(t, err)
	res, err := bare.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode,
		"unauthenticated /api/gateways must 401 (route mounted), not 404")
}
