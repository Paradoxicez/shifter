---
phase: 05-aggregates-reports-map-floor-plans
plan: 13
subsystem: http-router, report, config, cli
tags: [gap-closure, router-wiring, capabilities, documentation-fix]
dependency_graph:
  requires: [05-01, 05-04, 05-05, 05-06, 05-07, 05-12]
  provides: [map-endpoints-reachable, floorplan-endpoints-reachable, real-capabilities-gating, correct-requirements-evidence]
  affects: [internal/http/router.go, internal/cli/serve.go, internal/report/handlers.go, internal/report/pdf_worker.go, internal/config/config.go, .planning/REQUIREMENTS.md]
tech_stack:
  added: []
  patterns: [nil-guard-mount, TDD-red-green, LoadAndSave-session-injection]
key_files:
  created: []
  modified:
    - internal/http/router.go
    - internal/http/rbac_test.go
    - internal/config/config.go
    - internal/cli/serve.go
    - internal/report/handlers.go
    - internal/report/pdf_worker.go
    - internal/report/csv.go
    - internal/report/handlers_test.go
    - internal/report/pdf_worker_test.go
    - .planning/REQUIREMENTS.md
decisions:
  - "planRowToConfig signature changed from (plan, tz) to (plan, identity) so capabilities flow through without an additional DB call in the worker"
  - "injectUser/injectViewer helpers fixed to use sm.LoadAndSave pattern (SCS requires context initialized by middleware before sm.Put)"
  - "Rule 1 auto-fix: report test fixtures used INSERT INTO users (wrong) → INSERT INTO \"user\" + added name column"
metrics:
  duration_minutes: 14
  completed_date: "2026-05-12"
  tasks_completed: 3
  files_changed: 10
---

# Phase 05 Plan 13: Router Wiring + Capabilities Gap Closure Summary

Wave 7 gap-closure plan. Closes all 4 verification gaps surfaced by `/gsd-verify-work` on Phase 5: map router wiring (Gap 1), floor-plan router wiring (Gap 2), report capabilities hardcoding (Gap 3), REQUIREMENTS.md evidence trail migration filenames (Gap 4). Phase 5 score goes from 16/20 to 20/20 truths satisfiable.

## What Was Built

### Task 1: Wire mapapi + floorplan routes through the production router (TDD)

Added two nil-guard route mounts to `internal/http/router.go`, following the identical pattern as `report.RegisterRoutes` and `settings.RegisterRoutes`:

