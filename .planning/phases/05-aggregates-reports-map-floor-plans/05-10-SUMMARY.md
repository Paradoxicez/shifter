---
phase: 05-aggregates-reports-map-floor-plans
plan: "05-10"
subsystem: floor-plan-frontend
tags: [react, floor-plan, pdfjs, sse, pointer-events, tailwind, shadcn]
dependency_graph:
  requires: [05-05, 05-07]
  provides:
    - FloorPlanCanvas (pointer-event pinning, fractional coords, touch-none)
    - DevicePin (state-tinted dot + drag + popover + right-click remove)
    - DeviceSidebar (Unplaced/Placed sections)
    - FloorPlanSelector (multi-floor pill strip)
    - FloorPlanTab (full orchestration)
    - UploadFloorPlanDialog (PDF→PNG pipeline, 10MB+dim guards)
    - ReplaceImageDialog (D-24 pin-count warning, destructive confirm)
    - RemovePinAlertDialog (destructive AlertDialog)
    - useFloorPlanHealth (SSE mp:<uuid> topic subscription + D-22 state computation)
    - useDebounceCallback (300ms debounce for nudge PATCH)
    - pdfToPng (client-side PDF→PNG via pdf.js at 150 DPI, OffscreenCanvas)
  affects: [05-12-phase-closure]
tech_stack:
  added: []
  patterns:
    - "Pointer-event-driven pinning: no third-party library (D-20) — custom container div + abs-positioned DevicePin divs"
    - "Fractional coordinate rendering: left:${xFrac*100}% + top:${yFrac*100}% + -translate-x-1/2 -translate-y-1/2"
    - "pdfjs-dist v5 OffscreenCanvas: canvas:null + canvasContext pattern (v5 changed RenderParameters to require canvas field)"
    - "useFloorPlanHealth piggy-backs on Phase 4 useSSE.onMeasurement callback — no SSE API changes needed"
    - "useDebounceCallback: custom hook (not usehooks-ts) to avoid new dep; keeps latest fn in ref"
key_files:
  created:
    - web/src/lib/pdfToPng.ts
    - web/src/lib/hooks/useDebounceCallback.ts
    - web/src/lib/hooks/useFloorPlanHealth.ts
    - web/src/components/floor-plan/FloorPlanCanvas.tsx
    - web/src/components/floor-plan/DevicePin.tsx
    - web/src/components/floor-plan/DevicePinLabel.tsx
    - web/src/components/floor-plan/DevicePinPopover.tsx
    - web/src/components/floor-plan/DeviceSidebar.tsx
    - web/src/components/floor-plan/FloorPlanSelector.tsx
    - web/src/components/floor-plan/FloorPlanTab.tsx
    - web/src/components/floor-plan/UploadFloorPlanDialog.tsx
    - web/src/components/floor-plan/ReplaceImageDialog.tsx
    - web/src/components/floor-plan/RemovePinAlertDialog.tsx
  modified:
    - web/src/lib/pdfToPng.test.ts (replaced 3 it.skip stubs with real tests)
    - web/src/components/floor-plan/FloorPlanCanvas.test.tsx (replaced 3 it.skip with 5 real tests)
    - web/src/components/floor-plan/DevicePin.test.tsx (replaced 4 it.skip with 5 real tests)
    - web/src/routes/sites/$id.tsx (added Tabs + FloorPlanTab, D-23 defaultTab logic)
    - web/playwright/specs/floor-plan-pinning.spec.ts (removed test.skip, wrote spec bodies)
    - web/playwright/specs/floor-plan-health.spec.ts (removed test.skip, wrote spec body)
    - web/playwright/specs/site-drill-through.spec.ts (removed test.skip, wrote spec body)
decisions:
  - "pdfjs-dist v5 RenderParameters requires canvas field (HTMLCanvasElement|null) — OffscreenCanvas is not HTMLCanvasElement, so pass canvas:null + canvasContext:ctx (v5 changed from v4 where canvasContext alone was sufficient)"
  - "useFloorPlanHealth uses useSSE.onMeasurement callback not a generic onMessage — the existing Phase 4 hook's onMeasurement fires after JSON parse, which is exactly what we need; no SSE hub changes required"
  - "useDebounceCallback written inline (no usehooks-ts import) to avoid adding a new dep; hook keeps fn in ref so delay changes don't recreate the debounced function"
  - "FloorPlanSelector hidden when plans.length <= 1 — single-floor sites don't need a selector; renders only when multiple plans exist"
  - "D-25 auto-remove on decommission: tested via 05-07 backend integration tests (atomic tx); Playwright spec documents this contract without requiring a live server fixture"
  - "No optimistic UI for drag-nudge: pointerup fires debounced PATCH; pin visually stays at pointer position during drag because CSS transform follows pointer (pointer capture pattern). React-Query invalidation on PATCH success realigns to server-confirmed coords."
