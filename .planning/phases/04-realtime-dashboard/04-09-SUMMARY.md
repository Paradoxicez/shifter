---
phase: 04-realtime-dashboard
plan: "09"
subsystem: frontend
tags: [metering-point, detail-page, tabs, sse, virtualization, tdd]
dependency_graph:
  requires: [04-05, 04-06, 04-08]
  provides: [DETL-01, DETL-02, DETL-03]
  affects: [mp-detail-route, uplinks-log, advanced-tab]
tech_stack:
  added: ["@tanstack/react-virtual@^3.13.24"]
  patterns:
    - TDD red-green per task
    - URL-state tabs via useSearchParams
    - Parent-owned SSE pendingPayload state (forensic-safety D-20)
    - TanStack Table with optional react-virtual virtualization at >200 rows
    - Custom recursive JsonTree (no react-json-view)
key_files:
  created:
    - web/src/routes/metering-points/$id.tsx
    - web/src/routes/metering-points/$id.test.tsx
    - web/src/components/metering-point/NormalTab.tsx
    - web/src/components/metering-point/NormalTab.test.tsx
    - web/src/components/metering-point/AdvancedTab.tsx
    - web/src/components/metering-point/AdvancedTab.test.tsx
    - web/src/components/metering-point/UplinksLogTab.tsx
    - web/src/components/metering-point/UplinksLogTab.test.tsx
    - web/src/components/metering-point/JsonTree.tsx
    - web/src/components/metering-point/JsonTree.test.tsx
    - web/src/components/metering-point/QualityBadge.tsx
    - web/src/components/metering-point/QualityBadge.test.tsx
    - web/src/components/metering-point/SparklineTriplet.tsx
    - web/src/components/metering-point/SparklineTriplet.test.tsx
    - web/src/components/metering-point/HexPayloadCell.tsx
  modified:
    - web/package.json
decisions:
  - "Custom recursive JsonTree (D-20) — no react-json-view; pure React text nodes satisfy T-04-09-02 XSS mitigation"
  - "MAX_DEPTH=32 on JsonTree satisfies T-04-09-03 DoS mitigation; truncated node shows '...truncated'"
  - "Parent route ($id.tsx) owns pendingPayload state — Advanced tab receives three-prop contract (latestReading, pendingPayload, clearPending); prevents forensic-safety swap-on-receive (D-20)"
  - "Quality filter chips moved outside loading guard so they always render, preventing layout shift during load (deviation from naive implementation)"
  - "Virtualization threshold at 200 rows using @tanstack/react-virtual; plain map() below threshold"
  - "D-22 uses disabled TabsTrigger wrapped in span+Tooltip (disabled buttons cannot receive tooltip events)"
metrics:
  duration_minutes: ~120
  completed_date: "2026-05-11"
  tasks_completed: 3
  files_created: 16
  tests_added: 269
---

# Phase 04 Plan 09: Per-Meter Detail Page (3-Tab Layout) Summary

Replaced Phase 2's minimal metering-point page with a full 3-tab operator drill-down surface: Normal, Advanced, and Uplinks log. Ships 1 route, 7 components, 8 test files. All 269 tests pass; build clean.

## What Was Built

### $id.tsx Route (181 lines)

Fetches `GET /api/metering-points/:id` via `useQuery(['mp', id, 'detail'])` and `GET /api/metering-points/:id/signal-history` (enabled only when `latest_reading != null`). Subscribes to SSE topics `mp:<id>` and `mp:<id>:uplinks`. Holds `pendingPayload: MeasurementDelta | null` in local state — set from the SSE `onMeasurement` callback, passed as props to AdvancedTab.

**Tab URL state:** `?tab=normal|advanced|uplinks` (default: `normal`)

**D-22 empty case:** When `latest_reading == null`, Advanced and Uplinks tabs are rendered as `disabled` `TabsTrigger` elements wrapped in `<span><Tooltip>` so the tooltip fires on the wrapper (disabled buttons cannot receive pointer events).

### NormalTab (226 lines)

**Full case (latest_reading present):**
- Status row: online/offline badge + relative "last uplink" time + QualityBadge (right-aligned)
- Cumulative card (big mono number + unit: m³ or kWh)
- Instant card (L/h or kW)
- SparklineTriplet (battery, RSSI, SNR from signal history)

**D-22 empty case:** MP info card + "No device bound" message + "Add device" CTA (admin-only via `useCurrentUser()`).

### AdvancedTab (91 lines)

**Prop contract (single source of truth):**
```typescript
export interface AdvancedTabProps {
  latestReading: NonNullable<DetailResponse['latest_reading']>
  pendingPayload: MeasurementDelta | null
  clearPending: () => void
}
```

When `pendingPayload !== null`, renders an `<Alert>` with "Newer payload available · [Refresh]". Refresh button calls `clearPending()` — the parent's state resets to null, and the next detail query refetch surfaces the new decoded_object/extra via the `latestReading` prop.

`<JsonTree value={{ object: displayedReading.decoded_object, extra: displayedReading.extra }} defaultOpen />` — `extra` key auto-opens per D-20 specification.

### UplinksLogTab (452 lines)

TanStack Table with 9 columns (expand, time, quality, cumulative, instant, battery, RSSI, SNR, fcnt). Pagination: 100 initial + "Load more" (+100 per click, cap 500). Expanded row shows `HexPayloadCell` left + `JsonTree` right.

**Quality filter:** ToggleGroup chips for `ok | decode fail | missing canonical | out of range | duplicate fcnt`. URL state via `?quality=ok,decode_fail,...`. Chips always rendered (outside loading guards) to avoid layout shift.

**Virtualization:** `useVirtualizer` from `@tanstack/react-virtual` activates when `rows.length > 200`. Below threshold: plain `Array.map()`. Virtualized container gets `data-testid="virtualized"` for test assertion.

