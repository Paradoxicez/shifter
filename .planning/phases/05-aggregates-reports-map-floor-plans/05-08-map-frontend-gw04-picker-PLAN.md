---
phase: 05-aggregates-reports-map-floor-plans
plan: 08
type: execute
wave: 4
depends_on: [04]
files_modified:
  - web/src/routes/map.tsx
  - web/src/routes/map.test.tsx
  - web/src/components/map/MapView.tsx
  - web/src/components/map/MapView.test.tsx
  - web/src/components/map/SiteMarker.tsx
  - web/src/components/map/GatewayMarker.tsx
  - web/src/components/map/SitePopup.tsx
  - web/src/components/map/SiteMarker.test.tsx
  - web/src/components/map/MapEmptyState.tsx
  - web/src/components/map/MapPicker.tsx
  - web/src/components/shell/sidebar.tsx
  - web/src/components/shell/sidebar.test.tsx
  - web/src/lib/leaflet-css.ts
  - web/src/components/gateways/edit-gateway-dialog.tsx
  - web/src/App.tsx
  - web/playwright/specs/map-drill-down.spec.ts
autonomous: true
requirements: [MAP-01, MAP-02, MAP-03, MAP-04, SITE-06]
threat_refs: [T-05-08-01]

must_haves:
  truths:
    - "/map route renders react-leaflet MapContainer with OSM TileLayer + attribution"
    - "Three Leaflet CSS imports loaded at app bootstrap so tiles + popups + cluster icons render (Pitfall #4)"
    - "Site markers use custom divIcon with Building2 SVG; gateway markers use Antenna SVG with online/offline ring; className: '' set to suppress Leaflet's white box (Pitfall #5)"
    - "react-leaflet-cluster wraps the marker layer; clustering kicks in above ~50 markers (D-14)"
    - "Viewport auto-fits bounds with ±10% padding (D-13); single site → zoom 16; zero markers → Bangkok [13.7563, 100.5018] zoom 5"
    - "Site popup shows: name, MP count badge, online/offline badges, today's consumption per capability, View site CTA, Get directions OSM link"
    - "Clicking 'View site' navigates to /sites/:id"
    - "Sidebar gains Reports (FileText) and Map (Map) nav items between Dashboard and Gateways"
    - "Gateway create/edit dialog has 'Pick on map' button that opens a modal MapPicker reusing the same MapView (delivers Phase 3 GW-04 as a side-effect)"
    - "MAP-04 invariant: tile URL is `https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png` (no API key); attribution string includes OpenStreetMap copyright link"
    - "Zero shadcn registry additions — uses only the existing component set (UI-SPEC Dim 6 PASS preserved)"
    - "Capability-gated popup: water-only install shows only water consumption; electricity-only shows only electricity"
  artifacts:
    - path: "web/src/lib/leaflet-css.ts"
      provides: "Three CSS imports as a side-effect-only module (Pitfall #4)"
      contains: "leaflet/dist/leaflet.css"
    - path: "web/src/components/map/MapView.tsx"
      provides: "MapContainer + TileLayer + MarkerClusterGroup + BoundsController + EmptyState overlay"
      contains: "MapContainer"
    - path: "web/src/components/map/SiteMarker.tsx"
      provides: "L.divIcon Building2 marker with className:'' + popup integration"
      contains: "L.divIcon"
    - path: "web/src/components/map/GatewayMarker.tsx"
      provides: "L.divIcon Antenna marker with online/offline ring class"
      contains: "L.divIcon"
    - path: "web/src/components/map/SitePopup.tsx"
      provides: "Popup content with site stats + View site + Get directions"
      contains: "View site"
    - path: "web/src/components/map/MapPicker.tsx"
      provides: "Reusable picker dialog body for GW-04 — click on map → returns {lat, lng}"
      contains: "MapPicker"
    - path: "web/src/routes/map.tsx"
      provides: "/map route component; full-bleed Leaflet"
      contains: "h-[calc(100vh-3.5rem)]"
    - path: "web/src/components/shell/sidebar.tsx"
      provides: "Sidebar with Reports + Map nav items added between Dashboard and Gateways"
      contains: "/map"
  key_links:
    - from: "web/src/routes/map.tsx"
      to: "/api/map/data"
      via: "TanStack Query useQuery"
      pattern: "/api/map/data"
    - from: "web/src/components/map/MapPicker.tsx"
      to: "/api/map/data"
      via: "same fetch (re-uses existing data; suppresses popup)"
      pattern: "/api/map/data"
    - from: "web/src/components/gateways/edit-gateway-dialog.tsx"
      to: "MapPicker dialog → onPick(lat, lng) → form.setValue('latitude', …) etc."
      via: "modal trigger with Dialog component"
      pattern: "MapPicker"
