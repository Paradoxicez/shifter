---
phase: 01-foundation
plan: 18
type: execute
wave: 12
depends_on: [09, 10, 11, 13, 14, 15, 17]
files_modified:
  - go.mod
  - go.sum
  - internal/http/router.go
  - internal/http/middleware.go
  - internal/http/health.go
  - internal/http/health_test.go
  - internal/cli/serve.go
autonomous: true
requirements:
  - INST-05
  - INST-06
  - AUTH-06
  - CHIRP-01
  - CHIRP-02
  - CHIRP-03
  - SETT-01
  - SETT-03
must_haves:
  truths:
    - "GET /health returns {status, version, uptime_seconds} without auth (D-18; INST-06 reframed per D-19)"
    - "GET /health/detailed requires admin (RequireAction(sm, ActionHealthDetailed)) and returns DB ping result (D-19)"
    - "Router order: RequestID → RealIP → Logger → Recoverer → SessionLoadAndSave → FirstRunGate → routes → SPA fallback (PITFALL #4)"
    - "Mounts: /api/auth/login, /api/auth/logout, /api/account/me, /api/account/password, /api/install/state, /api/install/step/{1..4}, /api/install/finish, /api/install/regions, /api/settings/chirpstack[/test], /health[/detailed]"
    - "shifter serve refuses to start if config-loaded ChirpStack returns ErrChirpStackV3OrUnknown at boot (INST-05 startup gate)"
    - "shifter serve runs migrations + starts MQTT subscriber + opens HTTP listener; graceful shutdown on SIGTERM"
  artifacts:
    - path: "internal/http/router.go"
      provides: "NewRouter wiring all Phase 1 routes with proper middleware order"
      contains: "chi.NewRouter"
    - path: "internal/http/middleware.go"
      provides: "RequestID + Logger + Recoverer wrappers using slog (PITFALL #4 anchor)"
      contains: "Recoverer"
    - path: "internal/http/health.go"
      provides: "Health (public) + HealthDetailed (admin) handlers (D-18, D-19)"
      contains: "uptime_seconds"
    - path: "internal/cli/serve.go"
      provides: "Plan 05 stub replaced — full wiring per RESEARCH §Wiring serve"
      contains: "chirpstack.ProbeVersion"
  key_links:
    - from: "internal/http/router.go"
      to: "all Phase 1 handlers"
      via: "chi route groups + middleware composition"
      pattern: "chi.Router"
    - from: "internal/cli/serve.go"
      to: "internal/http/router.go NewRouter"
      via: "Deps struct construction in serve"
      pattern: "NewRouter"
---

