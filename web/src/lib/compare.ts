/**
 * Compare API client — Plan 07-12
 *
 * Wraps POST /api/reports/compare (Surface 5).
 *
 * Two modes:
 *   mode=entities   — two entities, one time range
 *   mode=time_ranges — one entity, two time ranges
 */

import { apiFetch } from './api'

// ---- request types ----------------------------------------------------------

export type CompareTimeRange = {
  from: string // ISO-8601
  to: string   // ISO-8601
}

export type CompareEntitiesRequest = {
  mode: 'entities'
  entity_type: 'site' | 'metering_point'
  entity_a_id: string
  entity_b_id: string
  range: CompareTimeRange
}

export type CompareTimeRangesRequest = {
  mode: 'time_ranges'
  entity_type: 'site' | 'metering_point'
  entity_id: string
  range_a: CompareTimeRange
  range_b: CompareTimeRange
}

export type CompareRequest = CompareEntitiesRequest | CompareTimeRangesRequest

// ---- response types ---------------------------------------------------------

export type CompareSeriesPoint = {
  bucket: string
  value: number
}

export type SeriesResult = {
  label: string
  series: CompareSeriesPoint[]
  total: number
  peak: number
  average: number
  unit: 'm3' | 'kWh'
}

export type CompareResponse = {
  a: SeriesResult
  b: SeriesResult
  delta: {
    total: number
    peak: number
    average: number
  }
}

// ---- API function -----------------------------------------------------------

export async function compareReports(req: CompareRequest): Promise<CompareResponse> {
  return apiFetch<CompareResponse>('/api/reports/compare', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}
