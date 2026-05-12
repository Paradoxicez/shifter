---
phase: 05-aggregates-reports-map-floor-plans
plan: "09"
subsystem: reports-frontend
tags: [react, reports, pdf-polling, url-state, zod, tanstack-query, sonner, recharts]
dependency_graph:
  requires: [05-06, 05-08]
  provides:
    - "/reports route with config + result panels"
    - "useReportGenerate hook (POST /api/reports/generate)"
    - "useReportPDFStatus hook (polls GET /api/reports/:id every 2s)"
    - "ReportConfigPanel (3-radio scope + pickers + range presets)"
    - "ReportResultPanel (3 download tiles + chart + tables)"
    - "PdfStatusPill (spinner/download/error states)"
    - "ReportSummaryChart (capability-gated BarChart)"
    - "ReportPeriodTable (D-03 YoY silent fallback)"
    - "ReportMeterTable (per-meter breakdown)"
    - "MeterCombobox (Command/Popover MP search)"
  affects: [web/src/App.tsx, sidebar navigation already updated in 05-08]
tech_stack:
  added: []
  patterns:
    - "URL state via useSearchParams + zod .catch() fallbacks (D-05, T-05-09-01 mitigation)"
    - "Dedicated mutation hook extracted to own file (no inline useMutation in index.tsx)"
    - "useReportPDFStatus: staleTime=Infinity for terminal initialStatus prevents spurious background refetch"
    - "D-03 YoY silent fallback: rows.some(r => r.delta_vs_yoy) gates column visibility"
    - "Real-timer waitFor pattern in useReportPDFStatus tests (fake timers + TanStack Query refetchInterval = infinite-loop risk)"
key_files:
  created:
    - web/src/routes/reports/useReportGenerate.ts
    - web/src/routes/reports/useReportPDFStatus.ts
    - web/src/routes/reports/useReportPDFStatus.test.ts
    - web/src/routes/reports/index.tsx
    - web/src/routes/reports/ReportConfigPanel.tsx
    - web/src/routes/reports/ReportResultPanel.tsx
    - web/src/routes/reports/PdfStatusPill.tsx
    - web/src/routes/reports/ReportSummaryChart.tsx
    - web/src/routes/reports/ReportPeriodTable.tsx
    - web/src/routes/reports/ReportMeterTable.tsx
    - web/src/routes/reports/MeterCombobox.tsx
  modified:
    - web/src/routes/reports/index.test.tsx
    - web/src/App.tsx
    - web/playwright/specs/reports-generate.spec.ts
decisions:
  - "MeterCombobox is new in Phase 5 (no existing MP combobox in the codebase). Built as a local Command/Popover wrapper in web/src/routes/reports/MeterCombobox.tsx rather than a shared component — single consumer for now."
  - "DateRangePicker reuse: the Phase 4 DateRangePicker (mode='controlled') handles today/24h/7d/30d presets. For the reports custom range, a simpler inline CustomDateRangePicker (Popover+Calendar) was built inside ReportConfigPanel.tsx — the Phase 4 presets don't map to reports ranges (daily/monthly/yearly/custom)."
  - "useReportPDFStatus fake-timer tests replaced with real-timer waitFor: vi.useFakeTimers() + TanStack Query's setInterval-based refetchInterval causes 'Aborting after 10000 timers, assuming infinite loop'. Real-timer approach with waitFor(..., {timeout:5000}) is more robust and tests the actual polling behavior."
  - "staleTime=Infinity + initialDataUpdatedAt=Date.now() for terminal initialStatus: TanStack Query treats initialData as stale by default (staleTime=0) and fires a background refetch immediately. For already-ready/failed/expired reports, this refetch is wasteful. staleTime=Infinity prevents it while still letting refetchInterval drive active polling for pending/running."
metrics:
  duration: "~13 min"
  completed_date: "2026-05-12"
  tasks_completed: 2
  files_modified: 13
---

# Phase 05 Plan 09: Reports Frontend Summary

**One-liner:** Full `/reports` surface — URL-state config panel with 3-radio scope picker, dedicated `useReportGenerate` hook (no inline mutation in index.tsx), PDF async poll via `useReportPDFStatus` with Sonner toast, 3 download tiles, D-03 YoY silent fallback, capability-gated summary chart, empty state for zero-sites.

## What Was Built

### Task 1 (committed 85e469f)

**`useReportGenerate.ts`** — Standalone TanStack `useMutation` wrapper for `POST /api/reports/generate`. Exports `ReportGenerateRequest` and `GenerateResponse` types. No mutation logic in `index.tsx`.

