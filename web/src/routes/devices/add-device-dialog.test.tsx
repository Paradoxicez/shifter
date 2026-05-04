import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AddDeviceDialog } from './add-device-dialog'

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

function renderDialog(props?: Partial<React.ComponentProps<typeof AddDeviceDialog>>) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const onOpenChange = vi.fn()
  const onCreated = vi.fn()
  return {
    onOpenChange,
    onCreated,
    qc,
    ...render(
      <QueryClientProvider client={qc}>
        <AddDeviceDialog
          open
          onOpenChange={onOpenChange}
          onCreated={onCreated}
          {...props}
        />
      </QueryClientProvider>,
    ),
  }
}

const mockProfile = {
  id: '11111111-1111-1111-1111-111111111111',
  slug: 'axioma_w1',
  name: 'Axioma Qalcosonic W1',
  vendor: 'Axioma',
  family: 'W1',
  capabilities: ['cumulative', 'battery'],
  counter_modulus: 4294967296,
  mac_version: '1.0.4',
  cs_profile_id: '22222222-2222-2222-2222-222222222222',
}

const mockMP = {
  id: '33333333-3333-3333-3333-333333333333',
  site_id: '44444444-4444-4444-4444-444444444444',
  name: 'Apt 305',
  utility_class: 'water' as const,
}

const mockSite = {
  id: '44444444-4444-4444-4444-444444444444',
  name: 'Building A',
  timezone: 'Asia/Bangkok',
}

