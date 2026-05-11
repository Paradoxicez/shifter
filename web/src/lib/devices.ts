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

export const listDevices = () => apiFetch<Device[]>('/api/devices')

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