---

<objective>
Ship the map frontend (MAP-01..04) — `/map` route with Leaflet + clustering + OSM tiles + site/gateway markers + popup with drill-down (SITE-06) — and add the Phase 3 GW-04 "Pick on map" feature as a side-effect by reusing the same MapView inside a modal picker invoked from the gateway create/edit dialog. Mandatory Leaflet CSS imports (Pitfall #4) ship via a single shim module loaded once at app bootstrap.

Purpose: The map view is one of three Phase 5 user-visible surfaces (alongside reports + floor plans). MAP-04's "no paid external API" invariant is reflexively enforced by using OSM tiles + react-leaflet (which has no API-key surface). GW-04 was deferred from Phase 3 with "needs map view" as the blocker — this plan unblocks it for zero incremental cost.

Output: 10 new React components/route files, 1 CSS shim, sidebar nav extension, gateway dialog extension, Playwright spec body, route + test files for MapView + SiteMarker.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-UI-SPEC.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-04-map-backend-PLAN.md
@web/src/components/shell/sidebar.tsx
@web/src/components/gateways
@web/src/App.tsx
@web/src/components/ui/dialog.tsx

<interfaces>
<!-- From plan 05-04 backend -->
```ts
type MapData = {
  sites: Array<{
    id: string; name: string; lat: number; lng: number;
    mp_count: number; online_count: number; offline_count: number;
    today_consumption: { water?: number; electricity?: number };  // keys subset by install capabilities
  }>;
  gateways: Array<{ id: string; name: string; lat: number; lng: number; online: boolean }>;
};
```

<!-- Required imports — these three MUST land in web/src/lib/leaflet-css.ts -->
```ts
import 'leaflet/dist/leaflet.css'
import 'react-leaflet-cluster/dist/assets/MarkerCluster.css'
import 'react-leaflet-cluster/dist/assets/MarkerCluster.Default.css'
```

<!-- divIcon HTML template per UI-SPEC §Map-Specific Contracts -->
```tsx
// Site marker — 32px navy circle + Building2 SVG (white)
<div class="flex h-8 w-8 items-center justify-center rounded-full bg-primary ring-2 ring-white shadow-sm">
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="2">
    <path d="${BUILDING2_PATH}"/>
  </svg>
</div>

// Gateway marker — 28px muted circle + Antenna SVG with online/offline ring
<div class="flex h-7 w-7 items-center justify-center rounded-full bg-muted ring-2 ring-{success|destructive} shadow-sm">
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
    <path d="${ANTENNA_PATH}"/>
  </svg>
</div>
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Leaflet CSS shim + MapView + markers + clustering + bounds controller</name>
  <read_first>
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-UI-SPEC.md §Map-Specific Contracts (tile URL, viewport rules, marker spec)
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md §React-Leaflet v5 + Clustering §Common Pitfalls #4 #5 #9
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md §D-12..D-15
    - web/src/components/shell/sidebar.tsx (existing sidebar structure)
    - web/src/App.tsx (router config)
    - web/src/lib/pdfWorker.ts (Plan 05-01 reference for side-effect-only modules)
  </read_first>
  <behavior>
    - Test 1: MapView renders MapContainer + TileLayer with OSM URL + OpenStreetMap attribution
    - Test 2: BoundsController fits bounds for ≥2 markers (mocked map.fitBounds called)
    - Test 3: Single-site case sets view to that site at zoom 16 (mocked map.setView called)
    - Test 4: Zero markers → MapView centers on Bangkok [13.7563, 100.5018] zoom 5
    - Test 5: MarkerClusterGroup wraps markers; >50 markers passed → cluster icon rendered (verified via DOM query for `.marker-cluster`)
    - Test 6: SiteMarker divIcon HTML contains "bg-primary" and "Building2" SVG path; className attribute set to empty string
    - Test 7: GatewayMarker divIcon has ring-success (online) vs ring-destructive (offline)
    - Test 8: Sidebar contains exactly the new nav items "Reports" and "Map" between "Dashboard" and "Gateways" in that order
  </behavior>
  <action>
**Step A — `web/src/lib/leaflet-css.ts`:**

```ts
// Leaflet + react-leaflet-cluster require three CSS files to be imported AT
// MODULE-LOAD TIME for tiles + popups + cluster icons to render correctly.
//
// Pitfall #4 (RESEARCH): without these imports, the map shows as a grey box
// and popups appear unstyled. Import THIS module once at the app entrypoint
// (web/src/main.tsx or App.tsx) — every component that uses Leaflet relies
// on this side effect.

import 'leaflet/dist/leaflet.css'
import 'react-leaflet-cluster/dist/assets/MarkerCluster.css'
import 'react-leaflet-cluster/dist/assets/MarkerCluster.Default.css'
```

Import in `web/src/App.tsx` (or main.tsx — pick whichever runs first) BEFORE any component imports:

```ts
import './lib/leaflet-css'
```

**Step B — `web/src/components/map/SiteMarker.tsx`:**

```tsx
import L from 'leaflet'
import { Marker, Popup } from 'react-leaflet'
import { SitePopup } from './SitePopup'

// Lucide Building2 SVG path (copy from lucide-react source verbatim — pinned
// here so a future lucide update doesn't silently alter the icon shape).
const BUILDING2_PATH = 'M6 22V4a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v18Z M6 12H4a2 2 0 0 0-2 2v6a2 2 0 0 0 2 2h2 M22 22h-2 M14 22v-4a2 2 0 0 0-2-2h-4 M18 5h0 M18 9h0 M10 5h0 M10 9h0 M10 13h0 M10 17h0'

export function siteMarkerIcon(): L.DivIcon {
  return L.divIcon({
    html: `
      <div class="flex h-8 w-8 items-center justify-center rounded-full bg-primary ring-2 ring-white shadow-sm" aria-label="Site marker">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="2">
          <path d="${BUILDING2_PATH}"/>
        </svg>
      </div>
    `,
    className: '',  // Pitfall #5 — clear default leaflet-div-icon white-box class
    iconSize: [32, 32],
    iconAnchor: [16, 32],
    popupAnchor: [0, -32],
  })
}

export function SiteMarker({ site, capabilities }: { site: MapSite; capabilities: 'water' | 'electricity' | 'both' }) {
  return (
    <Marker position={[site.lat, site.lng]} icon={siteMarkerIcon()}>
      <Popup>
        <SitePopup site={site} capabilities={capabilities} />
      </Popup>
    </Marker>
  )
}
```

**Step C — `web/src/components/map/GatewayMarker.tsx`:**

```tsx
import L from 'leaflet'
import { Marker, Popup } from 'react-leaflet'

const ANTENNA_PATH = 'M2 12a10 10 0 0 1 18 0 M5 12a7 7 0 0 1 12 0 M8 12a4 4 0 0 1 8 0 M12 9v13'

export function gatewayMarkerIcon(online: boolean): L.DivIcon {
  const ring = online ? 'ring-success' : 'ring-destructive'
  return L.divIcon({
    html: `
      <div class="flex h-7 w-7 items-center justify-center rounded-full bg-muted ${ring} ring-2 shadow-sm" aria-label="Gateway marker">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="${ANTENNA_PATH}"/>
        </svg>
      </div>
    `,
    className: '',
    iconSize: [28, 28],
    iconAnchor: [14, 28],
    popupAnchor: [0, -28],
  })
}

export function GatewayMarker({ gateway }: { gateway: MapGateway }) {
  return (
    <Marker position={[gateway.lat, gateway.lng]} icon={gatewayMarkerIcon(gateway.online)}>
      <Popup>
        <div className="space-y-1">
          <div className="font-semibold">{gateway.name}</div>
          <div className="text-xs text-muted-foreground">{gateway.online ? 'Online' : 'Offline'}</div>
        </div>
      </Popup>
    </Marker>
  )
}
```

**Step D — `web/src/components/map/SitePopup.tsx`:**

```tsx
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Link } from 'react-router-dom'

