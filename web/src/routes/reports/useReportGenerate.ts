/**
 * useReportGenerate — Plan 05-09 Task 1
 *
 * Wraps the POST /api/reports/generate mutation.
 *
 * Extracted to its own file so index.tsx stays a state-machine glue file
 * rather than mixing hook construction with render logic — matches the
 * companion pattern of useReportPDFStatus.ts.
 *
 * Backend contract (plan 05-06 GenerateHandler):
 *   - Request body: ReportGenerateRequest
 *   - Response: GenerateResponse (includes pdf_status seed for the result panel
 *     to pass into useReportPDFStatus's initialStatus)
 */

import { useMutation } from '@tanstack/react-query'
import type { UseMutationResult } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'

// ---------------------------------------------------------------------------
// Types — exported so index.tsx, ReportConfigPanel, ReportResultPanel can share
// ---------------------------------------------------------------------------

export type ReportGenerateRequest = {
  scope: 'all' | 'site' | 'meter'
  site_id?: string
  mp_id?: string
  group?: 'site' | 'category' | 'none'
  range: 'daily' | 'monthly' | 'yearly' | 'custom'
  start?: string
  end?: string
}

export type GenerateResponse = {
  report_id: string
  report: {
    summary: {
      total_consumption: number
      prior_delta?: { absolute: number; percent: number }
      yoy_delta?: { absolute: number; percent: number }
    }
    period_rows: Array<{
      period: string
      consumption: number
      delta_vs_prior?: { absolute: number; percent: number }
      delta_vs_yoy?: { absolute: number; percent: number }
    }>
    meter_rows: Array<{
      id: string
      name: string
      site_name: string
      utility_class: string
      consumption: number
    }>
  }
  pdf_status: 'pending' | 'running' | 'ready' | 'failed' | 'expired'
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

export function useReportGenerate(): UseMutationResult<GenerateResponse, Error, ReportGenerateRequest> {
  return useMutation<GenerateResponse, Error, ReportGenerateRequest>({
    mutationFn: async (body: ReportGenerateRequest) =>
      apiFetch<GenerateResponse>('/api/reports/generate', {
        method: 'POST',
        body: JSON.stringify(body),
      }),
  })
}
