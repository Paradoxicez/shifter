/**
 * dateRange — Plan 04-08
 *
 * Preset → time window computation matching server D-12 bucket schedule.
 *
 * D-12 bucket schedule:
 *   today / 24h  → 5-minute buckets (bucketSec=300)
 *   7d           → 1-hour buckets (bucketSec=3600)
 *   30d          → 4-hour buckets (bucketSec=14400)
 *   custom ≤30d  → 1-hour buckets (bucketSec=3600)
 *   custom >30d  → 1-day buckets (bucketSec=86400)
 *   custom max   → 365 days (server returns 400 if exceeded)
 */

export type RangePreset = 'today' | '24h' | '7d' | '30d' | 'custom'

export interface RangeWindow {
  start: Date
  end: Date
  bucketSec: number
}

/**
 * Compute the time window (start, end, bucketSec) for a given preset.
 * Mirrors the server-side D-12 bucket schedule exactly.
 *
 * T-04-08-01: zod `.catch('today')` fallback is applied at the component
 *             layer; this function can receive only valid RangePreset values.
 * T-04-08-02: custom ranges >1y throw immediately — UI catches + shows error;
 *             the API request is never fired.
 */
export function computeRangeWindow(
  preset: RangePreset,
  customStart?: string,
  customEnd?: string,
  now: Date = new Date(),
): RangeWindow {
  const end = now

  switch (preset) {
    case 'today': {
      const start = new Date(now)
      start.setHours(0, 0, 0, 0)
      return { start, end, bucketSec: 300 }
    }

    case '24h': {
      const start = new Date(now.getTime() - 24 * 3600 * 1000)
      return { start, end, bucketSec: 300 }
    }

    case '7d': {
      const start = new Date(now.getTime() - 7 * 24 * 3600 * 1000)
      return { start, end, bucketSec: 3600 }
    }

    case '30d': {
      const start = new Date(now.getTime() - 30 * 24 * 3600 * 1000)
      return { start, end, bucketSec: 14400 }
    }

    case 'custom': {
      if (!customStart || !customEnd) {
        throw new Error('custom range requires start and end')
      }
      const start = new Date(customStart)
      const cend = new Date(customEnd)
      const spanMs = cend.getTime() - start.getTime()
      if (spanMs <= 0) {
        throw new Error('custom range requires start < end')
      }
      if (spanMs > 365 * 24 * 3600 * 1000) {
        throw new Error('custom range exceeds 1 year')
      }
      const bucketSec = spanMs <= 30 * 24 * 3600 * 1000 ? 3600 : 24 * 3600
      return { start, end: cend, bucketSec }
    }
  }
}
