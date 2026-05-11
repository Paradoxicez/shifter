import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type React from 'react'
import { createMemoryRouter, RouterProvider } from 'react-router-dom'
import { ApiError } from '@/lib/api'
import { BulkImportDialog } from './bulk-import-dialog'

/**
 * Plan 03-09 Task 3 — BulkImportDialog (3-step).
 *
 * We mock the bulk-import API helpers directly because the upload path uses
 * `fetch` (multipart) rather than the json apiFetch wrapper. The dialog
 * embeds <ResponsiveDialog> which uses Sheet on mobile; we force desktop by
 * setting `window.innerWidth=1280` in beforeEach.
 */

vi.mock('@/lib/imports', async (orig) => {
  const real = await orig<typeof import('@/lib/imports')>()
  return {
    ...real,
    uploadImport: vi.fn(),
    commitImport: vi.fn(),
  }
})

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn() },
}))

function renderDialog(
  props?: Partial<React.ComponentProps<typeof BulkImportDialog>>,
) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const onOpenChange = vi.fn()
  const router = createMemoryRouter(
    [
      {
        path: '/',
        element: (
          <BulkImportDialog open onOpenChange={onOpenChange} {...props} />
        ),
      },
      { path: '/admin/imports/:jobId', element: <div data-testid="job-detail" /> },
    ],
    { initialEntries: ['/'] },
  )
  return {
    qc,
    onOpenChange,
    router,
    ...render(
      <QueryClientProvider client={qc}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    ),
  }
}