export function SitePopup({ site, capabilities }: { site: MapSite; capabilities: 'water' | 'electricity' | 'both' }) {
  const directionsURL = `https://www.openstreetmap.org/?mlat=${site.lat}&mlon=${site.lng}#map=16/${site.lat}/${site.lng}`

  return (
    <div className="space-y-2 min-w-[220px]">
      <div className="font-semibold text-base">{site.name}</div>
      <div className="flex gap-2 flex-wrap">
        <Badge variant="secondary">{site.mp_count} MP{site.mp_count === 1 ? '' : 's'}</Badge>
        {site.online_count > 0 && <Badge variant="default">{site.online_count} online</Badge>}
        {site.offline_count > 0 && <Badge variant="destructive">{site.offline_count} offline</Badge>}
      </div>
      {(capabilities === 'water' || capabilities === 'both') && site.today_consumption.water !== undefined && (
        <div className="text-sm">Today (water): <span className="font-mono">{site.today_consumption.water.toFixed(3)} m³</span></div>
      )}
      {(capabilities === 'electricity' || capabilities === 'both') && site.today_consumption.electricity !== undefined && (
        <div className="text-sm">Today (electricity): <span className="font-mono">{site.today_consumption.electricity.toFixed(3)} kWh</span></div>
      )}
      <div className="flex gap-2 pt-1">
        <Button asChild size="sm"><Link to={`/sites/${site.id}`}>View site</Link></Button>
        <Button asChild size="sm" variant="outline"><a href={directionsURL} target="_blank" rel="noreferrer">Get directions</a></Button>
      </div>
    </div>
  )
}
```

**Step E — `web/src/components/map/MapView.tsx`:**

```tsx
import { MapContainer, TileLayer, useMap } from 'react-leaflet'
import MarkerClusterGroup from 'react-leaflet-cluster'
import L from 'leaflet'
import { useEffect } from 'react'
import { SiteMarker } from './SiteMarker'
import { GatewayMarker } from './GatewayMarker'

