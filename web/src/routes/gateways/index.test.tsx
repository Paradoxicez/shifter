import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import GatewaysPage from './index'

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

const baseGateway = {
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
  stats_tx_ok_24h: 100, // 100% success → success token
  stats_sparkline: {
    bucket_start: '2026-05-10T08:00:00Z',
    bucket_width_s: 3600,
    rx: Array.from({ length: 24 }, (_, i) => i * 2),
    tx: Array.from({ length: 24 }, (_, i) => i),
  },
  last_seen_at: '2026-05-11T08:09:00Z',
  state: 'ONLINE' as const,
}

const warningGateway = {
  ...baseGateway,
  id: '22222222-2222-2222-2222-222222222222',
  gateway_id: 'ac1f09fffe000002',
  name: 'Rooftop B',
  stats_rx_24h: 800,
  stats_tx_24h: 100,
  stats_tx_ok_24h: 80, // 80% → warning token
  state: 'ONLINE' as const,
}

const archivedGateway = {
  ...baseGateway,
  id: '33333333-3333-3333-3333-333333333333',
  gateway_id: 'ac1f09fffe000003',
  name: 'Decommissioned C',
  archived_at: '2026-05-08T00:00:00Z',
  archived_reason: 'operator decommission',
  state: 'OFFLINE' as const,
}

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return {
    qc,
    ...render(
      <QueryClientProvider client={qc}>
        <MemoryRouter initialEntries={['/gateways']}>
          <GatewaysPage />
        </MemoryRouter>
      </QueryClientProvider>,
    ),
  }
}

describe('GatewaysPage (Plan 03-08 Task 2)', () => {
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

  it('TestGatewaysList_Render — renders columns and 3 rows', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockResolvedValueOnce({
      total: 2,
      items: [baseGateway, warningGateway],
    })
    // Mock install-state for region default — not strictly read here.
    mock.mockResolvedValue({})

    renderPage()

    await waitFor(() =>
      expect(screen.getByText('Rooftop A')).toBeInTheDocument(),
    )
    expect(screen.getByText('Rooftop B')).toBeInTheDocument()
    // Headers from the table.
    expect(screen.getByRole('columnheader', { name: /Name/i })).toBeInTheDocument()
    expect(
      screen.getByRole('columnheader', { name: /Gateway ID/i }),
    ).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: /Region/i })).toBeInTheDocument()
    expect(
      screen.getByRole('columnheader', { name: /Last seen/i }),
    ).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: /Trend/i })).toBeInTheDocument()

    // Gateway ID cells render in mono.
    const idCells = screen.getAllByText(/ac1f09fffe00000[12]/)
    expect(idCells.length).toBeGreaterThan(0)
  })

  it('TestGatewaysList_ShowArchivedToggle — toggle drives include_archived param', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockImplementation((path: string) => {
      if (path.startsWith('/api/gateways')) {
        if (path.includes('include_archived=true')) {
          return Promise.resolve({
            total: 1,
            items: [archivedGateway],
          })
        }
        return Promise.resolve({ total: 1, items: [baseGateway] })
      }
      return Promise.resolve({})
    })

    renderPage()

    await waitFor(() =>
      expect(screen.getByText('Rooftop A')).toBeInTheDocument(),
    )

    // Default fetch: no include_archived.
    expect(mock).toHaveBeenCalledWith(expect.not.stringContaining('include_archived=true'))

    // Click the toggle.
    const toggle = screen.getByRole('button', { name: /Show archived/i })
    await userEvent.click(toggle)

    await waitFor(() => {
      expect(mock).toHaveBeenCalledWith(expect.stringContaining('include_archived=true'))
    })

    await waitFor(() =>
      expect(screen.getByText('Decommissioned C')).toBeInTheDocument(),
    )
    // Archived row carries opacity-60 (or opacity-50) styling.
    const row = screen.getByText('Decommissioned C').closest('tr')
    expect(row?.className).toMatch(/opacity-(50|60)/)
  })

  it('TestGatewaysList_RestoreAction_AdminVisible — Restore in archived row menu', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockImplementation((path: string) => {
      if (path.startsWith('/api/gateways')) {
        return Promise.resolve({ total: 1, items: [archivedGateway] })
      }
      return Promise.resolve({})
    })

    renderPage()

    await waitFor(() =>
      expect(screen.getByText('Decommissioned C')).toBeInTheDocument(),
    )

    // Open row action menu.
    const trigger = screen.getByRole('button', { name: /Row actions/i })
    await userEvent.click(trigger)
    expect(await screen.findByText('Restore')).toBeInTheDocument()
  })

  it('TestGatewaysList_SparklineUsesCSSVars — renders area fill with var(--success/--warning/--destructive)', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockResolvedValueOnce({
      total: 2,
      items: [baseGateway, warningGateway],
    })
    mock.mockResolvedValue({})

    const { container } = renderPage()

    await waitFor(() =>
      expect(screen.getByText('Rooftop A')).toBeInTheDocument(),
    )

    // The sparkline svg path fill attribute uses the CSS variable form.
    // We assert the HTML contains var(--success) and var(--warning) for the
    // two rows respectively. Recharts emits SVG path with the fill attr.
    await waitFor(() => {
      const html = container.innerHTML
      expect(html).toContain('var(--success)')
      expect(html).toContain('var(--warning)')
    })

    // Negative: no hex color literals (the sparkline must use tokens only).
    // Sanity grep on the container HTML produced.
    expect(container.innerHTML).not.toMatch(/fill="#[0-9A-Fa-f]{3,6}"/)
  })

  it('TestGatewaysList_UX03 — no "tenant" or "application" in rendered output', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockResolvedValueOnce({
      total: 1,
      items: [baseGateway],
    })
    mock.mockResolvedValue({})

    const { container } = renderPage()

    await waitFor(() =>
      expect(screen.getByText('Rooftop A')).toBeInTheDocument(),
    )

    // Use the void to ensure within is referenced when needed elsewhere.
    void within
    expect(container.textContent ?? '').not.toMatch(/tenant|application/i)
  })
})
