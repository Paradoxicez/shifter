import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { SwapMeterDialog } from './swap-meter-dialog'

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

const MP_ID = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
const MP_NAME = 'Apt 305'

const mockMPDetail = {
  id: MP_ID,
  name: MP_NAME,
  site: { id: 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb', name: 'Building A' },
  utility_class: 'water' as const,
  active_binding: {
    id: 'cccccccc-cccc-cccc-cccc-cccccccccccc',
    valid_from: '2026-01-01T00:00:00.000000Z',
    reading_offset: '0',
    device: {
      id: 'dddddddd-dddd-dddd-dddd-dddddddddddd',
      dev_eui: '0102030405060708',
      name: 'Outgoing meter',
    },
    device_profile: {
      id: 'eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee',
      name: 'Axioma Qalcosonic W1',
      capabilities: ['cumulative'],
      counter_modulus: 4294967296,
    },
  },
  latest_measurement: {
    time: new Date(Date.now() - 5 * 60_000).toISOString(),
    raw_value: '12345',
    cumulative_value: '12345',
    instant_value: null,
    battery_pct: 87,
    quality: 'ok',
  },
}

const mockUnboundDevice = {
  id: 'ffffffff-ffff-ffff-ffff-ffffffffffff',
  dev_eui: '1122334455667788',
  name: 'New meter',
  device_profile_id: 'eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee',
}

function renderDialog(props?: Partial<React.ComponentProps<typeof SwapMeterDialog>>) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <SwapMeterDialog
        open
        onOpenChange={vi.fn()}
        meteringPointId={MP_ID}
        meteringPointName={MP_NAME}
        {...props}
      />
    </QueryClientProvider>,
  )
}

describe('SwapMeterDialog (Plan 02-14 / D-12 + D-13 + D-14)', () => {
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

  it('auto-fills outgoing reading R from latest_measurement.cumulative_value', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockImplementation((path: string) => {
      if (path === `/api/metering-points/${MP_ID}`) return Promise.resolve(mockMPDetail)
      if (path === '/api/devices') return Promise.resolve([mockUnboundDevice])
      return Promise.resolve({})
    })

    renderDialog()

    await waitFor(() => {
      expect(screen.getByText(/Outgoing reading:/)).toBeInTheDocument()
    })
    expect(screen.getByText('12345')).toBeInTheDocument()
    // The auto-fill card prose includes the relative-time tail.
    expect(screen.getByText(/min ago/)).toBeInTheDocument()
  })

  it('verify checkbox is required to advance from step 1', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockImplementation((path: string) => {
      if (path === `/api/metering-points/${MP_ID}`) return Promise.resolve(mockMPDetail)
      if (path === '/api/devices') return Promise.resolve([mockUnboundDevice])
      return Promise.resolve({})
    })

    renderDialog()

    await waitFor(() => expect(screen.getByText(/Outgoing reading:/)).toBeInTheDocument())

    const next = screen.getByRole('button', { name: /Next/i })
    expect(next).toBeDisabled()

    await userEvent.click(
      screen.getByLabelText(/I verified this matches the physical meter/i),
    )
    expect(next).not.toBeDisabled()
  })

  it('step 3 renders the math read-back panel with R, N, and proposed offset', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockImplementation((path: string) => {
      if (path === `/api/metering-points/${MP_ID}`) return Promise.resolve(mockMPDetail)
      if (path === '/api/devices') return Promise.resolve([mockUnboundDevice])
      return Promise.resolve({})
    })

    renderDialog()

    await waitFor(() => expect(screen.getByText(/Outgoing reading:/)).toBeInTheDocument())
    await userEvent.click(
      screen.getByLabelText(/I verified this matches the physical meter/i),
    )
    await userEvent.click(screen.getByRole('button', { name: /Next/i }))

    // Step 2 — pick the new device.
    await waitFor(() =>
      expect(
        screen.getByRole('heading', { name: 'Select the new device' }),
      ).toBeInTheDocument(),
    )
    await userEvent.click(screen.getByLabelText('New device'))
    await userEvent.click(await screen.findByText(/New meter — 1122334455667788/))
    await userEvent.click(screen.getByRole('button', { name: /Next/i }))

    // Step 3 — math read-back.
    await waitFor(() =>
      expect(
        screen.getByRole('heading', { name: 'Review the math and confirm' }),
      ).toBeInTheDocument(),
    )
    expect(screen.getByText(/Outgoing reading R/)).toBeInTheDocument()
    expect(screen.getByText(/New meter initial N/)).toBeInTheDocument()
    expect(screen.getByText(/Proposed offset/)).toBeInTheDocument()
    // R numeric value present in the read-back panel.
    expect(screen.getAllByText('12345').length).toBeGreaterThanOrEqual(1)
  })

  it('submit posts /api/metering-points/{id}/swap with the right body', async () => {
    const { apiFetch } = await import('@/lib/api')
    const { toast } = await import('sonner')
    const mock = apiFetch as ReturnType<typeof vi.fn>

    mock.mockImplementation((path: string, init?: RequestInit) => {
      if (path === `/api/metering-points/${MP_ID}`) return Promise.resolve(mockMPDetail)
      if (path === '/api/devices') return Promise.resolve([mockUnboundDevice])
      if (path === `/api/metering-points/${MP_ID}/swap` && init?.method === 'POST') {
        return Promise.resolve({ binding_id: 'newbinding' })
      }
      return Promise.resolve({})
    })

    renderDialog()

    await waitFor(() => expect(screen.getByText(/Outgoing reading:/)).toBeInTheDocument())
    await userEvent.click(
      screen.getByLabelText(/I verified this matches the physical meter/i),
    )
    await userEvent.click(screen.getByRole('button', { name: /Next/i }))

    await userEvent.click(screen.getByLabelText('New device'))
    await userEvent.click(await screen.findByText(/New meter — 1122334455667788/))
    await userEvent.click(screen.getByRole('button', { name: /Next/i }))

    await userEvent.click(screen.getByRole('button', { name: 'Confirm swap' }))

    await waitFor(() => {
      expect(mock).toHaveBeenCalledWith(
        `/api/metering-points/${MP_ID}/swap`,
        expect.objectContaining({ method: 'POST' }),
      )
    })
    // Decode body and check it carries the right fields.
    const swapCall = mock.mock.calls.find(
      (c: unknown[]) => c[0] === `/api/metering-points/${MP_ID}/swap`,
    )
    expect(swapCall).toBeTruthy()
    const body = JSON.parse((swapCall![1] as RequestInit).body as string)
    expect(body).toMatchObject({
      incoming_device_id: mockUnboundDevice.id,
      outgoing_reading_r: '12345',
      incoming_initial_n: '0',
    })

    expect(toast.success).toHaveBeenCalledWith(expect.stringContaining(MP_NAME))
  })
})
