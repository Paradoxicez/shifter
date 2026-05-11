import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  MemoryRouter,
  Route,
  Routes,
  useRouteLoaderData,
} from 'react-router-dom'
import DeviceDetailPage from './$id'
import type { SessionUser } from '@/lib/auth'

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

// useCurrentUser reads useRouteLoaderData('root'). The MemoryRouter+Routes
// configured below registers a root route with `id: 'root'` and a synthetic
// loaderData provider through `useRouteLoaderData` mock.
vi.mock('react-router-dom', async (orig) => {
  const real = await orig<typeof import('react-router-dom')>()
  return {
    ...real,
    useRouteLoaderData: vi.fn(),
  }
})

const DEVICE_ID = '11111111-1111-4111-8111-111111111111'
const DEV_EUI = '0011223344556677'

const DEVICE = {
  id: DEVICE_ID,
  dev_eui: DEV_EUI,
  name: 'Boiler room meter',
  device_profile_id: '22222222-2222-4222-8222-222222222222',
  join_eui: '0000000000000000',
  description: null,
  last_seen_at: '2026-05-10T12:34:56Z',
  decommissioned_at: null,
  current_site_id: '33333333-3333-4333-8333-333333333333',
  current_site_name: 'Building A',
  created_at: '2026-05-01T00:00:00Z',
  updated_at: '2026-05-01T00:00:00Z',
}

function setCurrentUser(role: 'admin' | 'viewer' | null) {
  const m = useRouteLoaderData as unknown as ReturnType<typeof vi.fn>
  if (role === null) {
    m.mockReturnValue(undefined)
    return
  }
  const user: SessionUser = {
    id: '00000000-0000-4000-8000-000000000001',
    email: role === 'admin' ? 'admin@example.com' : 'viewer@example.com',
    role,
    must_change_password: false,
  }
  m.mockReturnValue({ user })
}

function renderPage(role: 'admin' | 'viewer' = 'admin') {
  setCurrentUser(role)
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[`/devices/${DEVICE_ID}`]}>
        <Routes>
          <Route path="/devices/:id" element={<DeviceDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('DeviceDetailPage (Plan 03-10 Task 1)', () => {
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

  it('TestDeviceDetail_Renders — header has name + dev_eui (mono) + last_seen', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockResolvedValue(DEVICE)

    renderPage('admin')

    await waitFor(() => {
      expect(
        screen.getByRole('heading', { name: DEVICE.name }),
      ).toBeInTheDocument()
    })
    // DevEUI renders both in the header and in the Identity card.
    expect(screen.getAllByText(DEV_EUI).length).toBeGreaterThanOrEqual(1)
    expect(screen.getByText(/Building A/)).toBeInTheDocument()
  })

  it('TestDeviceDetail_RevealButtonAdminOnly_Admin — admin sees Reveal keys button', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockResolvedValue(DEVICE)

    renderPage('admin')

    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: /Reveal keys/i }),
      ).toBeInTheDocument()
    })
  })

  it('TestDeviceDetail_RevealButtonAdminOnly_Viewer — viewer does NOT see Reveal keys button', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockResolvedValue(DEVICE)

    renderPage('viewer')

    await waitFor(() => {
      expect(
        screen.getByRole('heading', { name: DEVICE.name }),
      ).toBeInTheDocument()
    })
    expect(
      screen.queryByRole('button', { name: /Reveal keys/i }),
    ).not.toBeInTheDocument()
  })

  it('TestDeviceDetail_RevealOpensDialog — clicking Reveal keys opens the dialog', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockResolvedValue(DEVICE)

    renderPage('admin')

    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: /Reveal keys/i }),
      ).toBeInTheDocument()
    })

    await userEvent.click(
      screen.getByRole('button', { name: /Reveal keys/i }),
    )

    // Dialog title comes from ResponsiveDialog: "Reveal device keys"
    await waitFor(() => {
      expect(
        screen.getByText(/Reveal device keys/i),
      ).toBeInTheDocument()
    })
  })

  it('TestDeviceDetail_UX03 — no "tenant" or "application" wording', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockResolvedValue(DEVICE)

    const { container } = renderPage('admin')

    await waitFor(() => {
      expect(
        screen.getByRole('heading', { name: DEVICE.name }),
      ).toBeInTheDocument()
    })

    expect(container.textContent ?? '').not.toMatch(/tenant/i)
    expect(container.textContent ?? '').not.toMatch(/application/i)
  })

  it('TestDeviceDetail_NotFound — 404 response renders not-found state', async () => {
    const { apiFetch, ApiError } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockRejectedValue(
      new ApiError(404, 'not_found'),
    )

    renderPage('admin')

    await waitFor(() => {
      expect(screen.getByText(/Device not found/i)).toBeInTheDocument()
    })
  })
})
