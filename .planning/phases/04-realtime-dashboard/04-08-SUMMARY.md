---
phase: 04-realtime-dashboard
plan: 08
subsystem: frontend-dashboard

tags: [dashboard, date-range, recharts, react-day-picker, shadcn, timeseries, sse, url-state, zod]

# Dependency graph
requires:
  - phase: 04-realtime-dashboard
    plan: 04
    provides: "GET /api/dashboard/timeseries endpoint; D-12 bucket schedule"
  - phase: 04-realtime-dashboard
    plan: 07
    provides: "DashboardPage route shell with Plan 08 insertion point comments"

provides:
  - "DateRangePicker component: segmented Today/24h/7d/30d + Custom popover with shadcn Calendar"
  - "URL state shape: ?range=today|24h|7d|30d|custom + ?start=ISO&end=ISO"
  - "computeRangeWindow utility: preset→{start,end,bucketSec} mirroring server D-12 schedule"
  - "ConsumptionChart: Recharts AreaChart with primary stroke + 0.1 fill + live pulse marker"
  - "CumulativeChartCard: shadcn Card wrapping ConsumptionChart + timeseries useQuery"
  - "dashboard.tsx: DateRangePicker in header; 1 or 2 chart cards per capability"
  - "react-day-picker v10 installed via shadcn calendar component"

affects:
  - "04-09-PLAN (per-MP detail page) — reuses DateRangePicker with mode='controlled'"

# Tech tracking
tech-stack:
  added:
    - "react-day-picker@10.0.0 (installed via pnpm dlx shadcn add calendar)"
  patterns:
    - "URL-state date range: useSearchParams + zod.catch('today') fallback (T-04-08-01)"
    - "computeRangeWindow mirrors server D-12 bucket schedule exactly"
    - "liveMode: preset==='today'||'24h' → ReferenceDot with animate-pulse motion-reduce:animate-none"
    - "CumulativeChartCard: staleTime=0 for live presets, 5min for historical"
    - "SVG className access: getAttribute('class') not .className (SVGAnimatedString)"

key-files:
  created:
    - "web/src/lib/dateRange.ts"
    - "web/src/lib/dateRange.test.ts"
    - "web/src/components/dashboard/DateRangePicker.tsx"
    - "web/src/components/dashboard/DateRangePicker.test.tsx"
    - "web/src/components/dashboard/ConsumptionChart.tsx"
    - "web/src/components/dashboard/ConsumptionChart.test.tsx"
    - "web/src/components/dashboard/CumulativeChartCard.tsx"
    - "web/src/components/dashboard/CumulativeChartCard.test.tsx"
    - "web/src/components/ui/calendar.tsx (added by shadcn)"
  modified:
    - "web/src/routes/dashboard.tsx (DateRangePicker + CumulativeChartCard inserted)"
    - "web/package.json (react-day-picker added)"
    - "web/pnpm-lock.yaml"

key-decisions:
  - "computeRangeWindow custom validation: throws for span<=0 or >1y; UI catches and disables commit button"
  - "SVG className is SVGAnimatedString — tests must use getAttribute('class') not .className"
  - "calendar.tsx table key removed: not present in react-day-picker v10 ClassNames type"
  - "waterCount/electricityCount derived from snapshot.latest_readings filtering by utility_class"

requirements-completed: [DASH-04, DASH-05]

# Metrics
duration: 8min
completed: 2026-05-11
---

# Phase 4 Plan 08: Date-Range Picker + Consumption Chart Cards Summary

**Segmented date-range picker (URL-state) + Recharts AreaChart consumption cards wired to /api/dashboard/timeseries with live-mode pulse marker.**

## Performance

- **Duration:** ~8 minutes
- **Started:** 2026-05-11T15:20:51Z
- **Completed:** 2026-05-11T15:28:30Z
- **Tasks:** 2
- **Files modified:** 9 new + 3 modified

## Accomplishments

### Task 1: dateRange utility + DateRangePicker component

