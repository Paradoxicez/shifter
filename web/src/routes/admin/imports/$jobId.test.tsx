import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryRouter, RouterProvider } from 'react-router-dom'
import ImportJobDetailPage from './$jobId'

/**
 * Plan 03-09 Task 3 — Import job detail page (`/admin/imports/:jobId`).
 *
 *   - Renders job summary + per-row outcomes table.
 *   - "Download errors.xlsx" only enabled when invalid_count + failed_count > 0.
 *   - Expired-job banner when status === 'expired'.
 *   - Admin-only route guard.
 */

vi.mock('@/lib/imports', async (orig) => {
  const real = await orig<typeof import('@/lib/imports')>()
  return { ...real, getImportJob: vi.fn() }
})

function renderDetail({
  jobId = 'cccccccc-1111-4111-8111-111111111111',
  user = { role: 'admin' as 'admin' | 'viewer' },
} = {}) {
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
          { path: 'admin/imports/:jobId', element: <ImportJobDetailPage /> },
          { path: 'admin/imports', element: <div data-testid="imports-list" /> },
          { path: '', element: <div data-testid="home" /> },
        ],
      },
    ],
    { initialEntries: [`/admin/imports/${jobId}`] },
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

const baseJob = {
  id: 'i1',
  job_id: 'cccccccc-1111-4111-8111-111111111111',
  owner_id: 'u',
  file_name: 'meters.xlsx',
  file_format: 'xlsx' as const,
  total_rows: 10,
  valid_count: 8,
  invalid_count: 2,
  already_exists_count: 1,
  created_count: 7,
  failed_count: 0,
  expires_at: null,
  committed_at: '2026-05-10T12:00:00Z',
  created_at: '2026-05-10T11:55:00Z',
  updated_at: '2026-05-10T12:00:00Z',
}

describe('ImportJobDetailPage (Plan 03-09)', () => {
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

  it('TestImportDetail_Render — summary card + rows table', async () => {
    const { getImportJob } = await import('@/lib/imports')
    ;(getImportJob as ReturnType<typeof vi.fn>).mockResolvedValue({
      job: { ...baseJob, status: 'committed' },
      rows: [
        {
          row_index: 2,
          status: 'created',
          raw: { dev_eui: '0102030405060708', name: 'meter-1' },
        },
        {
          row_index: 3,
          status: 'invalid',
          reason: 'invalid DevEUI hex',
          raw: { dev_eui: 'xxx', name: 'meter-2' },
        },
      ],
      total: 2,
      page: 1,
      per_page: 100,
    })

    renderDetail()

    await screen.findByRole('heading', { name: /Import job/i })
    // Short ID in heading (first 8 chars).
    expect(screen.getAllByText('cccccccc').length).toBeGreaterThan(0)
    // Outcome rows rendered.
    await waitFor(() => {
      expect(screen.getByText('meter-1')).toBeInTheDocument()
      expect(screen.getByText('meter-2')).toBeInTheDocument()
    })
    // Summary card big numbers visible.
    expect(screen.getByText('Created')).toBeInTheDocument()
    expect(screen.getByText('Failed')).toBeInTheDocument()
  })

  it('TestImportDetail_DownloadErrorsButton — enabled when invalid_count > 0', async () => {
    const { getImportJob } = await import('@/lib/imports')
    ;(getImportJob as ReturnType<typeof vi.fn>).mockResolvedValue({
      job: { ...baseJob, status: 'committed', invalid_count: 2 },
      rows: [],
      total: 0,
      page: 1,
      per_page: 100,
    })

    renderDetail()
    const link = await screen.findByRole('link', { name: /Download errors\.xlsx/i })
    expect(link).toHaveAttribute(
      'href',
      `/api/imports/${encodeURIComponent(baseJob.job_id)}/errors.xlsx`,
    )
  })

  it('TestImportDetail_ExpiredJob — banner shows when status=expired', async () => {
    const { getImportJob } = await import('@/lib/imports')
    ;(getImportJob as ReturnType<typeof vi.fn>).mockResolvedValue({
      job: { ...baseJob, status: 'expired' },
      rows: [],
      total: 0,
      page: 1,
      per_page: 100,
    })

    renderDetail()
    expect(
      await screen.findByText(/This preview has expired/i),
    ).toBeInTheDocument()
  })

  it('TestImportDetail_ViewerRedirects — viewer cannot view detail', async () => {
    const { getImportJob } = await import('@/lib/imports')
    ;(getImportJob as ReturnType<typeof vi.fn>).mockResolvedValue({
      job: baseJob,
      rows: [],
      total: 0,
      page: 1,
      per_page: 100,
    })

    const { router } = renderDetail({ user: { role: 'viewer' } })
    await waitFor(() => expect(router.state.location.pathname).toBe('/'))
  })

  it('TestImportDetail_UX03 — no tenant/application copy', async () => {
    const { getImportJob } = await import('@/lib/imports')
    ;(getImportJob as ReturnType<typeof vi.fn>).mockResolvedValue({
      job: { ...baseJob, status: 'committed' },
      rows: [],
      total: 0,
      page: 1,
      per_page: 100,
    })

    const { container } = renderDetail()
    await screen.findByRole('heading', { name: /Import job/i })
    expect(container.textContent ?? '').not.toMatch(/tenant/i)
    expect(container.textContent ?? '').not.toMatch(/application/i)
  })
})
