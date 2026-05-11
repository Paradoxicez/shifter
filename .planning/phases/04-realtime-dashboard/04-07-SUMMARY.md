---
phase: 04-realtime-dashboard
plan: 07
subsystem: frontend-dashboard

tags: [dashboard, kpi, sse, react-query, shadcn, onboarding, capability-gating, realtime, frontend]

# Dependency graph
requires:
  - phase: 04-realtime-dashboard
    plan: 04
    provides: "GET /api/dashboard/snapshot + /scope endpoints; UtilityKPI + Snapshot shapes"
  - phase: 04-realtime-dashboard
    plan: 06
    provides: "useSSE hook (SSEStatus, MeasurementDelta, UseSSEResult); useDashboardScope hook"

provides:
  - "DashboardPage route at `/` — operator's primary surface"
  - "KpiCard component (4 variants: today/instant/delta/online; full color contract)"
  - "KpiGrid component (capability-gated 4 or 8 tiles; responsive 1/2/4-col grid)"
  - "LiveChannelBanner component (reconnecting <30s / >30s / closed states)"
  - "EmptyStateOnboarding component (3-stage D-21 progressive onboarding)"
  - "Sidebar Dashboard nav at index 0 with LayoutDashboard icon"
  - "per-MP latestInstantMap state structure (Plan 08 chart wiring may reuse)"

affects:
  - "04-08-PLAN (consumption chart + date-range picker layers on top of DashboardPage)"
  - "04-09-PLAN (MP detail — reuses same useSSE pattern with mp:<uuid> topic)"

# Tech tracking
tech-stack:
  added: []  # zero new dependencies
  patterns:
    - "Per-MP latestInstantMap: Record<mpId, {utility, instant}> seeded from snapshot.latest_readings; updated on SSE measurement events"
    - "instantOverride derivation: useMemo sums map entries per utility → passed to KpiGrid"
    - "Snapshot query enabled: enabled: scope.data.onboarding.uplink_count > 0"
    - "TDD: test files written first (RED), then implementation (GREEN), then build fix"
    - "Query cache pre-seeding in tests: qc.setQueryData(['dashboard','snapshot'], fixture) + waitFor for async render"

key-files:
  created:
    - "web/src/components/dashboard/KpiCard.tsx"
    - "web/src/components/dashboard/KpiCard.test.tsx"
    - "web/src/components/dashboard/KpiGrid.tsx"
    - "web/src/components/dashboard/KpiGrid.test.tsx"
    - "web/src/components/dashboard/LiveChannelBanner.tsx"
    - "web/src/components/dashboard/LiveChannelBanner.test.tsx"
    - "web/src/components/dashboard/EmptyStateOnboarding.tsx"
    - "web/src/components/dashboard/EmptyStateOnboarding.test.tsx"
    - "web/src/routes/dashboard.tsx"
    - "web/src/routes/dashboard.test.tsx"
    - "web/src/components/shell/sidebar.test.tsx"
  modified:
    - "web/src/App.tsx (lazy DashboardPage import; index route replaces IndexRedirect)"
    - "web/src/components/shell/sidebar.tsx (LayoutDashboard at NAV index 0; updated comment)"

key-decisions:
  - "IndexRedirect deleted from App.tsx import (not the file itself — left as deprecated); App.tsx now registers DashboardPage directly at index: true"
  - "instantOverride as useMemo over latestInstantMap: avoids recomputing on every render; only updates when the map changes"
  - "Query cache pre-seeding in tests (qc.setQueryData) + waitFor: more reliable than act+prefetch for async component rendering"
  - "Test isolation: vi.resetAllMocks() in beforeEach+afterEach to prevent mock queue leakage between tests"

requirements-completed: [DASH-01, DASH-02, DASH-03, DASH-06]

# Metrics
duration: 8min
completed: 2026-05-11
---

# Phase 4 Plan 07: Dashboard Route + KPI Grid + Onboarding Summary

**Dashboard route shell + KPI tile grid + empty-state onboarding + live-channel banner — the operator's primary surface.**

## Performance

