/**
 * SiteMarker — Plan 05-08
 *
 * Leaflet divIcon marker for a site on the map.
 * Uses Building2 SVG (Lucide) with navy fill.
 *
 * Pitfall #5: className set to '' to suppress Leaflet's default white-box.
 */

import L from 'leaflet'
import { Marker, Popup } from 'react-leaflet'
import type { MapSite } from './MapView'
import { SitePopup } from './SitePopup'

// Lucide Building2 SVG path — pinned verbatim so a future lucide update does
// not silently alter the icon shape. From lucide-react v0.462.
const BUILDING2_PATH =
  'M6 22V4a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v18Z M6 12H4a2 2 0 0 0-2 2v6a2 2 0 0 0 2 2h2 M22 22h-2 M14 22v-4a2 2 0 0 0-2-2h-4 M18 5h0 M18 9h0 M10 5h0 M10 9h0 M10 13h0 M10 17h0'

/**
 * Returns a Leaflet DivIcon for a site marker.
 * Exported so tests can inspect the icon properties directly.
 */
export function siteMarkerIcon(): L.DivIcon {
  return L.divIcon({
    html: `<div class="flex h-8 w-8 items-center justify-center rounded-full bg-primary ring-2 ring-white shadow-sm" aria-label="Site marker">
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="2">
    <path d="${BUILDING2_PATH}"/>
  </svg>
</div>`,
    className: '', // Pitfall #5 — clear default leaflet-div-icon white-box class
    iconSize: [32, 32],
    iconAnchor: [16, 32],
    popupAnchor: [0, -32],
  })
}

export interface SiteMarkerProps {
  site: MapSite
  capabilities: 'water' | 'electricity' | 'both'
  /** When set (picker mode), clicking the marker emits coords instead of showing popup */
  onPick?: (lat: number, lng: number) => void
}

export function SiteMarker({ site, capabilities, onPick }: SiteMarkerProps) {
  const icon = siteMarkerIcon()
  if (onPick) {
    // Picker mode: no popup, emit lat/lng on click
    return (
      <Marker
        position={[site.lat, site.lng]}
        icon={icon}
        eventHandlers={{ click: () => onPick(site.lat, site.lng) }}
      />
    )
  }
  return (
    <Marker position={[site.lat, site.lng]} icon={icon}>
      <Popup>
        <SitePopup site={site} capabilities={capabilities} />
      </Popup>
    </Marker>
  )
}
