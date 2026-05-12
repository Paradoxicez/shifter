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

	"github.com/shifter-io/shifter/internal/alert"
	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/dashboard"
	"github.com/shifter-io/shifter/internal/device"
	"github.com/shifter-io/shifter/internal/events"
	"github.com/shifter-io/shifter/internal/floorplan"
	"github.com/shifter-io/shifter/internal/gateway"
	importpkg "github.com/shifter-io/shifter/internal/import"
	"github.com/shifter-io/shifter/internal/install"
	mapapi "github.com/shifter-io/shifter/internal/map"
	"github.com/shifter-io/shifter/internal/meteringpoint"
	"github.com/shifter-io/shifter/internal/profile"
	"github.com/shifter-io/shifter/internal/report"
	"github.com/shifter-io/shifter/internal/settings"
	"github.com/shifter-io/shifter/internal/site"
	"github.com/shifter-io/shifter/internal/swap"
	"github.com/shifter-io/shifter/internal/user"
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
	// pre-install paths). Plan 02-12 (cmd/serve wiring) constructs the full
	// shape: CS gRPC client + bootstrapper + gRPC ping + MQTT ping.
	DeviceDeps *device.Deps

	// SwapDeps wires Plan 02-11's POST /api/metering-points/{id}/swap. nil
	// when CS bootstrap hasn't completed (Phase 1 router unit tests + early-
	// boot pre-install paths). Plan 02-12 (cmd/serve wiring) constructs the
	// full shape: Pool + SessionMgr + Log + Resolver invalidator.
	SwapDeps *swap.HTTPDeps

	// ProfileDeps wires Plan 02-11's /api/device-profiles editor surface.
	// nil when CS gRPC client + ConnStore are not yet constructed.
	ProfileDeps *profile.HTTPDeps

	// GatewayDeps wires Plan 03-04's /api/gateways surface (CRUD + archive/
	// restore + cached metrics). nil when CS gRPC client + metrics cache
	// are not yet constructed — mirrors DeviceDeps nil-guard pattern.
	GatewayDeps *gateway.Deps

	// ImportDeps wires Plan 03-05's /api/imports surface (upload, dry-run,
	// commit, template, errors.xlsx). nil when CS client + bootstrap are
	// not yet constructed — mirrors DeviceDeps nil-guard pattern so router
	// unit tests can run without CS wiring.
	ImportDeps *importpkg.Deps

	// EventsDeps wires Plan 04-03's GET /api/events SSE endpoint. nil when
	// the Hub has not been started (router unit tests stay free of Hub deps).
	// Mounted under the authenticated group so both admin and viewer roles
	// can subscribe (D-23). No admin-only guard — see plan §design_notes.
	EventsDeps *events.Deps

	// DashboardDeps wires Plan 04-04's dashboard REST endpoints:
	//   GET /api/dashboard/scope      — D-09 + D-21
	//   GET /api/dashboard/snapshot   — KPI tiles + latest readings
	//   GET /api/dashboard/timeseries — D-12 time-bucket chart series
	// nil when pool is not yet available (router unit tests stay free of
	// pool deps). Mounted under authenticated group (D-23 any role).
	DashboardDeps *dashboard.Deps

	// ReportDeps wires Plan 05-06's report endpoints:
	//   POST /api/reports/generate          — enqueue PDF job + write CSV/Excel
	//   GET  /api/reports/{id}              — pdf_status poll (plan 05-09)
	//   GET  /api/reports/{id}/file/{kind}  — stream artifact (csv|xlsx|pdf)
	// nil in early-boot / router unit tests that don't need report routes.
	ReportDeps *report.Deps

	// SettingsDeps wires Plan 05-11's data retention endpoints:
	//   GET   /api/settings/retention  — admin + viewer (read)
	//   PATCH /api/settings/retention  — admin only (T-05-11-01)
	// nil in early-boot / router unit tests that don't need retention routes.
	SettingsDeps *settings.Deps

	// MapDeps wires Plan 05-04's map data endpoint:
	//   GET /api/map/data — admin + viewer (auth.ActionSiteRead)
	// nil in early-boot / router unit tests that don't need the map route.
	// Plan 05-13 gap closure — Phase 5 verification gap 1.
	MapDeps *mapapi.Deps

	// UserDeps wires Plan 06-05's /api/users CRUD + reset-password +
	// logout-everywhere endpoints. nil in early-boot / router unit tests
	// that don't need user-mgmt routes (router test fixture can keep its
	// minimal Deps shape).
	UserDeps *user.Deps

	// AlertDeps wires Plan 06-04's /api/alerts + /api/alerts/rules +
	// /api/anomaly-roster + /api/metering-points/{id}/anomaly-state surfaces.
	// nil in early-boot / router unit tests that don't need alert routes.
	AlertDeps *alert.HTTPDeps
	// AlertTestFireDeps wires the D-19 test-fire endpoint. Separate from
	// AlertDeps because it needs a River client closure for the auto-clear
	// schedule. nil-guarded so non-River tests can still mount AlertDeps.
	AlertTestFireDeps *alert.TestFireDeps

	// FloorPlanDeps wires Plan 05-05/07's 12 floor-plan endpoints:
	//   POST   /api/sites/{siteID}/floor-plans              (admin only)
	//   GET    /api/sites/{siteID}/floor-plans              (admin + viewer)
	//   GET    /api/floor-plans/{id}                        (admin + viewer)
	//   GET    /api/floor-plans/{id}/image                  (admin + viewer)
	//   GET    /api/floor-plans/{id}/placements             (admin + viewer)
	//   PATCH  /api/floor-plans/{id}                        (admin only)
	//   PATCH  /api/floor-plans/{id}/label                  (admin only)
	//   PATCH  /api/floor-plans/{id}/placements/{deviceID}  (admin only)
	//   DELETE /api/floor-plans/{id}                        (admin only)
	//   DELETE /api/floor-plans/{id}/placements/{deviceID}  (admin only)
	//   POST   /api/floor-plans/{id}/placements             (admin only)
	// nil in early-boot / router unit tests that don't need floor-plan routes.
	// Plan 05-13 gap closure — Phase 5 verification gap 2.
	FloorPlanDeps *floorplan.Deps

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
//	POST /api/metering-points/{id}/swap       ActionMeterSwap (admin) — Plan 02-11
//	GET  /api/device-profiles                 ActionDeviceProfileRead (both)
//	GET  /api/device-profiles/{id}            ActionDeviceProfileRead (both)
//	POST /api/device-profiles/{id}/decoded-sample ActionDeviceProfileRead (both)
//	POST /api/device-profiles                 ActionDeviceProfileCreate (admin)
//	PATCH /api/device-profiles/{id}           ActionDeviceProfileUpdate (admin)
//	POST /api/device-profiles/{id}/archive    ActionDeviceProfileArchive (admin)
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
	r.Post("/api/auth/logout", auth.LogoutHandlerWithAudit(deps.SessionMgr, deps.UserStore))

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
		//
		// Phase 3 (Plan 03-06) extends the device routes mounted here:
		//   GET   /api/devices                          — filter / sort / page (D-12..D-18)
		//   POST  /api/devices/bulk-decommission        — admin only (D-17)
		//   POST  /api/devices/{eui}/keys               — reveal secrets, admin only
		//                                                 (D-22 / D-26 / D-27 / D-28)
		device.RegisterRoutes(r, *deps.DeviceDeps)
	}
	if deps.SwapDeps != nil {
		// SwapDeps mounts POST /api/metering-points/{id}/swap (Plan 02-11).
		// Mirrors DeviceDeps nil-guard pattern — skip when not wired so
		// router unit tests stay free of pgxpool / SessionMgr requirements.
		swap.RegisterRoutes(r, *deps.SwapDeps)
	}
	if deps.ProfileDeps != nil {
		// ProfileDeps mounts /api/device-profiles editor REST surface
		// (Plan 02-11). nil when CS gRPC client + ConnStore are not yet
		// constructed.
		profile.RegisterRoutes(r, *deps.ProfileDeps)
	}
	if deps.GatewayDeps != nil {
		// GatewayDeps mounts /api/gateways CRUD + archive/restore + cached
		// metrics (Plan 03-04). nil-guard mirrors DeviceDeps: when CS gRPC
		// client + metrics cache are not yet wired, the gateway routes are
		// simply not mounted (router unit tests stay free of CS deps).
		gateway.RegisterRoutes(r, *deps.GatewayDeps)
	}
	if deps.ImportDeps != nil {
		// ImportDeps mounts /api/imports upload + dry-run + commit + template
		// + errors.xlsx (Plan 03-05). nil-guard mirrors DeviceDeps: skip when
		// CS client + bootstrap are not yet constructed so router unit tests
		// stay free of CS deps.
		importpkg.RegisterRoutes(r, *deps.ImportDeps)
	}
	if deps.EventsDeps != nil {
		// EventsDeps mounts GET /api/events SSE endpoint (Plan 04-03). Mounted
		// here (authenticated group, any role) so both admin and viewer can
		// connect (D-23). No admin-only sub-group needed.
		events.RegisterRoutes(r, *deps.EventsDeps)
	}
	if deps.DashboardDeps != nil {
		// DashboardDeps mounts the three Plan 04-04 dashboard endpoints.
		// Both admin and viewer roles have full read access (D-23).
		// Mounted before the SPA fallback (PITFALL #4 preserved).
		dashboard.RegisterRoutes(r, *deps.DashboardDeps)
	}
	if deps.ReportDeps != nil {
		// ReportDeps mounts Plan 05-06's three report endpoints.
		// Auth is enforced inside each handler (viewers see own reports,
		// admins see all). Mounted before the SPA fallback (PITFALL #4).
		report.RegisterRoutes(r, *deps.ReportDeps)
	}
	if deps.SettingsDeps != nil {
		// SettingsDeps mounts Plan 05-11's data retention endpoints:
		//   GET   /api/settings/retention — admin + viewer
		//   PATCH /api/settings/retention — admin only (T-05-11-01)
		// Mounted before the SPA fallback (PITFALL #4).
		settings.RegisterRoutes(r, *deps.SettingsDeps, deps.SessionMgr)
	}
	if deps.MapDeps != nil {
		// MapDeps mounts Plan 05-04's GET /api/map/data endpoint.
		// Auth (admin + viewer via ActionSiteRead) enforced inside the
		// package's RegisterRoutes. Mounted before SPA fallback (PITFALL #4).
		// Plan 05-13 gap closure.
		mapapi.RegisterRoutes(r, *deps.MapDeps)
	}
	if deps.UserDeps != nil {
		// UserDeps mounts Plan 06-05's /api/users surface. Auth is enforced
		// inside each route group via RequireAction (see user.RegisterRoutes
		// for the per-route mapping). Mounted before SPA fallback (PITFALL #4).
		user.RegisterRoutes(r, *deps.UserDeps)
	}
	if deps.FloorPlanDeps != nil {
		// FloorPlanDeps mounts Plan 05-05 + 05-07's 12 floor-plan routes.
		// Auth groups (site.read / site.create / site.update / site.archive)
		// are applied inside the package's RegisterRoutes per route.
		// Mounted before SPA fallback (PITFALL #4).
		// Plan 05-13 gap closure.
		floorplan.RegisterRoutes(r, *deps.FloorPlanDeps)
	}
	if deps.AlertDeps != nil {
		// Plan 06-04 alert center routes. D-11 viewer read-only enforced
		// at the RequireAction layer:
		//   GET    /api/alerts                 — admin + viewer
		//   GET    /api/alerts/recent          — admin + viewer (bell + drawer)
		//   GET    /api/alerts/{id}            — admin + viewer
		//   POST   /api/alerts/{id}/ack        — admin only
		//   POST   /api/alerts/{id}/snooze     — admin only
		//   GET    /api/alerts/rules           — admin + viewer
		//   POST   /api/alerts/rules           — admin only
		//   PATCH  /api/alerts/rules/{id}      — admin only
		//   POST   /api/alerts/rules/{id}/disable — admin only
		//   POST   /api/alerts/rules/{id}/enable  — admin only
		//   POST   /api/alerts/rules/{id}/test-fire — admin only (D-19)
		//   GET    /api/anomaly-roster         — admin + viewer
		//   GET    /api/metering-points/{id}/anomaly-state — admin + viewer
		//   PATCH  /api/metering-points/{id}/anomaly-rules/{kind} — admin only
		alertDeps := *deps.AlertDeps
		r.Route("/api/alerts", func(rt chi.Router) {
			rt.Group(func(g chi.Router) {
				g.Use(auth.RequireAction(deps.SessionMgr, auth.ActionAlertRead))
				g.Get("/", alert.ListHandler(alertDeps))
				g.Get("/recent", alert.RecentForDrawerHandler(alertDeps))
				g.Get("/{id}", alert.GetHandler(alertDeps))
			})
			rt.Group(func(g chi.Router) {
				g.Use(auth.RequireAction(deps.SessionMgr, auth.ActionAlertAck))
				g.Post("/{id}/ack", alert.AckHandler(alertDeps))
			})
			rt.Group(func(g chi.Router) {
				g.Use(auth.RequireAction(deps.SessionMgr, auth.ActionAlertSnooze))
				g.Post("/{id}/snooze", alert.SnoozeHandler(alertDeps))
			})
			rt.Route("/rules", func(rr chi.Router) {
				rr.Group(func(g chi.Router) {
					g.Use(auth.RequireAction(deps.SessionMgr, auth.ActionAlertRead))
					g.Get("/", alert.ListRulesHandler(alertDeps))
				})
				rr.Group(func(g chi.Router) {
					g.Use(auth.RequireAction(deps.SessionMgr, auth.ActionAlertRuleCreate))
					g.Post("/", alert.CreateRuleHandler(alertDeps))
				})
				rr.Group(func(g chi.Router) {
					g.Use(auth.RequireAction(deps.SessionMgr, auth.ActionAlertRuleUpdate))
					g.Patch("/{id}", alert.UpdateRuleHandler(alertDeps))
				})
				rr.Group(func(g chi.Router) {
					g.Use(auth.RequireAction(deps.SessionMgr, auth.ActionAlertRuleDisable))
					g.Post("/{id}/disable", alert.DisableRuleHandler(alertDeps))
				})
				rr.Group(func(g chi.Router) {
					g.Use(auth.RequireAction(deps.SessionMgr, auth.ActionAlertRuleEnable))
					g.Post("/{id}/enable", alert.EnableRuleHandler(alertDeps))
				})
				if deps.AlertTestFireDeps != nil {
					rr.Group(func(g chi.Router) {
						g.Use(auth.RequireAction(deps.SessionMgr, auth.ActionAlertTestFire))
						g.Post("/{id}/test-fire", alert.TestFireHandler(*deps.AlertTestFireDeps))
					})
				}
			})
		})
		r.Group(func(g chi.Router) {
			g.Use(auth.RequireAction(deps.SessionMgr, auth.ActionAlertRead))
			g.Get("/api/anomaly-roster", alert.RosterHandler(alertDeps))
			g.Get("/api/metering-points/{id}/anomaly-state", alert.MPAnomalyStateHandler(alertDeps))
		})
		r.Group(func(g chi.Router) {
			g.Use(auth.RequireAction(deps.SessionMgr, auth.ActionAlertRuleCreate))
			g.Patch("/api/metering-points/{id}/anomaly-rules/{kind}", alert.ToggleMPAnomalyHandler(alertDeps))
		})
	}

	// SPA fallback — MUST be the LAST route registered (PITFALL #4). Without
	// this guard, an unknown /api/foo would resolve to the SPA's index.html
	// and the SPA's fetch helper would fail to parse HTML as JSON.
	if deps.SPA != nil {
		r.Handle("/*", deps.SPA)
	}

	return r
}
