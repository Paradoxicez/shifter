---
phase: 05-aggregates-reports-map-floor-plans
plan: 08
subsystem: frontend
tags: [react, leaflet, map, clustering, osm, gw-04, map-picker, sidebar]

# Dependency graph
requires:
  - phase: 05-aggregates-reports-map-floor-plans
    plan: 04
    provides: "GET /api/map/data endpoint with sites + gateways + today_consumption"
  - phase: 04-realtime-dashboard
    provides: "useDashboardScope hook for capabilities gating"
  - phase: 03-provisioning-gateways-devices-bulk-import
    provides: "AddGatewayDialog with Phase 3 Pick-on-map forward-compat slot"
provides:
  - "/map route rendering OSM map with site/gateway markers + clustering"
  - "MapView component (MapContainer + TileLayer + MarkerClusterGroup + BoundsController)"
  - "SiteMarker + GatewayMarker divIcon components (Pitfall #5 className:'')"
  - "SitePopup with View site + Get directions CTA (SITE-06 drill-down)"
  - "MapPicker modal for lat/lng picking (GW-04)"
  - "Sidebar Reports + Map nav items between Dashboard and Gateways"
  - "leaflet-css shim module (Pitfall #4) imported at app bootstrap"
affects: [gateway-create-edit-dialog, sidebar-nav, app-bootstrap]

# Tech tracking
tech-stack:
  added:
    - "react-leaflet 5.0.0 (already installed by Plan 05-01)"
    - "leaflet 1.9.4 (already installed)"
    - "react-leaflet-cluster 4.1.3 (already installed)"
    - "@types/leaflet 1.9.21 (already installed)"
  patterns:
    - "vi.hoisted() for mock variables used inside vi.mock() factories (avoids ReferenceError on hoist)"
    - "MemoryRouter wrapper in component tests that render react-router Link components"
    - "MapView children slot threads additional Leaflet hooks (ClickEmitter) inside MapContainer"
    - "DivIcon className:'' suppresses Leaflet white-box default (Pitfall #5)"
    - "enabled: open in useQuery prevents fetching when MapPicker dialog is closed"

key-files:
  created:
    - web/src/lib/leaflet-css.ts
    - web/src/components/map/MapView.tsx
    - web/src/components/map/SiteMarker.tsx
    - web/src/components/map/GatewayMarker.tsx
    - web/src/components/map/SitePopup.tsx
    - web/src/components/map/MapEmptyState.tsx
    - web/src/components/map/MapPicker.tsx
    - web/src/components/map/MapPicker.test.tsx
    - web/src/routes/map.tsx
    - web/src/routes/map.test.tsx
  modified:
    - web/src/components/map/MapView.test.tsx
    - web/src/components/shell/sidebar.tsx
    - web/src/components/shell/sidebar.test.tsx
    - web/src/App.tsx
    - web/src/routes/gateways/add-gateway-dialog.tsx
    - web/src/routes/gateways/add-gateway-dialog.test.tsx
    - web/playwright/specs/map-drill-down.spec.ts

key-decisions:
  - "MapView children slot (not prop threading): threaded ClickEmitter inside MapContainer via React children prop on MapView rather than a dedicated clickMode prop — keeps the API clean and lets MapPicker drop in any additional Leaflet hooks"
  - "siteMarkerIcon/gatewayMarkerIcon return L.DivIcon (not plain object): matches Leaflet types; tests cast via unknown for html/className access since @types/leaflet doesn't expose those as public members"
  - "AddGatewayDialog uses string state for lat/lng (setLat/setLng), not react-hook-form.setValue: the dialog was Phase 3 uncontrolled state; MapPicker onPick calls setLat/setLng directly matching the existing pattern"
  - "stale Phase 3 TestAddGatewayDialog_PickOnMapDisabled test updated to TestAddGatewayDialog_PickOnMapEnabled — button is now enabled per GW-04 delivery"

# Metrics
duration: 12min
completed: 2026-05-12
---

# Phase 05 Plan 08: Map Frontend + GW-04 Picker Summary

**React-Leaflet MapView with OSM tiles, marker clustering, site/gateway markers, popup drill-down, MapPicker modal for gateway lat/lng entry, and sidebar nav extension**

## Performance

- **Duration:** ~12 min
- **Started:** 2026-05-12T01:57:19Z
- **Completed:** 2026-05-12T02:09:37Z
- **Tasks:** 2
- **Files created:** 10
- **Files modified:** 7

## Accomplishments

