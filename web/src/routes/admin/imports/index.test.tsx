import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryRouter, RouterProvider } from 'react-router-dom'
import AdminImportsPage from './index'

/**
 * Plan 03-09 Task 3 — Imports list page (`/admin/imports`).
 *
 *   - Renders mocked import jobs (newest first per backend default order).
 *   - Viewer cannot navigate to this page (route-level redirect).
 *   - Each row exposes a link to the detail page.
 */

vi.mock('@/lib/imports', async (orig) => {
  const real = await orig<typeof import('@/lib/imports')>()
  return { ...real, listImportJobs: vi.fn() }
})

function renderPage(user: { role: 'admin' | 'viewer' } = { role: 'admin' }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createMemoryRouter(
    [
      {
        id: 'root',
        path: '/',
        loader: () => ({
          user: { id: 'u', email: 'a@x', must_change_password: false, ...user },
        }),
        children: [
          { path: 'admin/imports', element: <AdminImportsPage /> },
          { path: '', element: <div data-testid="home" /> },
        ],
      },
    ],
    { initialEntries: ['/admin/imports'] },
  )
  return {
    router,
    ...render(
      <QueryClientProvider client={qc}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    ),
  }
}

describe('AdminImportsPage (Plan 03-09)', () => {
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

  it('TestImportsList_Render — mock listImportJobs returns rows; columns shown', async () => {
    const { listImportJobs } = await import('@/lib/imports')
    ;(listImportJobs as ReturnType<typeof vi.fn>).mockResolvedValue({
      total: 2,
      page: 1,
      per_page: 50,
      jobs: [
        {
          id: 'i1',
          job_id: 'cccccccc-1111-4111-8111-111111111111',
          owner_id: 'u',
          file_name: 'meters.xlsx',
          file_format: 'xlsx',
          total_rows: 10,
          status: 'committed',
          valid_count: 8,
          invalid_count: 2,
          already_exists_count: 1,
          created_count: 7,
          failed_count: 0,
          expires_at: null,
          committed_at: '2026-05-10T12:00:00Z',
          created_at: '2026-05-10T11:55:00Z',
          updated_at: '2026-05-10T12:00:00Z',
        },
        {
          id: 'i2',
          job_id: 'dddddddd-2222-4222-8222-222222222222',
          owner_id: 'u',
          file_name: 'devices.csv',
          file_format: 'csv',
          total_rows: 3,
          status: 'preview',
          valid_count: 3,
          invalid_count: 0,
          already_exists_count: 0,
          created_count: 0,
          failed_count: 0,
          expires_at: '2026-05-11T11:55:00Z',
          committed_at: null,
          created_at: '2026-05-11T10:55:00Z',
          updated_at: '2026-05-11T10:55:00Z',
        },
      ],
    })

    renderPage({ role: 'admin' })

    expect(
      await screen.findByRole('heading', { name: /Recent imports/i }),
    ).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.getByText('meters.xlsx')).toBeInTheDocument()
      expect(screen.getByText('devices.csv')).toBeInTheDocument()
    })
    // Status chips visible.
    expect(screen.getByText('committed')).toBeInTheDocument()
    expect(screen.getByText('preview')).toBeInTheDocument()
    // Job ID link uses first 8 chars.
    expect(screen.getByText('cccccccc')).toBeInTheDocument()
    expect(screen.getByText('dddddddd')).toBeInTheDocument()
  })

  it('TestImportsList_AdminOnly — viewer redirects home', async () => {
    const { listImportJobs } = await import('@/lib/imports')
    ;(listImportJobs as ReturnType<typeof vi.fn>).mockResolvedValue({
      total: 0,
      page: 1,
      per_page: 50,
      jobs: [],
    })

    const { router } = renderPage({ role: 'viewer' })
    await waitFor(() =>
      expect(router.state.location.pathname).toBe('/'),
    )
    // Viewer never sees the import job list.
    expect(screen.queryByText('Recent imports')).not.toBeInTheDocument()
  })

  it('TestImportsList_EmptyState — no jobs shows empty CTA', async () => {
    const { listImportJobs } = await import('@/lib/imports')
    ;(listImportJobs as ReturnType<typeof vi.fn>).mockResolvedValue({
      total: 0,
      page: 1,
      per_page: 50,
      jobs: [],
    })

    renderPage({ role: 'admin' })
    expect(await screen.findByText(/No imports yet/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Go to Devices/i })).toBeInTheDocument()
  })

  it('TestImportsList_UX03 — no tenant/application copy', async () => {
    const { listImportJobs } = await import('@/lib/imports')
    ;(listImportJobs as ReturnType<typeof vi.fn>).mockResolvedValue({
      total: 0,
      page: 1,
      per_page: 50,
      jobs: [],
    })

    const { container } = renderPage({ role: 'admin' })
    await screen.findByText(/No imports yet/i)
    expect(container.textContent ?? '').not.toMatch(/tenant/i)
    expect(container.textContent ?? '').not.toMatch(/application/i)
  })
})