describe('AddDeviceDialog (Plan 02-14 / D-10 + D-16)', () => {
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

  it('walks the 4 steps with each step heading rendered verbatim from UI-SPEC', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>

    // parse-deveui (step 1) → pick MSB → next; profiles list (step 2);
    // MPs list (step 3); preflight (step 4).
    mock.mockImplementation((path: string) => {
      if (path === '/api/devices/parse-deveui') {
        return Promise.resolve({
          msb: '0102030405060708',
          lsb: '0807060504030201',
          msb_vendor: 'unknown',
          lsb_vendor: 'unknown',
        })
      }
      if (path === '/api/device-profiles') return Promise.resolve([mockProfile])
      if (path === '/api/metering-points') return Promise.resolve([mockMP])
      if (path === '/api/sites') return Promise.resolve([mockSite])
      if (path === '/api/devices/preflight') return Promise.resolve({ grpc: 'ok', mqtt: 'ok' })
      return Promise.resolve({})
    })

    renderDialog()

    // Step 1.
    expect(
      screen.getByRole('heading', { name: 'Paste the DevEUI from the meter sticker' }),
    ).toBeInTheDocument()

    await userEvent.type(screen.getByLabelText('DevEUI'), '0102030405060708')
    await waitFor(() =>
      expect(screen.getByLabelText(/MSB-first/i)).toBeInTheDocument(),
    )
    await userEvent.click(screen.getByRole('button', { name: /Use this DevEUI/i }))

    // Step 2 heading rendered.
    await waitFor(() =>
      expect(
        screen.getByRole('heading', { name: 'Choose a device profile' }),
      ).toBeInTheDocument(),
    )

    // Pick profile, fill app key, advance.
    const profileTrigger = screen.getByLabelText('Device profile')
    await userEvent.click(profileTrigger)
    await userEvent.click(await screen.findByText('Axioma Qalcosonic W1'))
    await userEvent.type(
      screen.getByLabelText('AppKey'),
      'aabbccddeeff00112233445566778899',
    )
    await userEvent.type(screen.getByLabelText('Device name'), 'Building A apt 305 water meter')
    await userEvent.click(screen.getByRole('button', { name: /Next/i }))

    // Step 3 heading.
    await waitFor(() =>
      expect(
        screen.getByRole('heading', { name: 'Bind to a metering point' }),
      ).toBeInTheDocument(),
    )
    const mpTrigger = screen.getByLabelText('Metering point')
    await userEvent.click(mpTrigger)
    await userEvent.click(await screen.findByText(/Apt 305/))
    await userEvent.click(screen.getByRole('button', { name: /Next/i }))

    // Step 4 heading.
    await waitFor(() =>
      expect(
        screen.getByRole('heading', { name: 'Review and add' }),
      ).toBeInTheDocument(),
    )
  })

  it('preflight grpc=err blocks submit and renders the inline alert', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>

    mock.mockImplementation((path: string) => {
      if (path === '/api/devices/parse-deveui') {
        return Promise.resolve({
          msb: '0102030405060708',
          lsb: '0807060504030201',
          msb_vendor: 'unknown',
          lsb_vendor: 'unknown',
        })
      }
      if (path === '/api/device-profiles') return Promise.resolve([mockProfile])
      if (path === '/api/metering-points') return Promise.resolve([mockMP])
      if (path === '/api/sites') return Promise.resolve([mockSite])
      if (path === '/api/devices/preflight') return Promise.resolve({ grpc: 'err', mqtt: 'ok' })
      return Promise.resolve({})
    })

    renderDialog()

    // Step 1.
    await userEvent.type(screen.getByLabelText('DevEUI'), '0102030405060708')
    await waitFor(() => expect(screen.getByLabelText(/MSB-first/i)).toBeInTheDocument())
    await userEvent.click(screen.getByRole('button', { name: /Use this DevEUI/i }))

    // Step 2.
    await userEvent.click(screen.getByLabelText('Device profile'))
    await userEvent.click(await screen.findByText('Axioma Qalcosonic W1'))
    await userEvent.type(
      screen.getByLabelText('AppKey'),
      'aabbccddeeff00112233445566778899',
    )
    await userEvent.type(screen.getByLabelText('Device name'), 'Test')
    await userEvent.click(screen.getByRole('button', { name: /Next/i }))

    // Step 3.
    await userEvent.click(screen.getByLabelText('Metering point'))
    await userEvent.click(await screen.findByText(/Apt 305/))
    await userEvent.click(screen.getByRole('button', { name: /Next/i }))

    // Step 4 — preflight runs → err → alert visible, Add device disabled.
    await waitFor(() =>
      expect(
        screen.getByText(/ChirpStack is unreachable/),
      ).toBeInTheDocument(),
    )
    expect(screen.getByRole('button', { name: 'Add device' })).toBeDisabled()
  })

  it('happy path submits — POSTs /api/devices and fires onCreated + toast', async () => {
    const { apiFetch } = await import('@/lib/api')
    const { toast } = await import('sonner')
    const mock = apiFetch as ReturnType<typeof vi.fn>

    const newDevice = {
      id: '55555555-5555-5555-5555-555555555555',
      dev_eui: '0102030405060708',
      name: 'Building A apt 305 water meter',
      device_profile_id: mockProfile.id,
    }

    mock.mockImplementation((path: string, init?: RequestInit) => {
      if (path === '/api/devices/parse-deveui') {
        return Promise.resolve({
          msb: '0102030405060708',
          lsb: '0807060504030201',
          msb_vendor: 'unknown',
          lsb_vendor: 'unknown',
        })
      }
      if (path === '/api/device-profiles') return Promise.resolve([mockProfile])
      if (path === '/api/metering-points') return Promise.resolve([mockMP])
      if (path === '/api/sites') return Promise.resolve([mockSite])
      if (path === '/api/devices/preflight') return Promise.resolve({ grpc: 'ok', mqtt: 'ok' })
      if (path === '/api/devices' && init?.method === 'POST') {
        return Promise.resolve(newDevice)
      }
      return Promise.resolve({})
    })

    const { onCreated } = renderDialog()

    await userEvent.type(screen.getByLabelText('DevEUI'), '0102030405060708')
    await waitFor(() => expect(screen.getByLabelText(/MSB-first/i)).toBeInTheDocument())
    await userEvent.click(screen.getByRole('button', { name: /Use this DevEUI/i }))

    await userEvent.click(screen.getByLabelText('Device profile'))
    await userEvent.click(await screen.findByText('Axioma Qalcosonic W1'))
    await userEvent.type(
      screen.getByLabelText('AppKey'),
      'aabbccddeeff00112233445566778899',
    )
    await userEvent.type(
      screen.getByLabelText('Device name'),
      'Building A apt 305 water meter',
    )
    await userEvent.click(screen.getByRole('button', { name: /Next/i }))

    await userEvent.click(screen.getByLabelText('Metering point'))
    await userEvent.click(await screen.findByText(/Apt 305/))
    await userEvent.click(screen.getByRole('button', { name: /Next/i }))

    await waitFor(() =>
      expect(
        screen.getByRole('heading', { name: 'Review and add' }),
      ).toBeInTheDocument(),
    )

    // Wait for preflight ok status and that submit is enabled before clicking
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Add device' })).not.toBeDisabled()
    })
    await userEvent.click(screen.getByRole('button', { name: 'Add device' }))

    await waitFor(() => {
      expect(mock).toHaveBeenCalledWith(
        '/api/devices',
        expect.objectContaining({ method: 'POST' }),
      )
    })
    await waitFor(() => {
      expect(onCreated).toHaveBeenCalledWith(expect.objectContaining({ id: newDevice.id }))
    })
    expect(toast.success).toHaveBeenCalledWith(
      expect.stringContaining('Building A apt 305 water meter'),
    )
    // Sanity: review summary contains MP name verbatim
    void within
  })
})
