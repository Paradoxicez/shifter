// Package gatewaytest exposes a cross-package test fixture for the
// internal/gateway HTTP handlers.
//
// Why a dedicated package: Go does not let _test.go files (e.g. the
// canonical integration tests in internal/gateway/handlers_test.go) be
// imported from other packages. internal/api/gateways_*_test.go needs the
// same fixture, so the testcontainer + httptest scaffolding lives here as
// non-test code with `t *testing.T` entry points (so it's still only
// callable from tests).
//
// In production binaries this package is never imported.
package gatewaytest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
	"log/slog"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/chirpstack"
	"github.com/shifter-io/shifter/internal/db"
	gw "github.com/shifter-io/shifter/internal/gateway"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// Fixture is the shared bench: testcontainer Postgres + httptest server +
// fakeCS + fakeMetricsCache + fakeInstallState. Methods take a *testing.T
// so callers cannot accidentally use them outside tests.
type Fixture struct {
	pool     *pgxpool.Pool
	server   *httptest.Server
	client   *http.Client
	cs       *FakeCS
	cache    *FakeMetricsCache
	state    *FakeInstallState
	boot     *FakeBootstrapper
	adminID  string
	viewerID string
}

// FakeCS records every CS Gateway call so tests can assert ordering and
// inject errors.
type FakeCS struct {
	mu sync.Mutex

	CreateCalls atomic.Int64
	UpdateCalls atomic.Int64
	DeleteCalls atomic.Int64
	GetCalls    atomic.Int64

	CreateErrs []error
	UpdateErrs []error
	DeleteErrs []error
	GetErrs    []error

	gateways map[string]*api.Gateway
}

func newFakeCS() *FakeCS {
	return &FakeCS{gateways: map[string]*api.Gateway{}}
}