- `MapDeps *mapapi.Deps` field on `httpapi.Deps` struct
- `FloorPlanDeps *floorplan.Deps` field on `httpapi.Deps` struct
- Guard-mount `mapapi.RegisterRoutes(r, *deps.MapDeps)` when `MapDeps` non-nil
- Guard-mount `floorplan.RegisterRoutes(r, *deps.FloorPlanDeps)` when `FloorPlanDeps` non-nil
- Both placed before SPA fallback (PITFALL #4 preserved)

Two new router-level integration tests (TDD RED then GREEN):
- `TestRouter_MapRouteMounted`: asserts 401 with non-nil MapDeps, 404 with nil MapDeps
- `TestRouter_FloorPlanRouteMounted`: asserts 401 with non-nil FloorPlanDeps, 404 with nil FloorPlanDeps

Commits: RED `test(05-13)` + GREEN `feat(05-13)`

### Task 2: Construct MapDeps + FloorPlanDeps in serve.go; real capabilities in report

**Config (internal/config/config.go):**
- Added `FloorPlanRoot string \`mapstructure:"floor_plan_root"\`` to `Config` struct
- Added viper default `"floor_plan_root"` → `"/var/lib/shifter/floor-plans"` (matches compose volume mount)

**serve.go:**
- Added `floorplan` and `mapapi` imports
- Constructs `MapDeps: &mapapi.Deps{Pool, Logger.With("component", "mapapi"), SessionMgr}`
- Constructs `FloorPlanDeps: &floorplan.Deps{Pool, Queries, SessionMgr, ImageRoot: cfg.FloorPlanRoot}`
- Extended `sqlcIdentityProvider.Load` SQL to SELECT `capabilities` column; populates `InstallIdentity.Capabilities`

**report/csv.go:**
- Added `Capabilities string` field to `InstallIdentity` struct (D-09 capability gating)

**report/handlers.go (Gap 3 fix):**
- Replaced `cfg.Capabilities = "both"` with a real DB call: `deps.Queries.GetCapabilities(ctx)` → `cfg.Capabilities`
- Water-only installs no longer receive electricity sections in reports; electricity-only installs no longer receive water sections

**report/pdf_worker.go (Gap 3 worker fix):**
- Changed `planRowToConfig(plan, tz)` signature to `planRowToConfig(plan, identity)` — takes the full `InstallIdentity`
- `identity.Timezone` replaces the old `tz` parameter; `identity.Capabilities` replaces the hardcoded `"both"`
- Worker now propagates real capabilities into `ReportConfig.Capabilities`

### Task 3: Fix migration filename citations in REQUIREMENTS.md (Gap 4)

Corrected four stale migration filename citations in the Phase 5 evidence trail:

| Requirement | Old (wrong) | New (correct, on-disk) |
|-------------|-------------|------------------------|
| DATA-13 | `0030_retention_config.up.sql` | `0029_retention_config.up.sql` |
| SETT-04 | `0030_retention_config.up.sql` | `0029_retention_config.up.sql` |
| SITE-02 | `0031_floor_plan.up.sql` | `0032_floor_plan.up.sql` |
| SITE-04 | `0032_placement.up.sql` | `0033_device_floor_plan_placement.up.sql` |

Also appended a gap-closure note documenting Plan 05-13 and 2026-05-12 date.

## Verification Results

All success criteria pass:

| Check | Result |
|-------|--------|
| `grep "mapapi.RegisterRoutes" internal/http/router.go` | 1 match |
| `grep "floorplan.RegisterRoutes" internal/http/router.go` | 1 match |
| `grep 'cfg.Capabilities = "both"' internal/report/handlers.go` | 0 matches |
| `grep -E 'Capabilities:\s+"both"' internal/report/pdf_worker.go` | 0 matches |
| `grep "0030_retention_config" .planning/REQUIREMENTS.md` | 0 matches |
| `grep "0029_retention_config" .planning/REQUIREMENTS.md` | 2+ matches |
| `go test ./internal/http/... -race -count=1` | 25/25 passed |
| `go test ./internal/report/... -race -count=1` | 36/36 passed |
| `go build ./...` | success |
| `go vet ./...` | no issues |

## Commits

| Task | Hash | Message |
|------|------|---------|
| 1 RED | `git log --grep="RED"` | test(05-13): RED — TestRouter_MapRouteMounted + TestRouter_FloorPlanRouteMounted |
| 1 GREEN | `7a1ecb8` | feat(05-13): mount mapapi + floorplan routes in production router |
| 2 | `d3c2b08` | feat(05-13): construct MapDeps+FloorPlanDeps; pass real install capabilities to report |
| 3 | `4729417` | docs(05-13): correct Phase 5 evidence trail migration filenames |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Report test fixtures used wrong user table name**
- **Found during:** Task 2 test run
- **Issue:** `handlers_test.go` and `pdf_worker_test.go` both used `INSERT INTO users` but the schema uses the reserved-word-quoted `"user"` table. Also missing the NOT NULL `name` column.
- **Fix:** Changed to `INSERT INTO "user" (id, email, name, password_hash, role)` with appropriate name values in both files
- **Files modified:** `internal/report/handlers_test.go`, `internal/report/pdf_worker_test.go`

**2. [Rule 1 - Bug] injectUser/injectViewer called sm.Put on uninitialized SCS context**
- **Found during:** Task 2 test run (after fixing the users table bug, underlying SCS panic surfaced)
- **Issue:** `injectUser` and `injectViewer` in `handlers_test.go` called `sm.Put(ctx, ...)` directly on a plain context, causing a panic "no session data in context". SCS requires the context to be initialized by `sm.LoadAndSave` middleware before `sm.Put` can be called.
- **Fix:** Rewrote both helpers to wrap `sm.Put` calls inside `sm.LoadAndSave` via an inline HTTP handler, matching the pattern in `internal/settings/retention_test.go:injectSession`
- **Files modified:** `internal/report/handlers_test.go`

Both bugs were pre-existing (masked by the `users` table error in the original code); fixing them was required for the Task 2 `GetCapabilities` path to be tested.

## Known Stubs

None. All capability gating now reads from real `install_identity.capabilities` DB column.

## Threat Flags

No new security surface introduced. Plan 05-13 REMOVES a confidentiality leak (Gap 3 — hardcoded "both" exposing cross-capability data) rather than adding new risk. All new routes mount through their existing `RegisterRoutes` auth guards.

## Self-Check: PASSED