- **Duration:** ~8 minutes
- **Started:** 2026-05-11T15:09:12Z
- **Completed:** 2026-05-11T15:17:30Z
- **Tasks:** 2
- **Files created/modified:** 11 new + 3 modified

## Accomplishments

### Task 1: Dashboard components

**KpiCard** (`web/src/components/dashboard/KpiCard.tsx`) — 4 variants:
- `today`: `text-2xl font-semibold leading-8` value + `text-sm font-mono text-muted-foreground` unit
- `instant`: same layout, different label ("Current flow" for water, "Current load" for electricity)
- `delta`: DeltaFooter with full color contract — water+positive → `text-warning`, all electricity → `text-muted-foreground`, zero → Minus icon, null comparison → em-dash + tooltip "Comparison available after 24 hours"
- `online`: OnlineFooter with all-online/some-offline/all-offline icon colors (success/warning/destructive)

**KpiGrid** (`web/src/components/dashboard/KpiGrid.tsx`):
- Capability-gated: water capability → 4 water tiles; electricity capability → 4 electricity tiles; both → 8 tiles in same auto-flow grid (water row first)
- Responsive grid: `grid grid-cols-1 sm:grid-cols-2 md:grid-cols-4 gap-4 md:gap-6`
- `instantOverride` prop accepts `{ water?: number; electricity?: number }` for SSE-driven live recompute

**LiveChannelBanner** (`web/src/components/dashboard/LiveChannelBanner.tsx`):
- `status='open'` or `'connecting'` → null (no banner)
- `status='reconnecting'` ≤30s → Alert warning "Reconnecting to live updates…" + "Last snapshot N sec ago" (mono)
- `status='reconnecting'` >30s → "Live updates paused" + "Showing last-known values from {time}"
- `status='closed'` → "Session expired" + `<Link to="/login">Sign in again</Link>`
- RotateCw animate-spin with `motion-reduce:animate-none` per UI-SPEC

**EmptyStateOnboarding** (`web/src/components/dashboard/EmptyStateOnboarding.tsx`) — 3-stage D-21:
- `(0, _, _)` → "Add your first gateway" + CTA → `/gateways`
- `(>0, 0, _)` → "Now add your first device" + CTA → `/devices`
- `(>0, >0, 0)` → "Waiting for first uplink…" with Loader2 animate-spin

### Task 2: Dashboard route + App.tsx + Sidebar

**DashboardPage** (`web/src/routes/dashboard.tsx`):
- Mounted at index (`/`) under `<RootLayout />` — auth-required group (T-04-07-01 satisfied)
- `useDashboardScope()` drives empty-state vs full-dashboard branch
- `useQuery(['dashboard','snapshot'])` enabled only when `uplink_count > 0`
- Per-MP `latestInstantMap: Record<mpId, {utility, instant}>` state seeded from `snapshot.latest_readings` on each snapshot load
- SSE `onMeasurement` callback updates map entry; `useMemo` derives `instantOverride` sums per utility
- `KpiGrid` receives `instantOverride` for live instant_total without snapshot refetch

**Sidebar** (`web/src/components/shell/sidebar.tsx`):
- `LayoutDashboard` icon imported from lucide-react
- `{ to: '/', label: 'Dashboard', icon: LayoutDashboard }` inserted at NAV index 0
- Comment updated: "Dashboard at top — operator's primary surface"

**App.tsx**:
- `IndexRedirect` import removed
- `const DashboardPage = lazy(() => import('@/routes/dashboard'))` added
- Index route updated: `{ index: true, element: <Suspense fallback={null}><DashboardPage /></Suspense> }`

**IndexRedirect disposition**: File `web/src/routes/index-redirect.tsx` left on disk as-is (no imports remain; Phase cleanup can delete it). The route it was providing (`/` → `/settings`) is now replaced by DashboardPage.

## Per-MP latestInstantMap State Structure (Plan 08 reference)

```typescript
// Keyed by metering_point_id UUID
type LatestInstantMap = Record<string, {
  utility: 'water' | 'electricity'
  instant: number | null
}>

// Seeded from snapshot.latest_readings on each snapshot load
// Updated on every SSE 'measurement' event via onMeasurement callback
// instantOverride derived:
const instantOverride = useMemo(() => {
  const sums = { water: 0, electricity: 0 }
  for (const entry of Object.values(latestInstantMap)) {
    if (entry.instant != null) sums[entry.utility] += entry.instant
  }
  return sums
}, [latestInstantMap])
```

