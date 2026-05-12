/**
 * MapView — Plan 05-08
 *
 * Full-bleed Leaflet map container with:
 *   - OSM TileLayer (MAP-04: no API key, attribution required)
 *   - MarkerClusterGroup wrapping all markers (MAP-02, D-14)
 *   - BoundsController for auto-fit (D-13)
 *   - SiteMarker + GatewayMarker layers
 *   - picker mode: onSiteClick suppresses popups and emits coords for GW-04
 */

import { MapContainer, TileLayer, useMap } from 'react-leaflet'
import MarkerClusterGroup from 'react-leaflet-cluster'
import L from 'leaflet'
import { useEffect } from 'react'
import { SiteMarker } from './SiteMarker'
import { GatewayMarker } from './GatewayMarker'

// D-13 fallback: Bangkok at zoom 5 (AS923-2/Thailand install region default)
const BANGKOK: [number, number] = [13.7563, 100.5018]
const BANGKOK_ZOOM = 5
const SINGLE_SITE_ZOOM = 16

export type MapSite = {
  id: string
  name: string
  lat: number
  lng: number
  mp_count: number
  online_count: number
  offline_count: number
  today_consumption: { water?: number; electricity?: number }
}

export type MapGateway = {
  id: string
  name: string
  lat: number
  lng: number
  online: boolean
}

/**
 * BoundsController — sits inside MapContainer to access the Leaflet map
 * instance via useMap(). Fires on every sites/gateways change.
 *
 * D-13 rules:
 *   0 markers → Bangkok zoom 5
 *   1 marker  → setView at that position, zoom 16
 *   2+ markers → fitBounds with ±10% padding
 */
function BoundsController({
  sites,
  gateways,
}: {
  sites: MapSite[]
  gateways: MapGateway[]
}) {
  const map = useMap()
  useEffect(() => {
    const all: [number, number][] = [
      ...sites.map((s) => [s.lat, s.lng] as [number, number]),
      ...gateways.map((g) => [g.lat, g.lng] as [number, number]),
    ]
    if (all.length === 0) {
      map.setView(BANGKOK, BANGKOK_ZOOM)
      return
    }
    if (all.length === 1) {
      map.setView(all[0], SINGLE_SITE_ZOOM)
      return
    }
    // D-13: ±10% padding around the bounding box
    map.fitBounds(L.latLngBounds(all).pad(0.1))
  }, [sites, gateways, map])
  return null
}

export interface MapViewProps {
  sites: MapSite[]
  gateways: MapGateway[]
  capabilities: 'water' | 'electricity' | 'both'
  /**
   * When set, the map operates in picker mode:
   *   - Site popups are suppressed; clicking a site marker emits that site's coords
   *   - Clicking anywhere on the map emits the click coords
   * Used by MapPicker (GW-04).
   */
  onSiteClick?: (lat: number, lng: number) => void
  /** Optional additional children rendered inside the MapContainer (e.g. ClickEmitter) */
  children?: React.ReactNode
}

export function MapView({
  sites,
  gateways,
  capabilities,
  onSiteClick,
  children,
}: MapViewProps) {
  return (
    <MapContainer
      style={{ height: 'calc(100vh - 3.5rem)', width: '100%' }}
      center={BANGKOK}
      zoom={BANGKOK_ZOOM}
      scrollWheelZoom
      aria-label="Fleet map"
    >
      {/* MAP-04: OSM tiles, no API key, attribution required */}
      <TileLayer
        attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap contributors</a>'
        url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
      />
      <BoundsController sites={sites} gateways={gateways} />
      <MarkerClusterGroup chunkedLoading>
        {sites.map((s) => (
          <SiteMarker
            key={s.id}
            site={s}
            capabilities={capabilities}
            onPick={onSiteClick}
          />
        ))}
        {gateways.map((g) => (
          <GatewayMarker key={g.id} gateway={g} />
        ))}
      </MarkerClusterGroup>
      {children}
    </MapContainer>
  )
}
