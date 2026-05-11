/**
 * DashboardPage — Plan 04-07 Task 2 + Plan 04-08 Task 2
 *
 * The operator's primary surface. Mounted at `/` (replaces IndexRedirect → /settings).
 *
 * Layout:
 *  - LiveChannelBanner (when SSE is not 'open')
 *  - Heading "Dashboard" + DateRangePicker (right)
 *  - KpiGrid (capability-gated; hidden while empty-state is showing)
 *  - CumulativeChartCard(s) — 1 for water/electricity; 2 (water above electricity) for both
 *  - EmptyStateOnboarding (when uplink_count === 0)
 *
 * Live recomputation (D-06):
 *  - Maintain per-MP latest_instant_map in component state, seeded from snapshot
 *  - On SSE measurement event → update map entry for that MP
 *  - Derive instantOverride per utility by summing map entries
 *  - Other 3 KPIs (today, delta, online) recompute via snapshot refetch triggered by useSSE
 *
 * T-04-07-01: Route mounts under <RootLayout /> which wraps the auth-required group.
 * No additional auth check needed here — rootLoader handles the redirect.
 */

import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { CumulativeChartCard } from '@/components/dashboard/CumulativeChartCard'
import { DateRangePicker } from '@/components/dashboard/DateRangePicker'
import { EmptyStateOnboarding } from '@/components/dashboard/EmptyStateOnboarding'
import { KpiGrid, type KpiSnapshot } from '@/components/dashboard/KpiGrid'
import { LiveChannelBanner } from '@/components/dashboard/LiveChannelBanner'
import { Skeleton } from '@/components/ui/skeleton'
import { useDashboardScope } from '@/hooks/useDashboardScope'
import { useSSE, type MeasurementDelta } from '@/hooks/useSSE'
import { apiFetch } from '@/lib/api'

// ---------------------------------------------------------------------------
// Snapshot shape (from Plan 04 endpoint contract)
// ---------------------------------------------------------------------------

interface LatestReading {
  metering_point_id: string
  utility_class: string
  time: string
  cumulative_value: number | null
  instant_value: number | null
  quality: string
  battery_pct: number | null
  rssi: number | null
}

interface Snapshot {
  capabilities: 'water' | 'electricity' | 'both'
  generated_at: string
  kpis: KpiSnapshot
  latest_readings: LatestReading[]
}

// ---------------------------------------------------------------------------
// Per-MP latest instant map shape
// ---------------------------------------------------------------------------

interface InstantEntry {
  utility: 'water' | 'electricity'
  instant: number | null
}

// ---------------------------------------------------------------------------
// DashboardPage
// ---------------------------------------------------------------------------

export default function DashboardPage() {
  const scope = useDashboardScope()

  const hasUplinkData = (scope.data?.onboarding.uplink_count ?? 0) > 0

  const snapshot = useQuery<Snapshot>({
    queryKey: ['dashboard', 'snapshot'],
    queryFn: () => apiFetch<Snapshot>('/api/dashboard/snapshot'),
    // Only fetch once we know there's real data to show
    enabled: hasUplinkData,
  })

  // Per-MP latest instant value map — seeded from snapshot, updated live via SSE
  const [latestInstantMap, setLatestInstantMap] = useState<Record<string, InstantEntry>>({})

  // Seed map from snapshot once it lands
  useEffect(() => {
    if (snapshot.data) {
      const seed: Record<string, InstantEntry> = {}
      for (const r of snapshot.data.latest_readings) {
        seed[r.metering_point_id] = {
          utility: r.utility_class as 'water' | 'electricity',
          instant: r.instant_value,
        }
      }
      setLatestInstantMap(seed)
    }
  }, [snapshot.data])

  // SSE connection — subscribed to dashboard:global for real-time updates
  const sse = useSSE({
    topics: ['dashboard:global'],
    onMeasurement: (delta: MeasurementDelta) => {
      setLatestInstantMap((prev) => {
        const existing = prev[delta.metering_point_id]
        if (!existing) return prev // unknown MP — will appear on next snapshot refetch
        return {
          ...prev,
          [delta.metering_point_id]: { ...existing, instant: delta.instant_value },
        }
      })
    },
  })

  // Derive live instant totals per utility from the per-MP map
  const instantOverride = useMemo(() => {
    const sums: { water: number; electricity: number } = { water: 0, electricity: 0 }
    for (const entry of Object.values(latestInstantMap)) {
      if (entry.instant != null) {
        sums[entry.utility] += entry.instant
      }
    }
    return sums
  }, [latestInstantMap])

  // Loading skeleton
  if (scope.isLoading) {
    return <Skeleton className="h-96 w-full" />
  }

  if (!scope.data) {
    return null
  }

  const { capabilities, onboarding } = scope.data

  // D-21: empty state — show progressive onboarding when no uplinks yet
  if (onboarding.uplink_count === 0) {
    return (
      <div className="space-y-8">
        <LiveChannelBanner status={sse.status} lastEventAt={sse.lastEventAt} />
        <EmptyStateOnboarding onboarding={onboarding} />
      </div>
    )
  }

  return (
    <div className="space-y-8">
      <LiveChannelBanner status={sse.status} lastEventAt={sse.lastEventAt} />

      <div className="flex items-baseline justify-between">
        <h1 className="text-2xl font-semibold leading-8">Dashboard</h1>
        <DateRangePicker mode="shared-url" />
      </div>

      {snapshot.data && (
        <KpiGrid
          capabilities={capabilities}
          kpis={snapshot.data.kpis}
          instantOverride={instantOverride}
        />
      )}

      {/* Consumption chart cards — water above electricity for 'both' capability */}
      {(() => {
        const waterCount = snapshot.data
          ? snapshot.data.latest_readings.filter((r) => r.utility_class === 'water').length
          : 0
        const electricityCount = snapshot.data
          ? snapshot.data.latest_readings.filter((r) => r.utility_class === 'electricity').length
          : 0
        return (
          <>
            {capabilities !== 'electricity' && (
              <CumulativeChartCard utility="water" meteringPointCount={waterCount} />
            )}
            {capabilities !== 'water' && (
              <CumulativeChartCard utility="electricity" meteringPointCount={electricityCount} />
            )}
          </>
        )
      })()}
    </div>
  )
}