**`index.tsx`** — State machine: config panel ↔ result panel (ephemeral, D-07). URL state via `useSearchParams + zod` with `.catch()` fallbacks (T-05-09-01). Zero-sites empty state with "Nothing to report yet" / "Go to Sites" CTA.

**`ReportConfigPanel.tsx`** — 3-radio scope picker (All meters / Single site / Single meter) with conditional Group by select, site dropdown, meter combobox. Range presets (Daily / Monthly / Yearly / Custom) with inline `CustomDateRangePicker` popover. Generate button with Loader2 spinner + disabled state while `isPending`.

**`MeterCombobox.tsx`** — Command/Popover wrapper for metering point search (new Phase 5 component — no existing MP combobox).

**`App.tsx`** — `/reports` lazy route added under root layout.

**`index.test.tsx`** — 13 vitest cases: 3-radio render, conditional pickers, Generate POST assertion, loading state, result panel on success, navigate-away (D-07), empty state copy, URL state restoration, `useReportGenerate` hook smoke test.

### Task 2 (committed 4ce16e5)

**`useReportPDFStatus.ts`** — Polls `GET /api/reports/:id` every 2s while `pending`/`running`. Uses `refetchInterval` returning `false` for terminal statuses. `staleTime=Infinity` + `initialDataUpdatedAt=Date.now()` for terminal `initialStatus` prevents background refetch. Fires `toast.success('Your PDF is ready — click to download.')` with Download action once on `ready` transition; `toast.error` once on `failed`. `useRef` guards against duplicate toasts on React StrictMode double-invoke.

**`useReportPDFStatus.test.ts`** — 5 vitest cases: polls-while-pending, stops-after-ready, toast-success-once, toast-error-on-failed, no-poll-when-initialStatus-ready. Uses real timers + `waitFor` (fake timers caused TanStack Query interval infinite-loop — see Deviations).

**`PdfStatusPill.tsx`** — Switch on `PdfStatus`: `pending/running` → Loader2 spinner + "Generating PDF…" (`aria-live="polite"`, `motion-reduce:animate-none`); `ready` → "Download PDF" `<a download>`; `failed` → "PDF generation failed"; `expired` → "Report expired".

**`ReportResultPanel.tsx`** — 3-column download tile grid (CSV immediate, Excel immediate, PDF via `PdfStatusPill`). Wires `useReportPDFStatus`. Renders `ReportSummaryChart`, `ReportPeriodTable`, `ReportMeterTable` (meter table hidden for `scope=meter`).

**`ReportSummaryChart.tsx`** — BarChart per utility class using `ChartContainer` (shadcn Recharts wrapper). Capability-gated: derives utility classes from `meter_rows`; single-capability → 1 chart; both → 2 side-by-side on lg. Mirrors dashboard `ConsumptionChart` styling (primary color, muted-foreground ticks, border grid).

**`ReportPeriodTable.tsx`** — Period / Consumption / vs Prior / vs YoY columns. D-03 silent fallback: `rows.some(r => r.delta_vs_yoy != null)` gates YoY column visibility. `DeltaBadge` component with green/red color per sign.

**`ReportMeterTable.tsx`** — Meter / Site / Utility class / Total breakdown. Shown when `cfg.scope !== 'meter'`.

**`reports-generate.spec.ts`** — Playwright E2E spec (no `test.skip`): full generate-once flow + Configure-another back-navigation.

## Vitest Case Count

| File | Cases |
|------|-------|
| `index.test.tsx` | 13 |
| `useReportPDFStatus.test.ts` | 5 |
| **Total** | **18** |

## Playwright PDF Poll → Toast → Download Timing

- PDF generation via River background job: typically 1–5s for small reports (plan 05-06 `MaxWorkers=4`, maroto v2 is fast)
- Poll interval: 2s
- Expected end-to-end latency: 4–8s from Generate click to "Download PDF" link appearing
- Playwright budget: `timeout: 30_000` (generous for slow CI environments)

## Open Question: Custom Range Default

**Q:** Should "Custom" range default the picker to the last 30 days?

**Recommendation:** Yes — default `start = now - 30d`, `end = now` when user clicks Custom with no prior start/end in URL. This prevents an empty date picker being the first thing the user sees. Can be implemented by adding a `defaultCustomRange` helper in `ReportConfigPanel.tsx`. Deferred to a future polish plan.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Radix RadioGroupItem renders as `<button role="radio">` not `<input type="radio">`**
- **Found during:** Task 1 test run
- **Issue:** Test asserted `siteRadio.checked === true` (native input property). Radix RadioGroupItem renders a `<button>` with `aria-checked` attribute, not a native `<input>`.
- **Fix:** Changed assertion to `expect(siteRadio).toHaveAttribute('aria-checked', 'true')`
- **Files modified:** `index.test.tsx`
- **Commit:** `85e469f`