Plan 08 (consumption chart) can access the same SSE stream and `latestInstantMap` pattern by lifting state or passing `onMeasurement` callbacks into the same `useSSE` call.

## Task Commits

1. **Task 1: KpiCard + KpiGrid + LiveChannelBanner + EmptyStateOnboarding** — `90d12b7` (feat)
2. **Task 2: dashboard.tsx route + sidebar nav + App.tsx wiring** — `6efd812` (feat)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Unused `formatValue` helper caused TypeScript build error**
- **Found during:** Task 2 acceptance check (`pnpm build`)
- **Issue:** `KpiCard.tsx` had a `formatValue` function defined but never called (value formatting was inlined directly in the JSX). TypeScript strict mode (`error TS6133`) rejected the unused declaration.
- **Fix:** Removed the unused `formatValue` helper function; the inline `Intl.NumberFormat` expression in the component body is sufficient.
- **Files modified:** `web/src/components/dashboard/KpiCard.tsx`
- **Committed in:** `6efd812`

**2. [Rule 1 - Bug] Dashboard route tests needed query cache pre-seeding + waitFor**
- **Found during:** Task 2 test run (3 capability-gating tests failed with "element not found")
- **Issue:** The component only renders KPI tiles when `snapshot.data` is non-null. The initial approach used `act + prefetchQuery` which didn't synchronously populate the cache before the assertion. Tests saw only the "Dashboard" heading (rendered before the `{snapshot.data && <KpiGrid .../>}` guard) but not the tile labels.
- **Fix:** Switched to `qc.setQueryData(['dashboard','snapshot'], fixture)` (synchronous cache seeding before render) + `waitFor` for async render assertions. This matches the established pattern from other route tests in the project.
- **Files modified:** `web/src/routes/dashboard.test.tsx`
- **Committed in:** `6efd812`

## Known Stubs

None — all components render real data from the API. The "Plan 08 inserts DateRangePicker here" and "Plan 08 inserts ConsumptionChart cards here" are code comments marking extension points, not UI stubs. EmptyStateOnboarding renders real CTA links.

## Threat Flags

None — no new network surface. DashboardPage mounts under RootLayout (auth-required group — T-04-07-01 satisfied). CTAs use client-side `<Link>` (T-04-07-03: no new auth bypass). Capability gating is UX-only per T-04-07-02 acceptance.

## Self-Check: PASSED

- `web/src/components/dashboard/KpiCard.tsx` — FOUND
- `web/src/components/dashboard/KpiCard.test.tsx` — FOUND
- `web/src/components/dashboard/KpiGrid.tsx` — FOUND
- `web/src/components/dashboard/KpiGrid.test.tsx` — FOUND
- `web/src/components/dashboard/LiveChannelBanner.tsx` — FOUND
- `web/src/components/dashboard/LiveChannelBanner.test.tsx` — FOUND
- `web/src/components/dashboard/EmptyStateOnboarding.tsx` — FOUND
- `web/src/components/dashboard/EmptyStateOnboarding.test.tsx` — FOUND
- `web/src/routes/dashboard.tsx` — FOUND
- `web/src/routes/dashboard.test.tsx` — FOUND
- `web/src/components/shell/sidebar.test.tsx` — FOUND
- Commit `90d12b7` (Task 1) — FOUND in `git log`
- Commit `6efd812` (Task 2) — FOUND in `git log`
- `grep "LayoutDashboard" web/src/components/shell/sidebar.tsx` — PASS
- `grep "to: '/', label: 'Dashboard'" web/src/components/shell/sidebar.tsx` — PASS
- `grep "useSSE" web/src/routes/dashboard.tsx` — PASS
- `pnpm test -- --run src/routes/dashboard src/components/dashboard src/components/shell` — 187/187 PASS
- `pnpm build` — exits 0

---
*Phase: 04-realtime-dashboard*
*Plan: 07*
*Completed: 2026-05-11*
