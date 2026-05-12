---
phase: 05-aggregates-reports-map-floor-plans
plan: 13
type: execute
wave: 7
gap_closure: true
depends_on: [05-01, 05-04, 05-05, 05-06, 05-07, 05-12]
files_modified:
  - internal/http/router.go
  - internal/http/rbac_test.go
  - internal/config/config.go
  - internal/cli/serve.go
  - internal/report/handlers.go
  - internal/report/pdf_worker.go
  - internal/report/handlers_test.go
  - .planning/REQUIREMENTS.md
autonomous: true
requirements: [MAP-01, MAP-02, MAP-03, MAP-04, SITE-02, SITE-03, SITE-04, SITE-05, SITE-06, REPT-02]
requirements_addressed: [MAP-01, MAP-02, MAP-03, MAP-04, SITE-02, SITE-03, SITE-04, SITE-05, SITE-06, REPT-02]
evidence_for:
  - gap_1_map_router_wiring
  - gap_2_floor_plan_router_wiring
  - gap_3_report_capabilities_hardcoded
  - gap_4_requirements_evidence_migration_numbers

must_haves:
  truths:
    - "GET /api/map/data returns 200 (not 404) when called against the production router with valid auth"
    - "GET /api/sites/{siteID}/floor-plans returns 200/401 (not 404) when called against the production router"
    - "POST /api/reports/generate sets cfg.Capabilities from install_identity.capabilities, not hardcoded 'both'"
    - "PDF worker (planRowToConfig) sets ReportConfig.Capabilities from install_identity.capabilities, not hardcoded 'both'"
    - "REQUIREMENTS.md Phase 5 evidence trail references migration filenames that exist on disk"
  artifacts:
    - path: "internal/http/router.go"
      provides: "mapapi.RegisterRoutes + floorplan.RegisterRoutes guard-mounts after report.RegisterRoutes"
      contains: "mapapi.RegisterRoutes"
    - path: "internal/http/router.go"
      provides: "Deps.MapDeps + Deps.FloorPlanDeps fields"
      contains: "MapDeps"
    - path: "internal/cli/serve.go"
      provides: "MapDeps + FloorPlanDeps construction passed into httpapi.Deps"
      contains: "MapDeps:"
    - path: "internal/cli/serve.go"
      provides: "ImageRoot resolved from cfg.FloorPlanRoot (with sane default)"
      contains: "FloorPlanRoot"
    - path: "internal/config/config.go"
      provides: "FloorPlanRoot configuration field"
      contains: "FloorPlanRoot"
    - path: "internal/report/handlers.go"
      provides: "cfg.Capabilities derived from install_identity.capabilities loaded at handler time"
      excludes: "cfg.Capabilities = \"both\""
    - path: "internal/report/pdf_worker.go"
      provides: "planRowToConfig accepts capabilities parameter"
      excludes: "Capabilities:    \"both\""
    - path: ".planning/REQUIREMENTS.md"
      provides: "Phase 5 evidence trail with on-disk migration filenames"
      contains: "0029_retention_config.up.sql"
  key_links:
    - from: "internal/http/router.go"
      to: "internal/map (mapapi.RegisterRoutes)"
      via: "guarded RegisterRoutes call when MapDeps non-nil"
      pattern: "mapapi.RegisterRoutes"
    - from: "internal/http/router.go"
      to: "internal/floorplan (floorplan.RegisterRoutes)"
      via: "guarded RegisterRoutes call when FloorPlanDeps non-nil"
      pattern: "floorplan.RegisterRoutes"
    - from: "internal/cli/serve.go"
      to: "httpapi.Deps.MapDeps"
      via: "&mapapi.Deps{Pool, Logger, SessionMgr}"
      pattern: "MapDeps: "
    - from: "internal/cli/serve.go"
      to: "httpapi.Deps.FloorPlanDeps"
      via: "&floorplan.Deps{Pool, Queries, SessionMgr, ImageRoot}"
      pattern: "FloorPlanDeps: "
    - from: "internal/report/handlers.go GenerateHandler"
      to: "install_identity.capabilities"
      via: "q.GetCapabilities(ctx) (or GetInstallIdentity → identity.Capabilities)"
      pattern: "cfg.Capabilities = "
---

<objective>
Close the four verification gaps surfaced by /gsd-verify-work on Phase 5 so that map and floor-plan endpoints reach the production router, the report handler honors real install capabilities, and the REQUIREMENTS.md evidence trail cites migration filenames that exist on disk.

Purpose: 16/20 verifier truths shipped; this plan closes the four remaining gaps so MAP-01..04 and SITE-02..06 are reachable end-to-end (currently 404 in production) and REPT-02 capability gating is correct for single-capability installs.

