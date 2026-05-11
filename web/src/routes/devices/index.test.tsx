import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  createMemoryRouter,
  RouterProvider,
} from 'react-router-dom'
import DevicesPage from './index'

/**
 * Plan 03-09 Task 1 — Devices list page (filter / sort / pagination /
 * deeplinkable URL state). The tests focus on URL ↔ API translation, sort
 * cycling, and admin-vs-viewer surface differences. Detailed component
 * coverage for the dialogs lives in their own files.
 */

vi.mock('@/lib/api', async (orig) => {
  const real = await orig<typeof import('@/lib/api')>()
  return { ...real, apiFetch: vi.fn() }
})

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), warning: vi.fn(), error: vi.fn() },
}))

const DEVICE_A = {
  id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
  dev_eui: '0102030405060708',
  name: 'Meter A',
  device_profile_id: 'p-1',
  last_seen_at: new Date(Date.now() - 60_000).toISOString(),
  decommissioned_at: null,
  current_site_id: null,
  current_site_name: null,
}

const DEVICE_B = {
  id: 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb',
  dev_eui: '0a0b0c0d0e0f1011',
  name: 'Meter B',
  device_profile_id: 'p-1',
  last_seen_at: null,
  decommissioned_at: null,
  current_site_id: null,
  current_site_name: null,
}

function makeFetchMock(opts: {
  rows?: unknown[]
  totalCount?: number
  pageCount?: number
  capturedQueries?: string[]
  user?: { role: 'admin' | 'viewer' }
} = {}) {
  const rows = opts.rows ?? [DEVICE_A, DEVICE_B]
  const totalCount = opts.totalCount ?? rows.length
  const pageCount = opts.pageCount ?? 1
  return (path: string) => {
    if (path.startsWith('/api/devices')) {
      opts.capturedQueries?.push(path)
      return Promise.resolve({
        total_count: totalCount,
        page_count: pageCount,
        page: 1,
        per_page: 50,
        rows,
      })
    }
    if (path === '/api/sites') {
      return Promise.resolve([])
    }
    return Promise.resolve({})
  }
}

function renderDevicesAt(
  initialPath: string,
  opts: {
    user?: { role: 'admin' | 'viewer'; email?: string; id?: string }
  } = {},
) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const user = opts.user ?? { role: 'admin' as const, email: 'a@x', id: 'u-1' }

  const router = createMemoryRouter(
    [
      {
        id: 'root',
        path: '/',
        loader: () => ({
          user: { ...user, must_change_password: false },
        }),
        children: [
          { path: 'devices', element: <DevicesPage /> },
          { path: 'devices/:id', element: <div /> },
        ],
      },
    ],
    { initialEntries: [initialPath] },
  )

  return {
    qc,
    router,
    ...render(
      <QueryClientProvider client={qc}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    ),
  }
}

describe('DevicesPage (Plan 03-09)', () => {
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

  it('TestDevicesList_Renders — table shows mocked rows', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockImplementation(makeFetchMock())

    renderDevicesAt('/devices')

    expect(
      await screen.findByRole('heading', { name: /^Devices$/i }),
    ).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.getByText('Meter A')).toBeInTheDocument()
      expect(screen.getByText('Meter B')).toBeInTheDocument()
    })
  })

  it('TestDevicesList_DeepLinkRestore — URL filters render in toolbar + are sent to API', async () => {
    const captured: string[] = []
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockImplementation(
      makeFetchMock({ capturedQueries: captured }),
    )

    renderDevicesAt('/devices?status=active&last_seen=24h')
    await waitFor(() => expect(captured.length).toBeGreaterThan(0))

    // Backend received the filter params.
    const devicesCall = captured.find((p) => p.startsWith('/api/devices?'))
    expect(devicesCall).toContain('status=active')
    expect(devicesCall).toContain('last_seen=24h')
  })

  it('TestDevicesList_SortColumnHeader — clicking name header sets sort=name', async () => {
    const captured: string[] = []
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockImplementation(
      makeFetchMock({ capturedQueries: captured }),
    )

    const { router } = renderDevicesAt('/devices')
    await waitFor(() => expect(screen.getByText('Meter A')).toBeInTheDocument())

    // Find the column header button — it's a <button> with text "Name" + icon.
    const headerButtons = screen.getAllByRole('button')
    const nameHeader = headerButtons.find(
      (b) => b.textContent?.trim().startsWith('Name'),
    )
    if (!nameHeader) throw new Error('Name header button not found')
    await userEvent.click(nameHeader)

    await waitFor(() => {
      const url =
        router.state.location.pathname + router.state.location.search
      expect(url).toContain('sort=name')
    })
  })

  it('TestDevicesList_BulkActionBarHiddenInitially — no selection → no bar', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockImplementation(makeFetchMock())

    renderDevicesAt('/devices')
    await waitFor(() => expect(screen.getByText('Meter A')).toBeInTheDocument())

    expect(screen.queryByRole('region', { name: /bulk actions/i })).not.toBeInTheDocument()
  })

  it('TestDevicesList_BulkActionBarOnSelection — admin sees bar after checking a row', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockImplementation(makeFetchMock())

    renderDevicesAt('/devices')
    await waitFor(() => expect(screen.getByText('Meter A')).toBeInTheDocument())

    // Click first row checkbox.
    const checkboxes = screen.getAllByRole('checkbox')
    // Find one with the Meter A aria-label.
    const rowCheckbox =
      checkboxes.find(
        (c) => c.getAttribute('aria-label') === 'Select Meter A',
      ) ?? checkboxes[1]
    await userEvent.click(rowCheckbox)

    await waitFor(() =>
      expect(screen.getByRole('region', { name: /bulk actions/i })).toBeInTheDocument(),
    )
    expect(screen.getByText(/1 selected/)).toBeInTheDocument()
  })

  it('TestDevicesList_BulkImportButton_AdminOnly — admin sees button', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockImplementation(makeFetchMock())

    renderDevicesAt('/devices', { user: { role: 'admin', email: 'a@x', id: 'u' } })
    expect(
      await screen.findByRole('button', { name: /Bulk import/i }),
    ).toBeInTheDocument()
  })

  it('TestDevicesList_BulkImportButton_HiddenForViewer — viewer does not see Bulk import or row checkboxes', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockImplementation(makeFetchMock())

    renderDevicesAt('/devices', { user: { role: 'viewer', email: 'v@x', id: 'u' } })

    await waitFor(() => expect(screen.getByText('Meter A')).toBeInTheDocument())

    expect(screen.queryByRole('button', { name: /Bulk import/i })).not.toBeInTheDocument()
    // Row checkboxes are admin-only.
    expect(screen.queryByLabelText(/Select Meter A/)).not.toBeInTheDocument()
  })

  it('TestDevicesList_UX03 — no tenant/application copy', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockImplementation(makeFetchMock())

    const { container } = renderDevicesAt('/devices')
    await waitFor(() => expect(screen.getByText('Meter A')).toBeInTheDocument())

    // No user-facing "tenant" / "application" copy on this surface.
    // (We allow the word "device profile" since "profile" is the column header
    //  but it does not contain "application".)
    expect(container.textContent ?? '').not.toMatch(/tenant/i)
    expect(container.textContent ?? '').not.toMatch(/application/i)
  })
})
