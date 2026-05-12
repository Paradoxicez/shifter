---
phase: 05-aggregates-reports-map-floor-plans
plan: 07
subsystem: api
tags: [go, postgres, pgx, sqlc, chi, floor-plan, placement, audit, decommission]

# Dependency graph
requires:
  - phase: 05-05
    provides: floor_plan + device_floor_plan_placement schema + upload handlers (Deps, ImageRoot, sqlc base)
  - phase: 03-02
    provides: audit.WriteEntry pattern (audit-in-tx)
  - phase: 02-05
    provides: decommissionDevice handler + pgx.Serializable tx pattern
provides:
  - Placement CRUD endpoints (Upsert/Update/Delete/List) with same-site integrity guard
  - Auth-gated static image serve with path-traversal defense (T-05-07-02/03)
  - D-25: Device decommission auto-deletes placement in same pgx.Tx
  - 6 new sqlc queries for placement lifecycle
  - audit vocabulary extended with placement.pin/nudge/remove + EntityTypePlacement
affects: [05-10-floor-plan-frontend, 05-12-phase-closure]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "LATERAL JOIN for latest-measurement denormalization in list queries (battery_pct, rssi from measurement table not device)"
    - "UPSERT-on-PK pattern: INSERT … ON CONFLICT (device_id) DO UPDATE for pin-to-new-plan move"
    - "Same-site integrity guard: GetDeviceSiteID (LEFT JOIN binding+metering_point) compared to floor_plan.site_id → 409 site_mismatch"
    - "Auth re-assertion in static handler even inside authenticated middleware group (defense-in-depth against routing misconfig)"
    - "filepath.Clean + strings.HasPrefix containment check for image_path traversal defense"

key-files:
  created:
    - internal/floorplan/placement.go
    - internal/floorplan/static.go
    - internal/device/decommission_recovery_test.go
  modified:
    - internal/db/queries/floor_plan.sql
    - internal/db/sqlc/floor_plan.sql.go
    - internal/db/sqlc/querier.go
    - internal/db/migrations/0035_audit_vocab_placement.up.sql
    - internal/db/migrations/0035_audit_vocab_placement.down.sql
    - internal/db/migrations_test.go
    - internal/db/roundtrip_test.go
    - internal/audit/log.go
    - internal/floorplan/routes.go
    - internal/floorplan/placement_test.go
    - internal/http/floorplan_static_test.go
    - internal/device/handlers.go

key-decisions:
  - "LATERAL JOIN for battery_pct/rssi: those columns live on measurement (not device), so ListPlacementsByPlan uses a correlated subquery to get the most recent measurement row per active metering_point"
  - "Migration 0035 (not extending 0034): 0034 was already committed for floor_plan audit vocab; placement vocab needs its own migration to keep rollback granularity"
  - "Decommission placement audit uses EntityID=deviceID (not placementID): consistent with how the decommission audit references the device, allows Phase 6 audit browse to surface both entries by device"
  - "TestDecommission_PlacementRollsBackOnFailure uses already_decommissioned 409 to exercise early-exit without placement delete — avoids needing pgx-level fault injection"

patterns-established:
  - "Audit-in-tx for placements: every placement mutation (pin/nudge/remove) writes an audit.WriteEntry inside the same pgx.Tx as the placement INSERT/UPDATE/DELETE"
  - "Static asset serve with containment check: UUID-parse → DB lookup → filepath.Join → filepath.Clean → strings.HasPrefix guard before os.Open"

requirements-completed: [SITE-04, SITE-06]

# Metrics
duration: 7min
completed: 2026-05-12
---

# Phase 05 Plan 07: Floor Plan Placement + Decommission Integration Summary

**Placement CRUD with fractional-coord UPSERT, same-site integrity guard (SITE-04), auth-gated image serve with path-traversal defense (T-05-07-02/03), and atomic D-25 device decommission → placement removal in one pgx.Tx.**

## Performance

- **Duration:** ~7 min (wall clock across both task commits)
- **Started:** 2026-05-12T01:46:13Z
- **Completed:** 2026-05-12T01:52:52Z
- **Tasks:** 2
- **Files modified:** 15

## Accomplishments

- Placement CRUD fully implemented: POST (upsert/pin), PATCH (nudge), DELETE (remove), GET list — all with audit entries inside the same pgx.Tx, enforcing [0,1] fraction range and same-site integrity
- Auth-gated static image serve: UUID validation before any FS operation, filepath.Clean containment check, Content-Type sniffing (png/jpeg), 10-min private Cache-Control header
- D-25 atomicity: decommissioning a device deletes its floor plan pin in the same Serializable transaction; placement_removed flag propagates into the decommission audit after_state
- 13 new integration tests across 3 packages; all 15 floorplan package tests green

## Task Commits

1. **Task 1: Placement CRUD + same-site integrity guard** - `75ea302` (feat)
2. **Task 2: Auth-gated image serve + D-25 decommission placement removal + tests** - `8f6dae6` (feat)

**Plan metadata:** (added in final commit below)

## Files Created/Modified

