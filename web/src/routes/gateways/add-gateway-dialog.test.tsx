import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AddGatewayDialog } from './add-gateway-dialog'

// Leaflet mocks — required because AddGatewayDialog now imports MapPicker
// which pulls in react-leaflet + leaflet (Plan 05-08 GW-04).
vi.mock('react-leaflet', async () => ({
  MapContainer: ({ children }: React.PropsWithChildren<unknown>) => (
    <div data-testid="map-container">{children}</div>
  ),
  TileLayer: () => <div />,
  Marker: ({ children }: React.PropsWithChildren<unknown>) => <div>{children}</div>,
  Popup: ({ children }: React.PropsWithChildren<unknown>) => <div>{children}</div>,
  useMap: vi.fn(() => ({ setView: vi.fn(), fitBounds: vi.fn() })),
  useMapEvents: vi.fn(),
}))

vi.mock('react-leaflet-cluster', () => ({
  default: ({ children }: React.PropsWithChildren<unknown>) => <div>{children}</div>,
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
  return {
    ...real,
    apiFetch: vi.fn(),
  }
})

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

const fullGateway = {
  id: '11111111-1111-1111-1111-111111111111',
  gateway_id: 'ac1f09fffe000001',
  name: 'Rooftop A',
  description: 'desc',
  region: 'as923_2',
  lat: 13.7563,
  lng: 100.5018,
  altitude: null,
  tags: { area: 'rooftop' },
  archived_at: null,
  archived_reason: null,
  created_at: '2026-05-01T00:00:00Z',
  updated_at: '2026-05-01T00:00:00Z',
  stats_refreshed_at: null,
  stats_rx_24h: null,
  stats_tx_24h: null,
  stats_tx_ok_24h: null,
  stats_sparkline: null,
  last_seen_at: null,
  state: 'NEVER_SEEN' as const,
}

function renderDialog(props?: Partial<React.ComponentProps<typeof AddGatewayDialog>>) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const onOpenChange = vi.fn()
  return {
    qc,
    onOpenChange,
    ...render(
      <QueryClientProvider client={qc}>
        <AddGatewayDialog open onOpenChange={onOpenChange} {...props} />
      </QueryClientProvider>,
    ),
  }
}

describe('AddGatewayDialog (Plan 03-08 Task 2)', () => {
  beforeEach(() => {
    Object.defineProperty(window, 'innerWidth', {
      writable: true,
      configurable: true,
      value: 1280,
    })
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  it('TestAddGatewayDialog_PickOnMapEnabled — button is enabled (GW-04 shipped in Plan 05-08)', () => {
    // Phase 3 had this disabled with title="Available in v5." as a forward-compat slot.
    // Plan 05-08 (GW-04) ships the MapPicker — button is now enabled.
    renderDialog()

    const btn = screen.getByRole('button', { name: /Pick on map/i })
    expect(btn).not.toBeDisabled()
  })

  it('TestAddGatewayDialog_GatewayIDValidation — paste normalizes to lowercase no-separator', async () => {
    renderDialog()

    const idInput = screen.getByLabelText(/Gateway ID/i) as HTMLInputElement
    await userEvent.type(idInput, 'AC:1F:09:FF:FE:00:00:01')
    await userEvent.tab() // blur triggers normalize

    expect(idInput.value).toBe('ac1f09fffe000001')
  })

  it('TestAddGatewayDialog_LatLngRangeValidation — lat=91 surfaces inline error', async () => {
    renderDialog()

    await userEvent.type(screen.getByLabelText(/Latitude/i), '91')
    await userEvent.tab()
    expect(
      await screen.findByText('Latitude must be between −90 and 90.'),
    ).toBeInTheDocument()
  })

  it('TestAddGatewayDialog_LngOutOfRange — lng=-181 surfaces inline error', async () => {
    renderDialog()

    await userEvent.type(screen.getByLabelText(/Longitude/i), '-181')
    await userEvent.tab()
    expect(
      await screen.findByText('Longitude must be between −180 and 180.'),
    ).toBeInTheDocument()
  })

  it('TestAddGatewayDialog_SubmitCallsCreate — POST /api/gateways with normalized payload', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockResolvedValue(fullGateway)

    const { onOpenChange } = renderDialog()

    await userEvent.type(
      screen.getByLabelText(/Gateway ID/i),
      'ac1f09fffe000001',
    )
    await userEvent.type(screen.getByLabelText(/^Name$/i), 'Rooftop A')

    await userEvent.click(screen.getByRole('button', { name: /Add gateway/i }))

    await waitFor(() =>
      expect(mock).toHaveBeenCalledWith(
        '/api/gateways',
        expect.objectContaining({ method: 'POST' }),
      ),
    )

    // Body contains the normalized gateway_id + name.
    const calls = mock.mock.calls.filter((c) => c[0] === '/api/gateways')
    const lastInit = calls[calls.length - 1]?.[1] as RequestInit
    const body = JSON.parse(String(lastInit?.body ?? '{}'))
    expect(body.gateway_id).toBe('ac1f09fffe000001')
    expect(body.name).toBe('Rooftop A')

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
  })

  it('TestEditGatewayDialog_PrePopulated — opens with gateway values', async () => {
    renderDialog({ gateway: fullGateway })

    expect(screen.getByLabelText(/Gateway ID/i)).toHaveValue('ac1f09fffe000001')
    expect(screen.getByLabelText(/^Name$/i)).toHaveValue('Rooftop A')
    expect(screen.getByRole('button', { name: /Save changes/i })).toBeInTheDocument()
  })

  it('TestAddGatewayDialog_UX03 — no "tenant" or "application" in DOM', () => {
    const { container } = renderDialog()
    expect(container.textContent ?? '').not.toMatch(/tenant|application/i)
  })
})
