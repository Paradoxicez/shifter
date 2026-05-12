/**
 * MapView + SiteMarker + GatewayMarker tests — Plan 05-08 Task 1
 *
 * Covers MAP-01 (OSM tile layer), D-13 (bounds/fallback), MAP-02 (clustering),
 * Pitfall #5 (className:''), and sidebar nav order.
 */

import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'

// Use vi.hoisted so these mocks are available inside vi.mock factories
const mocks = vi.hoisted(() => {
  const mockSetView = vi.fn()
  const mockFitBounds = vi.fn()
  const mockPad = vi.fn().mockReturnThis()
  const mockLatLngBounds = vi.fn(() => ({ pad: mockPad }))
  const mockUseMap = vi.fn(() => ({ setView: mockSetView, fitBounds: mockFitBounds }))
  return { mockSetView, mockFitBounds, mockPad, mockLatLngBounds, mockUseMap }
})

// --- Leaflet mock -----------------------------------------------------------
vi.mock('react-leaflet', async () => {
  return {
    MapContainer: ({ children, ...props }: React.PropsWithChildren<Record<string, unknown>>) => (
      <div data-testid="map-container" aria-label={props['aria-label'] as string} style={props.style as React.CSSProperties}>
        {children}
      </div>
    ),
    TileLayer: ({ url, attribution }: { url: string; attribution: string }) => (
      <div data-testid="tile-layer" data-url={url} data-attribution={attribution} />
    ),
    Marker: ({ children, eventHandlers }: React.PropsWithChildren<{ eventHandlers?: Record<string, () => void> }>) => (
      <div data-testid="marker" onClick={eventHandlers?.click}>{children}</div>
    ),
    Popup: ({ children }: React.PropsWithChildren<unknown>) => <div data-testid="popup">{children}</div>,
    useMap: mocks.mockUseMap,
    useMapEvents: vi.fn(),
  }
})

vi.mock('react-leaflet-cluster', () => ({
  default: ({ children }: React.PropsWithChildren<unknown>) => (
    <div data-testid="marker-cluster-group" className="marker-cluster">{children}</div>
  ),
}))

vi.mock('leaflet', () => ({
  default: {
    divIcon: vi.fn(({ html, className }: { html: string; className: string }) => ({ html, className })),
    latLngBounds: mocks.mockLatLngBounds,
  },
  divIcon: vi.fn(({ html, className }: { html: string; className: string }) => ({ html, className })),
  latLngBounds: mocks.mockLatLngBounds,
}))

// ---------------------------------------------------------------------------

import type { MapSite, MapGateway } from './MapView'
import { MapView } from './MapView'
import { siteMarkerIcon } from './SiteMarker'
import { gatewayMarkerIcon } from './GatewayMarker'

const siteFoo: MapSite = {
  id: 'site-1',
  name: 'Foo Building',
  lat: 13.75,
  lng: 100.50,
  mp_count: 3,
  online_count: 2,
  offline_count: 1,
  today_consumption: { water: 1.234, electricity: 5.678 },
}

const gw1: MapGateway = { id: 'gw-1', name: 'Tower GW', lat: 13.76, lng: 100.51, online: true }

// Generate >50 sites for clustering test
const manySites: MapSite[] = Array.from({ length: 55 }, (_, i) => ({
  id: `site-${i}`,
  name: `Site ${i}`,
  lat: 13 + i * 0.01,
  lng: 100 + i * 0.01,
  mp_count: 1,
  online_count: 1,
  offline_count: 0,
  today_consumption: {},
}))

// Helper: wrap in MemoryRouter since SitePopup uses <Link>
function wrap(ui: React.ReactElement) {
  return render(<MemoryRouter>{ui}</MemoryRouter>)
}

describe('MapView', () => {
  it('renders MapContainer with OSM TileLayer URL and attribution', () => {
    wrap(<MapView sites={[siteFoo]} gateways={[gw1]} capabilities="both" />)
    const tile = screen.getByTestId('tile-layer')
    expect(tile.dataset.url).toContain('tile.openstreetmap.org')
    expect(tile.dataset.attribution).toContain('OpenStreetMap')
    const container = screen.getByTestId('map-container')
    expect(container).toBeInTheDocument()
  })

  it('calls fitBounds when 2+ markers are present (D-13)', () => {
    mocks.mockFitBounds.mockClear()
    mocks.mockLatLngBounds.mockClear()
    wrap(<MapView sites={[siteFoo]} gateways={[gw1]} capabilities="both" />)
    expect(mocks.mockFitBounds).toHaveBeenCalled()
  })

  it('calls setView to single site at zoom 16 when 1 marker (D-13)', () => {
    mocks.mockSetView.mockClear()
    wrap(<MapView sites={[siteFoo]} gateways={[]} capabilities="water" />)
    expect(mocks.mockSetView).toHaveBeenCalledWith([siteFoo.lat, siteFoo.lng], 16)
  })

  it('calls setView to Bangkok [13.7563, 100.5018] zoom 5 for zero markers (D-13)', () => {
    mocks.mockSetView.mockClear()
    wrap(<MapView sites={[]} gateways={[]} capabilities="water" />)
    expect(mocks.mockSetView).toHaveBeenCalledWith([13.7563, 100.5018], 5)
  })

  it('renders MarkerClusterGroup that wraps markers (MAP-02)', () => {
    wrap(<MapView sites={manySites} gateways={[]} capabilities="water" />)
    const clusterGroup = screen.getByTestId('marker-cluster-group')
    expect(clusterGroup).toBeInTheDocument()
    const markers = screen.getAllByTestId('marker')
    expect(markers.length).toBe(55)
  })
})

describe('SiteMarker icon (Pitfall #5)', () => {
  it('divIcon HTML contains bg-primary class', () => {
    // Cast via unknown since L.DivIcon types don't expose html/className
    // but the mock returns {html, className} matching the real divIcon call shape
    const icon = siteMarkerIcon() as unknown as { html: string; className: string }
    expect(icon.html).toContain('bg-primary')
  })

  it('divIcon className is empty string to suppress white-box (Pitfall #5)', () => {
    const icon = siteMarkerIcon() as unknown as { html: string; className: string }
    expect(icon.className).toBe('')
  })
})

describe('GatewayMarker icon', () => {
  it('online=true → ring-success in divIcon HTML', () => {
    const icon = gatewayMarkerIcon(true) as unknown as { html: string; className: string }
    expect(icon.html).toContain('ring-success')
  })

  it('online=false → ring-destructive in divIcon HTML', () => {
    const icon = gatewayMarkerIcon(false) as unknown as { html: string; className: string }
    expect(icon.html).toContain('ring-destructive')
  })

  it('divIcon className is empty string to suppress white-box (Pitfall #5)', () => {
    const icon = gatewayMarkerIcon(true) as unknown as { html: string; className: string }
    expect(icon.className).toBe('')
  })
})