- `internal/floorplan/placement.go` - UpsertPlacementHandler, UpdatePlacementHandler, DeletePlacementHandler, ListPlacementsHandler with same-site integrity guard and audit-in-tx
- `internal/floorplan/static.go` - ServeImageHandler with UUID validation, containment check, content-type sniff, auth gate
- `internal/device/decommission_recovery_test.go` - 3 tests: removes placement, rollback on conflict, no-op when unpinned
- `internal/db/queries/floor_plan.sql` - 6 new queries: UpsertPlacement, UpdatePlacement, DeletePlacementByDevice, ListPlacementsByPlan (LATERAL JOIN), GetPlacementByDevice, GetDeviceSiteID
- `internal/db/sqlc/floor_plan.sql.go` - regenerated with all 6 query implementations
- `internal/db/sqlc/querier.go` - interface extended with GetPlacementByDevice, DeletePlacementByDevice
- `internal/db/migrations/0035_audit_vocab_placement.up.sql` - DROP+re-ADD CHECK to include placement.pin/nudge/remove and 'placement' entity type
- `internal/db/migrations/0035_audit_vocab_placement.down.sql` - reverts to 0034 state
- `internal/db/migrations_test.go` - version bumped 34→35; step counts incremented
- `internal/db/roundtrip_test.go` - expected version 34→35
- `internal/audit/log.go` - ActionPlacementPin/Nudge/Remove + EntityTypePlacement constants
- `internal/floorplan/routes.go` - 5 new routes: GET placements, GET image, POST placements, PATCH placement, DELETE placement
- `internal/floorplan/placement_test.go` - 7 integration tests covering CRUD, fraction rejection, site_mismatch, unbound device, upsert move, denormalized list fields, DB CHECK enforcement
- `internal/http/floorplan_static_test.go` - TestFloorPlanImageAuthGated (anon→401, auth→200+image/png, non-UUID→400, missing plan→404) + TestFloorPlanImageAuthGated_PathTraversalRejected
- `internal/device/handlers.go` - decommissionDevice extended with D-25: GetPlacementByDevice → DeletePlacementByDevice → placement.remove audit, all inside existing Serializable tx

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] battery_pct/rssi columns do not exist on device table**
- **Found during:** Task 1 — sqlc generate rejected ListPlacementsByPlan referencing `d.battery_pct` and `d.rssi`
- **Issue:** The plan's query template joined directly on `device` for those columns, but they live on the `measurement` hypertable
- **Fix:** Replaced `d.battery_pct, d.rssi` with a LATERAL JOIN subquery: `LEFT JOIN LATERAL (SELECT battery_pct, rssi FROM measurement WHERE metering_point_id = mp.id ORDER BY time DESC LIMIT 1) latest ON mp.id IS NOT NULL`
- **Files modified:** `internal/db/queries/floor_plan.sql`, `internal/db/sqlc/floor_plan.sql.go`
- **Commit:** 75ea302

**2. [Rule 1 - Bug] device_dev_eui_hex16 CHECK violation in placement tests**
- **Found during:** Task 1 — test helper `seedDeviceRow` initially generated EUI using `uuid.NewString()[:14]`, producing non-hex characters
- **Fix:** Switched to `hex.EncodeToString(rawID[:8])` which produces exactly 16 lowercase hex chars satisfying the `CHECK (dev_eui ~ '^[0-9a-f]{16}$')` constraint
- **Files modified:** `internal/floorplan/placement_test.go`
- **Commit:** 75ea302

**3. [Rule 1 - Bug] UNIQUE(site_id, sort_order) violation in TestPlacement_UpsertMovesPin**
- **Found during:** Task 1 — two floor plans uploaded with sort_order=0 to the same site triggered a 23505 unique violation
- **Fix:** `planIDFromUpload` helper now queries `COALESCE(MAX(sort_order), -1) + 1` before each upload to assign the next available sort_order
- **Files modified:** `internal/floorplan/placement_test.go`
- **Commit:** 75ea302

**4. [Rule 2 - Missing] testsupport.Pool type referenced but does not exist**
- **Found during:** Task 2 — `internal/http/floorplan_static_test.go` compiled with `*testsupport.Pool` which is not exported by that package; `testsupport.StartPostgres` returns `*pgxpool.Pool`
- **Fix:** Replaced `*testsupport.Pool` with `*pgxpool.Pool` and added `pgxpool` import; removed the bogus `testsupportPool` interface alias
- **Files modified:** `internal/http/floorplan_static_test.go`
- **Commit:** 8f6dae6

## Known Stubs

None — all handlers are fully wired to real DB queries with no placeholder data.

## Threat Flags

| Flag | File | Description |
|------|------|-------------|
| threat_flag: path-traversal-mitigated | internal/floorplan/static.go | T-05-07-03 containment check: filepath.Clean + strings.HasPrefix prevents any DB-injected malicious image_path from escaping ImageRoot |

## Self-Check: PASSED

- `internal/floorplan/placement.go` - FOUND
- `internal/floorplan/static.go` - FOUND
- `internal/device/decommission_recovery_test.go` - FOUND
- Commit 75ea302 - FOUND (feat: placement CRUD)
- Commit 8f6dae6 - FOUND (feat: auth-gated image serve + D-25)
- All 15 `./internal/floorplan/...` tests: PASSED
- All 13 targeted tests (TestFloorPlanImageAuthGated|TestDecommission|TestPlacement): PASSED
