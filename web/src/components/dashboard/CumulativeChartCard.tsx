/**
 * CumulativeChartCard — Plan 04-08 Task 2
 *
 * Wraps ConsumptionChart with a shadcn Card. Reads date-range from URL
 * via useSearchParams, fires GET /api/dashboard/timeseries, shows Skeleton
 * while loading.
 *
 * Consumed by dashboard.tsx — one card per active utility.
 */

import { useQuery } from '@tanstack/react-query'
import { useSearchParams } from 'react-router-dom'
import { z } from 'zod'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { apiFetch } from '@/lib/api'
import { computeRangeWindow, type RangePreset } from '@/lib/dateRange'
import { ConsumptionChart } from './ConsumptionChart'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface TimeseriesResponse {
  utility: string
  bucket_interval_seconds: number
  series: { bucket: string; cumulative_delta: number }[]
}

interface CumulativeChartCardProps {
  utility: 'water' | 'electricity'
  meteringPointCount: number
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const rangeSchema = z.enum(['today', '24h', '7d', '30d', 'custom']).catch('today')

function titleSuffix(preset: RangePreset): string {
  switch (preset) {
    case 'today': return 'today'
    case '24h': return 'last 24 hours'
    case '7d': return 'last 7 days'
    case '30d': return 'last 30 days'
    case 'custom': return 'custom range'
  }
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function CumulativeChartCard({ utility, meteringPointCount }: CumulativeChartCardProps) {
  const [params] = useSearchParams()
  const preset = rangeSchema.parse(params.get('range') ?? 'today')
  const customStart = params.get('start') ?? undefined
  const customEnd = params.get('end') ?? undefined

  // Compute range window (may throw for invalid custom — catch and fall back)
  let start: Date
  let end: Date
  let bucketSec: number
  try {
    const window = computeRangeWindow(preset, customStart, customEnd)
    start = window.start
    end = window.end
    bucketSec = window.bucketSec
  } catch {
    // Invalid custom range — fall back to today
    const window = computeRangeWindow('today')
    start = window.start
    end = window.end
    bucketSec = window.bucketSec
  }

  const startISO = start.toISOString()
  const endISO = end.toISOString()

  const { data, isLoading } = useQuery<TimeseriesResponse>({
    queryKey: ['dashboard', 'timeseries', utility, preset, startISO, endISO],
    queryFn: () =>
      apiFetch<TimeseriesResponse>(
        `/api/dashboard/timeseries?range=${preset}&utility=${utility}&start=${encodeURIComponent(startISO)}&end=${encodeURIComponent(endISO)}`
      ),
    // Live presets: always re-fetch on mount; others cache for 5min
    staleTime: preset === 'today' || preset === '24h' ? 0 : 5 * 60_000,
  })

  const liveMode = preset === 'today' || preset === '24h'
  const utilityLabel = utility === 'water' ? 'Water' : 'Electricity'
  const suffix = titleSuffix(preset)
  const mpLabel = meteringPointCount === 1 ? 'metering point' : 'metering points'

  return (
    <Card>
      <CardHeader>
        <CardTitle>
          {utilityLabel} — {suffix}
        </CardTitle>
        <CardDescription>
          Cumulative across {meteringPointCount} {mpLabel}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {isLoading || !data ? (
          <Skeleton className="h-48 md:h-64 w-full" />
        ) : (
          <ConsumptionChart
            data={data.series}
            utility={utility}
            liveMode={liveMode}
            bucketSec={bucketSec}
          />
        )}
      </CardContent>
    </Card>
  )
}