describe('BulkImportDialog (Plan 03-09 Task 3)', () => {
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

  it('TestBulkImport_Step1Upload — file picker accepts .xlsx and Download template link present', () => {
    renderDialog()
    expect(
      screen.getByRole('heading', { name: /Upload your device list/i }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /Download template/i }),
    ).toBeInTheDocument()
    const fileInput = screen.getByLabelText(/Choose a file/i) as HTMLInputElement
    expect(fileInput.accept).toMatch(/\.xlsx/)
    expect(fileInput.accept).toMatch(/\.csv/)
  })

  it('TestBulkImport_Step1Upload_NonAcceptedExtension — picking .pdf shows format error inline', async () => {
    renderDialog()
    const fileInput = screen.getByLabelText(/Choose a file/i) as HTMLInputElement
    const pdf = new File(['x'], 'data.pdf', { type: 'application/pdf' })
    // Bypass the input's `accept` attribute so we can assert the dialog's
    // own client-side validation message (jsdom + userEvent.upload otherwise
    // silently drops the file).
    await userEvent.upload(fileInput, pdf, { applyAccept: false })
    expect(
      screen.getByText(/Supported formats: XLSX, CSV/i),
    ).toBeInTheDocument()
  })

  it('TestBulkImport_Step2Preview — upload success renders summary + outcomes', async () => {
    const { uploadImport } = await import('@/lib/imports')
    ;(uploadImport as ReturnType<typeof vi.fn>).mockResolvedValue({
      job_id: 'job-1',
      status: 'preview',
      total: 3,
      valid_count: 1,
      invalid_count: 1,
      already_exists_count: 1,
      expires_at: new Date().toISOString(),
      outcomes_limit: 50,
      outcomes: [
        {
          row_index: 2,
          status: 'valid',
          raw: { dev_eui: '0102030405060708', name: 'meter-1' },
        },
        {
          row_index: 3,
          status: 'invalid',
          reason: 'invalid DevEUI hex',
          raw: { dev_eui: 'xxx', name: 'meter-2' },
        },
        {
          row_index: 4,
          status: 'already_exists',
          reason: 'existing device with same DevEUI',
          raw: { dev_eui: '0a0b0c0d0e0f1011', name: 'meter-3' },
        },
      ],
    })

    renderDialog()

    const fileInput = screen.getByLabelText(/Choose a file/i) as HTMLInputElement
    const xlsx = new File(['x'], 'sheet.xlsx', { type: '' })
    await userEvent.upload(fileInput, xlsx)

    await userEvent.click(screen.getByRole('button', { name: /Validate file/i }))

    await waitFor(() =>
      expect(
        screen.getByRole('heading', { name: /Review what will happen/i }),
      ).toBeInTheDocument(),
    )
    // Summary banner counts visible.
    expect(screen.getByText('Will create')).toBeInTheDocument()
    expect(screen.getByText('Already exists (skip)')).toBeInTheDocument()
    expect(screen.getByText('Errors')).toBeInTheDocument()
    // Outcome rows rendered.
    expect(screen.getByText('meter-1')).toBeInTheDocument()
    expect(screen.getByText('meter-2')).toBeInTheDocument()
    expect(screen.getByText('meter-3')).toBeInTheDocument()
    // Errors XLSX button shown because invalid_count > 0.
    expect(
      screen.getByRole('link', { name: /Download errors\.xlsx/i }),
    ).toBeInTheDocument()
  })

  it('TestBulkImport_Step3Commit_CallsAPI — confirm triggers POST commit and navigates', async () => {
    const { uploadImport, commitImport } = await import('@/lib/imports')
    ;(uploadImport as ReturnType<typeof vi.fn>).mockResolvedValue({
      job_id: 'job-1',
      status: 'preview',
      total: 1,
      valid_count: 1,
      invalid_count: 0,
      already_exists_count: 0,
      expires_at: new Date().toISOString(),
      outcomes_limit: 50,
      outcomes: [
        { row_index: 2, status: 'valid', raw: { dev_eui: 'aa', name: 'm' } },
      ],
    })
    ;(commitImport as ReturnType<typeof vi.fn>).mockResolvedValue({
      job_id: 'job-1',
      total: 1,
      created: 1,
      already_exists: 0,
      failed: 0,
      invalid: 0,
      valid: 1,
      envelope_audit_written: 1,
    })

    const { router } = renderDialog()

    const fileInput = screen.getByLabelText(/Choose a file/i) as HTMLInputElement
    await userEvent.upload(
      fileInput,
      new File(['x'], 'sheet.xlsx', { type: '' }),
    )
    await userEvent.click(screen.getByRole('button', { name: /Validate file/i }))

    await waitFor(() =>
      expect(
        screen.getByRole('heading', { name: /Review what will happen/i }),
      ).toBeInTheDocument(),
    )
    await userEvent.click(
      screen.getByRole('button', { name: /Continue to commit/i }),
    )

    await waitFor(() =>
      expect(
        screen.getByRole('heading', { name: /Commit the import/i }),
      ).toBeInTheDocument(),
    )
    expect(
      screen.getByRole('button', { name: /Import 1 devices/i }),
    ).toBeInTheDocument()

    await userEvent.click(
      screen.getByRole('button', { name: /Import 1 devices/i }),
    )

    await waitFor(() => expect(commitImport).toHaveBeenCalledWith('job-1'))
    await waitFor(() =>
      expect(router.state.location.pathname).toBe('/admin/imports/job-1'),
    )
  })

  it('TestBulkImport_NonUTF8CSV_RejectedWithReadableError — upload error shown inline', async () => {
    const { uploadImport } = await import('@/lib/imports')
    ;(uploadImport as ReturnType<typeof vi.fn>).mockRejectedValue(
      new ApiError(400, 'CSV is not valid UTF-8 — re-save as UTF-8 CSV'),
    )

    renderDialog()
    await userEvent.upload(
      screen.getByLabelText(/Choose a file/i) as HTMLInputElement,
      new File(['x'], 'bad.csv', { type: '' }),
    )
    await userEvent.click(screen.getByRole('button', { name: /Validate file/i }))

    await waitFor(() =>
      expect(screen.getByText(/CSV is not valid UTF-8/i)).toBeInTheDocument(),
    )
  })

  it('TestBulkImport_UX03 — no tenant/application wording', () => {
    const { container } = renderDialog()
    expect(container.textContent ?? '').not.toMatch(/tenant/i)
    expect(container.textContent ?? '').not.toMatch(/application/i)
  })
})
