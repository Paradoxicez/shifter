import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DecommissionGatewayDialog } from './decommission-gateway-dialog'

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
  description: null,
  region: 'as923_2',
  lat: null,
  lng: null,
  altitude: null,
  tags: {},
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
  state: 'OFFLINE' as const,
}

function renderDialog(
  props?: Partial<React.ComponentProps<typeof DecommissionGatewayDialog>>,
) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const onOpenChange = vi.fn()
  return {
    qc,
    onOpenChange,
    ...render(
      <QueryClientProvider client={qc}>
        <DecommissionGatewayDialog
          open
          onOpenChange={onOpenChange}
          gateway={gateway}
          {...props}
        />
      </QueryClientProvider>,
    ),
  }
}

describe('DecommissionGatewayDialog (Plan 03-08 Task 2)', () => {
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

  it('TestDecommissionGatewayDialog_WarningBanner — D-31 warning copy rendered', () => {
    renderDialog()
    expect(
      screen.getByText(/route via other gateways/i),
    ).toBeInTheDocument()
  })

  it('TestDecommissionGatewayDialog_NoTypeToConfirm — only Decommission gateway button, no text confirm input', () => {
    renderDialog()
    expect(
      screen.getByRole('button', { name: /Decommission gateway/i }),
    ).toBeInTheDocument()
    // No "type to confirm" textbox.
    expect(
      screen.queryByLabelText(/Type .* to confirm/i),
    ).not.toBeInTheDocument()
  })

  it('TestDecommissionGatewayDialog_ConfirmCallsArchive — POSTs /archive with reason', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockResolvedValue({ ...gateway, archived_at: '2026-05-11T08:00:00Z' })

    const { onOpenChange } = renderDialog()

    await userEvent.click(
      screen.getByRole('button', { name: /Decommission gateway/i }),
    )

    await waitFor(() =>
      expect(mock).toHaveBeenCalledWith(
        `/api/gateways/${gateway.id}/archive`,
        expect.objectContaining({ method: 'POST' }),
      ),
    )

    const lastCall = mock.mock.calls.find(
      (c) => c[0] === `/api/gateways/${gateway.id}/archive`,
    )
    const body = JSON.parse(String((lastCall?.[1] as RequestInit)?.body ?? '{}'))
    expect(body.reason).toBeTruthy()

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
  })

  it('TestDecommissionGatewayDialog_UX03 — no "tenant" or "application" in DOM', () => {
    const { container } = renderDialog()
    expect(container.textContent ?? '').not.toMatch(/tenant|application/i)
  })
})
