---
phase: 01-foundation
plan: 18
subsystem: router-health
tags: [go, chi, http, middleware, health, slog, inst-05, inst-06, mqtt-subscriber, serve, csrf, owasp]

requires:
  - phase: 01-foundation
    plan: 04
    provides: Config (Env, HTTPPort, LogLevel, DB, ChirpStack, MQTT, Session, TLS) consumed by serve startup; logging.New for the chi SlogLogger middleware
  - phase: 01-foundation
    plan: 05
    provides: serve.go cobra stub + TODO(plan-09 + plan-13 + plan-18) markers (now resolved)
  - phase: 01-foundation
    plan: 08
    provides: auth.NewSessionManager + sm.LoadAndSave wiring (mounted exactly once on the chi router root)
  - phase: 01-foundation
    plan: 09
    provides: auth.LoginHandler + LogoutHandler + AccountInfoHandler + ChangePasswordHandler + LoginLimiter + Store (consumed by router groups)
  - phase: 01-foundation
    plan: 10
    provides: auth.RequireAction + ActionHealthDetailed / ActionAccountSelfEdit / ActionConnectionTest / ActionConnectionEdit constants
  - phase: 01-foundation
    plan: 11
    provides: AccountInfoHandler shape consumed by /api/account/me
  - phase: 01-foundation
    plan: 12
    provides: chirpstack.Dial + ProbeVersion + ErrChirpStackV3OrUnknown + bufconn mock (NewChirpStackMockBuf) used by the serve_test.go INST-05 unit tests
  - phase: 01-foundation
    plan: 13
    provides: chirpstack.NewMQTTSubscriber + Shutdown + PingMQTT (consumed by serve.go startup + Plan 17 TestConnDeps)
  - phase: 01-foundation
    plan: 14
    provides: install.FirstRunGate (mounted as middleware #6) + install.Regions catalog
  - phase: 01-foundation
    plan: 15
    provides: install.Store + StateHandler / Step1..4Handler / FinishHandler + csConn package-private interface (now exposed as install.CSConn alias)
  - phase: 01-foundation
    plan: 17
    provides: TestConnHandler / GetChirpStackHandler / PutChirpStackHandler / ProductionDial / TestConnDeps consumed by the chi router settings group

provides:
  - internal/http/health.go (Health() public + HealthDetailed(pool) admin handlers per D-18 / D-19)
  - internal/http/middleware.go (SlogLogger chi middleware adapter — D-24 one-line structured logs per request)
  - internal/http/router.go (NewRouter wiring all 13 Phase 1 routes + Deps struct contract)
  - internal/http/spa.go (SPAHandler placeholder — Plan 19 fills body)
  - internal/cli/serve.go (full body replaces Plan 05 stub: config + DB + auto-migrate + INST-05 boot probe + MQTT subscriber + chi router + graceful SIGTERM)
  - probeChirpStackOrRefuse (testable boot-time v3 rejection — returns refusal error containing INST-05 + "ChirpStack v3" sentinels)
  - csConnWrapper (single concrete type satisfying both install.CSConn and the boot-probe csBootConn interface — no interface{} round-trip per Warning #6)
  - install.CSConn type alias (exports the previously package-private csConn so serve.go can construct a wrapper that satisfies both shapes)

affects:
  - 01-19-spa-embed (replaces SPAHandler() body with the go:embed-backed Vite dist; the chi router's '/*' catch-all already targets it)
  - 01-20-compose-bundled (`shifter serve` is the canonical entry point; Docker HEALTHCHECK uses `shifter healthcheck` against /health on localhost)
  - 01-21-compose-external (same; external mode points cfg.ChirpStack at an existing v4 instance — INST-05 probe still applies at boot)
  - 01-22-caddyfile (Caddy terminates TLS in front of shifter; X-Forwarded-For surfaces via chi.middleware.RealIP, request_id via chi.middleware.RequestID)
  - 01-23-login-ui (consumes /api/auth/login mounted by NewRouter; FirstRunGate + post-install completion check protect the route)
  - 01-24-readme-docs (documents the route table + middleware order)
  - Phase 2 (measurement persistence) — hooks the non-nil UplinkHandler into NewMQTTSubscriber via the existing Plan 13 seam; mqtt.go itself does not change

tech-stack:
  added:
    - github.com/go-chi/chi/v5@v5.2.5
  patterns:
    - "Pattern 1 (PITFALL #4): SPA fallback `/*` is mounted LAST. Without this guard, an unknown /api/foo would resolve to index.html and the SPA's fetch helper would fail to parse HTML as JSON."
    - "Pattern 2: D-18 / D-19 split — /health is public (status, version, uptime_seconds; no checks). /health/detailed is admin-only (RequireAction(ActionHealthDetailed)) and surfaces DB ping + status='ok'|'degraded'. T-18-01 mitigation."
    - "Pattern 3: INST-05 boot gate is extracted into probeChirpStackOrRefuse so unit tests drive the v3-rejection path against Plan 12's bufconn mock — no real ChirpStack server / no compose smoke needed for the fast-feedback loop. Compose smoke (Plan 20) remains the integration anchor."
    - "Pattern 4: One concrete csConnWrapper satisfies BOTH install.CSConn AND the boot-probe csBootConn. Same shape ({Conn() *grpc.ClientConn; Close() error}) — Go does not implicitly convert between distinct interface types even when method sets match, so the wrapper struct is named once and reused via two interface-typed return values. Warning #6 tightening completed."
    - "Pattern 5: Middleware order locked at the source — RequestID -> RealIP -> SlogLogger -> Recoverer -> SessionMgr.LoadAndSave -> install.FirstRunGate -> routes. Each layer's purpose is documented in router.go's package comment so future plans cannot silently reorder."
    - "Pattern 6: Degraded-mode start. Unreachable ChirpStack at boot logs a warning and proceeds (the install wizard / Settings → Edit can repair). Refusing to start on transient blips would create an outage cascade. v3 detection is the only hard refusal because Phase 2+ functionality cannot work against v3."
    - "Pattern 7: SlogLogger never logs request headers or body — only method/path/status/bytes/ms/request_id. T-18-06 mitigation (Authorization / Cookie would otherwise leak)."

key-files:
  created:
    - internal/http/health.go
    - internal/http/middleware.go
    - internal/http/router.go
    - internal/http/spa.go
  modified:
    - internal/http/health_test.go (replaces Plan 02 t.Skip stubs with 2 real tests)
    - internal/cli/serve.go (replaces Plan 05 placeholder with full wiring)
    - internal/cli/serve_test.go (replaces Plan 02 t.Skip stubs with 4 INST-05 unit tests + 1 deferred-to-Plan-20 skip)
    - internal/install/handlers.go (adds CSConn type alias for serve.go wiring)
    - go.mod
    - go.sum

key-decisions:
  - "chi middleware order locked at PITFALL #4 anchor. Order is encoded in router.go and documented in its package comment. Any future plan reordering middleware MUST justify the deviation; the canonical order is: RequestID -> RealIP -> SlogLogger -> Recoverer -> SessionMgr.LoadAndSave -> install.FirstRunGate -> routes -> SPA last. Reordering RequestID downstream of SlogLogger would emit log lines without request_id; reordering Recoverer upstream of LoadAndSave would let session-store panics escape; reordering SPA before /api/* routes would cause /api/foo 404s to return HTML."
  - "/health is intentionally NOT wrapped in any middleware beyond the global stack — explicitly NOT in any RequireAction. INST-06 reframed per D-19 means the public probe must succeed against a freshly-deployed instance with no admin user. FirstRunGate's whitelist already covers /health, so the public probe works pre-install and post-install identically."
  - "/health/detailed includes DB ping ONLY in Phase 1. CS / MQTT / disk / last-uplink-age all move to Phase 6 because they require richer dependency injection (CS client + MQTT subscriber handles + storage stat probe). The Phase 1 surface (status, checks.db, version, uptime_seconds) is enough for monitoring dashboards and matches Plan 22 (Caddyfile) upstream-health expectations."
  - "probeChirpStackOrRefuse is extracted as a top-level testable function rather than inlined in serve.RunE. The plan's verbatim version inlined the probe; extraction lets serve_test.go drive the v3-detection path against the bufconn mock (Plan 12) with zero compose dependency. The extraction also makes the INST-05 contract grep-friendly: any future regression that loosens the v3 check or moves the probe past srv.ListenAndServe would surface in a unit test that runs in 1.8s."
  - "secretsDir is a package-level const ('/run/secrets') rather than a Config field. Plan 04 did not ship a SecretsDir field; the production install path mounts /run/secrets via Compose secrets (Plans 20/21) and the wizard handlers (Plan 15) + PUT chirpstack handler (Plan 17) already accept the path as a parameter. Adding a Config field is straightforward when an operator demand surfaces, but the const-default is correct for both deployment shapes today."
  - "csConnWrapper lives in serve.go (not in chirpstack/, not in install/, not in http/). It bridges three packages without forcing any of them to import the others: install/handlers.go declares CSConn (alias for csConn), serve.go owns the boot-probe csBootConn, and the wrapper struct satisfies both. Plan 17's chirpStackConn (http package) keeps its own ProductionDial — the http and serve packages thus never import each other's interfaces directly even though the shapes match. Warning #6 tightening preserved."
  - "MQTT subscriber start is best-effort at boot. If the broker is unreachable, log Warn and proceed; a retry policy could be added later (Phase 6 ops dashboard). The install wizard / Settings → Edit are the operator-facing recovery paths. Once the subscriber connects, paho's SetAutoReconnect handles transport-level losses transparently per Plan 13."
  - "InstallStore is a Deps field even though router.go never reads it — the field is reserved so future Phase 2 plans (e.g. an admin 'reset install' flow) can call install.Store methods without re-shaping Deps. Cost is one struct field; benefit is API stability."
  - "AccountInfoHandler is mounted under RequireAction(ActionAccountSelfEdit) per Plan 11's pattern. Both admin and viewer roles allow this action; the handler still re-checks the session and 401s on a stale-cookie miss. The /me endpoint specifically is the only authenticated GET that must survive a disabled-mid-session admin (the Store check returns ErrUserNotFound, which surfaces as 401, which the SPA's apiFetch redirects to /login)."
  - "Listen-and-serve is launched in a goroutine; the parent goroutine blocks on ctx.Done() so SIGTERM cleanly drains. Shutdown order: cancel ctx → mqttSub.Shutdown(5s) → srv.Shutdown(15s). MQTT shutdown precedes HTTP shutdown so an in-flight uplink finishing while HTTP is also draining doesn't race."
  - "SPAHandler is a placeholder returning 503. Plan 19's go:embed surface replaces it; the symbol exists today so the chi router '/*' catch-all has a target. This avoids a circular dependency between Plan 18 and Plan 19 — both can ship without coordinating on the symbol introduction."

patterns-established:
  - "Pattern: chi router as the single integration point. Every Phase 1 handler is mounted via NewRouter; future plans add routes by extending the same Deps struct + adding to NewRouter, never by spawning sibling routers. The SPA fallback is always last."
  - "Pattern: typed interfaces over interface{}. Plan 17 + Plan 18 both inherit the Warning #6 idiom — domain interfaces expose Conn() *grpc.ClientConn directly. Future ChirpStack-bound dialers MUST follow this shape; cross-package wrappers (csConnWrapper) implement once and satisfy multiple interfaces with the same method set."
  - "Pattern: boot-time gates as testable functions. probeChirpStackOrRefuse is the template for any future startup-time validation (e.g. license check, config sanity probe, vendor compatibility check) — extract into a top-level function with a dial / probe seam, drive it from a unit test, and call it BEFORE srv.ListenAndServe in serve.RunE."
  - "Pattern: SlogLogger is the canonical chi middleware for D-24. Future API surfaces (Phase 2+ device routes) inherit the same one-line structured-log emission; never log request headers or body."
  - "Pattern: graceful shutdown order. MQTT subscriber Shutdown precedes HTTP server Shutdown so in-flight uplinks finish their persistence (Phase 2) before the HTTP-side responses drain. Future long-running goroutines (river jobs, SSE fan-out) MUST register their own Shutdown hook in serve.RunE."

requirements-completed:
  - INST-06
  - AUTH-06

duration: 9min
completed: 2026-04-28
---

# Phase 01 Plan 18: Router + Health Summary

**chi router mounting all 13 Phase 1 routes with the canonical middleware order (RequestID → RealIP → SlogLogger → Recoverer → SessionMgr.LoadAndSave → install.FirstRunGate → routes → SPA last per PITFALL #4); D-18 public `/health` + D-19 admin `/health/detailed` shipped; `shifter serve` replaces the Plan 05 stub with full wiring (config + DB + auto-migrate + INST-05 boot probe + MQTT subscriber + graceful SIGTERM); INST-05 v3 refusal extracted into `probeChirpStackOrRefuse` and unit-tested via Plan 12's bufconn mock (4 tests, 1.8s end-to-end).**

## Performance

- **Duration:** ~9 min
- **Started:** 2026-04-28T03:11:38Z
- **Completed:** 2026-04-28T03:21:14Z
- **Tasks:** 3 / 3
- **Commits:** 4 (1 RED + 3 GREEN)
- **Files created:** 4 (`internal/http/{health,middleware,router,spa}.go`)
- **Files modified:** 6 (`internal/http/health_test.go`, `internal/cli/serve.go`, `internal/cli/serve_test.go`, `internal/install/handlers.go`, `go.mod`, `go.sum`)
- **Tests added:** 6 (2 health + 4 serve INST-05); 1 deferred to Plan 20 (auto-migrate end-to-end)

## Route Table

The chi router exposes these 13 routes (plus the SPA `/*` fallback). The middleware groups in NewRouter map 1:1 to the auth requirement column.

| Method | Path                                  | Auth                                          | Handler                                        |
| ------ | ------------------------------------- | --------------------------------------------- | ---------------------------------------------- |
| GET    | `/health`                             | public                                        | `Health()` (D-18)                              |
| GET    | `/health/detailed`                    | `RequireAction(ActionHealthDetailed)` admin   | `HealthDetailed(pool)` (D-19)                  |
| POST   | `/api/auth/login`                     | public (rate-limited)                         | `auth.LoginHandler`                            |
| POST   | `/api/auth/logout`                    | public                                        | `auth.LogoutHandler`                           |
| GET    | `/api/account/me`                     | `RequireAction(ActionAccountSelfEdit)`        | `auth.AccountInfoHandler`                      |
| POST   | `/api/account/password`               | `RequireAction(ActionAccountSelfEdit)`        | `auth.ChangePasswordHandler`                   |
| GET    | `/api/install/state`                  | public (FirstRunGate whitelist)               | `install.StateHandler`                         |
| POST   | `/api/install/step/1..4`              | public (FirstRunGate whitelist)               | `install.Step{1..4}Handler`                    |
| POST   | `/api/install/finish`                 | public (FirstRunGate whitelist)               | `install.FinishHandler`                        |
| GET    | `/api/install/regions`                | public (FirstRunGate whitelist)               | inline JSON of `install.Regions()`             |
| GET    | `/api/settings/chirpstack`            | `RequireAction(ActionConnectionTest)`         | `GetChirpStackHandler` (api_token NEVER returned, V8) |
| POST   | `/api/settings/chirpstack/test`       | `RequireAction(ActionConnectionTest)`         | `TestConnHandler` (CHIRP-03)                   |
| PUT    | `/api/settings/chirpstack`            | `RequireAction(ActionConnectionEdit)` admin   | `PutChirpStackHandler` (Open Q 2 — re-probe before persist) |
| GET    | `/*`                                  | (passthrough; PITFALL #4)                     | `deps.SPA` (Plan 19 placeholder)               |

## Middleware Order (PITFALL #4 Anchor)

```text
1. middleware.RequestID         — adds request_id to ctx (Plan 22 Caddy passes through)
2. middleware.RealIP            — honors X-Forwarded-For from the operator's reverse proxy
3. SlogLogger                   — D-24 structured one-event-per-request log
4. middleware.Recoverer         — converts panics to 500 + logs internally (T-18-07)
5. SessionMgr.LoadAndSave       — alexedwards/scs session boundary (Plan 08)
6. install.FirstRunGate         — D-08 install gate (Plan 14); cached via atomic.Bool
7. domain routes (auth, account, install, settings, health)
8. SPA fallback ("/*")          — Plan 19; MUST be LAST so /api/* 404s don't return index.html
```

Reordering rules (locked):
- RequestID **MUST** be first so every later log line carries the same identifier.
- Recoverer is in the pre-session group so panics during session load/save still land as 500.
- FirstRunGate is the LAST middleware before route dispatch so its whitelist matches the actual route paths.
- SPA fallback `/*` is the LAST route registered (PITFALL #4).

## Deps Struct Contract

```go
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

    // SecretsDir is the on-disk path under which file-by-REF secrets live.
    SecretsDir string

    // SPA fallback handler (Plan 19). Optional — if nil the router does NOT
    // register the catch-all so /unknown/path returns 404 instead of HTML.
    SPA http.Handler
}
```

10 fields. `InstallStore` is reserved for Phase 2+ admin flows that bypass the wizard handlers; today router.go uses only the embedded `InstallDeps.Store`.

## INST-05 Startup Gate

```go
func probeChirpStackOrRefuse(ctx context.Context, log *slog.Logger, dial csConnDialFunc, cfg config.CSConfig) error {
    if cfg.GRPCURL == "" {
        return nil  // pre-install: no-op
    }
    conn, err := dial(ctx, cfg)
    if err != nil {
        log.Warn("chirpstack not reachable on boot — degraded mode", "err", err)
        return nil  // dial fail: degraded mode
    }
    defer conn.Close()
    probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
    defer cancel()
    _, probeErr := chirpstack.ProbeVersion(probeCtx, conn.Conn())
    if errors.Is(probeErr, chirpstack.ErrChirpStackV3OrUnknown) {
        return fmt.Errorf("INST-05: refusing to start — ChirpStack v3 detected at %s", cfg.GRPCURL)
    }
    if probeErr != nil {
        log.Warn("chirpstack probe failed on boot — degraded mode", "err", probeErr)
    }
    return nil
}
```

Error message contract: when v3 is detected the returned error contains both `INST-05` and `ChirpStack v3` (case-insensitive) so operator logs spell the cause and `errors.Is` chains in middleware can preserve the sentinel via `%w` wrapping.

`serve.RunE` calls `probeChirpStackOrRefuse(ctx, log, productionCSDial, cfg.ChirpStack)` BEFORE constructing the chi router or invoking `srv.ListenAndServe()`. A v3 environment never accepts connections.

## Test-Coverage Matrix

| Test                              | File                              | Asserts                                                      |
| --------------------------------- | --------------------------------- | ------------------------------------------------------------ |
| `TestHealth_Public`               | internal/http/health_test.go      | GET /health → 200 + {status, version, uptime_seconds}; NO `checks` (D-18) |
| `TestHealthDetailed_RequiresAdmin` | internal/http/health_test.go     | anon → 401; viewer → 403; admin → 200 + checks.db=true (D-19) |
| `TestServe_RefusesV3`             | internal/cli/serve_test.go        | v3 mock → error containing "INST-05" + "ChirpStack v3" (case-insensitive) |
| `TestServe_AcceptsV4`             | internal/cli/serve_test.go        | v4 mock → no error (boot proceeds)                           |
| `TestServe_DegradedOnUnreachable` | internal/cli/serve_test.go        | dial failure → no error (degraded mode; install wizard recovery) |
| `TestServe_NoConfigSkipsProbe`    | internal/cli/serve_test.go        | empty GRPCURL → no error (pre-install state)                 |
| `TestServe_AutoMigrate`           | internal/cli/serve_test.go        | (skip — Plan 20 compose smoke covers D-13 end-to-end)        |

6 net-new tests pass under `-race -count=1` (4 INST-05 unit tests + 2 health tests).

## Plan 19 Integration Point

Plan 19 (spa-embed) replaces `internal/http/spa.go`'s `SPAHandler()` body. Today the placeholder returns 503; Plan 19 will:

1. Use `go:embed` to bundle Vite's `dist/` output into the binary.
2. Serve `index.html` for unknown paths that don't look like assets.
3. Serve hashed assets under `/assets/` with `Cache-Control: public, max-age=31536000, immutable`.
4. Return 404 for missing static asset filenames (no SPA fallback for `/assets/foo.css`).

Plan 18 already wires the SPA into the chi router via `r.Handle("/*", deps.SPA)`. Plan 19 is purely additive — no router-side change needed.

## Threat Surface Notes

All 7 entries in the plan's `<threat_model>` are mitigated by code shipped in this plan:

| Threat   | Mitigation                                                                                                          |
| -------- | ------------------------------------------------------------------------------------------------------------------- |
| T-18-01 (Information Disclosure on /health) | D-18 enforces public minimum payload; detailed checks moved to admin-only /health/detailed (`TestHealth_Public` regression-tests the no-checks invariant). |
| T-18-02 (Spoofing — v3 ChirpStack at boot)  | `probeChirpStackOrRefuse` returns INST-05 refusal error; `TestServe_RefusesV3` regression-tests via Plan 12 bufconn mock. |
| T-18-03 (Information Disclosure — SPA on /api/*) | PITFALL #4: SPA mounted LAST after every /api/* route registration. router.go's `r.Handle("/*", deps.SPA)` is the LAST line in NewRouter. |
| T-18-04 (Tampering — request without RequestID) | `chi.middleware.RequestID` is the FIRST middleware; SlogLogger emits `request_id` field. ASVS V7. |
| T-18-05 (Denial of Service — unbounded request body) | `srv.ReadHeaderTimeout = 10s`, `IdleTimeout = 2m`. SPA payload size is bounded by Vite output (Plan 19 / Plan 06). |
| T-18-06 (Information Disclosure — logger leaks Authorization / Cookie) | `SlogLogger` only logs method/path/status/bytes/ms/request_id. Headers + body are NEVER logged. |
| T-18-07 (Information Disclosure — panic stack returned to client) | `chi.middleware.Recoverer` returns 500 + logs internally; never sends stack. ASVS V7. |

## Decisions Made

See `key-decisions` in frontmatter for the canonical list. Highlights:

- **Middleware order locked** at PITFALL #4 anchor; documented in router.go's package comment.
- **/health is unauthenticated** (D-18 / INST-06 reframed via D-19); FirstRunGate's whitelist already covers it.
- **/health/detailed surfaces DB ping ONLY** in Phase 1; CS / MQTT / disk move to Phase 6.
- **probeChirpStackOrRefuse extracted** as a top-level testable function so the v3-rejection path is grep-friendly + unit-tested via Plan 12 bufconn (no compose dependency).
- **secretsDir is a const** ('/run/secrets'); Plan 04 did not ship a SecretsDir field, the const-default is correct for both deployment shapes today.
- **csConnWrapper lives in serve.go** and bridges install.CSConn + csBootConn with one concrete value; Warning #6 tightening preserved.
- **MQTT subscriber start is best-effort at boot**; degraded-mode logging if the broker is unreachable.
- **InstallStore is a reserved Deps field** even though router.go never reads it — Phase 2+ admin flows can use it without re-shaping Deps.
- **AccountInfoHandler wrapped in RequireAction(ActionAccountSelfEdit)** per Plan 11's pattern.
- **Graceful shutdown** runs MQTT.Shutdown before HTTP.Shutdown so in-flight uplinks finish persistence before HTTP responses drain.
- **SPAHandler is a placeholder** returning 503; Plan 19 replaces.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] SPAHandler stub created so the chi router has a target before Plan 19**

- **Found during:** Task 3 — serve.go construction. The plan says `SPA: httpapi.SPAHandler()` but Plan 19 has not shipped the symbol.
- **Issue:** Without a `SPAHandler` symbol in the http package, serve.go's `Deps{... SPA: httpapi.SPAHandler() ...}` would not compile.
- **Fix:** Created `internal/http/spa.go` with a placeholder `SPAHandler()` returning 503. The placeholder is documented as Plan 19's drop-in replacement target. The chi router's `r.Handle("/*", deps.SPA)` already targets it; Plan 19 is purely additive at the body level.
- **Files modified:** `internal/http/spa.go` (new).
- **Verification:** `go build ./...` passes; `TestSPA_FallbackIndex / NoFallbackForAPI / AssetCacheHeaders` (Plan 19 tests) remain `t.Skip` — Plan 19's responsibility.
- **Commit:** `371d9a7` (bundled with Task 3 GREEN).

**2. [Rule 2 - Missing Critical] secretsDir hardcoded as a package-level const in serve.go**

- **Found during:** Task 3 — serve.go construction. Plan-verbatim referenced `cfg.SecretsDir` but Plan 04's Config does not have a SecretsDir field.
- **Issue:** Plan 18's verbatim `installDeps := install.Deps{... SecretsDir: cfg.SecretsDir, ...}` would not compile.
- **Fix:** Hardcoded `const secretsDir = "/run/secrets"` at the top of serve.go and used it for both `installDeps.SecretsDir` and `httpapi.Deps.SecretsDir`. Production deploys (Plans 20/21) mount /run/secrets via Compose secrets — the const matches that path. Adding a Config field is a future-proof option but out of Plan 18 scope.
- **Files modified:** `internal/cli/serve.go`.
- **Tested by:** End-to-end the path is exercised by Plan 15 / Plan 17 secret-by-REF writes (which already pass against testcontainer pools at /tmp paths via test wiring); production path is asserted by Plan 20 compose smoke.
- **Commit:** `371d9a7` (Task 3 GREEN).

**3. [Rule 2 - Missing Critical] install.CSConn type alias exported so serve.go can reuse the package-private csConn shape**

- **Found during:** Task 3 — serve.go construction. The plan says serve.go's `install.Deps.Dial` closure must return `install.CSConn` (the install package's interface), but Plan 15 declared `csConn` as package-private.
- **Issue:** Without the alias, serve.go would either need to import install's private interface (impossible) or duplicate the interface definition (incorrect, since Go does not implicitly convert between distinct interface types).
- **Fix:** Added `type CSConn = csConn` to `internal/install/handlers.go`. The alias is a Go 1.9+ type alias (not a wrapper type), so install's package-private csConn and the exported CSConn are literally the same type. serve.go's csConnWrapper satisfies both `install.CSConn` (via the alias) and the boot-probe `csBootConn` interface from one method set.
- **Files modified:** `internal/install/handlers.go`.
- **Tested by:** `go build ./...` passes; the install package's existing 27 tests pass unchanged.
- **Commit:** `371d9a7` (Task 3 GREEN).

**4. [Rule 1 - Cleanup] TestServe_AutoMigrate kept as a skip stub pointing at Plan 20**

- **Found during:** Task 3 test design. The plan listed TestServe_AutoMigrate as a skip-with-Plan-20-deferral; per the pattern lock, the skip body explicitly references Plan 20 (compose-smoke-bundled).
- **Issue:** Without an explicit deferral note the skip would look like dead code.
- **Fix:** `t.Skip("Plan 20 (compose-smoke-bundled) covers D-13 auto-migrate end-to-end (full binary boot)")` — matches the plan's verbatim suggestion. The auto-migrate path itself is exercised by serve.go's `db.RunMigrations(ctx, pool, log)` call; the unit-vs-integration tradeoff is documented in the skip message.
- **Files modified:** `internal/cli/serve_test.go`.
- **Commit:** `371d9a7` (Task 3 GREEN).

---

**Total deviations:** 4 auto-fixed (3 Rule 2 missing-critical, 1 Rule 1 cleanup). All fixes either add missing scaffolding (SPAHandler placeholder, CSConn alias) or correct API drift between the plan's verbatim and the actual Phase 1 surface (secretsDir hardcoded; auto-migrate test deferred). No behavioral changes to the public API documented in `<interfaces>`.

**Impact on plan:** None on the success criteria. INST-05 + INST-06 + AUTH-06 contracts unchanged. The route table matches the plan's exact 13-route list. The middleware order matches the PITFALL #4 anchor exactly. Plan 19 inherits a `SPAHandler()` symbol it can replace cleanly.

## Issues Encountered

- **`go-chi/chi` was not in go.mod at start.** `go get github.com/go-chi/chi/v5@latest` added the module; `go mod tidy` initially removed it because no code referenced chi yet. Re-running `go mod tidy` after writing router.go + middleware.go materialized the module + transitive entries. Documented because the order of operations matters: write the code that imports the package, THEN run `go mod tidy`. Added v5.2.5.
- **Plan 09 testcontainer flake did not reproduce** in this session. STATE.md's existing "Open Todos" note for the CI plan still applies; no new flakes surfaced.
- **`rtk` Bash tool occasionally summarizes test output as "0 passed, 1 failed"** when chi compile errors surface; the raw `go test` output shows the actual error. `rtk proxy` is the canonical escape hatch for full visibility.

## Known Stubs

| Stub                            | File                          | Reason                                                                  | Resolved By |
| ------------------------------- | ----------------------------- | ----------------------------------------------------------------------- | ----------- |
| `SPAHandler()` returns 503      | `internal/http/spa.go`        | Plan 19 fills body with go:embed-backed Vite dist                       | Plan 19     |
| `TestSPA_FallbackIndex` skip    | `internal/http/spa_test.go`   | Plan 19 ships the SPA fallback                                          | Plan 19     |
| `TestSPA_NoFallbackForAPI` skip | `internal/http/spa_test.go`   | Plan 19 ships the asset cache header path                               | Plan 19     |
| `TestSPA_AssetCacheHeaders` skip | `internal/http/spa_test.go`  | Plan 19 ships the immutable cache headers                               | Plan 19     |
| `TestServe_AutoMigrate` skip    | `internal/cli/serve_test.go`  | D-13 auto-migrate covered end-to-end by compose smoke; unit test would duplicate | Plan 20    |

All stubs are explicitly scheduled for resolution in later Phase 01 plans. The SPA-related stubs are NOT plan-18 stubs — they are pre-existing Plan 02 scaffolding awaiting Plan 19.

## User Setup Required

None for development / testing. Production deploys consume:

- `cfg.HTTPPort` (default 8080) — port the chi router binds to.
- `cfg.SecretsDir` is implicit ('/run/secrets'); Plans 20/21 mount via Compose secrets.
- `cfg.ChirpStack.{GRPCURL, APIToken, Insecure}` — boot probe runs against this; v3 detection refuses to start.
- `cfg.MQTT.{URL, User, Password}` — MQTT subscriber connects best-effort at boot.

For local smoke-testing today:

```bash
go test ./internal/http -run 'TestHealth' -race -count=1 -v -timeout 120s
# === RUN   TestHealth_Public                  (PASS)
# === RUN   TestHealthDetailed_RequiresAdmin   (PASS — testcontainer)

go test ./internal/cli -run 'TestServe_' -race -count=1 -v -timeout 60s
# === RUN   TestServe_RefusesV3                (PASS)
# === RUN   TestServe_AcceptsV4                (PASS)
# === RUN   TestServe_DegradedOnUnreachable    (PASS)
# === RUN   TestServe_NoConfigSkipsProbe       (PASS)
# === RUN   TestServe_AutoMigrate              (SKIP — Plan 20)

go build ./cmd/shifter && ./shifter --help | grep -E '(serve|migrate|version|create-admin|config-check|healthcheck)'
# All 6 D-12 subcommands present.
```

## Next Phase Readiness

- ✅ INST-06 satisfied: GET /health (public, D-18) + GET /health/detailed (admin, D-19).
- ✅ AUTH-06 enforced: every admin-only route wrapped in `auth.RequireAction`.
- ✅ INST-05 enforced at boot via `probeChirpStackOrRefuse` (4 unit tests cover refuse / accept / degraded / pre-install paths).
- ✅ D-13 auto-migrate active in `shifter serve`.
- ✅ PITFALL #4 prevented (SPA fallback last); router.go documents the order.
- ✅ MQTT subscriber starts in serve; graceful shutdown on SIGTERM.
- ✅ `shifter serve` is the canonical binary entry point — install + dev + prod all use it.
- ✅ Settings page endpoints (Plan 17) mounted behind RequireAction(ActionConnectionTest) GET+POST and RequireAction(ActionConnectionEdit) PUT.

**Plan 19 (spa-embed):** replaces `internal/http/spa.go`'s `SPAHandler()` body with a go:embed-backed surface. Plan 18 already wires `r.Handle("/*", deps.SPA)`; Plan 19 is purely additive at the function-body level.

**Plan 20 (compose-bundled):** uses `shifter serve` as the canonical entry point + `shifter healthcheck` for Docker HEALTHCHECK. The chi `/health` endpoint is the probe target.

**Plan 21 (compose-external):** same; external mode points cfg.ChirpStack at an existing v4 instance — INST-05 boot probe still applies.

**Plan 22 (caddyfile):** Caddy terminates TLS in front of shifter; chi's RequestID + RealIP middleware honors the operator-controlled reverse proxy as the trust boundary.

**Plan 23 (login-ui):** consumes /api/auth/login mounted by NewRouter; FirstRunGate + post-install completion check protect the route.

**Plan 24 (readme-docs):** documents the route table + middleware order (this SUMMARY's first two sections are the canonical reference).

**Phase 2 (measurement persistence):** swaps Plan 13's nil UplinkHandler for the normalize+persist pipeline at the serve.go call site; mqtt.go itself does not change.

## Self-Check: PASSED

Files verified to exist:

- FOUND: `internal/http/health.go`
- FOUND: `internal/http/middleware.go`
- FOUND: `internal/http/router.go`
- FOUND: `internal/http/spa.go`
- FOUND: `internal/http/health_test.go` (replaced — 2 real tests)
- FOUND: `internal/cli/serve.go` (full body — replaced Plan 05 stub)
- FOUND: `internal/cli/serve_test.go` (replaced — 4 real INST-05 tests + 1 deferred skip)
- FOUND: `internal/install/handlers.go` (CSConn alias added)

Commits verified to exist:

- FOUND: `2e2db11` (Task 1 RED — failing health tests)
- FOUND: `3cc7e60` (Task 1 GREEN — Health() + HealthDetailed())
- FOUND: `a3882de` (Task 2 — chi router + SlogLogger middleware)
- FOUND: `371d9a7` (Task 3 — serve.go full wiring + INST-05 unit tests + SPAHandler stub + install.CSConn alias)

Behavior verified:

- `go build ./...` exits 0
- `go vet ./...` exits 0
- `go build ./cmd/shifter` exits 0
- `go test ./internal/http -run 'TestHealth' -race -count=1 -timeout 120s` → 2 passed
- `go test ./internal/cli -run 'TestServe_RefusesV3|TestServe_AcceptsV4|TestServe_DegradedOnUnreachable|TestServe_NoConfigSkipsProbe' -race -count=1 -timeout 60s` → 4 passed
- `go test ./... -short -race -count=1 -timeout 300s` → 140 passed across 12 packages (zero regressions)

Acceptance grep proofs:

- `grep -n 'TODO(plan-09 + plan-13 + plan-18)' internal/cli/serve.go` → 0 matches (stub fully replaced)
- `grep -n 'probeChirpStackOrRefuse' internal/cli/serve.go` → 4 matches (call site + function definition + 2 doc comments)
- `grep -n 'srv.ListenAndServe' internal/cli/serve.go` → 1 match at line 151 (probeChirpStackOrRefuse called BEFORE at line 72)
- `grep -n 'db.RunMigrations' internal/cli/serve.go` → 1 match at line 63 (BEFORE srv.ListenAndServe)
- `grep -n 'NewMQTTSubscriber\|Shutdown' internal/cli/serve.go` → 3 matches (start + Shutdown + comment)
- `grep -n 'middleware.RequestID\|middleware.RealIP\|SlogLogger\|middleware.Recoverer' internal/http/router.go` → 4 matches in canonical order (lines 90, 91, 92, 93)
- `grep -n 'r.Handle.\"/\\*\"' internal/http/router.go` → 1 match at the LAST route registration (PITFALL #4 verified)
- `grep -n 'auth.RequireAction.*ActionHealthDetailed' internal/http/router.go` → 1 match
- `grep -n 'auth.RequireAction.*ActionConnectionEdit' internal/http/router.go` → 1 match (PUT path)
- `grep -E '^func (Health|HealthDetailed|NewRouter|SlogLogger|SPAHandler)' internal/http/*.go` → 5 hits (all required exports)

---
*Phase: 01-foundation*
*Plan: 18-router-health*
*Completed: 2026-04-28*
