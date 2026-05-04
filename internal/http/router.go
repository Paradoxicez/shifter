// Package http — chi router that mounts every Phase 1 route.
//
// The middleware order is the canonical PITFALL #4 anchor:
//
//  1. middleware.RequestID         — adds request_id to ctx (Plan 22 Caddy passes through)
//  2. middleware.RealIP            — honors X-Forwarded-For from the operator's reverse proxy
//  3. SlogLogger                   — D-24 structured one-event-per-request log
//  4. middleware.Recoverer         — converts panics to 500 + logs internally (T-18-07)
//  5. SessionMgr.LoadAndSave       — alexedwards/scs session boundary (Plan 08)
//  6. install.FirstRunGate         — D-08 install gate (Plan 14); cached via atomic.Bool
//  7. domain routes (auth, account, install, settings, health)
//  8. SPA fallback (`/*`)          — Plan 19; MUST be LAST so /api/* 404s don't return index.html
//
// The fallback ordering is the entire reason this router file is non-trivial:
// chi serves the LONGEST matching pattern, but `/*` matches everything; if it
// is registered before /api/* routes, a missing `/api/foo` would return the
// SPA HTML body with status 200 and the SPA would render a "not found" route
// against an HTML payload that the SPA's fetch helpers don't know how to parse.
// Always-last avoids the entire class.
package http

import (
	"log/slog"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/device"
	"github.com/shifter-io/shifter/internal/install"
	"github.com/shifter-io/shifter/internal/meteringpoint"
	"github.com/shifter-io/shifter/internal/site"
)

// Deps groups every dependency the router needs. Plan 18 (serve.go) constructs
// one Deps value at startup and threads it into NewRouter.
//
// Field grouping mirrors the route groups so a future plan can locate the
// relevant struct slice when wiring a new endpoint family.
type Deps struct {
	// Core infrastructure shared across handler families.
	Pool       *pgxpool.Pool
	SessionMgr *scs.SessionManager
	Log        *slog.Logger

	// Auth wiring (Plans 08, 09).
	LoginLimiter *auth.LoginLimiter
	UserStore    *auth.Store

	// Install wiring (Plans 14, 15).
	InstallStore *install.Store
	InstallDeps  install.Deps

	// Settings + Test Connection wiring (Plan 17).
	TestConnDeps TestConnDeps

	// SecretsDir is the on-disk path under which file-by-REF secrets are
	// stored (mode 0600). Plan 18 sources this from cfg.SecretsDir; the PUT
	// chirpstack handler writes to {SecretsDir}/chirpstack_api_token etc.
	SecretsDir string

	// DeviceDeps wires Plan 02-10's CHIRP-04 atomic Add Device handler. nil
	// when CS is not bootstrapped (Phase 1 router unit tests + early-boot
	// pre-install paths). Plan 02-15 (cmd/serve wiring) constructs the full
	// shape: CS gRPC client + bootstrapper + gRPC ping + MQTT ping.
	DeviceDeps *device.Deps

	// SPA fallback handler (Plan 19). Optional — if nil the router does NOT
	// register the catch-all so /unknown/path returns 404 instead of HTML.
	SPA http.Handler
}

