/**
 * Plan 06-04 — URL-state schema for the /alerts page.
 *
 * Mirrors the 06-UI-SPEC §Route Architecture zod shape verbatim so the
 * server-side filter query params and the frontend URL chips stay 1:1.
 *
 * Phase 3 D-15 pattern: useSearchParams + zod with .catch() so a malformed
 * URL falls back to a safe default rather than throwing in the loader.
 */

import { useCallback } from 'react'
import { useSearchParams } from 'react-router-dom'
import { z } from 'zod'

export const alertsParams = z.object({
  severity: z.enum(['critical', 'warning', 'info', 'all']).catch('all'),
  status: z.enum(['open', 'firing', 'acknowledged', 'snoozed', 'cleared', 'all']).catch('open'),
  category: z.enum(['threshold', 'offline', 'anomaly', 'all']).catch('all'),
  target_type: z.enum(['metering_point', 'site', 'gateway', 'all']).catch('all'),
  from: z.string().optional(),
  to: z.string().optional(),
})

export type AlertsParams = z.infer<typeof alertsParams>

/**
 * useAlertParams returns the parsed (zod-validated) URL search params plus a
 * setter that merges a partial update. Mirrors web/src/lib/use-search-params
 * patterns used by /devices and /reports in earlier phases.
 */
export function useAlertParams(): [AlertsParams, (next: Partial<AlertsParams>) => void] {
  const [sp, setSp] = useSearchParams()
  const raw = Object.fromEntries(sp.entries())
  const parsed = alertsParams.parse(raw)

  const set = useCallback(
    (next: Partial<AlertsParams>) => {
      const merged: Record<string, string> = { ...raw }
      for (const [k, v] of Object.entries(next)) {
        if (v === undefined || v === 'all') {
          delete merged[k]
        } else {
          merged[k] = String(v)
        }
      }
      setSp(merged, { replace: false })
    },
    [raw, setSp],
  )

  return [parsed, set]
}