func (f *FakeCS) CreateGateway(_ context.Context, in chirpstack.CreateGatewayInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.CreateCalls.Add(1)
	if len(f.CreateErrs) > 0 {
		err := f.CreateErrs[0]
		f.CreateErrs = f.CreateErrs[1:]
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

func (f *FakeCS) CreateGatewayFromProto(_ context.Context, gp *api.Gateway) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.CreateCalls.Add(1)
	if len(f.CreateErrs) > 0 {
		err := f.CreateErrs[0]
		f.CreateErrs = f.CreateErrs[1:]
		if err != nil {
			return err
		}
	}
	f.gateways[gp.GatewayId] = gp
	return nil
}

func (f *FakeCS) GetGatewayProto(_ context.Context, gatewayID string) (*api.Gateway, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.GetCalls.Add(1)
	if len(f.GetErrs) > 0 {
		err := f.GetErrs[0]
		f.GetErrs = f.GetErrs[1:]
		if err != nil {
			return nil, err
		}
	}
	g, ok := f.gateways[gatewayID]
	if !ok {
		return nil, chirpstack.ErrNotFound
	}
	return g, nil
}

func (f *FakeCS) UpdateGateway(_ context.Context, in chirpstack.CreateGatewayInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.UpdateCalls.Add(1)
	if len(f.UpdateErrs) > 0 {
		err := f.UpdateErrs[0]
		f.UpdateErrs = f.UpdateErrs[1:]
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

func (f *FakeCS) DeleteGateway(_ context.Context, gatewayID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.DeleteCalls.Add(1)
	if len(f.DeleteErrs) > 0 {
		err := f.DeleteErrs[0]
		f.DeleteErrs = f.DeleteErrs[1:]
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

// HasGateway reports whether the fake CS holds a gateway with the given id.
func (f *FakeCS) HasGateway(gatewayID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.gateways[gatewayID]
	return ok
}

// FakeBootstrapper satisfies gw.CSBootstrapper.
type FakeBootstrapper struct {
	TenantID string
	AppID    string
}

func (b *FakeBootstrapper) EnsureTenantAndApplication(_ context.Context) (string, string, error) {
	return b.TenantID, b.AppID, nil
}

// FakeMetricsCache satisfies gw.MetricsCacheAccessor.
type FakeMetricsCache struct {
	mu sync.Mutex

	GetCalls        atomic.Int64
	InvalidateCalls atomic.Int64

	cached map[string]*api.GetGatewayMetricsResponse
}

func newFakeMetricsCache() *FakeMetricsCache {
	return &FakeMetricsCache{cached: map[string]*api.GetGatewayMetricsResponse{}}
}

func (m *FakeMetricsCache) Get(_ context.Context, gatewayID string) (*api.GetGatewayMetricsResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GetCalls.Add(1)
	if r, ok := m.cached[gatewayID]; ok {
		return r, nil
	}
	r := &api.GetGatewayMetricsResponse{}
	m.cached[gatewayID] = r
	return r, nil
}

func (m *FakeMetricsCache) Invalidate(gatewayID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.InvalidateCalls.Add(1)
	delete(m.cached, gatewayID)
}

// FakeInstallState always returns the configured region.
type FakeInstallState struct{ Region string }

func (f *FakeInstallState) GetLoRaWANRegionDefault(_ context.Context) (string, error) {
	return f.Region, nil
}

func nopLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// New builds a fresh fixture.
func New(t *testing.T) *Fixture {
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
		 VALUES ('admin-gwt@example.com', 'Admin GWT', 'x', 'admin') RETURNING id::text`,
	).Scan(&adminID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('viewer-gwt@example.com', 'Viewer GWT', 'x', 'viewer') RETURNING id::text`,
	).Scan(&viewerID))

	cs := newFakeCS()
	cache := newFakeMetricsCache()
	state := &FakeInstallState{Region: "as923_2"}
	boot := &FakeBootstrapper{TenantID: uuid.NewString(), AppID: uuid.NewString()}

	sm := auth.NewSessionManager(pool, true, 8*time.Hour, 24*time.Hour)
	deps := gw.Deps{
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
	gw.RegisterRoutes(r, deps)

	srv := httptest.NewServer(sm.LoadAndSave(r))
	t.Cleanup(srv.Close)

	jar, _ := cookiejar.New(nil)
	cli := &http.Client{Jar: jar}

	return &Fixture{
		pool: pool, server: srv, client: cli,
		cs: cs, cache: cache, state: state, boot: boot,
		adminID: adminID, viewerID: viewerID,
	}
}

// Pool returns the underlying pgxpool.
func (f *Fixture) Pool() *pgxpool.Pool { return f.pool }

// ServerURL returns the http test server URL prefix.
func (f *Fixture) ServerURL() string { return f.server.URL }

// CS returns the fake ChirpStack client for assertions and failure injection.
func (f *Fixture) CS() *FakeCS { return f.cs }

// Cache returns the fake metrics cache.
func (f *Fixture) Cache() *FakeMetricsCache { return f.cache }

// AdminID / ViewerID expose the seeded user UUIDs.
func (f *Fixture) AdminID() string  { return f.adminID }
func (f *Fixture) ViewerID() string { return f.viewerID }

// SeedAdmin logs the client in as the seeded admin user.
func (f *Fixture) SeedAdmin(t *testing.T) { f.seed(t, "admin") }

// SeedViewer logs the client in as the seeded viewer user.
func (f *Fixture) SeedViewer(t *testing.T) { f.seed(t, "viewer") }

func (f *Fixture) seed(t *testing.T, role string) {
	t.Helper()
	res, err := f.client.Post(f.server.URL+"/test/seed/"+role, "", nil)
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusNoContent, res.StatusCode)
}

// DoJSON issues a JSON request against the test server and returns the
// response. Caller is responsible for closing the body.
func (f *Fixture) DoJSON(t *testing.T, method, path string, body any) *http.Response {
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

// ValidGatewayID returns a deterministic 16-hex EUI64 ending in suffix.
func ValidGatewayID(suffix string) string {
	base := "aabbccddeeff00"
	if len(suffix) > 2 {
		suffix = suffix[:2]
	}
	for len(suffix) < 2 {
		suffix = "0" + suffix
	}
	return base + suffix
}
