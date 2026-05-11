import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import GatewayDetailPage from './$id'

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

const gateway = {
  id: '11111111-1111-1111-1111-111111111111',
  gateway_id: 'ac1f09fffe000001',
  name: 'Rooftop A',
  description: 'Building A rooftop',
  region: 'as923_2',
  lat: 13.7563,
  lng: 100.5018,
  altitude: 12,
  tags: { area: 'rooftop' },
  archived_at: null,
  archived_reason: null,
  created_at: '2026-05-01T00:00:00Z',
  updated_at: '2026-05-01T00:00:00Z',
  stats_refreshed_at: '2026-05-11T08:00:00Z',
  stats_rx_24h: 1200,
  stats_tx_24h: 100,
  stats_tx_ok_24h: 98,
  stats_sparkline: {
    bucket_start: '2026-05-10T08:00:00Z',
    bucket_width_s: 3600,
    rx: Array.from({ length: 24 }, (_, i) => i),
    tx: Array.from({ length: 24 }, () => 0),
  },
  last_seen_at: '2026-05-11T08:09:00Z',
  state: 'ONLINE' as const,
}

const archivedGateway = {
  ...gateway,
  archived_at: '2026-05-08T00:00:00Z',
  archived_reason: 'operator decommission',
  state: 'OFFLINE' as const,
}

function renderDetail() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return {
    qc,
    ...render(
      <QueryClientProvider client={qc}>
        <MemoryRouter initialEntries={[`/gateways/${gateway.id}`]}>
          <Routes>
            <Route path="/gateways/:id" element={<GatewayDetailPage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    ),
  }
}

describe('GatewayDetailPage (Plan 03-08 Task 3)', () => {
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

  it('TestGatewayDetail_Renders — name, gateway_id (mono), region in identity card', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockResolvedValue(gateway)

    renderDetail()

    await waitFor(() =>
      expect(
        screen.getByRole('heading', { name: 'Rooftop A' }),
      ).toBeInTheDocument(),
    )

    expect(screen.getByText('ac1f09fffe000001')).toBeInTheDocument()
    expect(screen.getByText(/as923_2/)).toBeInTheDocument()
  })

  it('TestGatewayDetail_StatsCard — RX / TX / Success% rendered', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockResolvedValue(gateway)

    renderDetail()

    await waitFor(() =>
      expect(screen.getByText('Last 24 hours')).toBeInTheDocument(),
    )

    // RX value 1200, TX 100, OK ratio 98% (success token).
    expect(screen.getByText('1200')).toBeInTheDocument()
    expect(screen.getByText('100')).toBeInTheDocument()
    expect(screen.getByText('98%')).toBeInTheDocument()
  })

  it('TestGatewayDetail_EditButton — Edit launches add-gateway-dialog in edit mode', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockResolvedValue(gateway)

    renderDetail()

    await waitFor(() =>
      expect(screen.getByRole('heading', { name: 'Rooftop A' })).toBeInTheDocument(),
    )

    await userEvent.click(screen.getByRole('button', { name: /Edit gateway/i }))

    expect(
      await screen.findByRole('heading', { name: /Edit gateway/i }),
    ).toBeInTheDocument()
    // Pre-populated.
    expect(screen.getByLabelText(/Gateway ID/i)).toHaveValue('ac1f09fffe000001')
  })

  it('TestGatewayDetail_DecommissionButton — opens decommission dialog', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockResolvedValue(gateway)

    renderDetail()

    await waitFor(() =>
      expect(screen.getByRole('heading', { name: 'Rooftop A' })).toBeInTheDocument(),
    )

    await userEvent.click(screen.getByRole('button', { name: /Decommission/i }))

    expect(
      await screen.findByRole('heading', { name: /Decommission this gateway\?/i }),
    ).toBeInTheDocument()
  })

  it('TestGatewayDetail_RestoreButton — archived gateway renders banner + Restore', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockResolvedValue(archivedGateway)

    renderDetail()

    await waitFor(() =>
      expect(screen.getByRole('heading', { name: 'Rooftop A' })).toBeInTheDocument(),
    )

    expect(screen.getByText(/operator decommission/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Restore/i })).toBeInTheDocument()
  })

  it('TestGatewayDetail_RestoreCallsAPI — click Restore → POST /restore + toast', async () => {
    const { apiFetch } = await import('@/lib/api')
    const { toast } = await import('sonner')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockImplementation((path: string, init?: RequestInit) => {
      if (path === `/api/gateways/${gateway.id}/restore` && init?.method === 'POST') {
        return Promise.resolve({ ...archivedGateway, archived_at: null })
      }
      // Initial detail fetch returns archived gateway.
      if (path === `/api/gateways/${gateway.id}`) {
        return Promise.resolve(archivedGateway)
      }
      return Promise.resolve({})
    })

    renderDetail()

    await waitFor(() =>
      expect(screen.getByRole('button', { name: /Restore/i })).toBeInTheDocument(),
    )

    await userEvent.click(screen.getByRole('button', { name: /Restore/i }))

    await waitFor(() =>
      expect(mock).toHaveBeenCalledWith(
        `/api/gateways/${gateway.id}/restore`,
        expect.objectContaining({ method: 'POST' }),
      ),
    )
    await waitFor(() => {
      expect(toast.success).toHaveBeenCalled()
    })
  })

  it('TestGatewayDetail_NotFound — 404 renders "Gateway not found"', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockRejectedValue(
      Object.assign(new Error('not found'), { status: 404 }),
    )

    renderDetail()

    await waitFor(() =>
      expect(screen.getByText(/Gateway not found/i)).toBeInTheDocument(),
    )
  })

  it('TestGatewayDetail_UX03 — no "tenant" / "application" in DOM', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockResolvedValue(gateway)

    const { container } = renderDetail()

    await waitFor(() =>
      expect(screen.getByRole('heading', { name: 'Rooftop A' })).toBeInTheDocument(),
    )

    expect(container.textContent ?? '').not.toMatch(/tenant|application/i)
  })
})
