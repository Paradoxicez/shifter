import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type React from 'react'
import { BulkDecommissionDialog } from './bulk-decommission-dialog'

/**
 * Plan 03-09 Task 2 — BulkDecommissionDialog.
 *
 * Mirrors the gateway decommission dialog test shape (Plan 03-08). Asserts
 * the partial-success toast routing and the request body shape.
 */

vi.mock('@/lib/api', async (orig) => {
  const real = await orig<typeof import('@/lib/api')>()
  return {
    ...real,
    apiFetch: vi.fn(),
  }
})

vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    warning: vi.fn(),
    error: vi.fn(),
  },
}))

const A = '11111111-1111-4111-8111-111111111111'
const B = '22222222-2222-4222-8222-222222222222'

function renderDialog(
  props?: Partial<React.ComponentProps<typeof BulkDecommissionDialog>>,
) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const onOpenChange = vi.fn()
  return {
    qc,
    onOpenChange,
    ...render(
      <QueryClientProvider client={qc}>
        <BulkDecommissionDialog
          open
          onOpenChange={onOpenChange}
          deviceIds={[A, B]}
          {...props}
        />
      </QueryClientProvider>,
    ),
  }
}

describe('BulkDecommissionDialog (Plan 03-09 Task 2)', () => {
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

  it('TestBulkDecommissionDialog_TitleHasCount — header reads "Decommission 2 devices?"', () => {
    renderDialog()
    expect(
      screen.getByRole('heading', { name: /Decommission 2 devices\?/i }),
    ).toBeInTheDocument()
  })

  it('TestBulkDecommissionDialog_SingularTitle — N=1 uses "Decommission this device?"', () => {
    renderDialog({ deviceIds: [A] })
    expect(
      screen.getByRole('heading', { name: /Decommission this device\?/i }),
    ).toBeInTheDocument()
  })

  it('TestBulkDecommissionDialog_ConfirmCallsAPI — POSTs /api/devices/bulk-decommission with ids + reason', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockResolvedValue({
      succeeded: 2,
      failed: 0,
      outcomes: [
        { id: A, status: 'decommissioned' },
        { id: B, status: 'decommissioned' },
      ],
    })

    renderDialog()
    await userEvent.click(
      screen.getByRole('button', { name: /Decommission 2 devices/i }),
    )

    await waitFor(() => expect(mock).toHaveBeenCalled())
    const call = mock.mock.calls.find(
      (c) => c[0] === '/api/devices/bulk-decommission',
    )
    expect(call).toBeDefined()
    const body = JSON.parse(String((call?.[1] as RequestInit)?.body ?? '{}'))
    expect(body.device_ids).toEqual([A, B])
    expect(body.reason).toBeTruthy()
  })

  it('TestBulkDecommissionDialog_PartialSuccessToast — succeeded+failed → toast.warning', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockResolvedValue({
      succeeded: 1,
      failed: 1,
      outcomes: [
        { id: A, status: 'decommissioned' },
        { id: B, status: 'failed', reason: 'binding_close' },
      ],
    })
    const { toast } = await import('sonner')

    renderDialog()
    await userEvent.click(
      screen.getByRole('button', { name: /Decommission 2 devices/i }),
    )

    await waitFor(() =>
      expect(toast.warning).toHaveBeenCalledWith(
        expect.stringMatching(/Decommissioned 1 of 2/),
      ),
    )
  })

  it('TestBulkDecommissionDialog_AllFailuresErrorToast — succeeded=0 → toast.error', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockResolvedValue({ succeeded: 0, failed: 2, outcomes: [] })
    const { toast } = await import('sonner')

    renderDialog()
    await userEvent.click(
      screen.getByRole('button', { name: /Decommission 2 devices/i }),
    )

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(
        expect.stringMatching(/Could not decommission/),
      ),
    )
  })

  it('TestBulkDecommissionDialog_NoTypeToConfirm — body has no "type to confirm" input', () => {
    renderDialog()
    expect(
      screen.queryByLabelText(/type .* to confirm/i),
    ).not.toBeInTheDocument()
  })

  it('TestBulkDecommissionDialog_UX03 — no "tenant" or "application" wording', () => {
    const { container } = renderDialog()
    expect(container.textContent ?? '').not.toMatch(/tenant|application/i)
  })
})
