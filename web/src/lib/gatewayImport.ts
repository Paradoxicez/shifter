/**
 * Plan 07-13 — Bulk gateway import API client.
 *
 * Endpoints:
 *   POST /api/gateways/bulk-import/validate  → ValidateResult
 *   POST /api/gateways/bulk-import/commit    → CommitResult
 *   GET  /api/gateways/bulk-import/template  → CSV download (handled via anchor href)
 */

import { ApiError } from './api'

export interface ValidateRowError {
  row_number: number
  gateway_eui: string
  outcome: string
  error_message?: string
}

export interface ValidateResult {
  valid_rows: number
  error_rows: number
  errors: ValidateRowError[]
}

export interface CommitRowOutcome {
  row_number: number
  gateway_eui: string
  outcome: 'created' | 'updated' | 'skipped' | 'error'
  error_message?: string
}

export interface CommitResult {
  created: number
  updated: number
  skipped: number
  outcomes: CommitRowOutcome[]
}

const GATEWAY_IMPORT_MAX_BYTES = 5 << 20 // 5 MiB

/**
 * validateGatewayCSV — POST /api/gateways/bulk-import/validate
 * Sends the CSV file as multipart/form-data and returns the ValidateResult.
 * Throws ApiError on server error.
 */
export async function validateGatewayCSV(file: File): Promise<ValidateResult> {
  const form = new FormData()
  form.append('file', file)

  const res = await fetch('/api/gateways/bulk-import/validate', {
    method: 'POST',
    body: form,
    credentials: 'same-origin',
    headers: { 'X-Requested-With': 'shifter' },
  })

  const text = await res.text()
  const body = text ? JSON.parse(text) : null

  if (!res.ok) {
    const message =
      body && typeof body === 'object' && 'error' in body && typeof body.error === 'string'
        ? body.error
        : `${res.status} ${res.statusText}`
    throw new ApiError(res.status, message, body)
  }

  return body as ValidateResult
}

/**
 * commitGatewayCSV — POST /api/gateways/bulk-import/commit
 * Sends the CSV file as multipart/form-data and returns the CommitResult.
 * Throws ApiError on server error.
 */
export async function commitGatewayCSV(file: File): Promise<CommitResult> {
  const form = new FormData()
  form.append('file', file)

  const res = await fetch('/api/gateways/bulk-import/commit', {
    method: 'POST',
    body: form,
    credentials: 'same-origin',
    headers: { 'X-Requested-With': 'shifter' },
  })

  const text = await res.text()
  const body = text ? JSON.parse(text) : null

  if (!res.ok) {
    const message =
      body && typeof body === 'object' && 'error' in body && typeof body.error === 'string'
        ? body.error
        : `${res.status} ${res.statusText}`
    throw new ApiError(res.status, message, body)
  }

  return body as CommitResult
}

/**
 * gatewayImportTemplateURL — returns the URL for the CSV template download.
 * Used as the `href` on the Download CSV template anchor.
 */
export function gatewayImportTemplateURL(): string {
  return '/api/gateways/bulk-import/template'
}

export { GATEWAY_IMPORT_MAX_BYTES, ApiError }
