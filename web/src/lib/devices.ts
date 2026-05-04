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

export interface AddDeviceRequest {
  dev_eui: string
  name: string
  device_profile_id: string
  app_key: string
  join_eui?: string
  description?: string
  metering_point_id?: string
  initial_reading?: string
}

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
  apiFetch<Device>('/api/devices', { method: 'POST', body: JSON.stringify(body) })

export const decommissionDevice = (id: string) =>
  apiFetch<Device>(`/api/devices/${id}/decommission`, { method: 'POST', body: '{}' })