Output:
- Map endpoints reachable: GET /api/map/data returns auth-gated JSON instead of 404
- Floor-plan endpoints reachable: 12 routes under /api/sites/{id}/floor-plans + /api/floor-plans/* mount in production
- Report capability gating correct: water-only install no longer shows electricity sections (handler + worker paths)
- Evidence trail accurate: REQUIREMENTS.md migration filenames match disk
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/STATE.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-VERIFICATION.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md

# Existing production wiring — executor MUST read before changes
@internal/http/router.go
@internal/cli/serve.go
@internal/map/routes.go
@internal/map/handler.go
@internal/floorplan/routes.go
@internal/floorplan/handlers.go
@internal/report/handlers.go
@internal/report/pdf_worker.go
@internal/config/config.go
@internal/http/rbac_test.go

<interfaces>
<!-- Key types and shapes the executor needs — extracted from the codebase. -->
<!-- Use these directly. Do NOT explore the codebase to "find the right shape". -->

From internal/map/handler.go (mapapi package):
```go
type Deps struct {
    Pool       *pgxpool.Pool
    Logger     *slog.Logger
    SessionMgr *scs.SessionManager
}

// RegisterRoutes(r chi.Router, deps Deps) mounts GET /api/map/data under
// auth.RequireAction(ActionSiteRead).
```

From internal/floorplan/handlers.go (floorplan package):
```go
type Deps struct {
    Pool       *pgxpool.Pool
    Queries    *sqlc.Queries
    SessionMgr *scs.SessionManager
    // ImageRoot is the absolute path to the floor-plans volume on disk
    // (e.g. /var/lib/shifter/floor-plans, or t.TempDir() in tests).
    ImageRoot  string
}

// RegisterRoutes(r chi.Router, deps Deps) mounts 12 floor-plan routes
// (upload, list, get, image-serve, replace, rename, delete, placement
// CRUD) under auth groups by action: site.read / site.create / site.update
// / site.archive.
```

From internal/http/router.go (httpapi package — current shape, MUST be extended):
```go
type Deps struct {
    Pool       *pgxpool.Pool
    SessionMgr *scs.SessionManager
    Log        *slog.Logger
    // ... existing fields (LoginLimiter, UserStore, InstallStore, InstallDeps,
    //                     TestConnDeps, SecretsDir, DeviceDeps, SwapDeps,
    //                     ProfileDeps, GatewayDeps, ImportDeps,
    //                     EventsDeps, DashboardDeps, ReportDeps, SettingsDeps,
    //                     SPA)

    // ↓ ADD these two fields (after SettingsDeps, before SPA) ↓
    MapDeps       *mapapi.Deps     // nil-tolerated; Plan 05-13 wiring
    FloorPlanDeps *floorplan.Deps  // nil-tolerated; Plan 05-13 wiring
}
```

From internal/db/sqlc/models.go:
```go
type InstallIdentity struct {
    ID           int32
    DisplayName  string
    LogoPath     *string
    Address      *string
    Timezone     string
    Units        UnitsSystem
    CreatedAt    pgtype.Timestamptz
    UpdatedAt    pgtype.Timestamptz
    Capabilities string  // <-- "water" | "electricity" | "both"
}
```

From internal/db/sqlc/dashboard.sql.go:
```go
// GetCapabilities :one
//   SELECT capabilities FROM install_identity WHERE id = 1
func (q *Queries) GetCapabilities(ctx context.Context) (string, error)

// GetInstallIdentity :one
//   SELECT id, display_name, logo_path, address, timezone, units, ... capabilities
//   FROM install_identity WHERE id = 1
func (q *Queries) GetInstallIdentity(ctx context.Context) (InstallIdentity, error)
```

From internal/report/handlers.go (current, line 188 is the bug):
```go
// Set capabilities from identity so the assembler filters correctly (D-09).
// The capabilities field would normally come from install_identity.capabilities;
// for now we use a default of "both" and plan 05-09 can wire the real value.
cfg.Capabilities = "both"  // <-- BUG: must use real install_identity.capabilities
```

From internal/report/pdf_worker.go planRowToConfig (current, line 156 is the worker-side bug):
```go
return ReportConfig{
    // ...
    Capabilities:    "both", // worker always generates full report; auth already checked at handler time
    // ↑ BUG: same as handler — must use real install_identity.capabilities
    Timezone:        tz,
}
```

From internal/config/config.go (current shape — needs FloorPlanRoot added):
```go
type Config struct {
    Env            string `mapstructure:"env"`
    HTTPPort       string `mapstructure:"http_port"`
    LogLevel       string `mapstructure:"log_level"`
    LogoStorageDir string `mapstructure:"logo_storage_dir"`
    ReportsRoot    string `mapstructure:"reports_root"`
    // ↓ ADD this field ↓
    FloorPlanRoot  string `mapstructure:"floor_plan_root"`
    // ... existing nested configs (DB, ChirpStack, MQTT, Session, TLS)
}
```

Default for FloorPlanRoot (mirrors LogoStorageDir + ReportsRoot pattern):
`v.SetDefault("floor_plan_root", "/var/lib/shifter/floor-plans")`

Existing wiring template in serve.go (lines 386-416) — current Deps construction
inside `httpapi.NewRouter(httpapi.Deps{...})`. The new fields go alongside
ReportDeps + SettingsDeps + SPA at the end of that struct literal.

Test pattern template (from internal/http/rbac_test.go lines 109-152 — verbatim):
```go
func TestRouter_<X>RouteMounted(t *testing.T) {
    pool := testsupport.StartPostgres(t)
    require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
    seedAdminForRouterTest(t, pool, "<x>-mount")
    sm := auth.NewSessionManager(pool, true, time.Hour, 24*time.Hour)
    logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

    // Deps non-nil → 401 from RequireAction (route reaches auth layer).
    depsWith<X> := Deps{
        Pool: pool, SessionMgr: sm, Log: logger,
        <X>Deps: &<pkg>.Deps{Pool: pool, SessionMgr: sm, /* ... */},
    }
    router := NewRouter(depsWith<X>)
    srv := httptest.NewServer(router); t.Cleanup(srv.Close)
    res, err := http.Get(srv.URL + "<path>")
    require.NoError(t, err); res.Body.Close()
    require.Equal(t, http.StatusUnauthorized, res.StatusCode,
        "GET <path> must reach RequireAction — 401 unauth (NOT 404 unmounted)")

    // Deps nil → 404 (route absent).
    depsNil := Deps{Pool: pool, SessionMgr: sm, Log: logger}
    router2 := NewRouter(depsNil)
    srv2 := httptest.NewServer(router2); t.Cleanup(srv2.Close)
    res2, err := http.Get(srv2.URL + "<path>")
    require.NoError(t, err); res2.Body.Close()
    require.Equal(t, http.StatusNotFound, res2.StatusCode,
        "nil <X>Deps must NOT mount routes — 404 expected")
}
```

Actual migration files on disk (verified `ls internal/db/migrations/`):
```
0025_cagg_hourly.up.sql
0026_cagg_daily.up.sql
0027_cagg_monthly.up.sql
0028_cagg_yearly.up.sql
0029_retention_config.up.sql        ← was cited as "0030_retention_config"
0030_report.up.sql
0031_audit_vocab_phase5.up.sql
0032_floor_plan.up.sql               ← was cited as "0031_floor_plan"
0033_device_floor_plan_placement.up.sql  ← was cited as "0032_placement"
```
</interfaces>

</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Wire mapapi + floorplan routes through the production router</name>
  <files>internal/http/router.go, internal/http/rbac_test.go</files>
  <read_first>
    - internal/http/router.go (full file — note Deps struct shape lines 51-130 and the report.RegisterRoutes / settings.RegisterRoutes guard pattern at lines 303-315)
    - internal/http/rbac_test.go lines 96-194 (seedAdminForRouterTest helper + the existing TestRouter_SwapRouteMounted / TestRouter_ProfileRouteMounted nil-guard tests — copy this pattern verbatim for the two new tests)
    - internal/map/routes.go (full — confirms mapapi package name + RegisterRoutes signature)
    - internal/map/handler.go lines 17-28 (Deps struct shape: Pool, Logger, SessionMgr)
    - internal/floorplan/routes.go (full — confirms 12-route mount pattern)
    - internal/floorplan/handlers.go lines 26-36 (Deps struct shape: Pool, Queries, SessionMgr, ImageRoot)
  </read_first>
  <behavior>
    Two new router-level integration tests in internal/http/rbac_test.go MUST be added BEFORE editing router.go (RED → GREEN). Both tests are mirrors of the existing TestRouter_ProfileRouteMounted pattern:

    - Test 1 (RED): TestRouter_MapRouteMounted
      - When `MapDeps` non-nil: GET http://srv/api/map/data → 401 (route reaches auth.RequireAction, no session cookie)
      - When `MapDeps` nil: GET http://srv/api/map/data → 404 (route NOT mounted)

    - Test 2 (RED): TestRouter_FloorPlanRouteMounted
      - When `FloorPlanDeps` non-nil: GET http://srv/api/sites/00000000-0000-0000-0000-000000000001/floor-plans → 401 (route reaches auth.RequireAction)
      - When `FloorPlanDeps` nil: GET http://srv/api/sites/00000000-0000-0000-0000-000000000001/floor-plans → 404 (route NOT mounted)

    Both tests reuse seedAdminForRouterTest (internal/http/rbac_test.go:96) so install.FirstRunGate is satisfied.

    Both tests MUST FAIL initially (route currently unmounted = 404 for both non-nil and nil cases). Then router.go edits make the non-nil branch return 401.

    Also extend TestRouter_NilDepsSafe (line 200) implicitly — adding MapDeps + FloorPlanDeps as nil-tolerated Deps fields means the existing nil-deps test continues to pass without change (zero-value struct fields are nil pointers, and the new guards skip RegisterRoutes calls).
  </behavior>
  <action>
    Step 1 — Add the two RED tests to internal/http/rbac_test.go (append at end of file):

    Required imports (add to existing import block, do not remove existing imports):
    ```go
    mapapi "github.com/shifter-io/shifter/internal/map"
    "github.com/shifter-io/shifter/internal/floorplan"
    ```

    Test bodies (verbatim — copy the seedAdminForRouterTest setup pattern from TestRouter_ProfileRouteMounted at lines 157-194):

    ```go
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
    ```

    Add a `sqlc` import alias if not already present in rbac_test.go:
    ```go
    sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
    ```

    RED commit: `git add internal/http/rbac_test.go && git commit -m "test(05-13): RED — TestRouter_MapRouteMounted + TestRouter_FloorPlanRouteMounted"`

    Run tests: `go test ./internal/http/ -run "TestRouter_(Map|FloorPlan)RouteMounted" -count=1` MUST report compile error (mapapi + floorplan imports unused fields MapDeps + FloorPlanDeps don't exist yet) OR test failure (depending on Go compiler — likely compile fail on unknown field). Either way, RED proven.

    Step 2 — Make tests GREEN by editing internal/http/router.go:

    A. Add imports to the existing import block (preserving sort order):
    ```go
    "github.com/shifter-io/shifter/internal/floorplan"
    mapapi "github.com/shifter-io/shifter/internal/map"
    ```

    B. In the Deps struct, AFTER the existing `SettingsDeps *settings.Deps` field (line 125) and BEFORE the `SPA http.Handler` field (line 129), add:
    ```go
        // MapDeps wires Plan 05-04's map data endpoint:
        //   GET /api/map/data — admin + viewer (auth.ActionSiteRead)
        // nil in early-boot / router unit tests that don't need the map route.
        // Plan 05-13 gap closure — Phase 5 verification gap 1.
        MapDeps *mapapi.Deps

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
    ```

    C. In NewRouter, AFTER the existing `settings.RegisterRoutes(...)` guard block (lines 309-315) and BEFORE the `// SPA fallback` comment (line 317), add:
    ```go
        if deps.MapDeps != nil {
            // MapDeps mounts Plan 05-04's GET /api/map/data endpoint.
            // Auth (admin + viewer via ActionSiteRead) enforced inside the
            // package's RegisterRoutes. Mounted before SPA fallback (PITFALL #4).
            // Plan 05-13 gap closure.
            mapapi.RegisterRoutes(r, *deps.MapDeps)
        }
        if deps.FloorPlanDeps != nil {
            // FloorPlanDeps mounts Plan 05-05 + 05-07's 12 floor-plan routes.
            // Auth groups (site.read / site.create / site.update / site.archive)
            // are applied inside the package's RegisterRoutes per route.
            // Mounted before SPA fallback (PITFALL #4).
            // Plan 05-13 gap closure.
            floorplan.RegisterRoutes(r, *deps.FloorPlanDeps)
        }
    ```

    Step 3 — Run `go test ./internal/http/ -run "TestRouter_(Map|FloorPlan)RouteMounted" -count=1`. MUST pass (GREEN).

    Step 4 — Run full http package tests to confirm no regression in existing nil-deps tests:
    `go test ./internal/http/... -race -count=1`.

    GREEN commit: `git add internal/http/router.go && git commit -m "feat(05-13): mount mapapi + floorplan routes in production router"`

    Why this exact ordering matters: report.RegisterRoutes (line 303-308) is already mounted before SPA fallback (line 320-322). Adding the two new RegisterRoutes calls in the same region preserves PITFALL #4 (SPA fallback ALWAYS last). Mounting AFTER settings (which is also Phase 5) keeps Phase 5 wiring contiguous and easy to grep.
  </action>
  <verify>
    <automated>go test ./internal/http/... -race -count=1</automated>
  </verify>
  <acceptance_criteria>
    - File `internal/http/router.go` contains `mapapi.RegisterRoutes` (exact string)
    - File `internal/http/router.go` contains `floorplan.RegisterRoutes` (exact string)
    - File `internal/http/router.go` contains `MapDeps *mapapi.Deps` (exact string)
    - File `internal/http/router.go` contains `FloorPlanDeps *floorplan.Deps` (exact string)
    - File `internal/http/router.go` contains `if deps.MapDeps != nil` AND `if deps.FloorPlanDeps != nil` (nil-guard pattern)
    - SPA fallback `r.Handle("/*", deps.SPA)` is LAST in NewRouter — `grep -n` shows mapapi.RegisterRoutes and floorplan.RegisterRoutes BEFORE the SPA Handle line
    - File `internal/http/rbac_test.go` contains `func TestRouter_MapRouteMounted` (exact string)
    - File `internal/http/rbac_test.go` contains `func TestRouter_FloorPlanRouteMounted` (exact string)
    - `go test ./internal/http/ -run "TestRouter_(Map|FloorPlan)RouteMounted" -count=1` exits 0
    - `go test ./internal/http/ -run "TestRouter_NilDepsSafe" -count=1` exits 0 (existing nil-deps test still passes)
    - `go build ./...` exits 0
    - `go vet ./internal/http/...` exits 0
  </acceptance_criteria>
  <done>
    GET /api/map/data returns 401 (auth required) instead of 404 when the router is built with non-nil MapDeps; GET /api/sites/{id}/floor-plans returns 401 instead of 404 when built with non-nil FloorPlanDeps. Two new router-level integration tests pin both behaviors and assert nil-deps still produces 404 (no leak). PITFALL #4 (SPA fallback last) preserved.
  </done>