**computeRangeWindow** (`web/src/lib/dateRange.ts`) — mirrors server D-12 exactly:
- `today` → midnight local time, bucketSec=300
- `24h` → now-24h, bucketSec=300
- `7d` → bucketSec=3600
- `30d` → bucketSec=14400
- `custom ≤30d` → bucketSec=3600; `custom >30d` → bucketSec=86400
- Throws for: missing start/end, start≥end, span>1y (T-04-08-02)

**DateRangePicker** (`web/src/components/dashboard/DateRangePicker.tsx`):
- 4 preset buttons (Today / 24h / 7d / 30d) + Custom popover trigger
- `mode='shared-url'`: reads/writes `?range`, `?start`, `?end` via `useSearchParams`
- `mode='controlled'`: parent owns state via `value`/`onChange` props (Plan 09 reuse)
- zod `.catch('today')` fallback on URL parse (T-04-08-01)
- Custom popover uses shadcn `<Calendar mode="range">` from react-day-picker v10
- Commit button disabled + tooltip when custom span outside 1h–1y bounds (T-04-08-02)
- Active preset indicated via `data-state="active"` attribute

### Task 2: ConsumptionChart + CumulativeChartCard + dashboard.tsx integration

**ConsumptionChart** (`web/src/components/dashboard/ConsumptionChart.tsx`):
- Recharts `AreaChart` wrapped in `ChartContainer`
- `stroke="var(--primary)"`, `fill="var(--primary)"`, `fillOpacity={0.1}` per UI-SPEC color contract
- `CartesianGrid` with `stroke="var(--border)"`
- X-axis formatted via date-fns: `HH:mm` for 5-min/hourly buckets, `MMM d` for daily
- Live-mode pulse marker: `ReferenceDot` at rightmost point with `animate-pulse motion-reduce:animate-none` (D-13)

**CumulativeChartCard** (`web/src/components/dashboard/CumulativeChartCard.tsx`):
- Reads `?range`, `?start`, `?end` from URL via `useSearchParams`
- `useQuery(['dashboard','timeseries',utility,preset,startISO,endISO])` → GET /api/dashboard/timeseries
- `staleTime=0` for `today`/`24h` (live mode); `staleTime=5min` for `7d`/`30d`/`custom`
- Title: `"Water — today"` / `"Electricity — last 30 days"` etc. (computed from preset)
- Skeleton while loading; ConsumptionChart once data resolves
- computeRangeWindow wrapped in try/catch — falls back to 'today' on invalid custom range

**dashboard.tsx integration**:
- `<DateRangePicker mode="shared-url" />` inserted in header right side
- `waterCount`/`electricityCount` derived from `snapshot.latest_readings` filtering by `utility_class`
- `capabilities !== 'electricity'` → render Water chart card
- `capabilities !== 'water'` → render Electricity chart card
- Water rendered above Electricity for `both` capability

## URL-State Shape (Plan 09 reference)

```
?range=today               (no start/end)
?range=24h                 (no start/end)
?range=7d                  (no start/end)
?range=30d                 (no start/end)
?range=custom&start=2026-01-01T00:00:00.000Z&end=2026-02-01T00:00:00.000Z
```

zod schema: `z.enum(['today','24h','7d','30d','custom']).catch('today')`

## computeRangeWindow → Server Bucket Schedule Alignment

| preset     | bucketSec | server D-12 |
|------------|-----------|-------------|
| today      | 300       | 5 minutes   |
| 24h        | 300       | 5 minutes   |
| 7d         | 3600      | 1 hour      |
| 30d        | 14400     | 4 hours     |
| custom≤30d | 3600      | 1 hour      |
| custom>30d | 86400     | 1 day       |

## Pulse Marker (D-13)

Applied when `preset === 'today' || preset === '24h'`. Recharts `<ReferenceDot>` at `data[data.length-1]` with:

```tsx
className="animate-pulse motion-reduce:animate-none"
```

`motion-reduce:animate-none` respects `prefers-reduced-motion` media query — operator with vestibular sensitivity sees no animation.

## Where Plan 09 Imports These Components