<objective>
Wire the chi router that mounts every Phase 1 handler with the canonical middleware order (PITFALL #4: SPA fallback LAST), implement the public `/health` and admin `/health/detailed` endpoints (D-18, D-19), and replace the Plan 05 `serve.go` stub with the full wiring from RESEARCH §"Wiring serve" — including the boot-time v3 refusal (INST-05).

Purpose: The router is the single integration point that consumes Plans 09/10/11/14/15/17. `serve` is the binary entry point that ties config + DB + ChirpStack + MQTT + HTTP + signal handling together.

Output: `shifter serve` opens a listener, all `/api/*` routes are reachable, /health responds, /health/detailed requires admin. `go test ./internal/http -run TestHealth_` passes.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/01-foundation/01-CONTEXT.md
@.planning/phases/01-foundation/01-RESEARCH.md
@01-09-login-ratelimit-PLAN.md
@01-10-authz-PLAN.md
@01-13-mqtt-subscriber-PLAN.md
@01-14-install-middleware-PLAN.md
@01-15-install-handlers-PLAN.md
@01-17-test-connection-PLAN.md

<interfaces>
RESEARCH §"Wiring serve" (lines 1390-1476) — verbatim serve skeleton; Plan 18 fills in the route mounting.

RESEARCH §"Pattern 15: /health and /health/detailed" (lines 1107-1135) — verbatim health handlers.

PITFALL #4 — SPA fallback MUST be mounted LAST so /api/* 404s don't return index.html.

Middleware order (chi convention + RESEARCH):
1. `chi.middleware.RequestID`
2. `chi.middleware.RealIP`
3. `chi.middleware.Logger` (slog adapter)
4. `chi.middleware.Recoverer`
5. `sessionMgr.LoadAndSave` (alexedwards/scs)
6. `install.FirstRunGate` (cached)

Routes group:
```
/health                              public
/health/detailed                     RequireAction(ActionHealthDetailed)
/api/auth/login                      public (rate-limited)
/api/auth/logout                     public
/api/account/me                      session required
/api/account/password                session required
/api/install/state                   public (whitelisted by FirstRunGate)
/api/install/step/1..4               public
/api/install/finish                  public
/api/install/regions                 public — returns install.Regions()
/api/settings/chirpstack             RequireAction(ActionConnectionTest) — viewer can read
/api/settings/chirpstack             PUT — RequireAction(ActionConnectionEdit) — admin only
/api/settings/chirpstack/test        POST — RequireAction(ActionConnectionTest)
/                                    SPA (last; index.html fallback)
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Health handlers + tests</name>
  <files>internal/http/health.go, internal/http/health_test.go</files>
  <read_first>
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pattern 15: /health and /health/detailed" (lines 1104-1135)
    - .planning/phases/01-foundation/01-CONTEXT.md (D-18, D-19 — INST-06 reframing)
    - .planning/phases/01-foundation/01-VALIDATION.md (TestHealth_Public, TestHealthDetailed_RequiresAdmin names)
  </read_first>
  <behavior>
    - TestHealth_Public: GET /health (no auth) → 200 + JSON {status: "ok", version: "<semver>", uptime_seconds: int}.
    - TestHealth_Public_NoAdminCheck: GET /health doesn't query the user table (no DB hit beyond startedAt).
    - TestHealthDetailed_RequiresAdmin: GET /health/detailed without session → 401; with viewer → 403; with admin → 200 + body has `checks.db = true`.
    - TestHealthDetailed_DBPingFailure: drop the DB pool; admin GET → 200 with `status: "degraded"`, `checks.db = false`.
  </behavior>
  <action>
1. Create `internal/http/health.go`:
   ```go
   package http

   import (
       "encoding/json"
       "net/http"
       "time"

       "github.com/jackc/pgx/v5/pgxpool"

       "github.com/shifter-io/shifter/internal/version"
   )

   var startedAt = time.Now()

   // Health is the PUBLIC /health endpoint (D-18). No auth; minimal payload so
   // anonymous probes can't enumerate install internals.
   func Health() http.HandlerFunc {
       return func(w http.ResponseWriter, r *http.Request) {
           writeJSON(w, http.StatusOK, map[string]any{
               "status":         "ok",
               "version":        version.Info(),
               "uptime_seconds": int(time.Since(startedAt).Seconds()),
           })
       }
   }

   // HealthDetailed is the ADMIN /health/detailed endpoint (D-19).
   // Phase 1 minimum: DB ping + version. CS/MQTT/disk/last-uplink-age move to Phase 6.
   func HealthDetailed(pool *pgxpool.Pool) http.HandlerFunc {
       return func(w http.ResponseWriter, r *http.Request) {
           dbOK := pool.Ping(r.Context()) == nil
           status := "ok"
           if !dbOK { status = "degraded" }
           writeJSON(w, http.StatusOK, map[string]any{
               "status":         status,
               "checks":         map[string]any{"db": dbOK},
               "version":        version.Info(),
               "uptime_seconds": int(time.Since(startedAt).Seconds()),
           })
       }
   }

   // Reset is for tests so startedAt can be overridden.
   func ResetStartedAtForTest(t time.Time) { startedAt = t }

   var _ = json.Encoder{} // keep import
   ```

2. Replace `internal/http/health_test.go`:
   ```go
   package http

   import (
       "context"
       "encoding/json"
       "log/slog"
       "net/http"
       "net/http/cookiejar"
       "net/http/httptest"
       "os"
       "testing"
       "time"

       "github.com/shifter-io/shifter/internal/auth"
       "github.com/shifter-io/shifter/internal/db"
       "github.com/shifter-io/shifter/internal/testsupport"
       "github.com/stretchr/testify/require"
   )

   func TestHealth_Public(t *testing.T) {
       handler := Health()
       req := httptest.NewRequest("GET", "/health", nil)
       w := httptest.NewRecorder()
       handler(w, req)
       require.Equal(t, 200, w.Code)
       var body map[string]any
       require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
       require.Equal(t, "ok", body["status"])
       require.NotNil(t, body["version"])
       require.NotNil(t, body["uptime_seconds"])
       _, hasChecks := body["checks"]
       require.False(t, hasChecks, "D-18: /health must NOT include detailed checks")
   }

   func TestHealthDetailed_RequiresAdmin(t *testing.T) {
       pool := testsupport.StartPostgres(t)
       require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
       sm := auth.NewSessionManager(pool, true, time.Hour, 24*time.Hour)
       protected := auth.RequireAction(sm, auth.ActionHealthDetailed)(HealthDetailed(pool))

       mux := http.NewServeMux()
       mux.Handle("POST /seed", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
           role := r.URL.Query().Get("role")
           _ = auth.PutUser(r.Context(), sm, auth.User{ID: "u1", Role: role})
       }))
       mux.Handle("GET /health/detailed", protected)

       srv := httptest.NewServer(sm.LoadAndSave(mux))
       t.Cleanup(srv.Close)

       // No session → 401
       res, err := http.Get(srv.URL + "/health/detailed")
       require.NoError(t, err); res.Body.Close()
       require.Equal(t, 401, res.StatusCode)

       // Viewer → 403
       j1, _ := cookiejar.New(nil); cli1 := &http.Client{Jar: j1}
       _, _ = cli1.Post(srv.URL+"/seed?role=viewer", "", nil)
       res, _ = cli1.Get(srv.URL + "/health/detailed"); res.Body.Close()
       require.Equal(t, 403, res.StatusCode, "viewer must not access /health/detailed")

       // Admin → 200 + body has checks.db
       j2, _ := cookiejar.New(nil); cli2 := &http.Client{Jar: j2}
       _, _ = cli2.Post(srv.URL+"/seed?role=admin", "", nil)
       res, _ = cli2.Get(srv.URL + "/health/detailed")
       defer res.Body.Close()
       require.Equal(t, 200, res.StatusCode)
       var body map[string]any
       require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
       checks := body["checks"].(map[string]any)
       require.Equal(t, true, checks["db"])
   }
   ```
  </action>
  <verify>
    <automated>go test ./internal/http -run 'TestHealth_' -race -count=1 -v</automated>
  </verify>
  <acceptance_criteria>
    - File `internal/http/health.go` exports `func Health() http.HandlerFunc` and `func HealthDetailed(pool *pgxpool.Pool) http.HandlerFunc`
    - `Health()` response body has keys `status`, `version`, `uptime_seconds` and NOT `checks`
    - `HealthDetailed` response body has keys `status`, `checks.db`, `version`, `uptime_seconds`
    - Command `go test ./internal/http -run TestHealth_Public -race` exits 0 (per VALIDATION.md)
    - Command `go test ./internal/http -run TestHealthDetailed_RequiresAdmin -race` exits 0 (per VALIDATION.md)
  </acceptance_criteria>
  <done>
    Health endpoints ready. Plan 22 (Caddyfile) routes /health to the binary; Plan 20/21 (compose) wire `shifter healthcheck` for Docker HEALTHCHECK.
  </done>
</task>

<task type="auto">
  <name>Task 2: chi Router with full Phase 1 wiring + middleware order</name>
  <files>go.mod, go.sum, internal/http/router.go, internal/http/middleware.go</files>
  <read_first>
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pitfall 4: SPA fallback serves index.html for /api/* 404s" (lines 1322-1336)
    - 01-09-login-ratelimit-PLAN.md (LoginHandler, LogoutHandler, ChangePasswordHandler, AccountInfoHandler)
    - 01-10-authz-PLAN.md (RequireAction)
    - 01-14-install-middleware-PLAN.md (FirstRunGate)
    - 01-15-install-handlers-PLAN.md (StateHandler, Step1..4Handler, FinishHandler)
    - 01-17-test-connection-PLAN.md (TestConnHandler, GetChirpStackHandler, PutChirpStackHandler, ProductionDial)
  </read_first>
  <action>
1. Install chi:
   ```bash
   go get github.com/go-chi/chi/v5@latest
   ```

2. Create `internal/http/middleware.go`:
   ```go
   package http

   import (
       "log/slog"
       "net/http"
       "time"

       "github.com/go-chi/chi/v5/middleware"
   )

   // SlogLogger emits a single structured log line per request.
   func SlogLogger(log *slog.Logger) func(http.Handler) http.Handler {
       return func(next http.Handler) http.Handler {
           return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
               t0 := time.Now()
               ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
               next.ServeHTTP(ww, r)
               log.Info("http",
                   "method", r.Method,
                   "path", r.URL.Path,
                   "status", ww.Status(),
                   "bytes", ww.BytesWritten(),
                   "ms", time.Since(t0).Milliseconds(),
                   "request_id", middleware.GetReqID(r.Context()),
               )
           })
       }
   }
   ```

3. Create `internal/http/router.go`:
   ```go
   package http

   import (
       "log/slog"
       "net/http"

       "github.com/alexedwards/scs/v2"
       "github.com/go-chi/chi/v5"
       "github.com/go-chi/chi/v5/middleware"
       "github.com/jackc/pgx/v5/pgxpool"

       "github.com/shifter-io/shifter/internal/auth"
       "github.com/shifter-io/shifter/internal/install"
   )

   // Deps groups every dependency the router needs.
   type Deps struct {
       Pool          *pgxpool.Pool
       SessionMgr    *scs.SessionManager
       LoginLimiter  *auth.LoginLimiter
       UserStore     *auth.Store
       InstallStore  *install.Store
       SecretsDir    string
       Log           *slog.Logger
       SPA           http.Handler          // Plan 19 supplies the SPA fallback handler
       TestConnDeps  TestConnDeps          // Plan 17 supplies (Dial + PingMQTT + Pool)
       InstallDeps   install.Deps          // Plan 15 + Plan 16 wiring
   }

   // NewRouter wires all Phase 1 routes.
   //
   // Middleware order (PITFALL #4):
   //   RequestID → RealIP → Logger → Recoverer → SessionLoadAndSave → FirstRunGate → routes → SPA last.
   func NewRouter(deps Deps) http.Handler {
       r := chi.NewRouter()

       r.Use(middleware.RequestID)
       r.Use(middleware.RealIP)
       r.Use(SlogLogger(deps.Log))
       r.Use(middleware.Recoverer)
       r.Use(deps.SessionMgr.LoadAndSave)
       r.Use(install.FirstRunGate(deps.Pool, deps.Log))

       // Public health
       r.Get("/health", Health())

       // Detailed health (admin)
       r.Group(func(rt chi.Router) {
           rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionHealthDetailed))
           rt.Get("/health/detailed", HealthDetailed(deps.Pool))
       })

       // Auth
       loginDeps := auth.LoginDeps{
           Store: deps.UserStore, SessionMgr: deps.SessionMgr,
           LoginLimiter: deps.LoginLimiter, Log: deps.Log,
       }
       r.Post("/api/auth/login",  auth.LoginHandler(loginDeps))
       r.Post("/api/auth/logout", auth.LogoutHandler(deps.SessionMgr))

       // Account (session required — RequireAction ActionAccountSelfEdit)
       r.Group(func(rt chi.Router) {
           rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionAccountSelfEdit))
           rt.Get("/api/account/me", auth.AccountInfoHandler(loginDeps))
           rt.Post("/api/account/password", auth.ChangePasswordHandler(auth.AccountDeps{
               Store: deps.UserStore, SessionMgr: deps.SessionMgr, Pool: deps.Pool, Log: deps.Log,
           }))
       })

       // Install (whitelisted by FirstRunGate; no auth required)
       r.Get("/api/install/state",     install.StateHandler(deps.InstallDeps))
       r.Post("/api/install/step/1",   install.Step1Handler(deps.InstallDeps))
       r.Post("/api/install/step/2",   install.Step2Handler(deps.InstallDeps))
       r.Post("/api/install/step/3",   install.Step3Handler(deps.InstallDeps))
       r.Post("/api/install/step/4",   install.Step4Handler(deps.InstallDeps))
       r.Post("/api/install/finish",   install.FinishHandler(deps.InstallDeps))
       r.Get("/api/install/regions",   func(w http.ResponseWriter, _ *http.Request) {
           writeJSON(w, http.StatusOK, install.Regions())
       })

       // Settings
       r.Group(func(rt chi.Router) {
           rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionConnectionTest))
           rt.Get("/api/settings/chirpstack",       GetChirpStackHandler(deps.TestConnDeps))
           rt.Post("/api/settings/chirpstack/test", TestConnHandler(deps.TestConnDeps))
       })
       r.Group(func(rt chi.Router) {
           rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionConnectionEdit))
           rt.Put("/api/settings/chirpstack", PutChirpStackHandler(deps.TestConnDeps, deps.SecretsDir))
       })

       // SPA fallback — MUST be mounted LAST (PITFALL #4)
       if deps.SPA != nil {
           r.Handle("/*", deps.SPA)
       }

       return r
   }
   ```
  </action>
  <verify>
    <automated>go build ./internal/http && go test ./internal/http -race -count=1 -v</automated>
  </verify>
  <acceptance_criteria>
    - File `internal/http/router.go` exports `func NewRouter(deps Deps) http.Handler` and `type Deps struct`
    - Middleware order in `NewRouter` is exactly: `middleware.RequestID`, `middleware.RealIP`, `SlogLogger`, `middleware.Recoverer`, `deps.SessionMgr.LoadAndSave`, `install.FirstRunGate(...)` — verified by reading the source line-by-line
    - `r.Handle("/*", deps.SPA)` is the LAST route registered (PITFALL #4)
    - All Phase 1 routes registered: `/health`, `/health/detailed`, `/api/auth/login`, `/api/auth/logout`, `/api/account/me`, `/api/account/password`, `/api/install/state`, `/api/install/step/1..4`, `/api/install/finish`, `/api/install/regions`, `/api/settings/chirpstack` (GET/PUT/test) — grep proof: 12+ `r.Get` / `r.Post` / `r.Put` calls
    - `/health/detailed` is wrapped by `auth.RequireAction(deps.SessionMgr, auth.ActionHealthDetailed)`
    - PUT `/api/settings/chirpstack` is wrapped by `auth.RequireAction(deps.SessionMgr, auth.ActionConnectionEdit)`
    - Command `go build ./internal/http` exits 0
  </acceptance_criteria>
  <done>
    Router fully wired. Plan 19 supplies `deps.SPA`. Task 3 below replaces serve.go.
  </done>
</task>

<task type="auto">
  <name>Task 3: serve.go full body — wire Deps + boot-time v3 refusal + MQTT subscriber start</name>
  <files>internal/cli/serve.go</files>
  <read_first>
    - .planning/phases/01-foundation/01-RESEARCH.md §"Wiring serve (the big one)" (lines 1390-1476)
    - 01-12-chirpstack-grpc-PLAN.md (Dial + ProbeVersion + ErrChirpStackV3OrUnknown)
    - 01-13-mqtt-subscriber-PLAN.md (NewMQTTSubscriber, Shutdown)
    - 01-14-install-middleware-PLAN.md (Store)
    - 01-17-test-connection-PLAN.md (ProductionDial, TestConnDeps)
  </read_first>
  <action>
1. Replace `internal/cli/serve.go` body completely with the full RESEARCH §"Wiring serve" implementation:
   ```go
   package cli

   import (
       "context"
       "errors"
       "fmt"
       "net/http"
       "os"
       "os/signal"
       "syscall"
       "time"

       "github.com/spf13/cobra"

       "github.com/shifter-io/shifter/internal/auth"
       "github.com/shifter-io/shifter/internal/chirpstack"
       "github.com/shifter-io/shifter/internal/config"
       "github.com/shifter-io/shifter/internal/db"
       httpapi "github.com/shifter-io/shifter/internal/http"
       "github.com/shifter-io/shifter/internal/install"
       "github.com/shifter-io/shifter/internal/logging"
   )

   var serveCmd = &cobra.Command{
       Use:   "serve",
       Short: "Run migrations then start the HTTP server (D-13)",
       RunE: func(cmd *cobra.Command, _ []string) error {
           cfg, err := config.Load()
           if err != nil {
               return err
           }
           log := logging.New(cfg.LogLevel)
           ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
           defer cancel()

           // 1. DB pool
           pool, err := db.NewPool(ctx, cfg.DB.DSN(), cfg.DB.MaxConns)
           if err != nil {
               return fmt.Errorf("db: %w", err)
           }
           defer pool.Close()

           // 2. D-13: auto-migrate
           if err := db.RunMigrations(ctx, pool, log); err != nil {
               return err
           }

           // 3. ChirpStack adapters — only meaningful AFTER install. We attempt the
           //    dial+probe at boot for diagnostics; if v3 is detected we REFUSE to start
           //    (INST-05 startup gate). If unreachable, log and continue (degraded mode).
           if cfg.ChirpStack.GRPCURL != "" {
               csConn, csErr := chirpstack.Dial(ctx, cfg.ChirpStack)
               if csErr == nil {
                   probeCtx, probeCancel := context.WithTimeout(ctx, 8*time.Second)
                   _, probeErr := chirpstack.ProbeVersion(probeCtx, csConn)
                   probeCancel()
                   if errors.Is(probeErr, chirpstack.ErrChirpStackV3OrUnknown) {
                       _ = csConn.Close()
                       return fmt.Errorf("INST-05: refusing to start — ChirpStack v3 detected at %s", cfg.ChirpStack.GRPCURL)
                   }
                   _ = csConn.Close()
               } else {
                   log.Warn("chirpstack not reachable on boot — degraded mode", "err", csErr)
               }
           }

           // 4. MQTT subscriber (CHIRP-02). Phase 1 only logs uplinks; Phase 2 wires persist.
           var mqttSub *chirpstack.MQTTSubscriber
           if cfg.MQTT.URL != "" {
               sub, mqttErr := chirpstack.NewMQTTSubscriber(
                   cfg.MQTT.URL, cfg.MQTT.User, cfg.MQTT.Password,
                   "shifter-"+os.Getenv("HOSTNAME"), log, nil,
               )
               if mqttErr != nil {
                   log.Warn("mqtt not reachable on boot — degraded mode", "err", mqttErr)
               } else {
                   mqttSub = sub
               }
           }

           // 5. Auth wiring
           sm := auth.NewSessionManager(pool, cfg.IsDev(), cfg.Session.IdleTimeout, cfg.Session.Lifetime)
           limiter := auth.NewLoginLimiter()
           defer limiter.Stop()
           userStore := auth.NewStore(pool)

           // 6. Install wiring
           installStore := install.NewStore(pool)
           installDeps := install.Deps{
               Pool:       pool,
               Store:      installStore,
               SecretsDir: "/run/secrets",
               Log:        log,
               Dial: func(ctx context.Context, c config.CSConfig) (install.CSConn, error) {
                   conn, err := chirpstack.Dial(ctx, c)
                   if err != nil { return nil, err }
                   return &csConnWrapper{c: conn}, nil
               },
           }

           // 7. Test-conn deps reuse the same Dial
           tcDeps := httpapi.TestConnDeps{
               Pool: pool, Log: log,
               Dial: httpapi.ProductionDial,
               PingMQTT: chirpstack.PingMQTT,
           }

           // 8. Router
           router := httpapi.NewRouter(httpapi.Deps{
               Pool:         pool,
               SessionMgr:   sm,
               LoginLimiter: limiter,
               UserStore:    userStore,
               InstallStore: installStore,
               SecretsDir:   "/run/secrets",
               Log:          log,
               SPA:          httpapi.SPAHandler(),  // Plan 19 implements
               TestConnDeps: tcDeps,
               InstallDeps:  installDeps,
           })

           // 9. HTTP server
           srv := &http.Server{
               Addr:              ":" + cfg.HTTPPort,
               Handler:           router,
               ReadHeaderTimeout: 10 * time.Second,
               IdleTimeout:       2 * time.Minute,
           }
           go func() {
               log.Info("listening", "addr", srv.Addr)
               if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
                   log.Error("listen failed", "err", err)
                   cancel()
               }
           }()

           <-ctx.Done()
           log.Info("shutting down")
           shutCtx, shutCancel := context.WithTimeout(context.Background(), 15*time.Second)
           defer shutCancel()
           if mqttSub != nil {
               mqttSub.Shutdown(5 * time.Second)
           }
           return srv.Shutdown(shutCtx)
       },
   }

   // csConnWrapper bridges chirpstack.Dial's *grpc.ClientConn into the install package's
   // CSConn interface (defined in install/handlers.go as csConn).
   type csConnWrapper struct{ c interface{ Close() error } }
   func (c *csConnWrapper) Real() interface{} { return c.c }
   func (c *csConnWrapper) Close() error      { return c.c.Close() }
   ```

   *Note:* Adjust import — the `install.CSConn` type alias may need to be named `csConn` (interface) and exported as `CSConn`. Update Plan 15's `internal/install/handlers.go` to export it:
   ```go
   // exported alias for serve to import
   type CSConn = csConn
   ```

2. Add a focused `serve` test in `internal/cli/serve_test.go` (replace skip stub):
   ```go
   package cli

   import (
       "testing"
   )

   // TestServe_RefusesV3 is best-tested as an integration test in CI smoke
   // (Plan 20 verifies via compose), so this unit form is a placeholder
   // that documents the contract.
   func TestServe_RefusesV3(t *testing.T) {
       t.Skip("Plan 20 (compose-smoke-bundled) covers INST-05 startup refusal end-to-end")
   }

   func TestServe_AutoMigrate(t *testing.T) {
       t.Skip("Plan 20 (compose-smoke-bundled) covers D-13 auto-migrate end-to-end")
   }
   ```
  </action>
  <verify>
    <automated>go build ./cmd/shifter && go vet ./...</automated>
  </verify>
  <acceptance_criteria>
    - File `internal/cli/serve.go` no longer contains `TODO(plan-09 + plan-13 + plan-18)` marker
    - File contains `chirpstack.ProbeVersion` call inside `serve` (INST-05 startup gate)
    - File returns an error mentioning "INST-05" and "ChirpStack v3" when `ErrChirpStackV3OrUnknown` is encountered at boot
    - File constructs `httpapi.Deps` with all 11 fields populated (Pool, SessionMgr, LoginLimiter, UserStore, InstallStore, SecretsDir, Log, SPA, TestConnDeps, InstallDeps)
    - File calls `db.RunMigrations` BEFORE `srv.ListenAndServe()` (D-13)
    - File starts MQTT subscriber via `chirpstack.NewMQTTSubscriber` and calls `Shutdown` on graceful exit
    - Command `go build ./cmd/shifter` exits 0
    - Command `go vet ./...` exits 0
  </acceptance_criteria>
  <done>
    `shifter serve` is end-to-end functional. Plan 19 (SPA embed) replaces the placeholder `httpapi.SPAHandler()`; Plans 20-22 wire compose + Caddy.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| browser → router | First touchpoint; all middleware applies here |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-18-01 | Information Disclosure | /health leaks DB/CS/MQTT details to anonymous | mitigate | D-18 enforces public minimum payload; detailed moves to admin-auth `/health/detailed`. ASVS V4. |
| T-18-02 | Spoofing | v3 ChirpStack accepted at boot | mitigate | `serve` calls `ProbeVersion` and refuses to start on `ErrChirpStackV3OrUnknown`. INST-05 + RESEARCH §Pattern 4. |
| T-18-03 | Information Disclosure | SPA fallback returns index.html for /api/ 404s | mitigate | PITFALL #4: SPA mounted LAST after all /api/* routes. |
| T-18-04 | Tampering | request without RequestID middleware → no traceability | mitigate | `chi.middleware.RequestID` is the FIRST middleware; logger emits request_id. ASVS V7. |
| T-18-05 | Denial of Service | request bodies unbounded | mitigate | `srv.ReadHeaderTimeout = 10s`, `IdleTimeout = 2m`; Plan 06 SPA is 5MB-bounded by Vite output. |
| T-18-06 | Information Disclosure | logger emits Authorization header / Cookie | mitigate | `SlogLogger` only logs method/path/status/bytes/ms; never headers. ASVS V7. |
| T-18-07 | Information Disclosure | panic stack traces returned to client | mitigate | `chi.middleware.Recoverer` returns 500 + logs internally; never sends stack. ASVS V7. |
</threat_model>

<verification>
- 13 routes registered (auth × 4, account × 2, install × 6, settings × 3, health × 2, plus SPA fallback)
- Middleware order matches PITFALL #4 contract
- INST-05 startup refusal active in `serve`
- MQTT subscriber starts in serve; graceful shutdown on SIGTERM
- 2 health tests pass (Public, AdminRequired)
- `go vet ./...` exits 0
</verification>

<success_criteria>
- All Phase 1 endpoints reachable through one chi router
- INST-06 satisfied via D-18/D-19 split
- INST-05 enforced at boot
- D-13 auto-migrate on serve
- AUTH-06 enforced (admin-required endpoints wrapped with RequireAction)
- PITFALL #4 prevented (SPA last)
- shifter serve is the canonical binary entry point — install + dev + prod all use it
</success_criteria>

<output>
After completion, create `.planning/phases/01-foundation/01-18-SUMMARY.md` documenting:
- Route table
- Middleware order
- Deps struct contract
- INST-05 startup gate
- Plan 19 (SPA embed) integration point
</output>