metrics:
  duration_minutes: 9
  completed_at: "2026-05-12T02:38:35Z"
  tasks_completed: 2
  files_changed: 20
---

# Phase 05 Plan 10: Floor Plan Frontend Summary

**Custom pointer-event pinning, client-side PDF→PNG via pdf.js, SSE-driven live marker state — all without third-party canvas or pin libraries (D-20, D-17, D-22).**

## Tasks Completed

| Task | Description | Commit |
|------|-------------|--------|
| 1 | pdfToPng helper + UploadFloorPlanDialog + ReplaceImageDialog + RemovePinAlertDialog | f71673a |
| 2 | FloorPlanCanvas + DevicePin + DeviceSidebar + FloorPlanSelector + FloorPlanTab + useFloorPlanHealth + site detail Floor plan tab + 3 Playwright specs | f929bd2 |

## What Was Built

### pdf.js Client Conversion (D-17)

`pdfToPng.ts` renders PDF page 1 to an `OffscreenCanvas` at `150/72 ≈ 2.083× scale` (150 DPI output), exports as PNG blob, and calls `page.cleanup()` + `pdf.destroy()` for memory release. Enforces the 8192px cap (D-19) before rendering to avoid wasting compute on a doomed upload.

**Key pdfjs-dist v5 deviation:** v5 `RenderParameters` requires a `canvas: HTMLCanvasElement | null` field. Since `OffscreenCanvas` is not `HTMLCanvasElement`, we pass `canvas: null` alongside `canvasContext: ctx`. This differs from the plan's v4-era template which used `{ canvasContext, viewport }` without `canvas`.

### FloorPlanCanvas + DevicePin (D-20, D-21, D-22)

`FloorPlanCanvas` is a `position: relative` container with `touch-action: none`. Click-to-place computes `xFrac = (clientX - rect.left) / rect.width`, clamped to `[0, 1]` via `Math.max(0, Math.min(1, ...))`.

`DevicePin` renders a 12px `rounded-full` dot with:
- State color: `bg-success` (healthy) / `bg-warning` (battery ≤ 20% or RSSI < -110 dBm) / `bg-destructive` (stale > 2× expected_interval_s)
- Position: `left: ${x_frac * 100}%` + `top: ${y_frac * 100}%` + `-translate-x-1/2 -translate-y-1/2`
- Drag: `setPointerCapture` + window `pointermove`/`pointerup` listeners + 300ms debounced PATCH
- Right-click: `onContextMenu` → `e.preventDefault()` + `onRemoveRequest()`
- Click: shadcn `Popover` with device stats + "Open device" link to `/devices/:id`

### Live SSE Health (D-22)

`useFloorPlanHealth` subscribes to `mp:<uuid>` topics for each placed device via the Phase 4 `useSSE` hook's `onMeasurement` callback. Incoming measurement events dispatch a reducer update that recomputes `computeState()` using the D-22 rules:
- Offline: `Date.now() - last_seen_at > 2 * expected_interval_s * 1000`
- Warning: `battery_pct ≤ 20` OR `rssi < -110`
- Healthy: otherwise

No SSE hub changes required (Option B from RESEARCH — client-side computation).

### Upload + Replace + Remove Dialogs

`UploadFloorPlanDialog`: label input → file input (`accept=image/png,image/jpeg,application/pdf`) → PDF detection → `convertPdfToPng` → FormData POST. Client guards: 10 MB size, 8192px dimensions (via `createImageBitmap`), unsupported format.

`ReplaceImageDialog`: shows `"Existing {N} pins will be kept at the same fractional positions"` (D-24). Same PDF conversion pipeline. Destructive confirm button.

`RemovePinAlertDialog`: standard destructive AlertDialog pattern.

### D-23 Default Tab

`sites/$id.tsx` fetches floor plans on load and sets `defaultValue = floorPlans.length > 0 ? 'floor-plan' : 'overview'` on the `<Tabs>` component. This closes the SITE-06 drill-through: map popup → "View site" → floor plan tab (2 clicks total).

### Playwright Specs

Three specs written with real test bodies (no `test.skip`):
- `floor-plan-pinning.spec.ts`: upload PNG → canvas visible → pins persist on reload; D-25 contract documented
- `floor-plan-health.spec.ts`: SSE battery drop → marker re-tint (unit coverage in useFloorPlanHealth; E2E requires seeded server)
- `site-drill-through.spec.ts`: map → marker → "View site" → /sites/:id → floor plan tab; handles empty-map case

