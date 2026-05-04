import { apiFetch } from './api'

/**
 * Typed client for the meter-swap commit endpoint (Plan 02-11 backend).
 *
 * Backend handler shape (internal/swap/handlers.go):
 *   POST /api/metering-points/{id}/swap → { binding_id }
 *
 * Decimal fields are TEXT to preserve precision (the backend parses with
 * math/big.Float at 128-bit precision). 409 conflict responses surface as
 * `concurrent_swap` — the SPA renders the inline retry alert per UI-SPEC.
 */

export interface SwapRequest {
  incoming_device_id: string
  outgoing_reading_r: string
  incoming_initial_n?: string
  operator_override?: string
  operator_notes?: string
}

export interface SwapResponse {
  binding_id: string
}

export const commitSwap = (mpId: string, body: SwapRequest) =>
  apiFetch<SwapResponse>(`/api/metering-points/${mpId}/swap`, {
    method: 'POST',
    body: JSON.stringify(body),
  })