const BANGKOK: [number, number] = [13.7563, 100.5018]
const BANGKOK_ZOOM = 5  // D-13 fallback for zero-data install
const SINGLE_SITE_ZOOM = 16

export type MapSite = {
  id: string; name: string; lat: number; lng: number;
  mp_count: number; online_count: number; offline_count: number;
  today_consumption: { water?: number; electricity?: number };
}
export type MapGateway = { id: string; name: string; lat: number; lng: number; online: boolean }

function BoundsController({ sites, gateways }: { sites: MapSite[]; gateways: MapGateway[] }) {
  const map = useMap()
  useEffect(() => {
    const all = [...sites.map(s => [s.lat, s.lng] as [number, number]),
                 ...gateways.map(g => [g.lat, g.lng] as [number, number])]
    if (all.length === 0) {
      map.setView(BANGKOK, BANGKOK_ZOOM)
      return
    }
    if (all.length === 1) {
      map.setView(all[0], SINGLE_SITE_ZOOM)
      return
    }
    map.fitBounds(L.latLngBounds(all).pad(0.1))  // D-13 ±10% padding
  }, [sites, gateways, map])
  return null
}

export function MapView({ sites, gateways, capabilities, onSiteClick }: {
  sites: MapSite[]; gateways: MapGateway[];
  capabilities: 'water' | 'electricity' | 'both';
  onSiteClick?: (lat: number, lng: number) => void;  // when set, suppresses popups + emits click (picker mode)
}) {
  return (
    <MapContainer
      style={{ height: 'calc(100vh - 3.5rem)' }}
      center={BANGKOK}
      zoom={BANGKOK_ZOOM}
      scrollWheelZoom
      aria-label="Fleet map"
    >
      <TileLayer
        attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap contributors</a>'
        url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
      />
      <BoundsController sites={sites} gateways={gateways} />
      <MarkerClusterGroup chunkedLoading>
        {sites.map(s => <SiteMarker key={s.id} site={s} capabilities={capabilities} />)}
        {gateways.map(g => <GatewayMarker key={g.id} gateway={g} />)}
      </MarkerClusterGroup>
    </MapContainer>
  )
}
```

**Step F — `web/src/components/map/MapEmptyState.tsx`:**

```tsx
import { Map } from 'lucide-react'
import { Card, CardContent, CardTitle, CardDescription } from '@/components/ui/card'
import { Button } from '@/components/ui/button'

