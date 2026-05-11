import { apiFetch } from './api'

/**
 * Typed client for /api/devices/* endpoints (Plan 02-10 backend).
 *
 * Backend handler shape (internal/device/handlers.go):
 *   GET   /api/devices                      → Device[]
 *   GET   /api/devices/search?q=…           → Device[]
 *   GET   /api/devices/by-site/{siteID}     → Device[]
 *   GET   /api/devices/{id}                 → Device
 *   POST  /api/devices/preflight            → PreflightResult (admin only)
 *   POST  /api/devices/parse-deveui         → DevEUIPreview (admin + viewer)
 *   POST  /api/devices                      → Device (admin only — CHIRP-04 atomic)
 *   POST  /api/devices/{id}/decommission    → Device (admin only — D-15)
 *
 * AppKey policy (DEV-09): the request body accepts AppKey; the backend
 * forwards it to ChirpStack via CreateDeviceKeys and Shifter NEVER persists
 * it in any local table.
 */

export interface Device {
  id: string
  dev_eui: string
  name: string
  device_profile_id: string
  join_eui?: string | null
  description?: string | null
  last_seen_at?: string | null
  decommissioned_at?: string | null
  created_at?: string
  updated_at?: string
  /** Phase 3 listDevices envelope adds these (active binding context). */
  current_site_id?: string | null
  current_site_name?: string | null
}

/**
 * Plan 03-09 — filtered list response shape (Plan 03-06 backend envelope).
 *
 *   GET /api/devices?site=&status=&last_seen=&q=&sort=&page=&per_page=
 *     → { total_count, page_count, page, per_page, rows: Device[] }
 */
export interface ListDevicesEnvelope {
  total_count: number
  page_count: number
  page: number
  per_page: number
  rows: Device[]
}

export interface ListDevicesFilters {
  site?: string[]
  status?: 'active' | 'inactive' | 'never_joined'
  last_seen?: '24h' | '7d' | '30d' | 'all'
  q?: string
  page?: number
  per_page?: 25 | 50 | 100
  sort?:
    | 'name' | '-name'
    | 'dev_eui' | '-dev_eui'
    | 'site' | '-site'
    | 'last_seen' | '-last_seen'
    | 'created_at' | '-created_at'
}

export interface DevEUIPreview {
  msb: string
  lsb: string
  msb_vendor: string
  lsb_vendor: string
}

export interface PreflightResult {
  grpc: 'ok' | 'err' | 'skipped'
  mqtt: 'ok' | 'err' | 'skipped'
  grpc_detail?: string
  mqtt_detail?: string
}

/**
 * Plan 03-07 D-19..D-25: discriminated-union add-device payload.
 *
 *   - OTAA branch — operator provides AppKey + (optional) JoinEUI;
 *     backend → CS CreateDevice + CreateDeviceKeys. Phase 2 baseline.
 *   - ABP branch — operator provides DevAddr + NwkSKey + AppSKey +
 *     (optional) FCntUp / FCntDown; backend → CS CreateDevice +
 *     ActivateDevice.
 *
 * DEV-09: keys flow client → Shifter API → ChirpStack. They are NEVER
 * persisted in Shifter PG and never appear in any list/detail response.
 * The POST response body echoes them back ONCE for the dialog's D-21
 * success state ("Copy keys" panel); after Done is clicked the local
 * React state is dropped and the mutation cache is configured with
 * gcTime:0 so TanStack Query never retains the response.
 */
export interface AddDeviceBase {
  dev_eui: string
  name: string
  device_profile_id: string
  description?: string
  metering_point_id?: string
  initial_reading?: string
}

export interface AddDeviceOTAA extends AddDeviceBase {
  activation_mode: 'OTAA'
  app_key: string
  join_eui?: string
  /** Optional in the v1.0.x flow; backend defaults NwkKey to AppKey. */
  nwk_key?: string
}

export interface AddDeviceABP extends AddDeviceBase {
  activation_mode: 'ABP'
  dev_addr: string
  nwk_s_key: string
  app_s_key: string
  fcnt_up?: number
  fcnt_down?: number
}

export type AddDeviceRequest = AddDeviceOTAA | AddDeviceABP

