import { apiFetch } from './api'

/**
 * Typed client for /api/settings/chirpstack/* endpoints (Plan 17 backend).
 *
 * - GET  /api/settings/chirpstack       → ChirpStackSettings (api_token NEVER returned, T-17-01)
 * - POST /api/settings/chirpstack/test  → TestConnResult (CHIRP-03 two-channel probe)
 * - PUT  /api/settings/chirpstack       → { ok: true } (admin-only; SETT-03)
 */

export interface ChirpStackSettings {
  mode: 'bundled' | 'external'
  grpc_url: string
  mqtt_url: string
  mqtt_user: string
  region: { name: string; common_name: string }
}

export type ChannelStatus = 'reachable' | 'unreachable' | 'skipped'

export interface ChannelResult {
  status: ChannelStatus
  latency_ms?: number
  detail?: string
}

export interface TestConnResult {
  grpc: ChannelResult
  mqtt: ChannelResult
}

export const fetchChirpStackSettings = () =>
  apiFetch<ChirpStackSettings>('/api/settings/chirpstack')

export interface TestConnBody {
  grpc_url: string
  api_token: string
  mqtt_url: string
  mqtt_user?: string
  mqtt_pass?: string
}

export const testChirpStackConnection = (body: TestConnBody) =>
  apiFetch<TestConnResult>('/api/settings/chirpstack/test', {
    method: 'POST',
    body: JSON.stringify(body),
  })

export interface PutChirpStackBody {
  mode: 'bundled' | 'external'
  grpc_url: string
  api_token?: string
  mqtt_url: string
  mqtt_user?: string
  mqtt_password?: string
  region_name: string
  region_common_name: string
}

export const putChirpStackSettings = (body: PutChirpStackBody) =>
  apiFetch<{ ok: true }>('/api/settings/chirpstack', {
    method: 'PUT',
    body: JSON.stringify(body),
  })