// Mirrors Phase 4 EmptyStateOnboarding pattern (UI-SPEC §Empty States).
export function MapEmptyState({ onAddSite }: { onAddSite: () => void }) {
  return (
    <Card className="max-w-md mx-auto mt-32">
      <CardContent className="pt-6 text-center space-y-4">
        <div className="mx-auto h-12 w-12 bg-primary/10 rounded-full flex items-center justify-center">
          <Map className="h-6 w-6 text-primary" />
        </div>
        <div>
          <CardTitle>No locations on the map</CardTitle>
          <CardDescription>Add a site or gateway to see them plotted here.</CardDescription>
        </div>
        <Button onClick={onAddSite}>Add a site</Button>
      </CardContent>
    </Card>
  )
}
```

**Step G — `web/src/routes/map.tsx`:**

```tsx
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { MapView, MapSite, MapGateway } from '@/components/map/MapView'
import { MapEmptyState } from '@/components/map/MapEmptyState'
import { Skeleton } from '@/components/ui/skeleton'
import { apiFetch } from '@/lib/apiFetch'
import { useDashboardScope } from '@/lib/hooks/useDashboardScope'  // existing Phase 4 hook for capabilities

export function MapPage() {
  const scope = useDashboardScope()
  const navigate = useNavigate()

  const { data, isLoading } = useQuery({
    queryKey: ['map', 'data'],
    queryFn: async () => apiFetch<{ sites: MapSite[]; gateways: MapGateway[] }>('/api/map/data'),
  })

  if (isLoading) return <Skeleton className="h-[calc(100vh-3.5rem)] w-full" />

  const sites = data?.sites ?? []
  const gateways = data?.gateways ?? []
  if (sites.length === 0 && gateways.length === 0) {
    return <MapEmptyState onAddSite={() => navigate('/sites?action=create')} />
  }

  return (
    <MapView
      sites={sites}
      gateways={gateways}
      capabilities={scope.capabilities}
    />
  )
}
```

**Step H — Sidebar nav extension (`web/src/components/shell/sidebar.tsx`):**

Read existing sidebar.tsx first. Add two new items between Dashboard and Gateways:

```tsx
{ to: '/reports', label: 'Reports', icon: FileText },
{ to: '/map',     label: 'Map',     icon: Map },
```

Import icons: `import { FileText, Map } from 'lucide-react'`. Update sidebar test in same step.

**Step I — Wire `/map` route in `web/src/App.tsx`:**

```tsx
import { MapPage } from './routes/map'
// inside <Routes> within the authenticated layout group:
<Route path="/map" element={<MapPage />} />
```

**Step J — Replace `it.skip` bodies in `web/src/components/map/MapView.test.tsx` and add `web/src/routes/map.test.tsx`:**

MapView.test.tsx scenarios:

1. Renders MapContainer with `data-testid` selectable; queries for TileLayer URL attribute = OSM URL
2. Mocks `useMap` and asserts `map.fitBounds(L.latLngBounds(...).pad(0.1))` called when 2+ markers
3. Asserts `map.setView([single.lat, single.lng], 16)` for exactly 1 marker
4. Asserts `map.setView([13.7563, 100.5018], 5)` for zero markers
5. >50 markers passed → asserts cluster icon present in DOM (`.marker-cluster` class)
6. SiteMarker icon HTML includes `bg-primary` and Building2 path; className attribute set to empty string
7. GatewayMarker online=true → ring-success in HTML; online=false → ring-destructive

map.test.tsx scenarios:

1. Loading state → Skeleton rendered
2. data.sites = [] AND data.gateways = [] → MapEmptyState rendered
3. data has sites → MapView rendered

**Step K — Sidebar test extension:**

Replace `it.skip` in `web/src/components/shell/sidebar.test.tsx` with new test that asserts the nav order: Dashboard → Reports → Map → Gateways → Sites → Devices → Profiles → Settings.

**Step L — Playwright spec body in `web/playwright/specs/map-drill-down.spec.ts`:**

Replace `test.skip` with a body that:

1. Login as admin via fixture
2. Visit /map
3. Wait for MapContainer to render (data-testid="map-container")
4. Click the first site marker
5. Wait for the popup to appear
6. Click "View site" — assert URL changes to /sites/<id>
  </action>
  <verify>
    <automated>pnpm --dir web test:run --reporter=basic web/src/components/map web/src/routes/map.test.tsx web/src/components/shell/sidebar.test.tsx &amp;&amp; pnpm --dir web build &amp;&amp; pnpm --dir web exec playwright test --list map-drill-down.spec.ts 2&gt;&amp;1 | grep -c "›"</automated>
  </verify>
  <acceptance_criteria>
    - `web/src/lib/leaflet-css.ts` exists with all 3 literal CSS imports
    - `web/src/App.tsx` imports `./lib/leaflet-css`
    - `web/src/components/map/MapView.tsx` contains literal `tile.openstreetmap.org` AND `OpenStreetMap contributors` AND `MarkerClusterGroup` AND `chunkedLoading` AND `13.7563` AND `100.5018`
    - `web/src/components/map/SiteMarker.tsx` contains literal `className: ''` (Pitfall #5)
    - `web/src/components/map/GatewayMarker.tsx` contains literal `className: ''` AND `ring-success` AND `ring-destructive`
    - `web/src/components/map/SitePopup.tsx` contains literals `View site`, `Get directions`, and `openstreetmap.org/?mlat=`
    - `web/src/components/shell/sidebar.tsx` contains literal `/reports` AND `/map` nav entries
    - `web/src/routes/map.tsx` calls `/api/map/data` and gates on `useDashboardScope`
    - NO MapLibre/Mapbox imports anywhere in web/src: `! grep -rE "maplibre|mapbox" web/src/`
    - NO API key strings: `! grep -rE "(api_key|access_token).*=.*[a-zA-Z0-9]{20,}" web/src/components/map/`
    - At least 7 named vitest cases in MapView.test.tsx
    - Sidebar test asserts nav order Dashboard → Reports → Map → Gateways (use exact order matching)
    - `pnpm --dir web build` exits 0
    - `pnpm --dir web test:run` exits 0
  </acceptance_criteria>
  <done>Map renders + clusters + drill-through works; OSM-only invariant preserved; sidebar wires new routes.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: MapPicker dialog wired into gateway create/edit dialog (GW-04)</name>
  <read_first>
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md §D-12 (GW-04 side-effect note)
    - web/src/components/gateways/ (find the existing edit-gateway-dialog or add-gateway-dialog)
    - web/src/components/responsive-dialog.tsx (Phase 1 dialog primitive)
    - web/src/components/map/MapView.tsx (reusing for picker mode — pass onClick that emits coords)
  </read_first>
  <behavior>
    - Test 1: Gateway create dialog has a "Pick on map" button next to the latitude/longitude inputs
    - Test 2: Clicking "Pick on map" opens a Dialog containing MapView in picker mode (popups disabled; click anywhere on the map → emits lat/lng)
    - Test 3: Selecting a point on the picker closes the dialog and populates the latitude + longitude form fields
    - Test 4: ESC / Cancel on the picker dialog leaves the form values unchanged
    - Test 5: Picker dialog also lets user click an existing site marker → emits that site's lat/lng (reuses MapView's marker layer)
  </behavior>
  <action>
**Step A — `web/src/components/map/MapPicker.tsx`:**

```tsx
import { useState } from 'react'
import { useMapEvents } from 'react-leaflet'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { MapView, MapSite, MapGateway } from './MapView'
import { useQuery } from '@tanstack/react-query'
import { apiFetch } from '@/lib/apiFetch'

