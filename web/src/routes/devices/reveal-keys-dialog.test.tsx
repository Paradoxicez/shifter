import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type React from 'react'
import { ApiError } from '@/lib/api'
import { RevealKeysDialog } from './reveal-keys-dialog'

/**
 * Plan 03-10 Task 1 — RevealKeysDialog (DEV-09 / D-22 / D-26..D-28).
 *
 * Asserts:
 *   - Idle/loading/success/error UI states.
 *   - OTAA vs ABP key panel rendering.
 *   - Clipboard copy writes JSON.stringify({...keys}, null, 2).
 *   - Error-message map (403/404/409/502).
 *   - Local state clears on close (no stale keys on reopen).
 *
 * The viewer-button-hidden behaviour is asserted in $id.test.tsx — the
 * dialog itself is shown only when an admin opens it from the detail page.
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
    error: vi.fn(),
    warning: vi.fn(),
  },
}))

const DEV_EUI = '0011223344556677'

const OTAA_KEYS = {
  activation_mode: 'OTAA' as const,
  dev_eui: DEV_EUI,
  join_eui: '0000000000000000',
  app_key: '00112233445566778899aabbccddeeff',
  nwk_key: 'ffeeddccbbaa99887766554433221100',
}

const ABP_KEYS = {
  activation_mode: 'ABP' as const,
  dev_eui: DEV_EUI,
  dev_addr: '01020304',
  nwk_s_key: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
  app_s_key: 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb',
  f_cnt_up: 12,
  n_f_cnt_down: 0,
  a_f_cnt_down: 8,
}

function makeApiError(status: number, errorCode: string) {
  // Mirror ApiError shape from web/src/lib/api.ts.
  return new ApiError(status, errorCode, { error: errorCode })
}

function renderDialog(
  props?: Partial<React.ComponentProps<typeof RevealKeysDialog>>,
) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const onOpenChange = vi.fn()
  const utils = render(
    <QueryClientProvider client={qc}>
      <RevealKeysDialog
        devEUI={DEV_EUI}
        open
        onOpenChange={onOpenChange}
        {...props}
      />
    </QueryClientProvider>,
  )
  return { qc, onOpenChange, ...utils }
}

describe('RevealKeysDialog (Plan 03-10 Task 1)', () => {
  beforeEach(() => {
    Object.defineProperty(window, 'innerWidth', {
      writable: true,
      configurable: true,
      value: 1280,
    })
    // Stub navigator.clipboard for the Copy keys test.
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: vi.fn().mockResolvedValue(undefined) },
      configurable: true,
    })
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  it('TestRevealKeys_IdleState — initial render shows Reveal button + warning copy', () => {
    renderDialog()
    expect(
      screen.getByRole('button', { name: /Reveal keys/i }),
    ).toBeInTheDocument()
    expect(screen.getAllByText(/does not retain/i).length).toBeGreaterThan(0)
  })

  it('TestRevealKeys_OTAAFlow — clicking Reveal renders OTAA keys panel', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockResolvedValue(OTAA_KEYS)

    renderDialog()
    await userEvent.click(screen.getByRole('button', { name: /Reveal keys/i }))

    await waitFor(() => {
      expect(screen.getByText(OTAA_KEYS.app_key)).toBeInTheDocument()
    })
    expect(screen.getByText(OTAA_KEYS.nwk_key)).toBeInTheDocument()
    expect(screen.getByText(OTAA_KEYS.join_eui)).toBeInTheDocument()
    expect(screen.getByText(/AppKey/)).toBeInTheDocument()
    expect(screen.getByText(/NwkKey/)).toBeInTheDocument()

    const call = mock.mock.calls.find((c) =>
      String(c[0]).includes(`/api/devices/${DEV_EUI}/keys`),
    )
    expect(call).toBeDefined()
    expect((call?.[1] as RequestInit)?.method).toBe('POST')
  })

  it('TestRevealKeys_ABPFlow — clicking Reveal renders ABP keys panel with FCnt counters', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockResolvedValue(ABP_KEYS)

    renderDialog()
    await userEvent.click(screen.getByRole('button', { name: /Reveal keys/i }))

    await waitFor(() => {
      expect(screen.getByText(ABP_KEYS.dev_addr)).toBeInTheDocument()
    })
    expect(screen.getByText(ABP_KEYS.nwk_s_key)).toBeInTheDocument()
    expect(screen.getByText(ABP_KEYS.app_s_key)).toBeInTheDocument()
    expect(screen.getByText('12')).toBeInTheDocument() // f_cnt_up
    expect(screen.getByText('8')).toBeInTheDocument() // a_f_cnt_down
  })

  it('TestRevealKeys_CopyKeys — clipboard receives JSON.stringify({...keys}, null, 2)', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockResolvedValue(OTAA_KEYS)

    renderDialog()
    await userEvent.click(screen.getByRole('button', { name: /Reveal keys/i }))

    await waitFor(() => {
      expect(screen.getByText(OTAA_KEYS.app_key)).toBeInTheDocument()
    })

    const copyAll = screen.getByRole('button', { name: /Copy all/i })
    await userEvent.click(copyAll)

    const writeText = navigator.clipboard.writeText as ReturnType<typeof vi.fn>
    expect(writeText).toHaveBeenCalledTimes(1)
    const arg = writeText.mock.calls[0]?.[0] as string
    // Must be valid JSON, two-space indent.
    expect(arg).toContain('\n  "app_key"')
    expect(arg).toContain(OTAA_KEYS.app_key)
    expect(JSON.parse(arg).activation_mode).toBe('OTAA')

    const { toast } = await import('sonner')
    expect(toast.success).toHaveBeenCalledWith('Keys copied')
  })

  it('TestRevealKeys_403 — viewer-style 403 maps to permission-denied error message', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockRejectedValue(makeApiError(403, 'forbidden'))

    renderDialog()
    await userEvent.click(screen.getByRole('button', { name: /Reveal keys/i }))

    await waitFor(() => {
      expect(
        screen.getByText(/do not have permission to reveal device keys/i),
      ).toBeInTheDocument()
    })
  })

  it('TestRevealKeys_404 — 404 maps to "Device not found" error', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockRejectedValue(makeApiError(404, 'not_found'))

    renderDialog()
    await userEvent.click(screen.getByRole('button', { name: /Reveal keys/i }))

    await waitFor(() => {
      expect(screen.getByText(/Device not found/i)).toBeInTheDocument()
    })
  })

  it('TestRevealKeys_409 — 409 no_credentials_in_cs maps to ChirpStack-removed error', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockRejectedValue(makeApiError(409, 'no_credentials_in_cs'))

    renderDialog()
    await userEvent.click(screen.getByRole('button', { name: /Reveal keys/i }))

    await waitFor(() => {
      expect(
        screen.getByText(/no credentials for this device/i),
      ).toBeInTheDocument()
    })
  })

  it('TestRevealKeys_502 — 502 cs_get_keys_failed maps to "unreachable" error', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockRejectedValue(makeApiError(502, 'cs_get_keys_failed'))

    renderDialog()
    await userEvent.click(screen.getByRole('button', { name: /Reveal keys/i }))

    await waitFor(() => {
      expect(
        screen.getByText(/unreachable.*Settings.*Connection/i),
      ).toBeInTheDocument()
    })
  })

  it('TestRevealKeys_ClearOnClose — closing the dialog clears keys so reopening shows idle state', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockResolvedValue(OTAA_KEYS)

    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const onOpenChange = vi.fn()
    const { rerender } = render(
      <QueryClientProvider client={qc}>
        <RevealKeysDialog
          devEUI={DEV_EUI}
          open
          onOpenChange={onOpenChange}
        />
      </QueryClientProvider>,
    )

    await userEvent.click(screen.getByRole('button', { name: /Reveal keys/i }))
    await waitFor(() => {
      expect(screen.getByText(OTAA_KEYS.app_key)).toBeInTheDocument()
    })

    // Close: open=false.
    rerender(
      <QueryClientProvider client={qc}>
        <RevealKeysDialog
          devEUI={DEV_EUI}
          open={false}
          onOpenChange={onOpenChange}
        />
      </QueryClientProvider>,
    )

    // Reopen.
    rerender(
      <QueryClientProvider client={qc}>
        <RevealKeysDialog
          devEUI={DEV_EUI}
          open
          onOpenChange={onOpenChange}
        />
      </QueryClientProvider>,
    )

    // Initial state: Reveal button visible, NO stale key in DOM.
    expect(
      screen.getByRole('button', { name: /Reveal keys/i }),
    ).toBeInTheDocument()
    expect(screen.queryByText(OTAA_KEYS.app_key)).not.toBeInTheDocument()
  })

  it('TestRevealKeys_UX03 — no "tenant" or user-facing "application" wording', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockResolvedValue(OTAA_KEYS)

    const { container } = renderDialog()
    await userEvent.click(screen.getByRole('button', { name: /Reveal keys/i }))

    await waitFor(() => {
      expect(screen.getByText(OTAA_KEYS.app_key)).toBeInTheDocument()
    })

    expect(container.textContent ?? '').not.toMatch(/tenant/i)
    expect(container.textContent ?? '').not.toMatch(/application/i)
  })
})
