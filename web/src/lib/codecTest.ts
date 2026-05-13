import { apiFetch } from '@/lib/api'

export type CodecTestResponse = {
  decoded_json?: Record<string, unknown>
  canonical_mapping?: unknown
  error_message?: string
  error_line?: number
  error_col?: number
  error_stack?: string
}

/**
 * POST /api/device-profiles/{id}/test-codec
 *
 * Note: route prefix is /api/device-profiles/ (not /api/profiles/) — consistent
 * with existing profile route surface (plan 07-07 deviation doc).
 */
export async function runCodecTest(
  profileId: string,
  hex: string,
  fPort: number,
): Promise<CodecTestResponse> {
  return apiFetch<CodecTestResponse>(
    `/api/device-profiles/${encodeURIComponent(profileId)}/test-codec`,
    {
      method: 'POST',
      body: JSON.stringify({ hex, fPort }),
    },
  )
}
