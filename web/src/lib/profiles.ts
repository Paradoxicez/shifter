import { apiFetch } from './api'

/**
 * Typed client for /api/device-profiles/* endpoints (Plan 02-08 + 02-11).
 *
 * Plan 02-14 Task 2 only needs `listProfiles` (Add Device dialog Step 2's
 * profile <Select>). Plan 02-14 Task 3 extends this module with the full
 * mapping editor surface (mappings, decoded-sample, save).
 */

export interface Profile {
  id: string
  slug: string
  name: string
  vendor: string
  family?: string | null
  capabilities: string[]
  counter_modulus: number | string
  region?: string | null
  mac_version: string
  codec_js?: string
  cs_profile_id?: string | null
  codec_js_synced_at?: string | null
  archived_at?: string | null
  created_at?: string
  updated_at?: string
}

export const listProfiles = () => apiFetch<Profile[]>('/api/device-profiles')

export const getProfile = (id: string) => apiFetch<Profile>(`/api/device-profiles/${id}`)
