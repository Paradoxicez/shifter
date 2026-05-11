import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
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

const DEV_EUI = '0102030405060708'

/**
 * Default apiFetch handler — wires parse-deveui, profiles, MPs, preflight, and
 * a POST /api/devices that returns OTAA echoed-keys.
 */
function defaultFetch(overrides: {
  postResponse?: unknown
  preflight?: { grpc: string; mqtt: string }
} = {}) {
  return (path: string, init?: RequestInit) => {
    if (path === '/api/devices/parse-deveui') {
      return Promise.resolve({
        msb: DEV_EUI,
        lsb: '0807060504030201',
        msb_vendor: 'unknown',
        lsb_vendor: 'unknown',
      })
    }
    if (path === '/api/device-profiles') return Promise.resolve([mockProfile])
    if (path === '/api/metering-points') return Promise.resolve([mockMP])
    if (path === '/api/devices/preflight') {
      return Promise.resolve(overrides.preflight ?? { grpc: 'ok', mqtt: 'ok' })
    }
    if (path === '/api/devices' && init?.method === 'POST') {
      return Promise.resolve(
        overrides.postResponse ?? {
          id: '55555555-5555-5555-5555-555555555555',
          dev_eui: DEV_EUI,
          name: 'Test Device',
          device_profile_id: mockProfile.id,
          activation_mode: 'OTAA',
          app_key: 'aabbccddeeff00112233445566778899',
          nwk_key: 'aabbccddeeff00112233445566778899',
          join_eui: '0000000000000000',
        },
      )
    }
    return Promise.resolve({})
  }
}

/**
 * Walks through steps 1 → 2 → 3 by entering identity + bind defaults. The
 * resulting state has DevEUI picked, name=Test Device, mpId=mockMP.id.
 *
 * Stops at step 3 (Activation).
 */
async function walkToActivationStep() {
  // Step 1: paste DevEUI, pick MSB, enter name.
  await userEvent.type(screen.getByLabelText('DevEUI'), DEV_EUI)
  await waitFor(() => expect(screen.getByLabelText(/MSB-first/i)).toBeInTheDocument())
  await userEvent.click(screen.getByRole('button', { name: /Use this DevEUI/i }))
  await userEvent.type(screen.getByLabelText('Device name'), 'Test Device')
  await userEvent.click(screen.getByRole('button', { name: /Next/i }))

  // Step 2: skip MP binding (optional).
  await waitFor(() =>
    expect(screen.getByRole('heading', { name: 'Bind to a metering point' })).toBeInTheDocument(),
  )
  await userEvent.click(screen.getByRole('button', { name: /Next/i }))

  // Step 3 active.
  await waitFor(() =>
    expect(screen.getByRole('heading', { name: 'Choose activation mode' })).toBeInTheDocument(),
  )
}

