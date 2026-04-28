import { ApiError, apiFetch } from './api'

/**
 * Typed client for /api/install/* endpoints (Plan 15 backend).
 *
 * Field names mirror the pgx-scanned struct fields from internal/install/state.go
 * (Go's exported field names: CurrentStep, Step1Admin, etc). The wizard shell
 * branches on `state.CurrentStep` (1..5) to render the active step component.
 */
export interface InstallState {
  CurrentStep: number
  StartedAt: string
  CompletedAt: string | null
  Step1Admin: unknown
  Step2ChirpStack: unknown
  Step3Region: unknown
  Step4Identity: unknown
}

/** Returns null when install is already completed (backend responds 410 Gone). */
export async function fetchInstallState(): Promise<InstallState | null> {
  try {
    return await apiFetch<InstallState>('/api/install/state')
  } catch (err) {
    if (err instanceof ApiError && err.status === 410) return null
    throw err
  }
}

export interface Step1Body {
  email: string
  name: string
  password: string
}

export interface Step2Body {
  mode: 'bundled' | 'external'
  grpc_url: string
  api_token: string
  mqtt_url: string
  mqtt_user?: string
  mqtt_password?: string
}

export interface Step4Body {
  display_name: string
  address?: string
  timezone: string
  units: 'metric' | 'imperial'
  logo_path?: string
}

export const postStep1 = (b: Step1Body) =>
  apiFetch<{ current_step: number }>('/api/install/step/1', {
    method: 'POST',
    body: JSON.stringify(b),
  })

export const postStep2 = (b: Step2Body) =>
  apiFetch<{ current_step: number; chirpstack_version: string }>('/api/install/step/2', {
    method: 'POST',
    body: JSON.stringify(b),
  })

export const postStep3 = (b: { name: string }) =>
  apiFetch<{ current_step: number }>('/api/install/step/3', {
    method: 'POST',
    body: JSON.stringify(b),
  })

export const postStep4 = (b: Step4Body) =>
  apiFetch<{ current_step: number }>('/api/install/step/4', {
    method: 'POST',
    body: JSON.stringify(b),
  })

export const postFinish = () =>
  apiFetch<{ ok: true }>('/api/install/finish', { method: 'POST' })

export interface Region {
  name: string
  display: string
  common_name: string
  group: 'asia' | 'europe' | 'americas' | 'oceania' | 'india'
  default_for_country?: string
  note?: string
}

/**
 * Hardcoded LoRaWAN region catalog mirroring internal/install/regions.go (Plan 14).
 * AS923-2 carries `default_for_country: 'TH'` per PITFALLS §8 — Thailand operator
 * base; the wizard pre-selects this entry on mount.
 */
export const REGIONS: Region[] = [
  { name: 'as923', display: 'AS923-1', common_name: 'AS923', group: 'asia' },
  {
    name: 'as923_2',
    display: 'AS923-2 (Thailand)',
    common_name: 'AS923_2',
    group: 'asia',
    default_for_country: 'TH',
    note: 'Required by Thai regulator NBTC.',
  },
  { name: 'as923_3', display: 'AS923-3', common_name: 'AS923_3', group: 'asia' },
  { name: 'as923_4', display: 'AS923-4', common_name: 'AS923_4', group: 'asia' },
  { name: 'eu868', display: 'EU868 (Europe)', common_name: 'EU868', group: 'europe' },
  { name: 'us915_0', display: 'US915 sub-band 1 (ch 0-7)', common_name: 'US915', group: 'americas' },
  { name: 'au915_0', display: 'AU915 sub-band 1', common_name: 'AU915', group: 'oceania' },
  { name: 'in865', display: 'IN865 (India)', common_name: 'IN865', group: 'india' },
]
