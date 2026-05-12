/**
 * Plan 06-07 — URL-state schema for the /audit page.
 *
 * Mirrors the 06-UI-SPEC §Route Architecture zod shape verbatim so the
 * server-side filter query params and the frontend URL chips stay 1:1.
 *
 * Phase 3 D-15 pattern: useSearchParams + zod with .catch() so a malformed
 * URL falls back to a safe default rather than throwing in the loader.
 *
 * D-33: Default cold-arrival view = last 7 days.
 */

import { useCallback } from 'react'
import { useSearchParams } from 'react-router-dom'
import { z } from 'zod'

function sub7days(d: Date): Date {
  return new Date(d.getTime() - 7 * 24 * 60 * 60 * 1000)
}

export const auditParams = z.object({
  from: z
    .string()
    .datetime({ offset: true })
    .catch(() => sub7days(new Date()).toISOString()),
  to: z
    .string()
    .datetime({ offset: true })
    .catch(() => new Date().toISOString()),
  user_id: z.string().uuid().optional(),
  entity_type: z
    .array(z.string())
    .or(z.string().transform((s) => s.split(',')))
    .catch([]),
  action: z
    .array(z.string())
    .or(z.string().transform((s) => s.split(',')))
    .catch([]),
  request_id: z.string().optional(),
  cursor: z.string().optional(),
})

export type AuditFilters = z.infer<typeof auditParams>

/**
 * useAuditParams returns the parsed (zod-validated) URL search params plus a
 * setter that merges a partial update.
 */
export function useAuditParams(): [AuditFilters, (next: Partial<AuditFilters>) => void] {
  const [sp, setSp] = useSearchParams()
  // Convert URLSearchParams to a plain object, handling multi-value keys.
  const raw: Record<string, string | string[]> = {}
  for (const key of sp.keys()) {
    const vals = sp.getAll(key)
    raw[key] = vals.length === 1 ? vals[0] : vals
  }
  const parsed = auditParams.parse(raw)

  const set = useCallback(
    (next: Partial<AuditFilters>) => {
      const merged = new URLSearchParams(sp)
      for (const [k, v] of Object.entries(next)) {
        merged.delete(k)
        if (Array.isArray(v)) {
          if (v.length > 0) {
            for (const item of v) merged.append(k, item)
          }
        } else if (v !== undefined && v !== '') {
          merged.set(k, String(v))
        }
      }
      setSp(merged, { replace: false })
    },
    [sp, setSp],
  )

  return [parsed, set]
}