describe('AddDeviceDialog — Plan 03-07 (5 steps, OTAA/ABP)', () => {
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

  it('renders 5-step stepper with labels Identity → Bind → Activation → Keys → Review', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockImplementation(defaultFetch())
    renderDialog()

    expect(screen.getByText('Identity')).toBeInTheDocument()
    expect(screen.getByText('Bind')).toBeInTheDocument()
    expect(screen.getByText('Activation')).toBeInTheDocument()
    expect(screen.getByText('Keys')).toBeInTheDocument()
    expect(screen.getByText('Review')).toBeInTheDocument()
  })

  it('Step 3 OTAA is default-selected and ABP carries the "not recommended" sub-text', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockImplementation(defaultFetch())
    renderDialog()
    await walkToActivationStep()

    // OTAA radio is default-checked.
    const otaaRadio = screen.getByRole('radio', { name: /OTAA \(recommended\)/i })
    expect(otaaRadio).toHaveAttribute('aria-checked', 'true')

    // ABP shows "not recommended".
    expect(screen.getByText(/not recommended/i)).toBeInTheDocument()
  })

  it('Step 3 → 4 with ABP selected renders ABP fields; OTAA renders OTAA fields', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockImplementation(defaultFetch())
    renderDialog()
    await walkToActivationStep()

    // Pick a profile so Next is enabled.
    await userEvent.click(screen.getByLabelText('Device profile'))
    await userEvent.click(await screen.findByText('Axioma Qalcosonic W1'))

    // Pick ABP.
    await userEvent.click(screen.getByRole('radio', { name: /^ABP/i }))

    // Advance to Step 4.
    await userEvent.click(screen.getByRole('button', { name: /Next/i }))
    await waitFor(() =>
      expect(screen.getByRole('heading', { name: 'Enter activation keys' })).toBeInTheDocument(),
    )

    // ABP-mode fields present.
    expect(screen.getByLabelText('Dev Addr')).toBeInTheDocument()
    expect(screen.getByLabelText('Network Session Key')).toBeInTheDocument()
    expect(screen.getByLabelText('Application Session Key')).toBeInTheDocument()
    expect(screen.getByLabelText('FCnt Up')).toBeInTheDocument()
    expect(screen.getByLabelText('FCnt Down')).toBeInTheDocument()

    // OTAA-mode fields absent.
    expect(screen.queryByLabelText('AppKey')).not.toBeInTheDocument()
  })

  it('Step 4 OTAA validation — Next disabled until AppKey 32 hex + Join EUI 16 hex', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockImplementation(defaultFetch())
    renderDialog()
    await walkToActivationStep()

    await userEvent.click(screen.getByLabelText('Device profile'))
    await userEvent.click(await screen.findByText('Axioma Qalcosonic W1'))
    await userEvent.click(screen.getByRole('button', { name: /Next/i }))

    await waitFor(() =>
      expect(screen.getByRole('heading', { name: 'Enter activation keys' })).toBeInTheDocument(),
    )

    const nextBtn = screen.getByRole('button', { name: /Next/i })
    expect(nextBtn).toBeDisabled()

    // 31-char AppKey → still disabled.
    await userEvent.type(screen.getByLabelText('AppKey'), 'aabbccddeeff0011223344556677889')
    expect(nextBtn).toBeDisabled()

    // Append one more char → enabled.
    await userEvent.type(screen.getByLabelText('AppKey'), 'a')
    expect(nextBtn).not.toBeDisabled()
  })

  it('Step 4 ABP validation — Next disabled until DevAddr 8 + NwkSKey 32 + AppSKey 32', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockImplementation(defaultFetch())
    renderDialog()
    await walkToActivationStep()

    await userEvent.click(screen.getByLabelText('Device profile'))
    await userEvent.click(await screen.findByText('Axioma Qalcosonic W1'))
    await userEvent.click(screen.getByRole('radio', { name: /^ABP/i }))
    await userEvent.click(screen.getByRole('button', { name: /Next/i }))

    await waitFor(() =>
      expect(screen.getByRole('heading', { name: 'Enter activation keys' })).toBeInTheDocument(),
    )

    const nextBtn = screen.getByRole('button', { name: /Next/i })
    expect(nextBtn).toBeDisabled()

    // Fill the three required hex fields. FCntUp / FCntDown default to 0.
    await userEvent.type(screen.getByLabelText('Dev Addr'), '01020304')
    expect(nextBtn).toBeDisabled() // still missing NwkSKey + AppSKey.
    await userEvent.type(
      screen.getByLabelText('Network Session Key'),
      '0102030405060708090a0b0c0d0e0f10',
    )
    expect(nextBtn).toBeDisabled()
    await userEvent.type(
      screen.getByLabelText('Application Session Key'),
      '1011121314151617181920212223242a',
    )
    expect(nextBtn).not.toBeDisabled()
  })

  it('OTAA happy path → success state shows keys + Copy keys; clipboard.writeText fires JSON blob', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockImplementation(defaultFetch())

    // Mock clipboard.writeText.
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    })

    renderDialog()
    await walkToActivationStep()

    // Profile select + OTAA default.
    await userEvent.click(screen.getByLabelText('Device profile'))
    await userEvent.click(await screen.findByText('Axioma Qalcosonic W1'))
    await userEvent.click(screen.getByRole('button', { name: /Next/i }))

    // Step 4 OTAA — fill AppKey.
    await waitFor(() =>
      expect(screen.getByRole('heading', { name: 'Enter activation keys' })).toBeInTheDocument(),
    )
    await userEvent.type(screen.getByLabelText('AppKey'), 'aabbccddeeff00112233445566778899')
    await userEvent.click(screen.getByRole('button', { name: /Next/i }))

    // Step 5 Review → Add device.
    await waitFor(() =>
      expect(screen.getByRole('heading', { name: 'Review and add' })).toBeInTheDocument(),
    )
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Add device' })).not.toBeDisabled(),
    )
    await userEvent.click(screen.getByRole('button', { name: 'Add device' }))

    // Success state renders.
    await waitFor(() =>
      expect(screen.getByRole('heading', { name: /Device .* added/ })).toBeInTheDocument(),
    )
    expect(screen.getByText('AppKey')).toBeInTheDocument()
    expect(screen.getByText('NwkKey')).toBeInTheDocument()
    expect(screen.getByText('Join EUI')).toBeInTheDocument()

    // Click Copy keys → clipboard.writeText called.
    await userEvent.click(screen.getByRole('button', { name: /Copy keys/i }))
    expect(writeText).toHaveBeenCalledTimes(1)
    const payload = JSON.parse(writeText.mock.calls[0][0] as string)
    expect(payload.Activation).toBe('OTAA')
    expect(payload.AppKey).toBe('aabbccddeeff00112233445566778899')
  })

  it('ABP happy path → success state shows DevAddr/NwkSKey/AppSKey + FCnt counters', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockImplementation(
      defaultFetch({
        postResponse: {
          id: '55555555-5555-5555-5555-555555555555',
          dev_eui: DEV_EUI,
          name: 'ABP Device',
          device_profile_id: mockProfile.id,
          activation_mode: 'ABP',
          dev_addr: '01020304',
          nwk_s_key: '0102030405060708090a0b0c0d0e0f10',
          app_s_key: '1011121314151617181920212223242a',
          f_cnt_up: 0,
          f_cnt_down: 0,
        },
      }),
    )
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText: vi.fn().mockResolvedValue(undefined) },
    })

    renderDialog()
    await walkToActivationStep()

    await userEvent.click(screen.getByLabelText('Device profile'))
    await userEvent.click(await screen.findByText('Axioma Qalcosonic W1'))
    await userEvent.click(screen.getByRole('radio', { name: /^ABP/i }))
    await userEvent.click(screen.getByRole('button', { name: /Next/i }))

    await userEvent.type(screen.getByLabelText('Dev Addr'), '01020304')
    await userEvent.type(
      screen.getByLabelText('Network Session Key'),
      '0102030405060708090a0b0c0d0e0f10',
    )
    await userEvent.type(
      screen.getByLabelText('Application Session Key'),
      '1011121314151617181920212223242a',
    )
    await userEvent.click(screen.getByRole('button', { name: /Next/i }))

    await waitFor(() =>
      expect(screen.getByRole('heading', { name: 'Review and add' })).toBeInTheDocument(),
    )
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Add device' })).not.toBeDisabled(),
    )
    await userEvent.click(screen.getByRole('button', { name: 'Add device' }))

    await waitFor(() =>
      expect(screen.getByRole('heading', { name: /Device .* added/ })).toBeInTheDocument(),
    )

    expect(screen.getByText('Dev Addr')).toBeInTheDocument()
    expect(screen.getByText('NwkSKey')).toBeInTheDocument()
    expect(screen.getByText('AppSKey')).toBeInTheDocument()
    expect(screen.getByText('FCnt Up')).toBeInTheDocument()
    expect(screen.getByText('FCnt Down')).toBeInTheDocument()
  })

  it('Submitted body uses activation_mode=ABP with snake-case fields', async () => {
    const { apiFetch } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>
    mock.mockImplementation(
      defaultFetch({
        postResponse: {
          id: '55555555-5555-5555-5555-555555555555',
          dev_eui: DEV_EUI,
          name: 'ABP Device',
          device_profile_id: mockProfile.id,
          activation_mode: 'ABP',
          dev_addr: '01020304',
          nwk_s_key: '0102030405060708090a0b0c0d0e0f10',
          app_s_key: '1011121314151617181920212223242a',
          f_cnt_up: 0,
          f_cnt_down: 0,
        },
      }),
    )

    renderDialog()
    await walkToActivationStep()

    await userEvent.click(screen.getByLabelText('Device profile'))
    await userEvent.click(await screen.findByText('Axioma Qalcosonic W1'))
    await userEvent.click(screen.getByRole('radio', { name: /^ABP/i }))
    await userEvent.click(screen.getByRole('button', { name: /Next/i }))

    await userEvent.type(screen.getByLabelText('Dev Addr'), '01020304')
    await userEvent.type(
      screen.getByLabelText('Network Session Key'),
      '0102030405060708090a0b0c0d0e0f10',
    )
    await userEvent.type(
      screen.getByLabelText('Application Session Key'),
      '1011121314151617181920212223242a',
    )
    await userEvent.click(screen.getByRole('button', { name: /Next/i }))

    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Add device' })).not.toBeDisabled(),
    )
    await userEvent.click(screen.getByRole('button', { name: 'Add device' }))

    await waitFor(() =>
      expect(mock).toHaveBeenCalledWith(
        '/api/devices',
        expect.objectContaining({ method: 'POST' }),
      ),
    )
    const postCall = mock.mock.calls.find(
      (call) => call[0] === '/api/devices' && (call[1] as RequestInit)?.method === 'POST',
    )
    const body = JSON.parse((postCall![1] as RequestInit).body as string)
    expect(body.activation_mode).toBe('ABP')
    expect(body.dev_addr).toBe('01020304')
    expect(body.nwk_s_key).toBe('0102030405060708090a0b0c0d0e0f10')
    expect(body.app_s_key).toBe('1011121314151617181920212223242a')
    expect(body.fcnt_up).toBe(0)
    expect(body.fcnt_down).toBe(0)
  })

  it('Step 4 OTAA renders Join EUI tooltip label "Join EUI (AppEUI for v1.0)"', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockImplementation(defaultFetch())
    renderDialog()
    await walkToActivationStep()

    await userEvent.click(screen.getByLabelText('Device profile'))
    await userEvent.click(await screen.findByText('Axioma Qalcosonic W1'))
    await userEvent.click(screen.getByRole('button', { name: /Next/i }))

    await waitFor(() =>
      expect(screen.getByRole('heading', { name: 'Enter activation keys' })).toBeInTheDocument(),
    )

    // Label text is verbatim from UI-SPEC §Add Device step 4.
    expect(screen.getByText(/Join EUI \(AppEUI for v1\.0\)/)).toBeInTheDocument()
  })
})