</task>

<task type="auto">
  <name>Task 2: Construct MapDeps + FloorPlanDeps in serve.go; pass real install capabilities to report handler + worker</name>
  <files>internal/config/config.go, internal/cli/serve.go, internal/report/handlers.go, internal/report/pdf_worker.go, internal/report/handlers_test.go</files>
  <read_first>
    - internal/cli/serve.go (full file — note the existing Deps construction at lines 386-416, identityProvider at line 331 that loads install_identity for the PDF worker, and the `q := sqlc.New(pool)` already constructed at line 329)
    - internal/config/config.go lines 24-36 (Config struct) + lines 116-136 (viper defaults — `v.SetDefault("reports_root", "/var/lib/shifter/reports")` is the template for FloorPlanRoot)
    - internal/report/handlers.go lines 152-200 (GenerateHandler — line 188 is the hardcoded "both" assignment to fix; note `q := deps.Queries.WithTx(tx)` at line 205 is INSIDE the tx, but capability load happens BEFORE the tx so can use deps.Queries directly)
    - internal/report/pdf_worker.go (full — note Work() loads identity at line 74 via `w.Identity.Load(ctx)` which returns InstallIdentity{DisplayName, Address, Timezone}; capabilities is NOT currently in this struct — see Step C below)
    - internal/report/csv.go lines 10-25 (InstallIdentity struct definition — DisplayName/Address/Logo/Timezone/Units; capabilities is NOT here yet — Step C extends it)
    - internal/report/handlers_test.go lines 250-260 (TestGenerateHandler setup — note `Identity: InstallIdentity{...}` literal which will need the new Capabilities field if Step C adds it)
    - compose/bundled.yml + compose/external.yml (search for `floor_plans:` volume mount — the FloorPlanRoot default MUST match the in-container mount path which Plan 05-05 already established as `/var/lib/shifter/floor-plans`)
  </read_first>
  <action>
    Step A — Add FloorPlanRoot to config (internal/config/config.go):

    1. In the Config struct, add immediately after `ReportsRoot string \`mapstructure:"reports_root"\``:
    ```go
        FloorPlanRoot  string `mapstructure:"floor_plan_root"`
    ```

    2. In Load(), in the viper defaults block (immediately after `v.SetDefault("reports_root", "/var/lib/shifter/reports")` at line 121), add:
    ```go
        v.SetDefault("floor_plan_root", "/var/lib/shifter/floor-plans")
    ```

    The path `/var/lib/shifter/floor-plans` matches the `floor_plans` volume already declared in compose/bundled.yml and compose/external.yml (Plan 05-05 ship; verified by VERIFICATION.md truth 8).

    Step B — Construct MapDeps + FloorPlanDeps in serve.go and pass to httpapi.Deps:

    1. Add imports to internal/cli/serve.go (preserving alphabetic sort within group):
    ```go
        "github.com/shifter-io/shifter/internal/floorplan"
        mapapi "github.com/shifter-io/shifter/internal/map"
    ```

    2. Inside the `serveCmd.RunE` function, in the existing `httpapi.NewRouter(httpapi.Deps{...})` literal (lines 387-416), add the two new fields immediately AFTER `SettingsDeps: &settings.Deps{...}` (line 411-414) and BEFORE `SPA: httpapi.SPAHandler(),` (line 415):
    ```go
            MapDeps: &mapapi.Deps{
                Pool:       pool,
                Logger:     log.With("component", "mapapi"),
                SessionMgr: sm,
            },
            FloorPlanDeps: &floorplan.Deps{
                Pool:       pool,
                Queries:    q,
                SessionMgr: sm,
                ImageRoot:  cfg.FloorPlanRoot,
            },
    ```

    Note: `q := sqlc.New(pool)` is already constructed at serve.go line 329. `cfg.FloorPlanRoot` is read from viper (default `/var/lib/shifter/floor-plans`, overridable via SHIFTER_FLOOR_PLAN_ROOT env var).

    Step C — Add Capabilities to InstallIdentity + pass through handler + worker:

    1. In internal/report/csv.go, extend the InstallIdentity struct (around line 13) to add Capabilities:
    ```go
    type InstallIdentity struct {
        DisplayName  string
        Address      string
        LogoPath     string
        Timezone     *time.Location
        Units        string
        Capabilities string  // <-- NEW: "water" | "electricity" | "both"
    }
    ```

    The Capabilities field is consumed by ReportConfig.Capabilities which assembler.go and excel.go already gate on.

    2. In internal/report/handlers.go GenerateHandler, REPLACE the hardcoded block at lines 185-188:
    ```go
        // Set capabilities from identity so the assembler filters correctly (D-09).
        // The capabilities field would normally come from install_identity.capabilities;
        // for now we use a default of "both" and plan 05-09 can wire the real value.
        cfg.Capabilities = "both"
    ```
    WITH (load real capabilities from install_identity, gap 3 fix):
    ```go
        // D-09: capability gating from install_identity.capabilities.
        // Plan 05-13 gap closure — verifier gap 3. A water-only install must
        // NOT show electricity sections in reports; an electricity-only install
        // must NOT show water sections.
        capabilities, err := deps.Queries.GetCapabilities(ctx)
        if err != nil {
            writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "capabilities_load_failed"})
            return
        }
        cfg.Capabilities = capabilities
    ```

    Position this immediately before `// Build report synchronously from CAGGs` (currently line 190). GetCapabilities is the sqlc query defined in internal/db/sqlc/dashboard.sql.go line 127 (`SELECT capabilities FROM install_identity WHERE id = 1`) — it returns the singleton row's capability string.

    3. In internal/report/pdf_worker.go Work(), extend the identity load to also pass capabilities through to planRowToConfig.

    First, change the InstallIdentityProvider.Load contract — since InstallIdentity now has Capabilities (Step C.1), the existing sqlcIdentityProvider in serve.go already loads `display_name, address, timezone` but NOT capabilities. Extend it:

    In internal/cli/serve.go sqlcIdentityProvider.Load (around line 578-595), change the SQL and Scan:
    ```go
    func (p *sqlcIdentityProvider) Load(ctx context.Context) (report.InstallIdentity, error) {
        var displayName, address, tzName, capabilities string
        err := p.pool.QueryRow(ctx,
            `SELECT display_name, COALESCE(address, ''), timezone, capabilities FROM install_identity WHERE id = 1`,
        ).Scan(&displayName, &address, &tzName, &capabilities)
        if err != nil {
            return report.InstallIdentity{}, fmt.Errorf("load identity: %w", err)
        }
        tz, tzErr := time.LoadLocation(tzName)
        if tzErr != nil {
            tz = time.UTC
        }
        return report.InstallIdentity{
            DisplayName:  displayName,
            Address:      address,
            Timezone:     tz,
            Capabilities: capabilities,
        }, nil
    }
    ```

    4. In internal/report/pdf_worker.go planRowToConfig (line 118-159), change the signature to accept capabilities, and replace the hardcoded "both":

    Old (lines 118-159 — the function signature and body):
    ```go
    func planRowToConfig(plan sqlc.Report, tz *time.Location) ReportConfig {
        // ...
        return ReportConfig{
            // ...
            Capabilities:    "both", // worker always generates full report; auth already checked at handler time
            Timezone:        tz,
        }
    }
    ```

    New signature + body:
    ```go
    func planRowToConfig(plan sqlc.Report, identity InstallIdentity) ReportConfig {
        tz := identity.Timezone
        if tz == nil {
            tz = time.UTC
        }
        // ... (existing siteID/mpID/groupBy/start/end logic unchanged)
        return ReportConfig{
            // ... existing fields ...
            Capabilities: identity.Capabilities, // D-09; from install_identity (Plan 05-13 gap closure)
            Timezone:     tz,
        }
    }
    ```

    Update the call site inside Work() (line 80):
    ```go
        cfg := planRowToConfig(plan, identity)
    ```

    Step D — Update test fixtures broken by Step C.1 (the InstallIdentity struct gained a field):

    In internal/report/handlers_test.go around line 255, find the `Identity: InstallIdentity{DisplayName: "Test", ...}` literal and add the Capabilities field:
    ```go
        Identity: InstallIdentity{DisplayName: "Test", Timezone: time.UTC, Units: "metric", Capabilities: "both"},
    ```

    Also seed install_identity.capabilities in the test setup so the handler's q.GetCapabilities(ctx) call returns a real value. Search handlers_test.go for the existing test pool / migration setup. If TestGenerateHandler uses a real Postgres testcontainer (the rest of the report package does), insert a fixture row or rely on the migration's default. Migration 0022 sets `capabilities TEXT NOT NULL DEFAULT 'both'`, so a default-row install will already return "both" — but only if the install_identity row exists. Check the test: if it inserts an install_identity row, add `Capabilities: "both"` to the row; if it relies on migration-default empty state, INSERT a row before calling GenerateHandler.

    Concretely: if handlers_test.go's setup does NOT already insert install_identity, add this fixture before any GenerateHandler call:
    ```go
        _, err := pool.Exec(ctx, `INSERT INTO install_identity (id, display_name, timezone, units, capabilities) VALUES (1, 'Test', 'UTC', 'metric', 'both') ON CONFLICT (id) DO UPDATE SET capabilities = EXCLUDED.capabilities`)
        require.NoError(t, err)
    ```

    If the test already inserts a row, just ensure `capabilities` column is populated.

    Also check pdf_worker_test.go for any planRowToConfig direct calls — update the signature there too if present. Run `grep -n "planRowToConfig" internal/report/` to find all callers.

    Step E — Run the full suite:
    `go test ./internal/report/... ./internal/cli/... ./internal/config/... -race -count=1`
    `go build ./...`
    `go vet ./...`

    All MUST be green. If a test fails because it asserted `capabilities = "both"` in a way that conflicts with the new dynamic load, update the assertion to match the test's install_identity fixture row.

    Commit: `git add internal/config/config.go internal/cli/serve.go internal/report/handlers.go internal/report/pdf_worker.go internal/report/csv.go internal/report/handlers_test.go && git commit -m "feat(05-13): construct MapDeps+FloorPlanDeps; pass real install capabilities to report"`
  </action>
  <verify>
    <automated>go test ./internal/report/... ./internal/cli/... ./internal/config/... ./internal/http/... -race -count=1 && go build ./...</automated>
  </verify>
  <acceptance_criteria>
    - File `internal/config/config.go` contains `FloorPlanRoot` field on Config struct
    - File `internal/config/config.go` contains `v.SetDefault("floor_plan_root", "/var/lib/shifter/floor-plans")`
    - File `internal/cli/serve.go` contains `MapDeps: &mapapi.Deps{` (exact prefix)
    - File `internal/cli/serve.go` contains `FloorPlanDeps: &floorplan.Deps{` (exact prefix)
    - File `internal/cli/serve.go` contains `ImageRoot:  cfg.FloorPlanRoot` (or equivalent — no hardcoded path string)
    - File `internal/cli/serve.go` sqlcIdentityProvider.Load SQL contains `capabilities` column
    - File `internal/report/handlers.go` contains `deps.Queries.GetCapabilities(ctx)` (exact substring)
    - File `internal/report/handlers.go` does NOT contain `cfg.Capabilities = "both"` (the hardcoded line is gone — `grep -F 'cfg.Capabilities = "both"' internal/report/handlers.go` returns 0 matches)
    - File `internal/report/pdf_worker.go` does NOT contain `Capabilities:    "both"` (the hardcoded line in planRowToConfig is gone — `grep -E 'Capabilities:\s+"both"' internal/report/pdf_worker.go` returns 0 matches)
    - File `internal/report/pdf_worker.go` contains `identity.Capabilities` (substring) inside planRowToConfig
    - File `internal/report/csv.go` InstallIdentity struct contains `Capabilities` field
    - `go test ./internal/report/... ./internal/cli/... ./internal/config/... ./internal/http/... -race -count=1` exits 0
    - `go build ./...` exits 0
    - `go vet ./...` exits 0
  </acceptance_criteria>
  <done>
    Production serve.go constructs MapDeps + FloorPlanDeps and passes them to httpapi.NewRouter — both endpoint families now serve real traffic. cfg.FloorPlanRoot resolves to the compose-mounted volume path. GenerateHandler loads install_identity.capabilities and uses it; PDF worker's planRowToConfig accepts the loaded identity (with Capabilities) and propagates it. Water-only and electricity-only installs no longer leak the other utility's sections into reports.
  </done>