/**
 * Plan 03-07 success-state response shape (D-21). The keys field set is
 * disjoint between OTAA and ABP — discriminated by activation_mode.
 */
export type AddDeviceResponse =
  | (Device & {
      activation_mode: 'OTAA'
      app_key: string
      nwk_key: string
      join_eui: string
    })
  | (Device & {
      activation_mode: 'ABP'
      dev_addr: string
      nwk_s_key: string
      app_s_key: string
      f_cnt_up: number
      f_cnt_down: number
    })

/**
 * Convenience: fetch a flat Device[] (unwraps the Plan 03-06 envelope).
 * Defaults to per_page=100 — used by surfaces that need the whole list, not
 * a filtered/paginated view (e.g. swap-meter-dialog's unbound-device picker).
 *
 * For filterable/paginated rendering on /devices, use `listDevicesFiltered`.
 */
export const listDevices = async (): Promise<Device[]> => {
  const env = await apiFetch<ListDevicesEnvelope>('/api/devices?per_page=100')
  return env.rows ?? []
}

/**
 * Plan 03-09 — typed wrapper around the Phase 3 backend filter/sort/page
 * surface. Multi-site uses URLSearchParams.append per element (NOT
 * comma-separated). The backend rejects unknown values; this client only
 * sends well-typed members of `ListDevicesFilters`.
 */
export async function listDevicesFiltered(
  filters: ListDevicesFilters = {},
): Promise<ListDevicesEnvelope> {
  const sp = new URLSearchParams()
  for (const id of filters.site ?? []) sp.append('site', id)
  if (filters.status) sp.set('status', filters.status)
  if (filters.last_seen && filters.last_seen !== 'all') {
    sp.set('last_seen', filters.last_seen)
  }
  if (filters.q) sp.set('q', filters.q)
  if (filters.page !== undefined) sp.set('page', String(filters.page))
  if (filters.per_page !== undefined) sp.set('per_page', String(filters.per_page))
  if (filters.sort) sp.set('sort', filters.sort)
  const qs = sp.toString()
  return apiFetch<ListDevicesEnvelope>(qs ? `/api/devices?${qs}` : '/api/devices')
}

export const searchDevices = (q: string) =>
  apiFetch<Device[]>(`/api/devices/search?q=${encodeURIComponent(q)}`)

export const listDevicesBySite = (siteId: string) =>
  apiFetch<Device[]>(`/api/devices/by-site/${siteId}`)

export const getDevice = (id: string) => apiFetch<Device>(`/api/devices/${id}`)

export const parseDevEUI = (raw: string) =>
  apiFetch<DevEUIPreview>('/api/devices/parse-deveui', {
    method: 'POST',
    body: JSON.stringify({ raw }),
  })

export const preflight = () =>
  apiFetch<PreflightResult>('/api/devices/preflight', { method: 'POST', body: '{}' })

export const addDevice = (body: AddDeviceRequest) =>
  apiFetch<AddDeviceResponse>('/api/devices', {
    method: 'POST',
    body: JSON.stringify(body),
  })

export const decommissionDevice = (id: string) =>
  apiFetch<Device>(`/api/devices/${id}/decommission`, { method: 'POST', body: '{}' })

/**
 * Plan 03-09 — bulk decommission (D-17).
 *
 * Backend (internal/device/handlers.go bulkDecommissionDevices) runs each
 * device in its own short-lived Serializable tx — failure of one row does
 * NOT abort the loop. Operator receives a partial-success summary that the
 * UI surfaces via toast (success / warning / error tone).
 *
 * Cap: 200 device_ids per request (T-3-56). Caller should slice if it
 * needs more.
 */
export interface BulkDecommissionOutcome {
  id: string
  status: 'decommissioned' | 'failed'
  reason?: string
}

export interface BulkDecommissionResponse {
  succeeded: number
  failed: number
  outcomes: BulkDecommissionOutcome[]
}

export async function bulkDecommissionDevices(
  deviceIds: string[],
  reason: string,
): Promise<BulkDecommissionResponse> {
  return apiFetch<BulkDecommissionResponse>('/api/devices/bulk-decommission', {
    method: 'POST',
    body: JSON.stringify({
      device_ids: deviceIds,
      reason: reason.trim() || 'operator bulk decommission',
    }),
  })
}