Plan 09 (per-MP detail page) can reuse:
- `DateRangePicker` with `mode='controlled'` (independent per-page state, not shared URL)
- `ConsumptionChart` directly with MP-specific timeseries data
- `computeRangeWindow` for the same bucket interval logic

## Task Commits

1. **Task 1: dateRange utility + DateRangePicker component** — `1887c8c` (feat)
2. **Task 2: ConsumptionChart + CumulativeChartCard + dashboard.tsx integration** — `58cfd8a` (feat)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] SVG className is SVGAnimatedString, not a plain string**
- **Found during:** Task 2 (ConsumptionChart tests)
- **Issue:** Test asserted `pulseEl?.className.toContain('motion-reduce:animate-none')` but SVG elements expose `className` as `SVGAnimatedString` — not a string — so `.toContain()` failed.
- **Fix:** Changed test to use `pulseEl?.getAttribute('class')` which returns the raw class attribute string.
- **Files modified:** `web/src/components/dashboard/ConsumptionChart.test.tsx`
- **Committed in:** `58cfd8a`

**2. [Rule 1 - Bug] calendar.tsx `table` key not in react-day-picker v10 ClassNames type**
- **Found during:** Task 2 (`pnpm build` TypeScript check)
- **Issue:** The shadcn-generated `calendar.tsx` included `table: "w-full border-collapse"` in the `classNames` object. react-day-picker v10's `ClassNames` type no longer has a `table` key (renamed to `month_grid`), causing TS2353 error.
- **Fix:** Removed the `table` key from the `classNames` object; the default styling is sufficient.
- **Files modified:** `web/src/components/ui/calendar.tsx`
- **Committed in:** `58cfd8a`

**3. [Rule 1 - Bug] Unused imports in test files caused TS6133 errors**
- **Found during:** Task 2 (`pnpm build`)
- **Issue:** `screen` imported but unused in `ConsumptionChart.test.tsx`; `skeletons` variable declared but unused in `CumulativeChartCard.test.tsx`.
- **Fix:** Removed unused `screen` import; replaced unused `skeletons` variable with direct `expect(document.body).toBeTruthy()`.
- **Files modified:** `ConsumptionChart.test.tsx`, `CumulativeChartCard.test.tsx`
- **Committed in:** `58cfd8a`

---

**Total deviations:** 3 auto-fixed (all Rule 1 bugs)
**Impact on plan:** All fixes were necessary for build correctness. No scope changes.

## Known Stubs

None — all components render real data from the API. CumulativeChartCard shows a Skeleton while loading (intentional UX, not a stub).

## Threat Flags

None — no new network surface. All chart data fetched via authenticated `/api/dashboard/timeseries` endpoint (auth required by RootLayout group). URL params validated client-side via zod `.catch('today')` and server-side by Plan 04 handler (T-04-08-01 defense-in-depth). Custom range >1y blocked before API call (T-04-08-02).

## Self-Check: PASSED

- `web/src/lib/dateRange.ts` — FOUND
- `web/src/lib/dateRange.test.ts` — FOUND
- `web/src/components/dashboard/DateRangePicker.tsx` — FOUND
- `web/src/components/dashboard/DateRangePicker.test.tsx` — FOUND
- `web/src/components/dashboard/ConsumptionChart.tsx` — FOUND
- `web/src/components/dashboard/ConsumptionChart.test.tsx` — FOUND
- `web/src/components/dashboard/CumulativeChartCard.tsx` — FOUND
- `web/src/components/dashboard/CumulativeChartCard.test.tsx` — FOUND
- `web/src/components/ui/calendar.tsx` — FOUND
- Commit `1887c8c` (Task 1) — FOUND
- Commit `58cfd8a` (Task 2) — FOUND
- `grep -q "useSearchParams" DateRangePicker.tsx` — PASS
- `grep -q "animate-pulse" ConsumptionChart.tsx` — PASS
- `grep -q "var(--primary)" ConsumptionChart.tsx` — PASS
- `pnpm test -- --run` — 214/214 PASS
- `pnpm build` — exits 0

---
*Phase: 04-realtime-dashboard*
*Plan: 08*
*Completed: 2026-05-11*