</task>

<task type="auto">
  <name>Task 3: Fix migration filename citations in REQUIREMENTS.md evidence trail</name>
  <files>.planning/REQUIREMENTS.md</files>
  <read_first>
    - .planning/REQUIREMENTS.md lines 388-409 (the Phase 5 closure evidence trail block — the three lines to fix are 391 (DATA-13), 403 (SITE-02), 405 (SITE-04), and 408 (SETT-04 also references the same retention migration as DATA-13))
    - `ls internal/db/migrations/002[5-9]_* internal/db/migrations/003[0-3]_*` to confirm actual filenames
  </read_first>
  <action>
    Edit `.planning/REQUIREMENTS.md` to correct three migration filename citations. The evidence trail block (lines 388-409) was written by Plan 05-12 using planned migration numbers, but the actual on-disk filenames differ because intervening migrations 0030_report and 0031_audit_vocab_phase5 shifted the floor-plan numbering up.

    Make these EXACT replacements (use sed-style substitutions or the Edit tool — each is a single-line change):

    1. Line 391 (DATA-13 evidence trail):
       - Find: `\`internal/db/migrations/0030_retention_config.up.sql\``
       - Replace with: `\`internal/db/migrations/0029_retention_config.up.sql\``

    2. Line 403 (SITE-02 evidence trail):
       - Find: `\`internal/db/migrations/0031_floor_plan.up.sql\``
       - Replace with: `\`internal/db/migrations/0032_floor_plan.up.sql\``

    3. Line 405 (SITE-04 evidence trail):
       - Find: `\`internal/db/migrations/0032_placement.up.sql\``
       - Replace with: `\`internal/db/migrations/0033_device_floor_plan_placement.up.sql\``

    4. Line 408 (SETT-04 evidence trail — references the same retention migration as DATA-13):
       - Find: `\`internal/db/migrations/0030_retention_config.up.sql\``
       - Replace with: `\`internal/db/migrations/0029_retention_config.up.sql\``

    Note: the migration NAMES change too (0033 is `device_floor_plan_placement` not `placement`), so be careful to use the full correct filename, not just bump the number.

    After the edits, also append a closure note at the end of the file (after line 408) referencing this plan:

    ```
    *Phase 5 gap-closure (Plan 05-13, 2026-05-12) — corrected three migration filename citations in the Phase 5 evidence trail to match on-disk filenames. The implementations were always at the correct paths; only the documentation pointers were stale (off-by-one to off-by-three due to intervening 0030_report + 0031_audit_vocab_phase5 migrations). DATA-13 + SETT-04 now cite `0029_retention_config.up.sql`; SITE-02 cites `0032_floor_plan.up.sql`; SITE-04 cites `0033_device_floor_plan_placement.up.sql`.*
    ```

    Commit: `git add .planning/REQUIREMENTS.md && git commit -m "docs(05-13): correct Phase 5 evidence trail migration filenames"`
  </action>
  <verify>
    <automated>grep -c "0029_retention_config" .planning/REQUIREMENTS.md && grep -c "0030_retention_config" .planning/REQUIREMENTS.md</automated>
  </verify>
  <acceptance_criteria>
    - `grep -c "0029_retention_config" .planning/REQUIREMENTS.md` returns ≥2 (DATA-13 + SETT-04 evidence trails)
    - `grep -c "0030_retention_config" .planning/REQUIREMENTS.md` returns 0 (stale citation gone)
    - `grep -c "0032_floor_plan" .planning/REQUIREMENTS.md` returns ≥1 (SITE-02 evidence corrected)
    - `grep -c "0031_floor_plan" .planning/REQUIREMENTS.md` returns 0 (stale citation gone)
    - `grep -c "0033_device_floor_plan_placement" .planning/REQUIREMENTS.md` returns ≥1 (SITE-04 evidence corrected)
    - `grep -c "0032_placement" .planning/REQUIREMENTS.md` returns 0 (stale citation gone)
    - `grep -c "Plan 05-13" .planning/REQUIREMENTS.md` returns ≥1 (gap-closure note appended)
    - All three referenced files actually exist on disk: `ls internal/db/migrations/0029_retention_config.up.sql internal/db/migrations/0032_floor_plan.up.sql internal/db/migrations/0033_device_floor_plan_placement.up.sql` exits 0
  </acceptance_criteria>
  <done>
    Phase 5 evidence trail cites migration filenames that exist on disk. A reader of REQUIREMENTS.md can now `cat` each cited path and see the actual schema/code that satisfies DATA-13, SETT-04, SITE-02, and SITE-04.
  </done>
