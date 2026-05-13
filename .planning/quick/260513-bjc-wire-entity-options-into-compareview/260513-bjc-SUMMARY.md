---
phase: quick
plan: "260513-bjc"
subsystem: frontend/reports
tags: [compare-view, react-query, metering-points, sites, dropdowns]
dependency_graph:
  requires: [07-12-compare-view]
  provides: [compare-flow-end-to-end-usable]
  affects: [web/src/routes/reports/CompareView.tsx]
tech_stack:
  added: []
  patterns: [useQuery cache key sharing, useMemo join, isLoading disabled-state prop]
key_files:
  modified:
    - web/src/routes/reports/CompareView.tsx
    - web/src/routes/reports/CompareView.test.tsx
decisions:
  - Reuse existing query keys ['metering-points'] and ['sites'] so React Query cache is shared with ReportConfigPanel/AddDeviceDialog/filter-toolbar
  - Disable the EntityDropdown button (not just visually indicate) while loading — prevents interaction on stale empty options
  - Test uses waitFor(expect(comboA).not.toBeDisabled()) before fireEvent.click to handle the async disabled→enabled transition
metrics:
  duration: "~15 minutes"
  completed: "2026-05-13"
  tasks_completed: 1
  files_modified: 2
---

# Quick Task 260513-bjc: Wire Entity Options into CompareView Summary

**One-liner:** CompareView EntityDropdown now populated via `useQuery(['metering-points'])` + `useQuery(['sites'])` joined into `${mp.name} — ${siteName}` labels; 5/5 tests pass.

## What Was Done

Resolved the known stub from 07-12-SUMMARY ("EntityDropdown options always empty — `const entityOptions: EntityOption[] = []` at CompareView.tsx:234").

### CompareView.tsx changes

- Added `useMemo` to the `useState` import; added `useQuery` to the `@tanstack/react-query` import
- Added imports: `listMPs` from `@/lib/metering-points`, `listSites` from `@/lib/sites`
- Replaced `const entityOptions: EntityOption[] = []` with:
  - `useQuery({ queryKey: ['metering-points'], queryFn: () => listMPs() })`
  - `useQuery({ queryKey: ['sites'], queryFn: () => listSites() })`
  - `isLoadingOptions = mpsQuery.isPending || sitesQuery.isPending`
  - `useMemo` join building `{ id, label: "${mp.name} — ${siteName}" }` pairs
- Extended `EntityDropdownProps` with `isLoading?: boolean`; button renders "Loading…" and is `disabled` when true
- Passed `isLoading={isLoadingOptions}` to all three EntityDropdown instances (Entity A, Entity B in entities mode; single entity in time_ranges mode)

### CompareView.test.tsx changes

- Replaced blanket `mockResolvedValue([])` with path-aware `mockImplementation`:
  - `/api/metering-points` → returns 2 MP fixtures (mp-1/MP One/site-1, mp-2/MP Two/site-2)
  - `/api/sites` → returns 2 Site fixtures (site-1/Building A, site-2/Building B)
  - anything else → `[]`
- Added `waitFor` to imports
- Added new test: "populates entity dropdowns with metering-point options once data loads"
  - Uses `findAllByRole('combobox')` + `waitFor(() => expect(comboA).not.toBeDisabled())` to handle the disabled→enabled transition
  - Asserts "MP One — Building A" and "MP Two — Building B" appear after clicking

## Test Results

| Suite | Before | After |
|-------|--------|-------|
| CompareView tests | 4 pass, 1 fail (new) | 5 pass, 0 fail |

## Known Stub Resolution

The stub tracked in 07-12-SUMMARY is now resolved:
- **Was:** `const entityOptions: EntityOption[] = []` — dropdowns always empty, compare flow structurally complete but functionally inert
- **Now:** options populated from real API data; operator can pick entity A, entity B, a date range, and fire `POST /api/reports/compare` against real entity IDs

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Test click timing: button disabled during loading**

- **Found during:** Task 1 GREEN verification
- **Issue:** The plan's test template used `fireEvent.click(comboA)` immediately after render. Since the button is now `disabled` while queries are pending (`isLoadingOptions=true`), the click was a no-op — the popover never opened, and `findByText('MP One — Building A')` timed out.
- **Fix:** Added `await waitFor(() => expect(comboA).not.toBeDisabled())` before the click, ensuring the test waits for both queries to resolve before interacting with the dropdown.
- **Files modified:** `web/src/routes/reports/CompareView.test.tsx`
- **Commit:** 85d7c38

## Self-Check: PASSED

- `web/src/routes/reports/CompareView.tsx` — modified (useQuery + useMemo wiring)
- `web/src/routes/reports/CompareView.test.tsx` — modified (path-aware mock + new test)
- Commit 85d7c38 exists
- `pnpm typecheck` — exit 0
- `pnpm build` — clean (CompareView-CUmL_FDS.js in dist)
- 5/5 CompareView tests pass
