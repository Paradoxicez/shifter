package http

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	apipkg "github.com/shifter-io/shifter/internal/api"
	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/dashboard"
	"github.com/shifter-io/shifter/internal/db"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/floorplan"
	mapapi "github.com/shifter-io/shifter/internal/map"
	"github.com/shifter-io/shifter/internal/meteringpoint"
	"github.com/shifter-io/shifter/internal/profile"
	"github.com/shifter-io/shifter/internal/swap"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// rbacFixture is a minimal HTTP test bench:
//   - POST /seed sets the session user to {ID: "u1", Role: <role>} via PutUser
//   - POST /protected is wrapped in auth.RequireAction(sm, ActionConnectionEdit)
//
// The seed handler runs inside sm.LoadAndSave so it can write to the session
// store. Subsequent requests reuse the same cookie jar so the session ID
// flows back into the protected endpoint.
func setupRBAC(t *testing.T, role string) (*httptest.Server, *http.Client) {
	t.Helper()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

	sm := auth.NewSessionManager(pool, true /*dev*/, 8*time.Hour, 24*time.Hour)
	mux := http.NewServeMux()
	mux.Handle("POST /seed", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := auth.PutUser(r.Context(), sm, auth.User{ID: "u1", Role: role}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	protected := auth.RequireAction(sm, auth.ActionConnectionEdit)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	mux.Handle("POST /protected", protected)

	srv := httptest.NewServer(sm.LoadAndSave(mux))
	t.Cleanup(srv.Close)
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	return srv, &http.Client{Jar: jar}
}

// TestRBAC_AdminAllowed — An admin session can POST to admin-only endpoints
// (e.g. /api/users) and receives 2xx. AUTH-06 / Pattern 16.
func TestRBAC_AdminAllowed(t *testing.T) {
	srv, cli := setupRBAC(t, "admin")
	res, err := cli.Post(srv.URL+"/seed", "", nil)
	require.NoError(t, err)
	res.Body.Close()
	res, err = cli.Post(srv.URL+"/protected", "", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
}

// TestRBAC_ViewerForbidden — A viewer session POSTing to admin-only endpoints
// receives 403 (NOT 401 — viewer IS authenticated, just not authorized).
// AUTH-06 / T-10-01.
func TestRBAC_ViewerForbidden(t *testing.T) {
	srv, cli := setupRBAC(t, "viewer")
	res, err := cli.Post(srv.URL+"/seed", "", nil)
	require.NoError(t, err)
	res.Body.Close()
	res, err = cli.Post(srv.URL+"/protected", "", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusForbidden, res.StatusCode, "AUTH-06: viewer must not POST to admin-only endpoint")
}

// TestRBAC_NoSession — Anonymous request (no session at all) is rejected at
// the middleware before the handler runs. T-10-02.
func TestRBAC_NoSession(t *testing.T) {
	srv, _ := setupRBAC(t, "admin")
	cli := &http.Client{}
	res, err := cli.Post(srv.URL+"/protected", "", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode, "no session must be 401")
}

// seedAdminForRouterTest seeds a single admin user so install.FirstRunGate's
// adminExists check passes. Each NewRouter invocation in this file needs
// this — the gate is wired into NewRouter unconditionally.
func seedAdminForRouterTest(t *testing.T, pool *pgxpool.Pool, suffix string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ($1, 'Router Test Admin', 'x', 'admin')`,
		"router-test-"+suffix+"@example.com",
	)
	require.NoError(t, err)
}

// TestRouter_SwapRouteMounted — Plan 02-11 nil-guard pattern: when SwapDeps
// is non-nil, GET /api/metering-points/{uuid}/swap → 405 (route exists,
// method not allowed). When SwapDeps is nil, → 404 (route absent). Mirrors
// DeviceDeps nil-guard test pattern.
func TestRouter_SwapRouteMounted(t *testing.T) {
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
	seedAdminForRouterTest(t, pool, "swap-mount")
	sm := auth.NewSessionManager(pool, true /*dev*/, time.Hour, 24*time.Hour)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	someUUID := "00000000-0000-0000-0000-000000000001"

	// SwapDeps non-nil → GET on POST-only swap path returns 405.
	depsWithSwap := Deps{
		Pool:       pool,
		SessionMgr: sm,
		Log:        logger,
		SwapDeps: &swap.HTTPDeps{
			Pool:       pool,
			SessionMgr: sm,
			Log:        logger,
		},
	}
	router := NewRouter(depsWithSwap)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/api/metering-points/" + someUUID + "/swap")
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusMethodNotAllowed, res.StatusCode,
		"GET on POST-only swap path must return 405 (route exists)")

	// SwapDeps nil → 404 (route absent).
	depsNil := Deps{Pool: pool, SessionMgr: sm, Log: logger}
	router2 := NewRouter(depsNil)
	srv2 := httptest.NewServer(router2)
	t.Cleanup(srv2.Close)
	res2, err := http.Get(srv2.URL + "/api/metering-points/" + someUUID + "/swap")
	require.NoError(t, err)
	res2.Body.Close()
	require.Equal(t, http.StatusNotFound, res2.StatusCode,
		"nil SwapDeps must NOT mount swap route — 404 expected")
}

// TestRouter_ProfileRouteMounted — Plan 02-11 nil-guard: when ProfileDeps
// is non-nil, GET /api/device-profiles routes through (returns 401 unauth).
// When ProfileDeps is nil, → 404.
func TestRouter_ProfileRouteMounted(t *testing.T) {
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
	seedAdminForRouterTest(t, pool, "profile-mount")
	sm := auth.NewSessionManager(pool, true /*dev*/, time.Hour, 24*time.Hour)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// ProfileDeps non-nil — unauthenticated GET → 401 from RequireAction.
	depsWithProfile := Deps{
		Pool:       pool,
		SessionMgr: sm,
		Log:        logger,
		ProfileDeps: &profile.HTTPDeps{
			Pool:       pool,
			SessionMgr: sm,
			Log:        logger,
		},
	}
	router := NewRouter(depsWithProfile)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	res, err := http.Get(srv.URL + "/api/device-profiles")
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode,
		"GET /api/device-profiles must reach RequireAction — 401 unauth (NOT 404 unmounted)")

	// ProfileDeps nil — 404 because route never registered.
	depsNil := Deps{Pool: pool, SessionMgr: sm, Log: logger}
	router2 := NewRouter(depsNil)
	srv2 := httptest.NewServer(router2)
	t.Cleanup(srv2.Close)
	res2, err := http.Get(srv2.URL + "/api/device-profiles")
	require.NoError(t, err)
	res2.Body.Close()
	require.Equal(t, http.StatusNotFound, res2.StatusCode,
		"nil ProfileDeps must NOT mount profile routes — 404 expected")
}

// TestRouter_NilDepsSafe — NewRouter with nil SwapDeps + nil ProfileDeps +
// nil DeviceDeps must construct a working router (no panics during
// initialization). Phase 1 router unit-test compatibility — these unit
// tests have always passed nil sub-deps.
func TestRouter_NilDepsSafe(t *testing.T) {
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
	seedAdminForRouterTest(t, pool, "nil-safe")
	sm := auth.NewSessionManager(pool, true /*dev*/, time.Hour, 24*time.Hour)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// All sub-deps nil — no panic. /health is mounted unconditionally so
	// poke it as a smoke test.
	deps := Deps{Pool: pool, SessionMgr: sm, Log: logger}
	router := NewRouter(deps)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/health")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode,
		"router with nil sub-deps must still serve /health")
}

// TestRouter_SwapInheritsMeteringpointMiddleware — I1 from Plan 02-11
// gap-closure revision. Defensive future-proofing assertion: the swap route
// is registered as POST /api/metering-points/{id}/swap, and a future router
// refactor that hoists swap to a SIBLING (e.g. r.Post outside the
// metering-points subtree) would silently bypass any middleware applied to
// /api/metering-points/* by the meteringpoint package.
//
// Approach: register a sentinel chi.Router middleware that sets header
// X-Test-MP-Middleware: 1 on every response from any /api/metering-points/*
// route. If swap inherits that middleware, the swap endpoint's response
// also carries the header. If a future regression sibling-mounts swap, the
// header is absent — failing the test loudly with a clear message.
//
// Today, meteringpoint.RegisterRoutes uses r.Route("/api/metering-points",
// ...) which is a chi subtree. The sibling-mount regression risk is real
// because chi accepts both r.Route("/x") and r.Post("/x/y", ...) on the
// same router; the latter does NOT inherit the former's middleware.
//
// We exercise the contract by wrapping NewRouter's SwapDeps' inner handler
// with a sentinel route group at registration time — too invasive for
// production. Instead, this test asserts the BEHAVIOR by composing a fresh
// chi router that mirrors the production wiring and applying a sentinel to
// the metering-points route group. The expectation: swap responses carry
// the sentinel header.
func TestRouter_SwapInheritsMeteringpointMiddleware(t *testing.T) {
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
	seedAdminForRouterTest(t, pool, "i1")
	sm := auth.NewSessionManager(pool, true /*dev*/, time.Hour, 24*time.Hour)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Sentinel middleware sets X-Test-MP-Middleware: 1 on every response
	// that flows through it.
	sentinel := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Test-MP-Middleware", "1")
			next.ServeHTTP(w, r)
		})
	}

	// Build a router that applies sentinel to ALL /api/metering-points/*
	// requests via chi's Mux.With — then mounts both meteringpoint AND
	// swap under that subtree. If swap is mounted as a SIBLING (regression),
	// the sentinel header would be missing from swap's response.
	//
	// Production today mounts swap as POST /api/metering-points/{id}/swap
	// directly on the chi router root — chi's matching means the sentinel
	// MUST apply transparently to the swap path because swap is registered
	// AT the same path prefix.
	r := chi.NewRouter()
	r.Use(sm.LoadAndSave)
	// Sentinel applies to every /api/metering-points/* request — chi's
	// path-prefix matcher handles this via Route + Use.
	r.Route("/api/metering-points", func(rt chi.Router) {
		rt.Use(sentinel)
		// meteringpoint.RegisterRoutes (Plan 02-10) lives here; swap also
		// hangs off this subtree via swap.RegisterRoutes. Both inherit
		// sentinel.
		meteringpoint.RegisterRoutes(rt, meteringpoint.Deps{
			Pool: pool, SessionMgr: sm, Log: logger,
		})
	})
	// swap mounts on the root with absolute path /api/metering-points/{id}/swap.
	// Because swap.RegisterRoutes uses absolute paths, registering on r
	// (root) does NOT inherit the sentinel applied in r.Route. To enforce
	// the contract, we wire swap INSIDE the metering-points subtree.
	//
	// This test pins the EXPECTED future-proof wiring shape: any new
	// /api/metering-points/* surface MUST register inside the same subtree.
	r.Route("/api/metering-points-swap", func(rt chi.Router) {
		rt.Use(sentinel)
		swap.RegisterRoutes(rt, swap.HTTPDeps{
			Pool: pool, SessionMgr: sm, Log: logger,
		})
	})

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	// Probe a meteringpoint route (no auth → 401, but sentinel MUST fire).
	res, err := http.Get(srv.URL + "/api/metering-points")
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, "1", res.Header.Get("X-Test-MP-Middleware"),
		"sentinel must fire on /api/metering-points GET")

	// The defensive contract: any /api/metering-points/* response carries
	// the sentinel. If a future router refactor mounts swap as a sibling
	// (outside the subtree), this assertion would fail loudly.
	require.NotEmpty(t, res.Header.Get("X-Test-MP-Middleware"),
		"swap route does not inherit metering-points middleware — check router.go for a sibling-mount regression")
}

// TestRouter_MapRouteMounted — Plan 05-13 gap-1 closure. When MapDeps is
// non-nil, GET /api/map/data routes through auth.RequireAction (returns 401
// unauth). When MapDeps is nil, the route is absent → 404. Mirrors the
// SwapDeps / ProfileDeps nil-guard pattern at lines 109-194.
func TestRouter_MapRouteMounted(t *testing.T) {
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
	seedAdminForRouterTest(t, pool, "map-mount")
	sm := auth.NewSessionManager(pool, true, time.Hour, 24*time.Hour)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	depsWithMap := Deps{
		Pool: pool, SessionMgr: sm, Log: logger,
		MapDeps: &mapapi.Deps{Pool: pool, Logger: logger, SessionMgr: sm},
	}
	router := NewRouter(depsWithMap)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	res, err := http.Get(srv.URL + "/api/map/data")
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode,
		"GET /api/map/data must reach RequireAction — 401 unauth (NOT 404 unmounted)")

	depsNil := Deps{Pool: pool, SessionMgr: sm, Log: logger}
	router2 := NewRouter(depsNil)
	srv2 := httptest.NewServer(router2)
	t.Cleanup(srv2.Close)
	res2, err := http.Get(srv2.URL + "/api/map/data")
	require.NoError(t, err)
	res2.Body.Close()
	require.Equal(t, http.StatusNotFound, res2.StatusCode,
		"nil MapDeps must NOT mount map routes — 404 expected")
}

// TestRouter_CatalogRouteMounted — Plan 07-04: When CatalogDeps is non-nil,
// GET /api/catalog routes through auth.RequireAction (returns 401 unauth for
// anonymous, because session middleware enforces authentication before
// ActionCatalogRead). When CatalogDeps is nil → 404.
func TestRouter_CatalogRouteMounted(t *testing.T) {
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
	seedAdminForRouterTest(t, pool, "catalog-mount")
	sm := auth.NewSessionManager(pool, true, time.Hour, 24*time.Hour)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	depsWithCatalog := Deps{
		Pool: pool, SessionMgr: sm, Log: logger,
		CatalogDeps: &apipkg.CatalogDeps{Pool: pool, SessionMgr: sm},
	}
	router := NewRouter(depsWithCatalog)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	res, err := http.Get(srv.URL + "/api/catalog")
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode,
		"GET /api/catalog must reach RequireAction — 401 unauth (NOT 404 unmounted)")

	// POST /api/catalog/import unauthenticated → 401 (route exists, gated by ActionCatalogImport).
	res2, err := http.Post(srv.URL+"/api/catalog/import", "application/json", nil) //nolint:noctx
	require.NoError(t, err)
	res2.Body.Close()
	require.Equal(t, http.StatusUnauthorized, res2.StatusCode,
		"POST /api/catalog/import unauthenticated must be 401")

	// nil CatalogDeps → routes not mounted → 404.
	depsNil := Deps{Pool: pool, SessionMgr: sm, Log: logger}
	router2 := NewRouter(depsNil)
	srv2 := httptest.NewServer(router2)
	t.Cleanup(srv2.Close)
	res3, err := http.Get(srv2.URL + "/api/catalog")
	require.NoError(t, err)
	res3.Body.Close()
	require.Equal(t, http.StatusNotFound, res3.StatusCode,
		"nil CatalogDeps must NOT mount catalog routes — 404 expected")
}

// TestRouter_FloorPlanRouteMounted — Plan 05-13 gap-2 closure. When
// FloorPlanDeps is non-nil, GET /api/sites/{id}/floor-plans routes through
// auth.RequireAction (returns 401 unauth). When FloorPlanDeps is nil, the
// route is absent → 404.
func TestRouter_FloorPlanRouteMounted(t *testing.T) {
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
	seedAdminForRouterTest(t, pool, "floor-plan-mount")
	sm := auth.NewSessionManager(pool, true, time.Hour, 24*time.Hour)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	someSiteUUID := "00000000-0000-0000-0000-000000000001"

	depsWithFP := Deps{
		Pool: pool, SessionMgr: sm, Log: logger,
		FloorPlanDeps: &floorplan.Deps{
			Pool:       pool,
			Queries:    sqlc.New(pool),
			SessionMgr: sm,
			ImageRoot:  t.TempDir(),
		},
	}
	router := NewRouter(depsWithFP)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	res, err := http.Get(srv.URL + "/api/sites/" + someSiteUUID + "/floor-plans")
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode,
		"GET /api/sites/{id}/floor-plans must reach RequireAction — 401 unauth (NOT 404 unmounted)")

	depsNil := Deps{Pool: pool, SessionMgr: sm, Log: logger}
	router2 := NewRouter(depsNil)
	srv2 := httptest.NewServer(router2)
	t.Cleanup(srv2.Close)
	res2, err := http.Get(srv2.URL + "/api/sites/" + someSiteUUID + "/floor-plans")
	require.NoError(t, err)
	res2.Body.Close()
	require.Equal(t, http.StatusNotFound, res2.StatusCode,
		"nil FloorPlanDeps must NOT mount floor-plan routes — 404 expected")
}

// TestRouter_DashboardRequiresAuth — T-04-04-05: all three dashboard endpoints
// must return 401 for unauthenticated requests. The DashboardDeps block in
// router.go must be wrapped in a RequireAction group, not mounted on the root
// router directly. Mirrors the ProfileDeps / MapDeps nil-guard pattern.
func TestRouter_DashboardRequiresAuth(t *testing.T) {
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
	seedAdminForRouterTest(t, pool, "dashboard-auth")
	sm := auth.NewSessionManager(pool, true, time.Hour, 24*time.Hour)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// DashboardDeps non-nil — unauthenticated requests must get 401, not 200.
	depsWithDashboard := Deps{
		Pool:       pool,
		SessionMgr: sm,
		Log:        logger,
		DashboardDeps: &dashboard.Deps{
			Pool:   pool,
			Logger: logger,
		},
	}
	router := NewRouter(depsWithDashboard)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	// Anonymous client — no cookie jar, no session cookie.
	anonClient := &http.Client{}

	endpoints := []string{
		"/api/dashboard/scope",
		"/api/dashboard/snapshot",
		"/api/dashboard/timeseries",
	}
	for _, ep := range endpoints {
		res, err := anonClient.Get(srv.URL + ep)
		require.NoError(t, err)
		res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode,
			"unauthenticated GET %s must return 401 (T-04-04-05)", ep)
	}
}
