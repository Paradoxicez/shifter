/**
 * GatewayMarker — Plan 05-08
 *
 * Leaflet divIcon marker for a gateway on the map.
 * Uses Antenna SVG (Lucide) with online/offline ring color.
 *
 * Pitfall #5: className set to '' to suppress Leaflet's default white-box.
 */

import L from 'leaflet'
import { Marker, Popup } from 'react-leaflet'
import type { MapGateway } from './MapView'

// Lucide Antenna SVG path — pinned verbatim.
const ANTENNA_PATH =
  'M2 12a10 10 0 0 1 18 0 M5 12a7 7 0 0 1 12 0 M8 12a4 4 0 0 1 8 0 M12 9v13'

/**
 * Returns a Leaflet DivIcon for a gateway marker.
 * Exported so tests can inspect the icon properties directly.
 */
export function gatewayMarkerIcon(online: boolean): L.DivIcon {
  const ring = online ? 'ring-success' : 'ring-destructive'
  return L.divIcon({
    html: `<div class="flex h-7 w-7 items-center justify-center rounded-full bg-muted ring-2 ${ring} shadow-sm" aria-label="Gateway marker">
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
    <path d="${ANTENNA_PATH}"/>
  </svg>
</div>`,
    className: '', // Pitfall #5 — clear default leaflet-div-icon white-box class
    iconSize: [28, 28],
    iconAnchor: [14, 28],
    popupAnchor: [0, -28],
  })
}

export interface GatewayMarkerProps {
  gateway: MapGateway
}

export function GatewayMarker({ gateway }: GatewayMarkerProps) {
  return (
    <Marker
      position={[gateway.lat, gateway.lng]}
      icon={gatewayMarkerIcon(gateway.online)}
    >
      <Popup>
        <div className="space-y-1 min-w-[160px]">
          <div className="font-semibold">{gateway.name}</div>
          <div className="text-xs text-muted-foreground">
            {gateway.online ? 'Online' : 'Offline'}
          </div>
        </div>
      </Popup>
    </Marker>
  )
}
