/**
 * MeteringPointDetailPage — Plan 04-09 Task 3
 *
 * Per-meter detail page replacing Phase 2 minimal page.
 * 3-tab layout: Normal | Advanced | Uplinks log (D-15).
 *
 * URL state:
 *   ?tab=normal|advanced|uplinks  (default: normal)
 *   ?quality=ok,decode_fail,...   (uplinks tab quality filter)
 *   ?range=today|24h|7d|30d|custom + ?start + ?end (per-MP picker)
 *
 * SSE wiring:
 *   Subscribes to topics ['mp:<id>', 'mp:<id>:uplinks'].
 *   New measurement events set `pendingPayload` state.
 *   AdvancedTab receives pendingPayload + clearPending (3-prop contract from Task 2).
 *
 * D-22 empty case:
 *   When latest_reading is null, Advanced and Uplinks tabs are disabled
 *   with tooltip "Available after first uplink".
 *
 * Security (T-04-09-01):
 *   Admin-only CTAs are gated via useCurrentUser() inside child tabs.
 */

import { useState } from 'react'
import { useParams, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { apiFetch } from '@/lib/api'
import { useSSE, type MeasurementDelta } from '@/hooks/useSSE'
import { useCurrentUser } from '@/lib/use-current-user'
import { AdvancedTab } from '@/components/metering-point/AdvancedTab'
import { AnomalyStateCard } from '@/components/metering-point/AnomalyStateCard'
import { NormalTab } from '@/components/metering-point/NormalTab'
import { UplinksLogTab } from '@/components/metering-point/UplinksLogTab'
import type { DetailResponse } from '@/components/metering-point/NormalTab'
import { SwapMeterDialog } from './swap-meter-dialog'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface SignalHistoryResponse {
  window_start: string
  window_end: string
  bucket_interval_seconds: number
  series: Array<{ bucket: string; battery_pct: number | null; rssi: number | null; snr: number | null }>
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function MeteringPointDetailPage() {
  const { id } = useParams<{ id: string }>()
  const [searchParams, setSearchParams] = useSearchParams()
  const activeTab = searchParams.get('tab') ?? 'normal'

  // Fetch detail
  const detailQuery = useQuery<DetailResponse>({
    queryKey: ['mp', id, 'detail'],
    queryFn: () => apiFetch<DetailResponse>(`/api/metering-points/${id}`),
    enabled: !!id,
  })

  // Fetch signal history (sparklines) — only when detail has a reading
  const signalQuery = useQuery<SignalHistoryResponse>({
    queryKey: ['mp', id, 'signal-history'],
    queryFn: () => apiFetch<SignalHistoryResponse>(`/api/metering-points/${id}/signal-history`),
    enabled: !!id && detailQuery.data?.latest_reading != null,
  })

  // Swap dialog state + admin gate
  const [swapOpen, setSwapOpen] = useState(false)
  const user = useCurrentUser()
  const isAdmin = user?.role === 'admin'

  // Pending payload for AdvancedTab (forensic-safety — D-20)
  const [pendingPayload, setPendingPayload] = useState<MeasurementDelta | null>(null)

  // SSE subscription — D-17 live updates
  useSSE({
    topics: id ? [`mp:${id}`, `mp:${id}:uplinks`] : [],
    onMeasurement: (delta: MeasurementDelta) => {
      if (delta.metering_point_id === id) {
        // Queue for Advanced tab alert rather than auto-swapping (D-20 forensic-safety)
        setPendingPayload(delta)
      }
    },
  })

  const detail = detailQuery.data
  if (!detail) {
    return <div className="p-6 text-sm text-muted-foreground">Loading…</div>
  }

  const { metering_point: mp, latest_reading } = detail
  const tabsDisabled = latest_reading == null

  const handleTabChange = (value: string) => {
    setSearchParams((p) => {
      p.set('tab', value)
      return p
    })
  }

  return (
    <div className="flex flex-col gap-6 p-6">
      {/* Page header */}
      <header className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold leading-8">{mp.name}</h1>
          <div className="flex items-center gap-2 text-sm text-muted-foreground mt-1">
            <span>{mp.site_name ?? '—'}</span>
            <span>·</span>
            <Badge variant="secondary" className="text-xs uppercase">
              {mp.utility_class}
            </Badge>
          </div>
        </div>
        {isAdmin && (
          <Button variant="outline" size="sm" onClick={() => setSwapOpen(true)}>
            Swap Meter
          </Button>
        )}
      </header>

      {/* Plan 06-04 D-16 — anomaly state card above the existing tabs */}
      <AnomalyStateCard meteringPointId={mp.id} />

      {/* 3-tab layout */}
      <TooltipProvider>
        <Tabs value={activeTab} onValueChange={handleTabChange}>
          <TabsList>
            <TabsTrigger value="normal">Normal</TabsTrigger>

            {tabsDisabled ? (
              <Tooltip>
                <TooltipTrigger asChild>
                  {/* span wrapper required — disabled button can't receive events */}
                  <span>
                    <TabsTrigger value="advanced" disabled>
                      Advanced
                    </TabsTrigger>
                  </span>
                </TooltipTrigger>
                <TooltipContent>Available after first uplink</TooltipContent>
              </Tooltip>
            ) : (
              <TabsTrigger value="advanced">Advanced</TabsTrigger>
            )}

            {tabsDisabled ? (
              <Tooltip>
                <TooltipTrigger asChild>
                  <span>
                    <TabsTrigger value="uplinks" disabled>
                      Uplinks log
                    </TabsTrigger>
                  </span>
                </TooltipTrigger>
                <TooltipContent>Available after first uplink</TooltipContent>
              </Tooltip>
            ) : (
              <TabsTrigger value="uplinks">Uplinks log</TabsTrigger>
            )}
          </TabsList>

          {/* Normal tab — always rendered */}
          <TabsContent value="normal">
            <NormalTab
              detail={detail}
              signalHistory={signalQuery.data}
              online={detail.online}
            />
          </TabsContent>

          {/* Advanced tab — only when reading exists */}
          <TabsContent value="advanced">
            {latest_reading && (
              <AdvancedTab
                latestReading={latest_reading}
                pendingPayload={pendingPayload}
                clearPending={() => setPendingPayload(null)}
              />
            )}
          </TabsContent>

          {/* Uplinks log tab — only when reading exists */}
          <TabsContent value="uplinks">
            {id && latest_reading && <UplinksLogTab meteringPointId={id} />}
          </TabsContent>
        </Tabs>
      </TooltipProvider>
      {isAdmin && id && (
        <SwapMeterDialog
          open={swapOpen}
          onOpenChange={setSwapOpen}
          meteringPointId={id}
          meteringPointName={mp.name}
        />
      )}
    </div>
  )
}