**2. [Rule 1 - Bug] TypeScript unused variable errors in test mocks**
- **Found during:** Task 1 build verification
- **Issue:** Mock function signatures declared `value` parameter but didn't use it, causing `TS6133`.
- **Fix:** Changed `{ value, onChange }` to `{ onChange }` with optional `value?` in mock signatures.
- **Files modified:** `index.test.tsx`
- **Commit:** `85e469f`

**3. [Rule 1 - Bug] vi.useFakeTimers() + TanStack Query refetchInterval = infinite timer loop**
- **Found during:** Task 2 test run (useReportPDFStatus.test.ts)
- **Issue:** `vi.advanceTimersByTimeAsync` inside `act()` caused TanStack Query's `setInterval`-based refetchInterval to spin to "10000 timers, assuming infinite loop" abort.
- **Fix:** Replaced fake-timer approach with real timers + `waitFor(..., {timeout: 5000})`. Tests are behavior-focused (fetch called during pending window, toast fires once) rather than exact-call-count at exact time.
- **Files modified:** `useReportPDFStatus.test.ts`
- **Commit:** `4ce16e5`

**4. [Rule 2 - Missing Critical] staleTime=Infinity for terminal initialStatus**
- **Found during:** Task 2 test run
- **Issue:** TanStack Query treats `initialData` as stale by default, firing an immediate background refetch even when status is already `ready`. Test for "does not poll when initialStatus is already ready" was failing with call count = 1.
- **Fix:** Added `initialDataUpdatedAt: isTerminal(initialStatus) ? Date.now() : 0` and `staleTime: isTerminal(initialStatus) ? Infinity : 0` to prevent spurious refetch for terminal statuses.
- **Files modified:** `useReportPDFStatus.ts`
- **Commit:** `4ce16e5`

**5. [Rule 3 - Blocking] DateRangePicker Phase 4 component not reused for custom range**
- **Found during:** Task 1 implementation
- **Issue:** Phase 4 `DateRangePicker` (mode='controlled') has its own preset set (today/24h/7d/30d) and a `DateRangeValue` shape that doesn't map to the reports range schema (daily/monthly/yearly/custom). Importing it for the custom-range case would bring in unwanted preset buttons and incompatible state shape.
- **Fix:** Built a minimal inline `CustomDateRangePicker` (Popover + shadcn Calendar in range mode) within `ReportConfigPanel.tsx`. Zero new shadcn primitives added (Calendar and Popover were already installed).
- **Files modified:** `ReportConfigPanel.tsx`
- **Commit:** `85e469f`

## Known Stubs

None. All data paths are wired:
- CSV/Excel tiles link directly to `/api/reports/:id/file/{csv|xlsx}` (immediate)
- PDF tile polls via `useReportPDFStatus` → `PdfStatusPill` reflects live status
- Charts and tables render from `GenerateResponse.report` data returned by the generate endpoint
- Empty state queries `/api/sites` for zero-sites detection

## Threat Surface Scan

No new network endpoints beyond those established in plans 05-03 and 05-06. The frontend makes:
- `GET /api/sites` (already auth-gated)
- `GET /api/metering-points` (already auth-gated)
- `POST /api/reports/generate` (already auth-gated, plan 05-06)
- `GET /api/reports/:id` (already auth-gated, plan 05-06)
- `GET /api/reports/:id/file/{csv|xlsx|pdf}` (already auth-gated, plan 05-06)

T-05-09-01 mitigated: zod `.catch()` fallbacks on all URL params prevent injection of invalid scope/range/uuid values.

No new trust boundaries introduced.

## Self-Check

Files exist:
- `web/src/routes/reports/useReportGenerate.ts` — EXISTS
- `web/src/routes/reports/useReportPDFStatus.ts` — EXISTS
- `web/src/routes/reports/useReportPDFStatus.test.ts` — EXISTS
- `web/src/routes/reports/index.tsx` — EXISTS
- `web/src/routes/reports/ReportConfigPanel.tsx` — EXISTS
- `web/src/routes/reports/ReportResultPanel.tsx` — EXISTS
- `web/src/routes/reports/PdfStatusPill.tsx` — EXISTS
- `web/src/routes/reports/ReportSummaryChart.tsx` — EXISTS
- `web/src/routes/reports/ReportPeriodTable.tsx` — EXISTS
- `web/src/routes/reports/ReportMeterTable.tsx` — EXISTS
- `web/src/routes/reports/MeterCombobox.tsx` — EXISTS

Commits exist:
- `85e469f` — Task 1
- `4ce16e5` — Task 2

Tests: 307 passed / 0 failed
Build: clean (✓ built in 3.46s)