- `web/src/lib/leaflet-css.ts` — Pitfall #4 shim: three CSS imports (`leaflet/dist/leaflet.css`, `react-leaflet-cluster/dist/assets/MarkerCluster.css`, `react-leaflet-cluster/dist/assets/MarkerCluster.Default.css`) as a side-effect-only module imported once in App.tsx before any component
- `MapView` — MapContainer + OSM TileLayer (MAP-04: no API key, attribution required) + MarkerClusterGroup (chunkedLoading, MAP-02) + BoundsController (D-13: 0 markers → Bangkok zoom 5; 1 marker → zoom 16; 2+ → fitBounds pad 0.1); children slot allows MapPicker to thread ClickEmitter inside the map context
- `SiteMarker` / `GatewayMarker` — Leaflet `divIcon` with Tailwind classes, `className: ''` (Pitfall #5); site uses navy `bg-primary` + Building2 SVG; gateway uses muted fill + `ring-success`/`ring-destructive` for online/offline state
- `SitePopup` — capability-gated consumption display (water-only/electricity-only/both), View site → `/sites/:id`, Get directions → OSM external link (D-15)
- `MapEmptyState` — empty-state card (Phase 4 D-21 pattern) with "Add a site" CTA
- `/map` route — TanStack Query + `useDashboardScope` for capabilities, skeleton on loading, empty-state when no data
- Sidebar — Reports (FileText) + Map (Map) nav items inserted between Dashboard and Gateways (order: Dashboard → Reports → Map → Gateways → Sites → Devices → Profiles → Settings)
- `MapPicker` — modal dialog reusing MapView in picker mode; ClickEmitter (useMapEvents) captures raw map clicks; site markers in picker mode emit their site's lat/lng via onSiteClick; `enabled: open` prevents unnecessary fetch
- `AddGatewayDialog` — "Pick on map" button enabled (was disabled Phase 3 placeholder `title="Available in v5."`); MapPicker wired with `onPick → setLat/setLng`

## Bundle Size Delta

- `MapView-CqxJosGg.js`: 194 kB raw / 55.7 kB gzip (Leaflet + react-leaflet + react-leaflet-cluster bundled in the lazy chunk)
- Map route is code-split via `lazy()` in App.tsx — zero impact on initial load

## Task Commits

1. **Task 1: Leaflet CSS shim + MapView + markers + clustering + sidebar nav** — `3b047ab`
2. **Task 2: MapPicker dialog wired into gateway create/edit dialog (GW-04)** — `e92fd73`

## Files Created/Modified

**Created:**
- `web/src/lib/leaflet-css.ts` — three CSS imports (Pitfall #4 side-effect module)
- `web/src/components/map/MapView.tsx` — MapContainer + TileLayer + BoundsController + MarkerClusterGroup
- `web/src/components/map/SiteMarker.tsx` — Building2 divIcon, className:'', popup integration
- `web/src/components/map/GatewayMarker.tsx` — Antenna divIcon, ring-success/ring-destructive, className:''
- `web/src/components/map/SitePopup.tsx` — capability-gated popup with View site + Get directions
- `web/src/components/map/MapEmptyState.tsx` — zero-data empty state card
- `web/src/components/map/MapPicker.tsx` — modal picker dialog with ClickEmitter + onSiteClick support
- `web/src/components/map/MapPicker.test.tsx` — 5 GW-04 integration tests
- `web/src/routes/map.tsx` — /map route component
- `web/src/routes/map.test.tsx` — 3 route-level tests (loading, empty, with-data)

**Modified:**
- `web/src/components/map/MapView.test.tsx` — replaced 4 it.skip stubs with 10 passing tests; uses vi.hoisted() + MemoryRouter
- `web/src/components/shell/sidebar.tsx` — added Reports + Map nav items
- `web/src/components/shell/sidebar.test.tsx` — extended with nav order assertion
- `web/src/App.tsx` — imports leaflet-css shim; adds /map lazy route
- `web/src/routes/gateways/add-gateway-dialog.tsx` — enabled Pick on map button; wired MapPicker
- `web/src/routes/gateways/add-gateway-dialog.test.tsx` — updated stale disabled-button test; added Leaflet mocks
- `web/playwright/specs/map-drill-down.spec.ts` — replaced test.skip stubs with full Playwright spec bodies

## Decisions Made

- **MapView children slot vs picker prop**: MapPicker needs to render `ClickEmitter` inside `MapContainer` (requires Leaflet context). Added a `children?: React.ReactNode` prop to MapView rather than a separate `onMapClick` prop, keeping the component composable and avoiding drilling an extra callback through MarkerClusterGroup.
- **String state for lat/lng in AddGatewayDialog**: The dialog uses `useState<string>` for lat/lng (not react-hook-form). MapPicker's `onPick` calls `setLat(String(lat))` / `setLng(String(lng))` to match the existing state pattern without refactoring the form.
- **vi.hoisted() for mock variables**: Vitest hoists `vi.mock()` calls to the top of the file before variable declarations. Using `vi.hoisted()` creates the mock functions before hoisting so they're available in factory functions — resolves the `ReferenceError: Cannot access 'mockUseMap' before initialization` pitfall.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed vi.mock() hoisting error in MapView.test.tsx**
- **Found during:** Task 1 GREEN (first test run)
- **Issue:** `const mockUseMap = vi.fn(...)` declared before `vi.mock('react-leaflet', ...)` factory, but Vitest hoists the mock factory to before the variable declaration. Caused `ReferenceError: Cannot access 'mockUseMap' before initialization`
- **Fix:** Replaced top-level `const mockXxx = vi.fn()` declarations with `vi.hoisted(() => ({ ... }))` pattern
- **Files modified:** `web/src/components/map/MapView.test.tsx`
- **Commit:** `3b047ab`

**2. [Rule 1 - Bug] Added MemoryRouter wrapper in MapView tests**
- **Found during:** Task 1 GREEN (second test run)
- **Issue:** `SitePopup` uses `<Link>` from react-router-dom which requires Router context. Rendering `MapView` without a Router caused `TypeError: Cannot destructure property 'basename' of React.useContext(...)`
- **Fix:** Wrapped all MapView test renders in `<MemoryRouter>`
- **Files modified:** `web/src/components/map/MapView.test.tsx`
- **Commit:** `3b047ab`

**3. [Rule 1 - Bug] Fixed TypeScript cast on siteMarkerIcon/gatewayMarkerIcon return type**
- **Found during:** Task 1 build verification
- **Issue:** Functions returning `L.DivIcon` initially had complex `as unknown as {html,className}` return types; separately, `@types/leaflet`'s `DivIcon` doesn't expose `html`/`className` as public members — tests accessing these properties failed TypeScript type-checking
- **Fix:** Functions return `L.DivIcon` (correct); test assertions use `as unknown as {html: string; className: string}` casts at call sites
- **Files modified:** `web/src/components/map/SiteMarker.tsx`, `web/src/components/map/GatewayMarker.tsx`, `web/src/components/map/MapView.test.tsx`
- **Commit:** `3b047ab`

**4. [Rule 1 - Bug] Updated stale TestAddGatewayDialog_PickOnMapDisabled test**
- **Found during:** Task 2 GREEN (test run revealed existing test asserting `disabled`)
- **Issue:** Phase 3 test `TestAddGatewayDialog_PickOnMapDisabled` asserted `expect(btn).toBeDisabled()` + `title="Available in v5."`. GW-04 delivery enables the button — the test was now a false failure
- **Fix:** Renamed test to `TestAddGatewayDialog_PickOnMapEnabled`, assertion changed to `expect(btn).not.toBeDisabled()`
- **Files modified:** `web/src/routes/gateways/add-gateway-dialog.test.tsx`
- **Commit:** `e92fd73`

**5. [Rule 2 - Missing Critical] Added Leaflet mocks to add-gateway-dialog.test.tsx**
- **Found during:** Task 2 GREEN (test run after wiring MapPicker)
- **Issue:** Existing gateway dialog tests had no Leaflet mocks; importing MapPicker pulled in `react-leaflet`/`leaflet` which require DOM/canvas that jsdom doesn't provide
- **Fix:** Added `vi.mock('react-leaflet', ...)`, `vi.mock('react-leaflet-cluster', ...)`, `vi.mock('leaflet', ...)` to the gateway dialog test file
- **Files modified:** `web/src/routes/gateways/add-gateway-dialog.test.tsx`
- **Commit:** `e92fd73`

## Playwright Drill-Down Scenarios Shipped

2 scenarios:
1. Click site marker → popup → "View site" → /sites/:id (MAP-03, SITE-06)
2. Click cluster → zoom to cluster bounds (D-14) — with graceful skip if <50 markers in test environment

## Open Question: "Get directions" URI scheme

The plan asked to record this: should "Get directions" use the user's default map app via a `geo:lat,lng` URI instead of the OSM external link?

**Recommendation: keep OSM URL for v1.** The `geo:` URI scheme is handled inconsistently across browsers and OSes (Chrome on desktop often has no handler; iOS opens Apple Maps; Android opens Google Maps). The OSM link is universally reliable in a browser context and matches the MAP-04 invariant (no paid external API). A `geo:` URI with OSM fallback could be added in v1.x if operators request native navigation app integration.

## Known Stubs

None. All map components query live data from `/api/map/data`; the empty-state correctly renders when the API returns zero results.

## Threat Surface Scan

No new network endpoints beyond `/api/map/data` (gated by `auth.RequireAction(ActionSiteRead)` in Plan 05-04). The frontend makes one additional call to `/api/map/data` from MapPicker — same endpoint, same auth gate. divIcon HTML is constructed from hardcoded SVG path constants, not user data, so no XSS surface. OSM tile requests are outbound-only with no credentials. T-05-08-01 invariant confirmed: no `maplibre`, `mapbox`, or API-key strings in `web/src/components/map/`.

## Self-Check: PASSED

Files exist:
- `web/src/lib/leaflet-css.ts` — EXISTS
- `web/src/components/map/MapView.tsx` — EXISTS
- `web/src/components/map/SiteMarker.tsx` — EXISTS
- `web/src/components/map/GatewayMarker.tsx` — EXISTS
- `web/src/components/map/SitePopup.tsx` — EXISTS
- `web/src/components/map/MapEmptyState.tsx` — EXISTS
- `web/src/components/map/MapPicker.tsx` — EXISTS
- `web/src/routes/map.tsx` — EXISTS

Commits exist:
- `3b047ab` — EXISTS (Task 1)
- `e92fd73` — EXISTS (Task 2)

Test results: 289 passed / 0 failed
Build: clean (✓ built in 3.48s)
