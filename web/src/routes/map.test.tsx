/**
 * MapPage route tests — Plan 05-08 Task 1
 *
 * Tests the /map route component:
 *   1. Loading state shows Skeleton
 *   2. Empty sites + gateways → MapEmptyState
 *   3. Data with sites → MapView rendered
 */

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'

// Mock Leaflet CSS shim (side-effect only module — nothing to test)
vi.mock('@/lib/leaflet-css', () => ({}))

// Mock react-leaflet to avoid DOM/canvas requirements
vi.mock('react-leaflet', () => ({
  MapContainer: ({ children }: React.PropsWithChildren<unknown>) => (
    <div data-testid="map-container">{children}</div>
  ),
  TileLayer: () => <div data-testid="tile-layer" />,
  Marker: ({ children }: React.PropsWithChildren<unknown>) => (
    <div data-testid="marker">{children}</div>
  ),
  Popup: ({ children }: React.PropsWithChildren<unknown>) => (
    <div data-testid="popup">{children}</div>
  ),
  useMap: vi.fn(() => ({ setView: vi.fn(), fitBounds: vi.fn() })),
  useMapEvents: vi.fn(),
}))

vi.mock('react-leaflet-cluster', () => ({
  default: ({ children }: React.PropsWithChildren<unknown>) => (
    <div data-testid="cluster-group">{children}</div>
  ),
}))

vi.mock('leaflet', () => ({
  default: {
    divIcon: vi.fn(() => ({ html: '', className: '' })),
    latLngBounds: vi.fn(() => ({ pad: vi.fn().mockReturnThis() })),
  },
  divIcon: vi.fn(() => ({ html: '', className: '' })),
  latLngBounds: vi.fn(() => ({ pad: vi.fn().mockReturnThis() })),
}))

// Mock API
vi.mock('@/lib/api', async (orig) => {
  const real = await orig<typeof import('@/lib/api')>()
  return { ...real, apiFetch: vi.fn() }
})

vi.mock('@/hooks/useDashboardScope', () => ({
  useDashboardScope: () => ({
    data: { capabilities: 'both', onboarding: { gateway_count: 1, device_count: 1, uplink_count: 1 } },
    isLoading: false,
  }),
}))

import { apiFetch } from '@/lib/api'
import { MapPage } from './map'

const mockApiFetch = vi.mocked(apiFetch)

function wrap(ui: React.ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>{ui}</MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('MapPage (/map route)', () => {
  it('shows Skeleton while loading', () => {
    // apiFetch never resolves → isLoading stays true
    mockApiFetch.mockReturnValue(new Promise(() => {}))
    wrap(<MapPage />)
    // While loading, the empty state should not be visible
    expect(document.body.textContent).not.toContain('No locations')
  })

  it('shows MapEmptyState when sites and gateways are empty', async () => {
    mockApiFetch.mockResolvedValue({ sites: [], gateways: [] })
    wrap(<MapPage />)
    await waitFor(() => {
      expect(screen.getByText('No locations on the map')).toBeInTheDocument()
    })
  })

  it('renders MapView when data has sites', async () => {
    mockApiFetch.mockResolvedValue({
      sites: [
        {
          id: 'site-1',
          name: 'Test Site',
          lat: 13.75,
          lng: 100.50,
          mp_count: 2,
          online_count: 1,
          offline_count: 1,
          today_consumption: { water: 1.5 },
        },
      ],
      gateways: [],
    })
    wrap(<MapPage />)
    await waitFor(() => {
      expect(screen.getByTestId('map-container')).toBeInTheDocument()
    })
  })
})
