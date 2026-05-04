import { apiFetch } from './api'

/**
 * Typed client for /api/device-profiles/* endpoints (Plan 02-08 + 02-11).
 *
 * Plan 02-14 Task 2 only needed `listProfiles` (Add Device dialog Step 2's
 * profile <Select>). Plan 02-14 Task 3 extends this module with the full
 * mapping editor surface (mappings, decoded-sample, save).
 *
 * Backend handler shape (internal/profile/handlers.go):
 *   GET   /api/device-profiles                         → Profile[]            (admin + viewer)
 *   GET   /api/device-profiles/{id}                    → ProfileWithMappings  (admin + viewer)
 *   POST  /api/device-profiles/{id}/decoded-sample     → DecodedSample        (admin + viewer)
 *   POST  /api/device-profiles                         → { id }               (admin only)
 *   PATCH /api/device-profiles/{id}                    → { id }               (admin only)
 *   POST  /api/device-profiles/{id}/archive            → Profile              (admin only)
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

export interface Mapping {
  id?: string
  json_pointer: string
  target: string
  scale: string
  data_type: 'numeric' | 'int' | 'bool' | 'text'
  position: number
}

export interface ProfileWithMappings {
  profile: Profile
  mappings: Mapping[]
}

/**
 * MappingRequest matches the wire shape of profile.MappingRequest in
 * internal/profile/handlers.go. Same fields as Mapping, but `id` is omitted
 * (server-assigned).
 */
export interface MappingRequest {
  json_pointer: string
  target: string
  scale: string
  data_type: 'numeric' | 'int' | 'bool' | 'text'
  position: number
}

/**
 * ProfileRequest matches profile.ProfileRequest in
 * internal/profile/handlers.go — the shape POST/PATCH expect.
 */
export interface ProfileRequest {
  slug: string
  name: string
  vendor: string
  family?: string
  capabilities: string[]
  counter_modulus: number
  region?: string
  mac_version: string
  codec_js?: string
  mappings: MappingRequest[]
}

export interface DecodedSampleLeaf {
  json_pointer: string
  value: unknown
}

export interface DecodedSample {
  leaves: DecodedSampleLeaf[]
  truncated?: boolean
}

export const listProfiles = () => apiFetch<Profile[]>('/api/device-profiles')

export const getProfileWithMappings = (id: string) =>
  apiFetch<ProfileWithMappings>(`/api/device-profiles/${id}`)

/**
 * Backwards-compat helper retained for Task 2 callers (Add Device dialog
 * profile select renders Profile only, not the full mapping bundle).
 */
export const getProfile = (id: string) =>
  apiFetch<Profile>(`/api/device-profiles/${id}`)

export const createProfile = (body: ProfileRequest) =>
  apiFetch<{ id: string }>('/api/device-profiles', {
    method: 'POST',
    body: JSON.stringify(body),
  })

export const updateProfile = (id: string, body: ProfileRequest) =>
  apiFetch<{ id: string }>(`/api/device-profiles/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(body),
  })

export const archiveProfile = (id: string) =>
  apiFetch<Profile>(`/api/device-profiles/${id}/archive`, {
    method: 'POST',
    body: '{}',
  })

/**
 * decodedSamplePreview asks the backend to walk a sample JSON object and
 * return every RFC 6901 leaf. Plan 02-14 Task 3 also implements a client-side
 * flatten helper in lib/json-flatten.ts so the mapping editor's left pane can
 * render the JSON tree before the profile has been saved (the backend
 * endpoint is path-scoped to {id} which doesn't exist in 'new' mode).
 */
export const decodedSamplePreview = (id: string, sample: unknown) =>
  apiFetch<DecodedSample>(`/api/device-profiles/${id}/decoded-sample`, {
    method: 'POST',
    body: JSON.stringify({ sample }),
  })
