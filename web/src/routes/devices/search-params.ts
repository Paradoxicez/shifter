import { useCallback } from 'react'
import { useSearchParams } from 'react-router-dom'
import { z } from 'zod'

/**
 * Plan 03-09 — URL state for `/devices`.
 *
 * **CORRECTION per 03-RESEARCH §react-router-dom v7 URL State for Filters:**
 * the CONTEXT D-15 wording referenced TanStack Router. Phase 3 uses
 * `react-router-dom` v7 (Vite SPA, no SSR) — the canonical hook is
 * `useSearchParams()`. We layer zod-based validation on top so invalid URL
 * states fall back to defaults silently (T-3-91 — defense in depth; server
 * validates again).
 *
 * Multi-value pattern: `site=uuid1&site=uuid2` (URLSearchParams.append +
 * getAll). NEVER comma-separated — the backend `listDevices` handler reads
 * `qv["site"]` as a slice and parses each entry as a UUID.
 *
 * Defaults are omitted from the URL when written via `update()` so shareable
 * links stay clean (`/devices` not `/devices?last_seen=all&per_page=50&...`).
 */
export const devicesSearchSchema = z.object({
  site: z.array(z.string().uuid()).optional().default([]),
  status: z.enum(['active', 'inactive', 'never_joined']).optional(),
  last_seen: z.enum(['24h', '7d', '30d', 'all']).optional().default('all'),
  q: z.string().optional().default(''),
  page: z.coerce.number().int().min(1).default(1),
  per_page: z
    .union([z.literal(25), z.literal(50), z.literal(100)])
    .default(50),
  sort: z
    .enum([
      'name',
      '-name',
      'dev_eui',
      '-dev_eui',
      'site',
      '-site',
      'last_seen',
      '-last_seen',
      'created_at',
      '-created_at',
    ])
    .default('-last_seen'),
})

export type DevicesSearch = z.infer<typeof devicesSearchSchema>

const DEFAULTS: DevicesSearch = devicesSearchSchema.parse({})

/**
 * Returns true when `value` equals the schema default for `key`, meaning it
 * should be stripped from the URL.
 */
function isDefault<K extends keyof DevicesSearch>(key: K, value: DevicesSearch[K]): boolean {
  const def = DEFAULTS[key]
  if (Array.isArray(value) && Array.isArray(def)) return value.length === 0
  return value === def
}

/**
 * Parse the current URLSearchParams into a DevicesSearch. Bad values (e.g.
 * `status=banana`, `site=not-a-uuid`) cause a zod safeParse failure and we
 * fall back to defaults — the operator sees a clean filter state rather than
 * an exception.
 */
function parseFromParams(params: URLSearchParams): DevicesSearch {
  const raw = {
    site: params.getAll('site'),
    status: params.get('status') ?? undefined,
    last_seen: params.get('last_seen') ?? undefined,
    q: params.get('q') ?? undefined,
    page: params.get('page') ?? undefined,
    per_page: params.get('per_page') ? Number(params.get('per_page')) : undefined,
    sort: params.get('sort') ?? undefined,
  }
  const parsed = devicesSearchSchema.safeParse(raw)
  return parsed.success ? parsed.data : { ...DEFAULTS }
}

/**
 * `useDevicesSearch` reads + writes URL state for the /devices page.
 *
 * - `search` is always a fully-typed DevicesSearch with defaults applied.
 * - `update(partial)` writes a partial — values equal to the schema default
 *   are removed from the URL (clean shareable URLs); arrays use
 *   URLSearchParams.append per element (multi-value pattern).
 * - Browser back/forward restores prior filter states via react-router's
 *   history integration.
 */
export function useDevicesSearch(): [
  DevicesSearch,
  (next: Partial<DevicesSearch>) => void,
] {
  const [params, setParams] = useSearchParams()
  const search = parseFromParams(params)

  const update = useCallback(
    (next: Partial<DevicesSearch>) => {
      setParams(
        (prev) => {
          const sp = new URLSearchParams(prev)
          for (const [k, v] of Object.entries(next) as [
            keyof DevicesSearch,
            DevicesSearch[keyof DevicesSearch],
          ][]) {
            // Array (site multi-select)
            if (Array.isArray(v)) {
              sp.delete(k)
              if (!isDefault(k, v)) {
                for (const item of v) sp.append(k, item)
              }
              continue
            }
            // Falsy / default → drop the param (clean URL).
            if (v === undefined || v === null || v === '' || isDefault(k, v)) {
              sp.delete(k)
              continue
            }
            sp.set(k, String(v))
          }
          return sp
        },
        { replace: false },
      )
    },
    [setParams],
  )

  return [search, update]
}

/**
 * Helper used by the device list query: maps a DevicesSearch into the exact
 * URLSearchParams the backend expects. Reusable from tests and the page so
 * the wire format is canonical in one place.
 */
export function devicesSearchToQuery(s: DevicesSearch): URLSearchParams {
  const sp = new URLSearchParams()
  for (const id of s.site) sp.append('site', id)
  if (s.status) sp.set('status', s.status)
  if (s.last_seen && s.last_seen !== 'all') sp.set('last_seen', s.last_seen)
  if (s.q) sp.set('q', s.q)
  if (s.page !== 1) sp.set('page', String(s.page))
  if (s.per_page !== 50) sp.set('per_page', String(s.per_page))
  if (s.sort !== '-last_seen') sp.set('sort', s.sort)
  return sp
}