**Empty state:** "No uplinks match your filters" + "Clear filters" button (clears URL quality param).

### JsonTree (127 lines)

Custom recursive collapsible tree. No third-party dependencies. Type colors: string=`text-success`, number=`text-info`, boolean=`text-warning`, null=`text-muted-foreground italic`. Expand/collapse via chevron button. `MAX_DEPTH=32` — beyond renders `<span>...truncated</span>`. Auto-opens when `defaultOpen=true` or `name === 'extra'` (D-20).

### QualityBadge (50 lines)

`flaggedCount === 0` → "All ok" Badge (no click). `flaggedCount > 0` → clickable button with warning Badge showing "N of last M uplinks flagged". Click navigates to `?tab=uplinks&quality=decode_fail,missing_canonical,out_of_range,duplicate_fcnt` (D-19 handoff).

### SparklineTriplet (147 lines)

Three Recharts AreaCharts. Band-based fill colors per metric:
- Battery: `<10%` critical (destructive), `10-30%` warning, `>30%` healthy (success)
- RSSI: `<-115 dBm` critical, `-115 to -105` warning, `>-105` healthy
- SNR: `<-10 dB` critical, `-10 to 0` warning, `>0` healthy

`data-metric` and `data-band` attributes on containers for test assertions.

### HexPayloadCell (41 lines)

Renders hex payload as monospace word-wrapped text + copy-to-clipboard `<Button variant="ghost">` + Sonner toast on copy.

## Decisions Made

1. **Custom JsonTree (D-20):** No `react-json-view` or other third-party tree library. Pure React text nodes for all values — satisfies T-04-09-02 (no `dangerouslySetInnerHTML`). MAX_DEPTH=32 satisfies T-04-09-03.

2. **Parent holds pendingPayload state:** Route `$id.tsx` owns the SSE subscription and `pendingPayload` state. AdvancedTab is a pure-ish component receiving three props. This preserves forensic-safety (D-20): new payloads do not auto-swap the displayed reading — user must explicitly click Refresh.

3. **Quality chips outside loading guard:** UplinksLogTab restructured so `ToggleGroup` renders unconditionally; `{loading && <div>Loading...</div>}`, `{!loading && rows.length === 0 && <div>No uplinks...</div>}`, and `{!loading && rows.length > 0 && <table>}` are separate conditional blocks. This prevents chips disappearing during loads.

4. **Virtualization threshold 200:** `useVirtualizer` activates at `rows.length > 200`. The `ROW_CAP = 500` ensures the virtualizer never exceeds 500 items.

5. **D-22 disabled tab tooltip wrapper:** Disabled HTML buttons cannot receive pointer events; `<span>` wraps the disabled `TabsTrigger` so the `TooltipTrigger asChild` fires on pointer-enter of the span.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Quality filter chips rendered inside loading guard — chips disappeared during fetches**
- **Found during:** Task 2 (UplinksLogTab)
- **Issue:** Initial implementation had chips inside the early-return loading block; they vanished on refetch
- **Fix:** Restructured UplinksLogTab to always render chips; conditional loading/empty/table blocks follow
- **Files modified:** web/src/components/metering-point/UplinksLogTab.tsx
- **Commit:** 3a6ca1f

**2. [Rule 1 - Bug] NormalTab.test.tsx mock SessionUser missing required fields**
- **Found during:** Task 3 build verification
- **Issue:** `vi.mock` factory and `beforeEach` returned `{ role: 'admin', email: '...' }` missing `id` and `must_change_password` from `SessionUser` interface — TypeScript error TS2345
- **Fix:** Added `id: 'admin-id'` and `must_change_password: false` to both mock locations
- **Files modified:** web/src/components/metering-point/NormalTab.test.tsx
- **Commit:** 6baf760

**3. [Rule 1 - Bug] $id.test.tsx imported unused `fireEvent` — TypeScript error TS6133**
- **Found during:** Task 3 build verification
- **Issue:** `fireEvent` imported but never used in the test file
- **Fix:** Removed `fireEvent` from the import line
- **Files modified:** web/src/routes/metering-points/$id.test.tsx
- **Commit:** 6baf760

## Commits

| Task | Name | Commit |
|------|------|--------|
| 1 | JsonTree + QualityBadge + SparklineTriplet + HexPayloadCell | df33f2a |
| 2 | NormalTab + AdvancedTab + UplinksLogTab | 3a6ca1f |
| 3 | $id.tsx route 3-tab layout + bug fixes | 6baf760 |

## Verification

All plan acceptance checks pass:

- `pnpm test -- --run src/routes/metering-points/ src/components/metering-point/` — 269 tests, 43 files, all pass
- `pnpm build` — clean, no TypeScript errors
- `@tanstack/react-virtual` present in `web/package.json`
- `JsonTree` present in `AdvancedTab.tsx`
- "Newer payload available" present in `AdvancedTab.tsx`
- `pendingPayload: MeasurementDelta | null` present in `AdvancedTab.tsx`

## Known Stubs

None. All data flows are wired to real API calls; no hardcoded mock values in production code.

## Threat Flags

No new security-relevant surface beyond the threat model. T-04-09-01/02/03/04 all mitigated:
- T-04-09-01: Admin CTAs gated via `useCurrentUser()` — viewer sees no "Add device" button
- T-04-09-02: All JsonTree rendering uses React text nodes — no `dangerouslySetInnerHTML`
- T-04-09-03: MAX_DEPTH=32 in JsonTree — beyond depth cap renders `...truncated`
- T-04-09-04: `useVirtualizer` activates at >200 rows — 500-row cap plus virtualization

## Self-Check: PASSED
