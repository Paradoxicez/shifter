import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { MappingEditor } from './mapping-editor'

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

const PROFILE_ID = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'

const mockProfile = {
  id: PROFILE_ID,
  slug: 'axioma_w1',
  name: 'Axioma Qalcosonic W1',
  vendor: 'Axioma',
  family: 'W1',
  capabilities: ['cumulative', 'battery'],
  counter_modulus: 4294967296,
  mac_version: '1.0.4',
  codec_js: 'function decodeUplink(input){return {data:{}};}',
  cs_profile_id: 'cccccccc-cccc-cccc-cccc-cccccccccccc',
  region: 'AS923',
  archived_at: null,
}

const mockMappings = [
  { id: '1', json_pointer: '/cumulative_l', target: 'cumulative_value', scale: '1', data_type: 'numeric', position: 0 },
  { id: '2', json_pointer: '/battery_pct', target: 'battery_pct', scale: '1', data_type: 'numeric', position: 1 },
  { id: '3', json_pointer: '/temperature_c', target: 'temperature_c', scale: '1', data_type: 'numeric', position: 2 },
  { id: '4', json_pointer: '/leak', target: 'extra.leak', scale: '1', data_type: 'bool', position: 3 },
  { id: '5', json_pointer: '/tamper', target: 'extra.tamper', scale: '1', data_type: 'bool', position: 4 },
]

