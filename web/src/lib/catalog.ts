/**
 * Catalog API client — Plan 07-05 (Surface 1, D-40).
 *
 * Talks to:
 *   GET /api/catalog        → CatalogResponse
 *   GET /api/catalog/{slug} → CatalogEntry
 */
import { apiFetch } from '@/lib/api'

export type CatalogEntry = {
  slug: string
  name: string
  vendor: string
  family: string
  capabilities: string[]
  version: string
  codec_js_path: string
  counter_modulus: number
  mac_version: string
  region: string | null
  expected_uplink_interval_seconds: number
  offline_threshold_multiplier: number
  anomaly_compatibility: 'full' | 'limited' | 'unsupported'
  battery_curve: 'linear_pct' | 'li_socl2_3v6' | 'li_mnox_3v0' | 'alkaline_3v0' | 'none'
  vendor_has_separate_meter_serial: boolean
}

export type CatalogListProfile = {
  profile_id: string | null
  slug: string
  installed_version: string
  status: 'not-installed' | 'installed' | 'update-available'
  devices_using_count: number
  customer_edited: boolean
  codec_js_synced_at: string | null
}

export type CatalogResponse = {
  entries: CatalogEntry[]
  profiles: CatalogListProfile[]
}

export async function fetchCatalog(): Promise<CatalogResponse> {
  return apiFetch<CatalogResponse>('/api/catalog')
}

export async function fetchCatalogEntry(slug: string): Promise<CatalogEntry> {
  return apiFetch<CatalogEntry>(`/api/catalog/${encodeURIComponent(slug)}`)
}

// ---------------------------------------------------------------------------
// Plan 07-06 mutations — import + update
// ---------------------------------------------------------------------------

export type ImportFromCatalogResponse = { profile_id: string }

export async function importFromCatalog(
  slug: string,
  name?: string,
): Promise<ImportFromCatalogResponse> {
  try {
    return await apiFetch<ImportFromCatalogResponse>('/api/catalog/import', {
      method: 'POST',
      body: JSON.stringify({ slug, name }),
    })
  } catch (err: unknown) {
    // apiFetch throws ApiError for non-ok responses; surface 409 as a
    // recognisable sentinel so ImportFromCatalogDialog can show inline error.
    if (err instanceof Error && 'status' in err && (err as { status: number }).status === 409) {
      throw new Error('already imported')
    }
    throw err
  }
}

export type ApplyUpdateBody = {
  accepted_fields: string[] // e.g. ['codec_js', 'battery_curve']
  target_version: string // e.g. '1.1.0'
}
export type ApplyUpdateResponse = { updated_at: string; new_version: string }

export async function applyCatalogUpdate(
  profileId: string,
  body: ApplyUpdateBody,
): Promise<ApplyUpdateResponse> {
  return apiFetch<ApplyUpdateResponse>(
    `/api/catalog/${encodeURIComponent(profileId)}/update`,
    {
      method: 'POST',
      body: JSON.stringify(body),
    },
  )
}
