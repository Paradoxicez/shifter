import { apiFetch } from './api'

/**
 * Typed client for /api/metering-points/* endpoints (Plan 02-10 backend).
 *
 * Backend handler shape (internal/meteringpoint/handlers.go):
 *   GET    /api/metering-points              → MeteringPoint[]
 *   GET    /api/metering-points/archived     → MeteringPoint[]
 *   GET    /api/metering-points/by-site/{siteID} → MeteringPoint[]
 *   GET    /api/metering-points/{id}         → MeteringPointDetail
 *   GET    /api/metering-points/{id}/quality → QualitySummary
 *   POST   /api/metering-points              → MeteringPoint (admin only)
 *   PATCH  /api/metering-points/{id}         → MeteringPoint (admin only)
 *   POST   /api/metering-points/{id}/archive → MeteringPoint (admin only)
 *   POST   /api/metering-points/{id}/restore → MeteringPoint (admin only)
 */

export interface MeteringPoint {
  id: string
  site_id: string
  name: string
  utility_class: 'water' | 'electricity'
  location_description?: string | null
  archived_at?: string | null
  created_at?: string
  updated_at?: string
}

export interface ActiveBindingDevice {
  id: string
  dev_eui: string
  name: string
}

export interface ActiveBindingProfile {
  id: string
  name: string
  capabilities: string[]
  counter_modulus: number | string
}

export interface ActiveBinding {
  id: string
  valid_from: string
  reading_offset: string
  device: ActiveBindingDevice
  device_profile: ActiveBindingProfile
}

export interface LatestMeasurement {
  time: string
  raw_value: string | null
  cumulative_value: string | null
  instant_value: string | null
  battery_pct: number | null
  quality: string
}

export interface MeteringPointDetail {
  id: string
  name: string
  site: { id: string; name: string }
  utility_class: 'water' | 'electricity'
  location_description?: string | null
  archived_at?: string | null
  created_at?: string
  updated_at?: string
  active_binding: ActiveBinding | null
  latest_measurement: LatestMeasurement | null
}

export interface CreateMPRequest {
  site_id: string
  name: string
  utility_class: 'water' | 'electricity'
  location_description?: string
}

export type UpdateMPRequest = Omit<CreateMPRequest, 'site_id'>

export const listMPs = (opts: { archived?: boolean } = {}) =>
  apiFetch<MeteringPoint[]>(opts.archived ? '/api/metering-points/archived' : '/api/metering-points')

export const listMPsBySite = (siteId: string) =>
  apiFetch<MeteringPoint[]>(`/api/metering-points/by-site/${siteId}`)

export const getMPDetail = (id: string) =>
  apiFetch<MeteringPointDetail>(`/api/metering-points/${id}`)

export const createMP = (body: CreateMPRequest) =>
  apiFetch<MeteringPoint>('/api/metering-points', {
    method: 'POST',
    body: JSON.stringify(body),
  })

export const updateMP = (id: string, body: UpdateMPRequest) =>
  apiFetch<MeteringPoint>(`/api/metering-points/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(body),
  })

export const archiveMP = (id: string) =>
  apiFetch<MeteringPoint>(`/api/metering-points/${id}/archive`, {
    method: 'POST',
    body: '{}',
  })

export const restoreMP = (id: string) =>
  apiFetch<MeteringPoint>(`/api/metering-points/${id}/restore`, {
    method: 'POST',
    body: '{}',
  })