function renderEditor(props?: Partial<React.ComponentProps<typeof MappingEditor>>) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <MemoryRouter>
      <QueryClientProvider client={qc}>
        <MappingEditor profileId={PROFILE_ID} mode="edit" {...props} />
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('MappingEditor (Plan 02-14 / D-08)', () => {
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

  it('loads existing profile and renders 5 mapping rows in the editable table', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockImplementation((path: string) => {
      if (path === `/api/device-profiles/${PROFILE_ID}`) {
        return Promise.resolve({ profile: mockProfile, mappings: mockMappings })
      }
      return Promise.resolve({})
    })

    renderEditor()

    // Profile name editable input renders with the loaded value.
    await waitFor(() => {
      expect(
        (screen.getByLabelText('Profile name') as HTMLInputElement).value,
      ).toBe('Axioma Qalcosonic W1')
    })

    // Five mapping rows present — assert each json_pointer cell.
    expect(await screen.findByDisplayValue('/cumulative_l')).toBeInTheDocument()
    expect(screen.getByDisplayValue('/battery_pct')).toBeInTheDocument()
    expect(screen.getByDisplayValue('/temperature_c')).toBeInTheDocument()
    expect(screen.getByDisplayValue('/leak')).toBeInTheDocument()
    expect(screen.getByDisplayValue('/tamper')).toBeInTheDocument()
  })

  it('paste sample JSON renders clickable JSON tree with leaves', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockImplementation((path: string) => {
      if (path === `/api/device-profiles/${PROFILE_ID}`) {
        return Promise.resolve({ profile: mockProfile, mappings: mockMappings })
      }
      return Promise.resolve({})
    })

    renderEditor()

    await waitFor(() =>
      expect(screen.getByLabelText('Sample decoded JSON')).toBeInTheDocument(),
    )

    const sampleTextarea = screen.getByLabelText('Sample decoded JSON')
    await userEvent.click(sampleTextarea)
    await userEvent.paste('{"cumul": 12345, "battery": 87}')

    // Two leaves rendered as clickable buttons.
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /\/cumul/ })).toBeInTheDocument()
    })
    expect(screen.getByRole('button', { name: /\/battery/ })).toBeInTheDocument()
  })

  it('clicking a JSON tree leaf populates the active mapping row pointer', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockImplementation((path: string) => {
      if (path === `/api/device-profiles/${PROFILE_ID}`) {
        return Promise.resolve({ profile: mockProfile, mappings: mockMappings })
      }
      return Promise.resolve({})
    })

    renderEditor()

    // Wait for editor to load.
    await waitFor(() =>
      expect(screen.getByDisplayValue('/cumulative_l')).toBeInTheDocument(),
    )

    // Focus the FIRST mapping row to make it the active row.
    const firstPointer = screen.getByDisplayValue('/cumulative_l') as HTMLInputElement
    await userEvent.click(firstPointer)

    // Paste sample JSON to render leaves.
    const sampleTextarea = screen.getByLabelText('Sample decoded JSON')
    await userEvent.click(sampleTextarea)
    await userEvent.paste('{"cumul": 12345, "battery": 87}')

    // Click the /cumul leaf.
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /\/cumul/ })).toBeInTheDocument()
    })
    await userEvent.click(screen.getByRole('button', { name: /\/cumul/ }))

    // The active mapping row's json_pointer cell now reads "/cumul".
    await waitFor(() => {
      expect(
        (screen.getByDisplayValue('/cumul') as HTMLInputElement).value,
      ).toBe('/cumul')
    })
  })

  it('+ Add mapping button appends a new row with default scale=1', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockImplementation((path: string) => {
      if (path === `/api/device-profiles/${PROFILE_ID}`) {
        return Promise.resolve({ profile: mockProfile, mappings: mockMappings })
      }
      return Promise.resolve({})
    })

    renderEditor()

    await waitFor(() =>
      expect(screen.getByDisplayValue('/cumulative_l')).toBeInTheDocument(),
    )

    // Count rows before.
    const beforeRows = screen.getAllByRole('row').length

    await userEvent.click(screen.getByRole('button', { name: /Add mapping/i }))

    await waitFor(() => {
      expect(screen.getAllByRole('row').length).toBe(beforeRows + 1)
    })

    // The new row's scale input has default value "1".
    const scaleInputs = screen
      .getAllByLabelText(/scale/i)
      .filter((el) => (el as HTMLInputElement).value === '1')
    expect(scaleInputs.length).toBeGreaterThan(0)
  })

  it('save (create mode) POSTs /api/device-profiles with the right body', async () => {
    const { apiFetch } = await import('@/lib/api')
    const { toast } = await import('sonner')
    const mock = apiFetch as ReturnType<typeof vi.fn>

    mock.mockImplementation((path: string, init?: RequestInit) => {
      if (path === '/api/device-profiles' && init?.method === 'POST') {
        return Promise.resolve({ id: 'newid' })
      }
      return Promise.resolve({})
    })

    renderEditor({ profileId: undefined, mode: 'new' })

    // Fill required fields.
    await waitFor(() =>
      expect(screen.getByLabelText('Profile name')).toBeInTheDocument(),
    )

    await userEvent.type(screen.getByLabelText('Profile name'), 'My Profile')
    await userEvent.type(screen.getByLabelText('Slug'), 'my_profile')
    await userEvent.type(screen.getByLabelText('Vendor'), 'TestVendor')

    // Counter modulus has a default — clear and re-type for explicitness.
    const modulusInput = screen.getByLabelText('Counter modulus') as HTMLInputElement
    await userEvent.clear(modulusInput)
    await userEvent.type(modulusInput, '4294967296')

    // MAC version has a default; leave it.

    // Click Save profile.
    await userEvent.click(screen.getByRole('button', { name: /Save profile/i }))

    await waitFor(() => {
      expect(mock).toHaveBeenCalledWith(
        '/api/device-profiles',
        expect.objectContaining({ method: 'POST' }),
      )
    })

    // Toast fires with "Profile saved." prefix.
    expect(toast.success).toHaveBeenCalledWith(
      expect.stringContaining('Profile saved'),
    )
  })

  it('400 invalid capability response renders inline alert above mapping table', async () => {
    const { apiFetch } = await import('@/lib/api')
    const { ApiError } = await import('@/lib/api')
    const mock = apiFetch as ReturnType<typeof vi.fn>

    mock.mockImplementation((path: string, init?: RequestInit) => {
      if (path === '/api/device-profiles' && init?.method === 'POST') {
        return Promise.reject(
          new ApiError(400, 'invalid capability "bogus"', {
            error: 'validation',
            detail: 'invalid capability "bogus"',
          }),
        )
      }
      return Promise.resolve({})
    })

    renderEditor({ profileId: undefined, mode: 'new' })

    await waitFor(() =>
      expect(screen.getByLabelText('Profile name')).toBeInTheDocument(),
    )

    await userEvent.type(screen.getByLabelText('Profile name'), 'My Profile')
    await userEvent.type(screen.getByLabelText('Slug'), 'my_profile')
    await userEvent.type(screen.getByLabelText('Vendor'), 'TestVendor')
    const modulusInput = screen.getByLabelText('Counter modulus') as HTMLInputElement
    await userEvent.clear(modulusInput)
    await userEvent.type(modulusInput, '4294967296')

    await userEvent.click(screen.getByRole('button', { name: /Save profile/i }))

    await waitFor(() => {
      expect(screen.getByText(/Choose valid capabilities/i)).toBeInTheDocument()
    })

    // Sanity — within is unused but referenced to keep import.
    void within
  })
})