// NewRouter builds the production chi router with the canonical middleware
// stack and every Phase 1 route mounted in order.
//
// The route table (12 routes + SPA fallback):
//
//	GET  /health                              public
//	GET  /health/detailed                     ActionHealthDetailed (admin)
//	POST /api/auth/login                      public (rate-limited)
//	POST /api/auth/logout                     public
//	GET  /api/account/me                      ActionAccountSelfEdit
//	POST /api/account/password                ActionAccountSelfEdit
//	GET  /api/install/state                   public (whitelisted by FirstRunGate)
//	POST /api/install/step/1                  public (whitelisted)
//	POST /api/install/step/2                  public (whitelisted)
//	POST /api/install/step/3                  public (whitelisted)
//	POST /api/install/step/4                  public (whitelisted)
//	POST /api/install/finish                  public (whitelisted)
//	GET  /api/install/regions                 public (whitelisted)
//	GET  /api/settings/chirpstack             ActionConnectionTest (both roles)
//	POST /api/settings/chirpstack/test        ActionConnectionTest (both roles)
//	PUT  /api/settings/chirpstack             ActionConnectionEdit (admin)
//	GET  /*                                   SPA fallback (Plan 19)
//
// SPA fallback is registered LAST (PITFALL #4); it MUST NOT shadow /api/*.
func NewRouter(deps Deps) http.Handler {
	r := chi.NewRouter()

	// 1-4: pre-session middleware. RequestID first so every later log line
	// carries the same identifier. Recoverer last in this group so panics
	// during session load/save (5) are still caught.
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(SlogLogger(deps.Log))
	r.Use(middleware.Recoverer)

	// 5: session boundary — wired exactly once (Plan 08 invariant).
	r.Use(deps.SessionMgr.LoadAndSave)

	// 6: install gate (Plan 14). Whitelist already covers /install,
	// /api/install, /login, /health, /assets/, common static suffixes.
	r.Use(install.FirstRunGate(deps.Pool, deps.Log))

	// Public health (D-18).
	r.Get("/health", Health())

	// Detailed health — admin only (D-19).
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionHealthDetailed))
		rt.Get("/health/detailed", HealthDetailed(deps.Pool))
	})

	// Auth — login is rate-limited inside the handler (Plan 09).
	loginDeps := auth.LoginDeps{
		Store:        deps.UserStore,
		SessionMgr:   deps.SessionMgr,
		LoginLimiter: deps.LoginLimiter,
		Log:          deps.Log,
	}
	r.Post("/api/auth/login", auth.LoginHandler(loginDeps))
	r.Post("/api/auth/logout", auth.LogoutHandler(deps.SessionMgr))

	// Account — every authenticated user (admin + viewer) can read /me and
	// rotate their own password.
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionAccountSelfEdit))
		rt.Get("/api/account/me", auth.AccountInfoHandler(loginDeps))
		rt.Post("/api/account/password", auth.ChangePasswordHandler(auth.AccountDeps{
			Store:      deps.UserStore,
			SessionMgr: deps.SessionMgr,
			Log:        deps.Log,
		}))
	})

	// Install wizard — pre-install endpoints; FirstRunGate's whitelist lets
	// these through unauthenticated. Once an admin user exists, the gate
	// allows traffic generally and these endpoints (plus the wizard
	// completion check inside StateHandler / FinishHandler) return 410 Gone.
	r.Get("/api/install/state", install.StateHandler(deps.InstallDeps))
	r.Post("/api/install/step/1", install.Step1Handler(deps.InstallDeps))
	r.Post("/api/install/step/2", install.Step2Handler(deps.InstallDeps))
	r.Post("/api/install/step/3", install.Step3Handler(deps.InstallDeps))
	r.Post("/api/install/step/4", install.Step4Handler(deps.InstallDeps))
	r.Post("/api/install/finish", install.FinishHandler(deps.InstallDeps))
	r.Get("/api/install/regions", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, install.Regions())
	})

	// Settings — read + probe accessible to viewer; mutate is admin-only.
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionConnectionTest))
		rt.Get("/api/settings/chirpstack", GetChirpStackHandler(deps.TestConnDeps))
		rt.Post("/api/settings/chirpstack/test", TestConnHandler(deps.TestConnDeps))
	})
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionConnectionEdit))
		rt.Put("/api/settings/chirpstack", PutChirpStackHandler(deps.TestConnDeps, deps.SecretsDir))
	})

	// Phase 2 (Plan 02-10) — Site / Metering Point / Device CRUD + atomic
	// Add Device flow. Each package's RegisterRoutes mounts its routes under
	// /api/sites, /api/metering-points, /api/devices respectively, with
	// per-route RequireAction wrappers (defense in depth on the in-handler
	// auth.Can checks).
	site.RegisterRoutes(r, site.Deps{
		Pool:       deps.Pool,
		SessionMgr: deps.SessionMgr,
		Log:        deps.Log,
	})
	meteringpoint.RegisterRoutes(r, meteringpoint.Deps{
		Pool:       deps.Pool,
		SessionMgr: deps.SessionMgr,
		Log:        deps.Log,
	})
	if deps.DeviceDeps != nil {
		// DeviceDeps requires a CS gRPC client + bootstrap + ping wiring that
		// only cmd/serve constructs (chirpstack.Client + ConnectionStore
		// adapter). Routes mount only when DeviceDeps is non-nil so unit
		// tests of the http router don't need full CS wiring.
		device.RegisterRoutes(r, *deps.DeviceDeps)
	}

	// SPA fallback — MUST be the LAST route registered (PITFALL #4). Without
	// this guard, an unknown /api/foo would resolve to the SPA's index.html
	// and the SPA's fetch helper would fail to parse HTML as JSON.
	if deps.SPA != nil {
		r.Handle("/*", deps.SPA)
	}

	return r
}
