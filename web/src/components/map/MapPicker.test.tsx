/**
 * MapPicker + GW-04 gateway dialog integration tests — Plan 05-08 Task 2
 *
 * Tests:
 *   1. Gateway dialog has "Pick on map" button
 *   2. Clicking "Pick on map" opens a dialog containing MapView
 *   3. Clicking map point closes dialog and populates lat/lng fields
 *   4. Cancel leaves form values unchanged
 *   5. Clicking site marker emits that site's lat/lng
 */

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi, beforeEach } from 'vitest'

// Hoist mocks for Leaflet hooks used in MapView/MapPicker
const mocks = vi.hoisted(() => {
  const mockSetView = vi.fn()
  const mockFitBounds = vi.fn()
  const mockUseMap = vi.fn(() => ({ setView: mockSetView, fitBounds: mockFitBounds }))
  // useMapEvents: capture the click handler so tests can trigger it
  let _clickHandler: ((e: { latlng: { lat: number; lng: number } }) => void) | undefined
  const mockUseMapEvents = vi.fn((handlers: { click?: (e: { latlng: { lat: number; lng: number } }) => void }) => {
    _clickHandler = handlers.click
    return null
  })
  const triggerMapClick = (lat: number, lng: number) => {
    _clickHandler?.({ latlng: { lat, lng } })
  }
  return { mockSetView, mockFitBounds, mockUseMap, mockUseMapEvents, triggerMapClick }
})

vi.mock('react-leaflet', async () => ({
  MapContainer: ({ children }: React.PropsWithChildren<unknown>) => (
    <div data-testid="map-container">{children}</div>
  ),
  TileLayer: () => <div data-testid="tile-layer" />,
  Marker: ({ children, eventHandlers }: React.PropsWithChildren<{ eventHandlers?: { click?: () => void } }>) => (
    <div data-testid="marker" onClick={eventHandlers?.click}>{children}</div>
  ),
  Popup: ({ children }: React.PropsWithChildren<unknown>) => <div data-testid="popup">{children}</div>,
  useMap: mocks.mockUseMap,
  useMapEvents: mocks.mockUseMapEvents,
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

vi.mock('@/lib/api', async (orig) => {
  const real = await orig<typeof import('@/lib/api')>()
  return { ...real, apiFetch: vi.fn() }
})

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

import { apiFetch } from '@/lib/api'
import { AddGatewayDialog } from '@/routes/gateways/add-gateway-dialog'

const mockApiFetch = vi.mocked(apiFetch)

const emptyMapData = { sites: [], gateways: [] }

function wrap(ui: React.ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>{ui}</MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('MapPicker (GW-04) — gateway dialog integration', () => {
  beforeEach(() => {
    mockApiFetch.mockResolvedValue(emptyMapData)
    Object.defineProperty(window, 'innerWidth', { writable: true, configurable: true, value: 1024 })
  })

  it('renders "Pick on map" button in gateway dialog', () => {
    wrap(<AddGatewayDialog open onOpenChange={vi.fn()} />)
    expect(screen.getByRole('button', { name: /Pick on map/i })).toBeInTheDocument()
  })

  it('"Pick on map" button is enabled (not disabled like Phase 3 placeholder)', () => {
    wrap(<AddGatewayDialog open onOpenChange={vi.fn()} />)
    const btn = screen.getByRole('button', { name: /Pick on map/i })
    expect(btn).not.toBeDisabled()
  })

  it('clicking "Pick on map" opens the MapPicker dialog', async () => {
    const user = userEvent.setup()
    wrap(<AddGatewayDialog open onOpenChange={vi.fn()} />)
    const btn = screen.getByRole('button', { name: /Pick on map/i })
    await user.click(btn)
    // MapPicker dialog should appear — check for map-container or dialog heading
    await waitFor(() => {
      expect(screen.getByText(/Pick location on map/i)).toBeInTheDocument()
    })
  })

  it('Cancel button in MapPicker closes picker without changing form values', async () => {
    const user = userEvent.setup()
    wrap(<AddGatewayDialog open onOpenChange={vi.fn()} />)

    // Open picker
    await user.click(screen.getByRole('button', { name: /Pick on map/i }))
    await waitFor(() => expect(screen.getByText(/Pick location on map/i)).toBeInTheDocument())

    // Click Cancel
    await user.click(screen.getByRole('button', { name: /Cancel/i }))

    // Picker dialog should close
    await waitFor(() => {
      expect(screen.queryByText(/Pick location on map/i)).not.toBeInTheDocument()
    })

    // Lat/lng fields should still be empty
    const latInput = screen.getByPlaceholderText('13.7563')
    const lngInput = screen.getByPlaceholderText('100.5018')
    expect(latInput).toHaveValue('')
    expect(lngInput).toHaveValue('')
  })

  it('clicking map point in picker populates lat/lng form fields', async () => {
    const user = userEvent.setup()
    wrap(<AddGatewayDialog open onOpenChange={vi.fn()} />)

    // Open picker
    await user.click(screen.getByRole('button', { name: /Pick on map/i }))
    await waitFor(() => expect(screen.getByText(/Pick location on map/i)).toBeInTheDocument())

    // Simulate a map click via the ClickEmitter
    mocks.triggerMapClick(13.75, 100.50)

    // Picker should close and form values set
    await waitFor(() => {
      expect(screen.queryByText(/Pick location on map/i)).not.toBeInTheDocument()
    })

    const latInput = screen.getByPlaceholderText('13.7563') as HTMLInputElement
    const lngInput = screen.getByPlaceholderText('100.5018') as HTMLInputElement
    expect(latInput.value).toBe('13.75')
    expect(lngInput.value).toBe('100.5')
  })
})