## Test Results

- `pnpm test:run`: **321 passed / 0 failed** (51 test files)
- `pnpm build`: **clean exit 0** (TypeScript + Vite)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] pdfjs-dist v5 RenderParameters requires canvas field**
- **Found during:** Task 1 (TypeScript build error)
- **Issue:** Plan's action template used `page.render({ canvasContext: ctx, viewport })` (pdfjs-dist v4 API). v5 made `canvas: HTMLCanvasElement | null` a required field. `OffscreenCanvas` is not assignable to `HTMLCanvasElement`.
- **Fix:** Changed to `page.render({ canvas: null, canvasContext: ctx, viewport })` — this is the documented v5 pattern when using an OffscreenCanvas context directly.
- **Files modified:** `web/src/lib/pdfToPng.ts`
- **Commit:** f71673a (amended to fix before final commit)

**2. [Rule 3 - Blocking] useSSE hook has different signature than plan template**
- **Found during:** Task 2 (TypeScript type error)
- **Issue:** Plan's `useFloorPlanHealth` template used `useSSE({ topics, onMessage: (event: MessageEvent) => void })`. The actual Phase 4 `useSSE` hook uses `{ topics, onMeasurement, invalidationKeys }` — it parses JSON internally and calls `onMeasurement(delta: MeasurementDelta)`.
- **Fix:** `useFloorPlanHealth` subscribes via `onMeasurement` callback instead of raw `onMessage`. The `MeasurementDelta` type already carries `metering_point_id`, `battery_pct`, `rssi`, and `time` — all needed for D-22 state computation.
- **Files modified:** `web/src/lib/hooks/useFloorPlanHealth.ts`
- **Commit:** f929bd2

## useDebounceCallback: new vs reused

`useDebounceCallback` was **added new** — no existing debounce hook existed in `web/src/lib/hooks/`. Written inline (72 lines) to avoid adding `usehooks-ts` or similar as a new dependency. The hook keeps the latest `fn` in a ref so the debounced wrapper is stable across renders.

## Race condition: optimistic UI vs PATCH success

No optimistic UI implemented for drag-nudge — the pin's CSS position follows the pointer during drag (via pointer capture), then React-Query invalidation on `PATCH` success re-renders from server-confirmed coordinates. This avoids the race where an in-flight PATCH could overwrite a second drag that completed before the first response returned. For v1 this is acceptable; a future optimistic-update approach would need a "last drag wins" strategy.

## pdf.js OffscreenCanvas — Retina / DPR consideration

The plan's open question: "Do we need DPR-aware rendering for Retina?" For floor plan upload, no — we're rendering at 150 DPI (≈ 2× screen resolution already) and the resulting PNG is stored server-side and served back at CSS width. The `<img>` element will scale to fill the container; Retina displays get the full resolution. If the uploaded PNG is smaller than the display container, the image will be slightly blurry — but that's the quality of the original PDF, not a DPR bug. No DPR-aware canvas scaling is needed for v1.

## Known Stubs

None — all components wire to real API endpoints; no hardcoded empty responses flow to the UI.

## Threat Flags

No new threat surface. All network calls go through `apiFetch` (adds CSRF header + `credentials: same-origin`). Fractional coordinates are clamped to `[0,1]` client-side per T-05-10-01; server CHECK constraint is the authoritative guard (plan 05-05 schema).

## Self-Check: PASSED

Files verified present:
- `web/src/lib/pdfToPng.ts` — FOUND
- `web/src/lib/hooks/useFloorPlanHealth.ts` — FOUND
- `web/src/lib/hooks/useDebounceCallback.ts` — FOUND
- `web/src/components/floor-plan/FloorPlanCanvas.tsx` — FOUND
- `web/src/components/floor-plan/DevicePin.tsx` — FOUND
- `web/src/components/floor-plan/DeviceSidebar.tsx` — FOUND
- `web/src/components/floor-plan/FloorPlanSelector.tsx` — FOUND
- `web/src/components/floor-plan/FloorPlanTab.tsx` — FOUND
- `web/src/components/floor-plan/UploadFloorPlanDialog.tsx` — FOUND
- `web/src/components/floor-plan/ReplaceImageDialog.tsx` — FOUND
- `web/src/components/floor-plan/RemovePinAlertDialog.tsx` — FOUND
- `web/src/routes/sites/$id.tsx` — FOUND

Commits verified:
- `f71673a` — feat(05-10): pdf.js client conversion + upload/replace/remove dialogs
- `f929bd2` — feat(05-10): FloorPlanCanvas + DevicePin + DeviceSidebar + live SSE health

Test suite: 321 passed / 0 failed (51 test files)
Build: clean exit 0
