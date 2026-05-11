import { ApiError, apiFetch } from './api'

/**
 * Plan 03-09 — typed client for /api/imports/* (Plan 03-05 backend).
 *
 * Backend handler shape (internal/import/handlers.go):
 *   POST /api/imports                        → UploadResponse  (multipart)
 *   POST /api/imports/{job_id}/commit        → CommitSummary
 *   GET  /api/imports                        → { jobs, total, page, per_page }
 *   GET  /api/imports/{job_id}               → { job, rows, total, page, per_page }
 *   GET  /api/imports/template.xlsx          → XLSX bytes
 *   GET  /api/imports/{job_id}/errors.xlsx   → XLSX bytes
 *
 * All endpoints are admin-only (auth.RequireAction(ActionDeviceBulkImport)).
 * UX-03: no "tenant" / "application" wording in any client-side string.
 */

export type ImportJobStatus =
  | 'preview'
  | 'committed'
  | 'committing'
  | 'expired'
  | 'failed'

export type ImportRowStatus =
  | 'valid'
  | 'invalid'
  | 'already_exists'
  | 'created'
  | 'failed'

export interface ImportJob {
  id: string
  job_id: string
  owner_id: string
  file_name: string
  file_format: 'xlsx' | 'csv'
  total_rows: number
  status: ImportJobStatus
  valid_count: number
  invalid_count: number
  already_exists_count: number
  created_count: number
  failed_count: number
  expires_at: string | null
  committed_at: string | null
  created_at: string | null
  updated_at: string | null
  failed_reason?: string
}

export interface ImportJobRow {
  row_index: number
  status: ImportRowStatus
  reason?: string
  created_device_id?: string
  raw?: Record<string, unknown>
}

export interface UploadResponse {
  job_id: string
  status: ImportJobStatus
  total: number
  valid_count: number
  invalid_count: number
  already_exists_count: number
  expires_at: string
  outcomes: ImportJobRow[]
  outcomes_limit: number
}

export interface CommitSummary {
  job_id: string
  total: number
  created: number
  already_exists: number
  failed: number
  invalid: number
  valid: number
  envelope_audit_written: number
}

export interface ListImportJobsResponse {
  jobs: ImportJob[]
  total: number
  page: number
  per_page: number
}

export interface GetImportJobResponse {
  job: ImportJob
  rows: ImportJobRow[]
  total: number
  page: number
  per_page: number
}

/**
 * Upload an XLSX/CSV for dry-run validation. Wraps `apiFetch` for the 401
 * redirect + CSRF header, but uses FormData (Content-Type set by the browser).
 */
export async function uploadImport(file: File): Promise<UploadResponse> {
  const form = new FormData()
  form.append('file', file)
  // Use fetch directly so multipart Content-Type boundary is set by browser,
  // but mirror apiFetch's CSRF header + credentials + 401 redirect.
  const res = await fetch('/api/imports', {
    method: 'POST',
    body: form,
    credentials: 'same-origin',
    headers: { Accept: 'application/json', 'X-Requested-With': 'shifter' },
  })
  const text = await res.text()
  const body = text ? JSON.parse(text) : null
  if (!res.ok) {
    if (res.status === 401) {
      window.location.assign(
        `/login?next=${encodeURIComponent(window.location.pathname)}`,
      )
    }
    const message =
      (body && typeof body === 'object' && 'error' in body
        ? String(
            (body as { detail?: string; error?: string }).detail ??
              (body as { error?: string }).error,
          )
        : null) ?? `${res.status} ${res.statusText}`
    throw new ApiError(res.status, message, body)
  }
  return body as UploadResponse
}

export async function commitImport(jobID: string): Promise<CommitSummary> {
  return apiFetch<CommitSummary>(
    `/api/imports/${encodeURIComponent(jobID)}/commit`,
    { method: 'POST', body: '{}' },
  )
}

export async function listImportJobs(
  opts: { page?: number; per_page?: number } = {},
): Promise<ListImportJobsResponse> {
  const sp = new URLSearchParams()
  if (opts.page !== undefined) sp.set('page', String(opts.page))
  if (opts.per_page !== undefined) sp.set('per_page', String(opts.per_page))
  const qs = sp.toString()
  return apiFetch<ListImportJobsResponse>(
    qs ? `/api/imports?${qs}` : '/api/imports',
  )
}

export async function getImportJob(
  jobID: string,
  opts: { page?: number; per_page?: number } = {},
): Promise<GetImportJobResponse> {
  const sp = new URLSearchParams()
  if (opts.page !== undefined) sp.set('page', String(opts.page))
  if (opts.per_page !== undefined) sp.set('per_page', String(opts.per_page))
  const qs = sp.toString()
  return apiFetch<GetImportJobResponse>(
    qs
      ? `/api/imports/${encodeURIComponent(jobID)}?${qs}`
      : `/api/imports/${encodeURIComponent(jobID)}`,
  )
}

export function templateDownloadURL(): string {
  return '/api/imports/template.xlsx'
}

export function errorsXLSXDownloadURL(jobID: string): string {
  return `/api/imports/${encodeURIComponent(jobID)}/errors.xlsx`
}