function ClickEmitter({ onPick }: { onPick: (lat: number, lng: number) => void }) {
  useMapEvents({ click: (e) => onPick(e.latlng.lat, e.latlng.lng) })
  return null
}

export function MapPicker({ open, onOpenChange, onPick, initialLat, initialLng }: {
  open: boolean
  onOpenChange: (v: boolean) => void
  onPick: (lat: number, lng: number) => void
  initialLat?: number
  initialLng?: number
}) {
  const { data } = useQuery({
    queryKey: ['map', 'picker-data'],
    queryFn: async () => apiFetch<{ sites: MapSite[]; gateways: MapGateway[] }>('/api/map/data'),
    enabled: open,  // only fetch when dialog opens
  })

  const handlePick = (lat: number, lng: number) => {
    onPick(lat, lng)
    onOpenChange(false)
  }

  return (
    <ResponsiveDialog open={open} onOpenChange={onOpenChange} title="Pick location on map">
      <div className="h-[60vh]">
        <MapView
          sites={data?.sites ?? []}
          gateways={data?.gateways ?? []}
          capabilities="both"
          onSiteClick={(lat, lng) => handlePick(lat, lng)}
        />
        {/* ClickEmitter sits inside MapView but requires the map context; refactor:
            either thread ClickEmitter into MapView via prop, or rewrite MapPicker
            to compose its own MapContainer with the same TileLayer + ClusterGroup
            children and the ClickEmitter as a sibling. Pick the cleaner approach.
            Plan-time recommendation: thread `children` slot into MapView so
            MapPicker can drop in <ClickEmitter onPick={handlePick} />. */}
      </div>
      <div className="flex justify-end gap-2 pt-4">
        <Button variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
      </div>
    </ResponsiveDialog>
  )
}
```

**Step B — Extend the gateway create/edit dialog:**

Find the existing dialog in `web/src/components/gateways/` (likely `edit-gateway-dialog.tsx` or `add-gateway-dialog.tsx`). Add the "Pick on map" button next to the latitude/longitude inputs:

```tsx
import { MapPin } from 'lucide-react'
import { MapPicker } from '@/components/map/MapPicker'

