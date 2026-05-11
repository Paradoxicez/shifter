---
phase: 05
plan: 01
subsystem: wave-0-foundation
tags: [deps, migrations, skeletons, validation]
dependency_graph:
  requires: []
  provides: [maroto/v2, river, riverpgxv5, react-leaflet, leaflet, leaflet.markercluster, react-leaflet-cluster, pdfjs-dist, 0024_river_tables, pdfWorker-shim, phase-5-test-surface]
  affects: [05-02, 05-03, 05-04, 05-05, 05-06, 05-07, 05-08, 05-09, 05-10, 05-11, 05-12]
tech_stack:
  added:
    - github.com/johnfercher/maroto/v2 v2.4.0
    - github.com/riverqueue/river v0.36.0
    - github.com/riverqueue/river/riverdriver/riverpgxv5 v0.36.0
    - react-leaflet@5.0.0
    - leaflet@1.9.4
    - leaflet.markercluster@1.5.3
    - react-leaflet-cluster@4.1.3
    - pdfjs-dist@5.7.284
    - "@types/leaflet@1.9.21"
    - "@types/leaflet.markercluster@1.5.6"
  patterns:
    - River schema embedded in golang-migrate (Pitfall #8 mitigation)
    - pdfjs-dist v5 ESM worker shim via import.meta.url (Pitfall #3 mitigation)
    - Go skeleton tests use t.Skip("Plan 05-NN Task M: ...") naming convention
    - Vitest skeletons use it.skip(...) in describe blocks
    - Playwright skeletons use test.skip(...) in test.describe blocks
key_files:
  created:
    - go.mod (maroto/v2, river, riverpgxv5 added; go version upgraded 1.25→1.26.1)
    - web/package.json (7 new frontend deps + 2 devDeps)
    - internal/aggregate/CUMULATIVE_DELTA.md (pre-check verdict: ABSENT)
    - internal/db/migrations/0024_river_tables.up.sql (River v0.36.0 schema)
    - internal/db/migrations/0024_river_tables.down.sql (idempotent rollback)
    - web/src/lib/pdfWorker.ts (pdfjs-dist worker shim)
    - internal/aggregate/aggregate_test.go (3 t.Skip skeletons)
    - internal/report/csv_test.go
    - internal/report/excel_test.go
    - internal/report/pdf_test.go
    - internal/report/delta_test.go
    - internal/report/pdf_worker_test.go
    - internal/report/handlers_test.go
    - internal/floorplan/handlers_test.go
    - internal/floorplan/placement_test.go
    - internal/map/handler_test.go
    - internal/http/floorplan_static_test.go
    - internal/settings/retention_test.go
    - web/src/components/map/MapView.test.tsx
    - web/src/components/floor-plan/DevicePin.test.tsx
    - web/src/components/floor-plan/FloorPlanCanvas.test.tsx
    - web/src/lib/pdfToPng.test.ts
    - web/src/routes/reports/index.test.tsx
    - web/playwright/specs/reports-generate.spec.ts
    - web/playwright/specs/map-drill-down.spec.ts
    - web/playwright/specs/floor-plan-pinning.spec.ts
    - web/playwright/specs/site-drill-through.spec.ts
    - web/playwright/specs/floor-plan-health.spec.ts
    - web/playwright/specs/retention-settings.spec.ts
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-VALIDATION.md (updated)
  modified:
    - internal/db/migrations_test.go (version 23→24, River assertions, RiverDownUpClean test)
    - go.sum (new hashes for all new deps)
    - web/pnpm-lock.yaml (new lockfile entries)
decisions:
  - cumulative_delta is ABSENT from measurement hypertable; plan 05-02 computes via LAG() in hourly CAGG
  - go.mod upgraded from go 1.25.0 to go 1.26.1 (required by maroto/v2 v2.4.0)
  - river_leader uses CREATE UNLOGGED TABLE (correct per River schema — acceptance criteria misstated TABLE vs UNLOGGED TABLE)
  - River schema embedded as 0024 with all 6 migration version rows seeded (main v1-v6)
metrics:
  duration: 17 minutes
  completed: 2026-05-12
  tasks: 3
  files: 30
---

# Phase 05 Plan 01: Wave 0 Deps + Skeletons Summary

**One-liner:** Wave 0 foundation: 3 new Go deps (maroto/v2 + River + riverpgxv5) + 7 new frontend deps, River schema as golang-migrate 0024, pdfjs worker shim, 22 skeleton test files covering all Phase 5 testable behaviors, and populated VALIDATION.md Per-Task Verification Map (23 rows across plans 02–12).

## What Was Built

### Task 1: cumulative_delta pre-check + Go + frontend dep install

**cumulative_delta verdict:** ABSENT. The `measurement` hypertable stores `cumulative_value` (absolute reading) only. No stored `cumulative_delta` column exists anywhere in migrations, ingest code, or queries. Plan 05-02 will compute delta via `LAG()` in the hourly CAGG SELECT.

**Backend deps added (go.mod):**
- `github.com/johnfercher/maroto/v2 v2.4.0` — PDF generation (CLAUDE.md mandated)
- `github.com/riverqueue/river v0.36.0` — Postgres-native job queue (CLAUDE.md mandated)
- `github.com/riverqueue/river/riverdriver/riverpgxv5 v0.36.0` — pgx/v5 driver for River

Note: `go.mod` upgraded from `go 1.25.0` to `go 1.26.1` because maroto/v2 v2.4.0 requires Go 1.26.1+. This is a non-breaking upgrade.

**Frontend deps added (package.json):**
- `react-leaflet@5.0.0` — React 19 compatible Leaflet wrapper
- `leaflet@1.9.4` — peer dep of react-leaflet
- `leaflet.markercluster@1.5.3` — clustering engine
- `react-leaflet-cluster@4.1.3` — React wrapper for leaflet.markercluster
- `pdfjs-dist@5.7.284` — client-side PDF→PNG (D-17)
- `@types/leaflet@1.9.21` (devDep)
- `@types/leaflet.markercluster@1.5.6` (devDep)

`go build ./...` exits 0; `pnpm --dir web build` exits 0.

### Task 2: River schema as golang-migrate file 0024 + pdf.js worker shim

**River schema (0024_river_tables.up.sql):**
Generated by running `go run github.com/riverqueue/river/cmd/river@v0.36.0 migrate-up --line main` against a clean `postgres:16-alpine` Docker container and capturing the schema via `docker exec ... pg_dump -s -t 'river_*'`. The schema includes:
- `river_job_state` enum (available/cancelled/completed/discarded/pending/retryable/running/scheduled)
- `river_job_state_in_bitmask()` SQL function (used by unique partial index)
- `river_migration` table (tracks applied schema versions; seeded with 6 rows for v1-v6)
- `river_job` table (primary job store — regular table, durable)
- `river_leader` table (UNLOGGED — performance; distributed election)
- `river_queue` table (named queue definitions)
- `river_client` + `river_client_queue` tables (UNLOGGED — worker tracking)
- 6 indexes on river_job (btree + GIN for args/metadata, state/queue prioritization, unique_key partial)

`0024_river_tables.down.sql` drops all tables + type + function idempotently.

**migrations_test.go updates:**
- `TestRunMigrations_Clean`: version assertion bumped 23→24; added `river_migration` version=6 check; added `river_job` exists check
- `TestRunMigrations_Idempotent`: version assertion bumped 23→24
- `TestRunMigrations_RiverDownUpClean` (new): up → verify exists → down → verify absent (`to_regclass`) → up → verify exists again + version=6 check

All 3 migration tests pass (testcontainer, ~45s each).

**pdfWorker.ts:**
Single-source shim at `web/src/lib/pdfWorker.ts` using `new URL('pdfjs-dist/build/pdf.worker.min.mjs', import.meta.url).toString()`. Every consumer imports this once (or imports `pdfToPng` which imports it). Pitfall #3 fully mitigated.

### Task 3: 22 skeleton test files + 6 Playwright specs + VALIDATION.md map

**12 Go skeleton test files:**
- `internal/aggregate/aggregate_test.go`: TestCAGGHierarchy, TestRefreshPolicyParams, TestRetentionPolicy
- `internal/report/csv_test.go`: TestCSVFormat
- `internal/report/excel_test.go`: TestExcelFormat
- `internal/report/pdf_test.go`: TestPDFBranding
- `internal/report/delta_test.go`: TestPeriodDelta
- `internal/report/pdf_worker_test.go`: TestPDFJob
- `internal/report/handlers_test.go`: TestStatusHandler_PendingReadyFailed, TestReportScopeGrouping
- `internal/floorplan/handlers_test.go`: TestImageUpload, TestMultiFloor, TestReplaceKeepsPins
- `internal/floorplan/placement_test.go`: TestPlacementCRUD, TestPlacementDecommissionInTx
- `internal/map/handler_test.go`: TestMapData, TestOSMTileURL (package `mapapi`)
- `internal/http/floorplan_static_test.go`: TestFloorPlanImageAuthGated (package `http_test`)
- `internal/settings/retention_test.go`: TestRetentionConfigCRUD

**5 vitest skeleton files:**
- `web/src/components/map/MapView.test.tsx` (4 it.skip)
- `web/src/components/floor-plan/DevicePin.test.tsx` (4 it.skip)
- `web/src/components/floor-plan/FloorPlanCanvas.test.tsx` (3 it.skip)
- `web/src/lib/pdfToPng.test.ts` (3 it.skip)
- `web/src/routes/reports/index.test.tsx` (5 it.skip)

**6 Playwright skeleton specs** (all use `test.skip`):
reports-generate, map-drill-down, floor-plan-pinning, site-drill-through, floor-plan-health, retention-settings

**VALIDATION.md Per-Task Verification Map:**
23 rows across plans 02–12 (one per task). Each row's `Automated Command` column sourced verbatim from the target plan's `<verify><automated>` block. `wave_0_complete: true` flipped. `nyquist_compliant` remains `false` (plan 05-12 flips it).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] go mod tidy removed new deps**
- **Found during:** Task 1
- **Issue:** `go mod tidy` pruned maroto/v2, river, riverpgxv5 from go.mod because no code imports them yet (skeleton test bodies not written until Task 3)
- **Fix:** Re-ran `go get` for all 3 packages without subsequent `go mod tidy`; deps remain as `// indirect` in go.mod but are present (as required by acceptance criteria check)
- **Files modified:** go.mod, go.sum

**2. [Rule 1 - Bug] go 1.25.0 → go 1.26.1 upgrade**
- **Found during:** Task 1
- **Issue:** maroto/v2 v2.4.0 requires `go >= 1.26.1`; `go get` automatically upgraded go.mod directive
- **Fix:** Accepted upgrade; no code changes required; all existing tests still pass
- **Files modified:** go.mod

**3. [Rule 1 - Bug] ADD CONSTRAINT IF NOT EXISTS not valid Postgres syntax**
- **Found during:** Task 2 (migration test failure)
- **Issue:** Initial 0024_river_tables.up.sql used `ALTER TABLE ONLY river_job ADD CONSTRAINT IF NOT EXISTS river_job_pkey PRIMARY KEY (id)` which is invalid — Postgres does not support `IF NOT EXISTS` on `ADD CONSTRAINT`
- **Fix:** Moved PK constraint inline into the `CREATE TABLE IF NOT EXISTS` definition; created sequence before table so `DEFAULT nextval(...)` works
- **Files modified:** internal/db/migrations/0024_river_tables.up.sql

**4. [Rule 1 - Clarification] river_leader is UNLOGGED**
- **Found during:** Task 2 (schema capture from River CLI)
- **Issue:** Plan acceptance criteria stated `CREATE TABLE IF NOT EXISTS river_leader` but the actual River schema uses `CREATE UNLOGGED TABLE IF NOT EXISTS` for performance
- **Fix:** Used the correct River schema (`CREATE UNLOGGED TABLE IF NOT EXISTS river_leader`) as captured from River's own CLI. The acceptance criteria check was slightly imprecise.
- **Files modified:** internal/db/migrations/0024_river_tables.up.sql

## Known Stubs

None. This plan creates only skeleton test files (all `t.Skip` / `it.skip` / `test.skip`). No UI components, no data-rendering code, no stubs that flow to UI rendering.

## Threat Flags

None. This plan only installs deps, creates migrations, and adds skeleton test files. No new network endpoints, auth paths, or file access patterns introduced beyond the River job-queue tables (which are internal to the application).

## Self-Check: PASSED

All 28 created/modified files found on disk. All 3 task commits verified in git log.

| Item | Status |
|------|--------|
| internal/aggregate/CUMULATIVE_DELTA.md | FOUND |
| internal/db/migrations/0024_river_tables.up.sql | FOUND |
| internal/db/migrations/0024_river_tables.down.sql | FOUND |
| web/src/lib/pdfWorker.ts | FOUND |
| 12 Go skeleton test files | ALL FOUND |
| 5 vitest skeleton files | ALL FOUND |
| 6 Playwright skeleton specs | ALL FOUND |
| .planning/phases/.../05-VALIDATION.md | FOUND |
| Commit c780d7c (Task 1) | FOUND |
| Commit f39ad33 (Task 2) | FOUND |
| Commit 8a9a8e7 (Task 3) | FOUND |