</task>

</tasks>

<threat_model>

## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Browser → /api/map/data | Untrusted HTTP request crosses into Go handler reading sites + gateways DB tables |
| Browser → /api/sites/{id}/floor-plans + /api/floor-plans/* | Untrusted HTTP request crosses into floor-plan CRUD + static image serve (12 routes) |
| Browser → /api/reports/generate | Untrusted JSON body with scope + range; capability gating decides which utility class's data is returned |
| Background River worker → install_identity | Worker-context read of capabilities — must reflect current install identity, not boot-time snapshot |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-05-13-01 | I (Information Disclosure) | mapapi.RegisterRoutes wiring | mitigate | RegisterRoutes (internal/map/routes.go:20-23) wraps the handler in `auth.RequireAction(deps.SessionMgr, auth.ActionSiteRead)` — Task 1 must NOT bypass this group (mount RegisterRoutes directly, do NOT inline the handler outside the auth group). Task 1's TestRouter_MapRouteMounted asserts 401 (not 200) for unauth callers as the GREEN condition. |
| T-05-13-02 | I + S (Spoofing) | floorplan.RegisterRoutes wiring | mitigate | Same as T-05-13-01 — RegisterRoutes splits the 12 routes across four auth groups by action (site.read / site.create / site.update / site.archive). Task 1 mounts via RegisterRoutes; never expose any floor-plan handler outside the auth groups. TestRouter_FloorPlanRouteMounted asserts 401 for unauth GET as the GREEN condition. |
| T-05-13-03 | I (Information Disclosure) | floorplan.ServeImageHandler path traversal | accept (already-mitigated) | The handler already validates the floor-plan UUID + reads image_path from the floor_plan row + joins to ImageRoot (internal/floorplan/static.go). Task 2's wiring passes the correct ImageRoot from cfg.FloorPlanRoot — no new traversal surface. The existing handler's validation is unchanged. |
| T-05-13-04 | I (Information Disclosure) | Report capabilities gating | mitigate | Task 2 replaces the hardcoded `cfg.Capabilities = "both"` with a real DB load. A water-only install will now produce a report with electricity sections suppressed by the assembler's switch on rpt.Config.Capabilities (assembler.go:449). This is itself the threat mitigation — the prior state was a confidentiality leak across capability scopes. |
| T-05-13-05 | T (Tampering) | Background PDF worker capability gating | mitigate | Task 2 changes the worker's planRowToConfig to read capabilities from the InstallIdentity loaded at worker time (via sqlcIdentityProvider.Load). This means a capability change between report queue and worker execution is picked up; the worker NEVER trusts the report row alone to decide capability scope. |
| T-05-13-06 | D (Denial of Service) | FloorPlanRoot config default | accept | Default `/var/lib/shifter/floor-plans` matches compose volume mount. If the operator overrides SHIFTER_FLOOR_PLAN_ROOT to a non-existent path, upload handlers return 500 — not a DoS, the route returns a clean error. No mitigation needed in this plan. |
| T-05-13-07 | R (Repudiation) | REQUIREMENTS.md edit | accept | Doc-only edit. Audit trail is git history; the closure note appended in Task 3 references Plan 05-13 explicitly. |

Security severity threshold: block on `high`. All mitigations in this plan are wiring-correctness (mount routes through their RegisterRoutes guard) — there is no novel attack surface introduced by Plan 05-13. The plan REMOVES a confidentiality leak (Gap 3) rather than adding new risk.
</threat_model>

<verification>
After all three tasks complete:

```bash
# Backend tests
go test ./... -race -count=1
# Note: -short omitted so testcontainer-backed router tests run

# Verify gap 1 closed:
grep -c "mapapi.RegisterRoutes" internal/http/router.go   # ≥1
grep -c "MapDeps:" internal/cli/serve.go                   # ≥1

# Verify gap 2 closed:
grep -c "floorplan.RegisterRoutes" internal/http/router.go # ≥1
grep -c "FloorPlanDeps:" internal/cli/serve.go             # ≥1

# Verify gap 3 closed:
grep -F 'cfg.Capabilities = "both"' internal/report/handlers.go   # 0
grep -E 'Capabilities:\s+"both"' internal/report/pdf_worker.go    # 0
grep -c "GetCapabilities" internal/report/handlers.go             # ≥1

# Verify gap 4 closed:
grep -c "0030_retention_config" .planning/REQUIREMENTS.md  # 0
grep -c "0029_retention_config" .planning/REQUIREMENTS.md  # ≥2
grep -c "0031_floor_plan" .planning/REQUIREMENTS.md         # 0
grep -c "0032_floor_plan" .planning/REQUIREMENTS.md         # ≥1
grep -c "0032_placement" .planning/REQUIREMENTS.md          # 0
grep -c "0033_device_floor_plan_placement" .planning/REQUIREMENTS.md  # ≥1

# Build clean
go build ./...
go vet ./...
```

The Phase 5 verifier's re-run (anticipated `/gsd-verify-work 5 --reverify`) should now report 20/20 truths verified.
</verification>

<success_criteria>
- [ ] `go test ./internal/http/... -race -count=1` passes (Task 1 router tests + existing nil-deps tests)
- [ ] `go test ./internal/report/... ./internal/cli/... ./internal/config/... -race -count=1` passes (Task 2 handler + worker + config changes)
- [ ] `go build ./...` exits 0 (no compile errors anywhere)
- [ ] `go vet ./...` exits 0 (no static analysis errors)
- [ ] router.go contains both `mapapi.RegisterRoutes` and `floorplan.RegisterRoutes` calls, each guarded by nil-check
- [ ] serve.go constructs both `MapDeps` and `FloorPlanDeps` and passes them to NewRouter
- [ ] No `cfg.Capabilities = "both"` hardcode remains in `internal/report/`
- [ ] REQUIREMENTS.md migration citations match on-disk filenames; closure note appended
- [ ] Frontmatter requirements list (MAP-01..04, SITE-02..06, REPT-02) traceably affected by gap-closure work
</success_criteria>

<output>
After completion, create `.planning/phases/05-aggregates-reports-map-floor-plans/05-13-SUMMARY.md` documenting:
- Wave 7 gap-closure summary (4 verifier gaps closed)
- Files touched (router.go, rbac_test.go, config.go, serve.go, report/handlers.go, report/pdf_worker.go, report/csv.go, report/handlers_test.go, REQUIREMENTS.md)
- Two new router-level tests (TestRouter_MapRouteMounted + TestRouter_FloorPlanRouteMounted)
- Verification: 20/20 truths now satisfiable (16 already shipped + 4 new closures)
- Any Rule deviations (e.g., if Step C InstallIdentity struct extension cascaded to additional test fixture updates beyond handlers_test.go)
- Commit hashes for each task's git commit
</output>