// inside the gateway form body, next to latitude/longitude inputs:
const [pickerOpen, setPickerOpen] = useState(false)

<div className="grid grid-cols-2 gap-2">
  <Input {...form.register('latitude', { valueAsNumber: true })} placeholder="Latitude" />
  <Input {...form.register('longitude', { valueAsNumber: true })} placeholder="Longitude" />
</div>
<Button type="button" variant="outline" onClick={() => setPickerOpen(true)}>
  <MapPin className="h-4 w-4 mr-2" /> Pick on map
</Button>
<MapPicker
  open={pickerOpen}
  onOpenChange={setPickerOpen}
  onPick={(lat, lng) => {
    form.setValue('latitude', lat)
    form.setValue('longitude', lng)
  }}
  initialLat={form.watch('latitude')}
  initialLng={form.watch('longitude')}
/>
```

**Step C — Add vitest cases in `web/src/components/map/MapView.test.tsx` (the file is shared with Task 1):**

```tsx
describe('MapPicker (GW-04)', () => {
  it('renders Pick on map button in gateway dialog', () => { /* render gateway dialog; query for "Pick on map" */ })
  it('opens MapPicker dialog when button clicked', () => { /* click → dialog visible */ })
  it('clicking map point sets form latitude + longitude', () => { /* fire useMapEvents click; assert form values */ })
  it('clicking site marker sets form coords to site lat/lng', () => { /* click site marker → handlePick */ })
  it('Cancel button leaves form unchanged', () => { /* open picker, cancel, assert form values preserved */ })
})
```
  </action>
  <verify>
    <automated>pnpm --dir web test:run --reporter=basic web/src/components/map/MapView.test.tsx web/src/components/gateways/ &amp;&amp; pnpm --dir web build</automated>
  </verify>
  <acceptance_criteria>
    - `web/src/components/map/MapPicker.tsx` exports `MapPicker` and contains literal `useMapEvents` AND `apiFetch<{ sites: MapSite[]; gateways: MapGateway[] }>('/api/map/data')`
    - Gateway create/edit dialog contains literal `MapPicker` import AND `Pick on map` button text AND `form.setValue('latitude'` AND `form.setValue('longitude'`
    - At least 5 MapPicker test scenarios in MapView.test.tsx
    - `pnpm --dir web build` exits 0
    - `pnpm --dir web test:run` exits 0
  </acceptance_criteria>
  <done>GW-04 "Pick on map" delivered as side-effect of map view; gateway dialog has working picker.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Client browser → OSM tile servers | Outbound only; no credentials |
| Map data response → React render | All values come from authenticated /api/map/data; no XSS surface (divIcon innerHTML is server-validated SVG paths from constants, NOT user data) |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-05-08-01 | Information Disclosure | MAP-04 invariant: no paid external API / API key | low | mitigate | Tile URL is OSM hard-coded constant in MapView.tsx; attribution required. Test grep asserts NO `maplibre`, `mapbox`, or long API-key-shaped strings anywhere in `web/src/components/map/`. |
</threat_model>

<verification>
1. `pnpm --dir web test:run` exits 0
2. `pnpm --dir web build` exits 0
3. `grep "tile.openstreetmap.org" web/src/components/map/MapView.tsx` returns ≥1 match
4. `! grep -rE "maplibre|mapbox" web/src/` (no banned tile libs)
5. `! grep -rE "(api_key|access_token).*=.*[a-zA-Z0-9]{20,}" web/src/components/map/` (no API keys)
6. `grep -E "className: ''" web/src/components/map/{Site,Gateway}Marker.tsx` returns 2 matches (Pitfall #5)
7. `grep -E "MapPicker" web/src/components/gateways/*.tsx` returns ≥1 match (GW-04 wired)
</verification>

<success_criteria>
- /map route renders site + gateway markers with clustering + auto-fit bounds
- Empty-state shows Bangkok zoom 5 fallback
- Popup → "View site" navigates to /sites/:id
- MAP-04 invariant verified: only OSM tiles, no API keys
- GW-04 picker integrated into gateway create/edit dialog
- Sidebar gains Reports + Map nav items
- All 3 Leaflet CSS imports live (Pitfall #4)
- divIcon className cleared (Pitfall #5)
</success_criteria>

<output>
After completion, create `.planning/phases/05-aggregates-reports-map-floor-plans/05-08-SUMMARY.md` recording:
- Whether MapView's `onSiteClick` prop / ClickEmitter composition needed restructuring
- Bundle size delta from leaflet + react-leaflet-cluster (rough — pnpm-build output)
- Number of Playwright drill-down scenarios shipped
- Open question: should "Get directions" use the user's default map app (`geo:` URI) instead of OSM URL?
</output>
