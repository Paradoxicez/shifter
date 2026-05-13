---
phase: quick
plan: 260513-pkp
subsystem: backend+frontend
tags: [uat-fix, devices, compare, install, router, chart]
dependency_graph:
  requires: []
  provides: [device-profile-name-in-list, compare-entity-labels, install-410-silence, hydrate-fallback, consumption-empty-state]
  affects: [devices-api, compare-api, install-state, app-router, dashboard-chart]
tech_stack:
  added: []
  patterns: [sessionStorage-caching, sqlc-left-join, early-return-empty-state]
key_files:
  created: []
  modified:
    - internal/db/queries/devices.sql
    - internal/db/sqlc/devices.sql.go
    - internal/db/sqlc/querier.go
    - internal/device/handlers.go
    - internal/api/compare_handler.go
    - web/src/lib/install.ts
    - web/src/routes/_root.tsx
    - web/src/App.tsx
    - web/src/components/dashboard/ConsumptionChart.tsx
    - web/src/lib/devices.ts
decisions:
  - Used sessionStorage (not localStorage) for install-done flag: clears on tab close, correct scope for session-level caching
  - labelForEntity falls back to 'type:uuid' on DB error to avoid hard failures in compare view
  - HydrateFallback uses shadcn Skeleton (already used elsewhere) rather than a new component
  - Empty-state early-return placed before ChartContainer so Recharts never renders an empty SVG
metrics:
  duration: ~25min
  completed: "2026-05-13"
  tasks_completed: 5
  files_modified: 10
---

# Quick 260513-pkp: Fix 5 Medium/Low UAT Issues Summary

**One-liner:** Five independent UAT fixes — device_profile JOIN in list API, compare view human labels via DB lookup, sessionStorage caching of 410 to silence console noise, React Router HydrateFallback skeleton, and ConsumptionChart empty-state guard.

## Tasks Completed

| # | Task | Commit | Files |
|---|------|--------|-------|
| 1 | Add device_profile JOIN to ListDevicesFiltered | `2580352` | devices.sql, devices.sql.go, querier.go, handlers.go, devices.ts |
| 2 | Resolve compare entity labels from DB | `14309b9` | compare_handler.go |
| 3 | Cache install-complete 410 in sessionStorage | `ff4e202` | install.ts, _root.tsx |
| 4 | Add HydrateFallback to root route | `79705d7` | App.tsx |
| 5 | Add empty-state to ConsumptionChart | `865db07` | ConsumptionChart.tsx |

## What Changed

### Task 1: device_profile JOIN (backend + frontend types)

`ListDevicesFiltered` SQL now includes `LEFT JOIN device_profile dp ON dp.id = d.device_profile_id` and selects `dp.name AS device_profile_name, dp.expected_interval_s`. sqlc regenerated `ListDevicesFilteredRow` with two new nullable fields (`DeviceProfileName *string`, `ExpectedIntervalS *int32`). The `filteredDevicesToJSON` handler exposes them. The TypeScript `Device` interface gained `device_profile_name?` and `expected_interval_s?`.

### Task 2: Compare entity labels (backend)

`labelForEntity` changed from returning `"entityType:uuid"` string to querying `GetSite` or `GetMP` via the already-in-scope `*sqlc.Queries`. Falls back to the old format on DB error. Both `handleEntitiesMode` (two call sites) and `handleTimeRangesMode` (one call site) updated to pass `ctx` and `q`.

### Task 3: sessionStorage install-done cache (frontend)

Exported `INSTALL_DONE_KEY = 'shifter_install_completed'` constant from `install.ts`. `fetchInstallState` short-circuits with `return null` if the key is already `'true'`. On the first 410, sets the key. `handleSignOut` in `_root.tsx` calls `sessionStorage.removeItem(INSTALL_DONE_KEY)` before redirecting to `/login`.

### Task 4: HydrateFallback (frontend)

Added `import { Skeleton } from '@/components/ui/skeleton'` to `App.tsx`. Added `HydrateFallback: () => <Skeleton className="h-screen w-full" />` to the `id: 'root'` route object in `createBrowserRouter`.

### Task 5: ConsumptionChart empty-state (frontend)

Added early-return guard `if (data.length === 0)` before the `ChartContainer` rendering path. Returns a `div` with matching height classes and centered `"No data for this range"` muted text, preventing blank/broken Recharts SVG from appearing.

## Verification Results

- `go build ./...` — passed
- `go test ./internal/device/... -count=1` — 65 tests passed
- `go test ./internal/api/... -count=1` — passed
- `cd web && pnpm build` — passed (all 3 frontend tasks)
- Container rebuilt and restarted; live curl confirmed: `device_profile_name: "Itron KINMY LoRa Module"`, `expected_interval_s: 3600` for seeded devices

## Deviations from Plan

None — plan executed exactly as written. All 5 tasks completed atomically with individual commits.

## Known Stubs

None.

## Self-Check: PASSED

- All 5 commits exist: 2580352, 14309b9, ff4e202, 79705d7, 865db07
- All modified files have been verified to compile (Go + TypeScript)
- Test suites pass for affected packages
