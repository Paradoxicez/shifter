/**
 * NormalTab — Plan 04-09 Task 2
 *
 * Normal tab for the MP detail page (D-15).
 *
 * Full case (latest_reading present):
 *   - Status row: online badge + last update relative time + QualityBadge (D-19)
 *   - Cumulative card: big number + unit
 *   - Instant card
 *   - SparklineTriplet: battery/RSSI/SNR from signal history (D-17)
 *
 * D-22 empty case (latest_reading == null):
 *   - MP info card
 *   - "No device bound" indicator
 *   - "Add device" CTA (admin-only via useCurrentUser)
 */

import { formatDistanceToNow } from 'date-fns'
import { Plus, Wifi, WifiOff } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useCurrentUser } from '@/lib/use-current-user'
import { QualityBadge } from './QualityBadge'
import { SparklineTriplet, type SignalSeriesPoint } from './SparklineTriplet'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface MeteringPoint {
  id: string
  name: string
  site_id: string | null
  site_name: string | null
  utility_class: 'water' | 'electricity'
  location_description: string | null
}

interface ActiveBinding {
  device_id: string
  dev_eui: string
  device_profile_name: string
  valid_from: string
}

interface LatestReading {
  time: string
  cumulative_value: number | null
  instant_value: number | null
  quality: string
  battery_pct: number | null
  rssi: number | null
  snr: number | null
  fcnt: number | null
  decoded_object: Record<string, unknown>
  extra: Record<string, unknown>
  raw_payload_hex: string
}

interface QualitySummary {
  window_size: number
  flagged_count: number
  by_quality: Record<string, number>
}

export interface DetailResponse {
  metering_point: MeteringPoint
  active_binding: ActiveBinding | null
  latest_reading: LatestReading | null
  quality_summary: QualitySummary
  online: boolean | null
}

interface SignalHistoryResponse {
  window_start: string
  window_end: string
  bucket_interval_seconds: number
  series: SignalSeriesPoint[]
}

interface NormalTabProps {
  detail: DetailResponse
  signalHistory: SignalHistoryResponse | undefined
  online: boolean | null
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function NormalTab({ detail, signalHistory, online }: NormalTabProps) {
  const user = useCurrentUser()
  const isAdmin = user?.role === 'admin'
  const { metering_point: mp, latest_reading, quality_summary } = detail

  // D-22 empty case
  if (!latest_reading) {
    return (
      <div className="space-y-4">
        <Card>
          <CardHeader>
            <CardTitle>{mp.name}</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-2 text-sm">
            <div>
              <span className="font-semibold">Site:</span>{' '}
              {mp.site_name ?? '—'}
            </div>
            <div>
              <span className="font-semibold">Type:</span>{' '}
              <Badge variant="secondary" className="text-xs uppercase">
                {mp.utility_class}
              </Badge>
            </div>
            {mp.location_description && (
              <div>
                <span className="font-semibold">Location:</span>{' '}
                {mp.location_description}
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardContent className="flex flex-col items-center gap-3 py-8 text-center">
            <p className="text-sm font-semibold">No device bound</p>
            <p className="text-sm text-muted-foreground">
              Add a device to start receiving telemetry on this metering point.
            </p>
            {isAdmin && (
              <Button>
                <Plus className="h-4 w-4" />
                Add device
              </Button>
            )}
          </CardContent>
        </Card>
      </div>
    )
  }

  // Full case
  const unit = mp.utility_class === 'water' ? 'm³' : 'kWh'
  const lastUplink = formatDistanceToNow(new Date(latest_reading.time), { addSuffix: true })

  return (
    <div className="space-y-4">
      {/* MP name */}
      <h2 className="text-lg font-semibold">{mp.name}</h2>

      {/* Status row */}
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-2">
          {online === true ? (
            <Badge variant="secondary" className="gap-1.5 text-success">
              <Wifi className="h-3 w-3" />
              Online
            </Badge>
          ) : online === false ? (
            <Badge variant="secondary" className="gap-1.5 text-destructive">
              <WifiOff className="h-3 w-3" />
              Offline
            </Badge>
          ) : null}
          <span className="text-xs text-muted-foreground">Last uplink {lastUplink}</span>
        </div>
        <QualityBadge
          flaggedCount={quality_summary.flagged_count}
          windowSize={quality_summary.window_size}
        />
      </div>

      {/* Reading cards */}
      <div className="grid gap-4 sm:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
              Cumulative
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex items-baseline gap-2">
              <span className="text-3xl font-semibold font-mono">
                {latest_reading.cumulative_value !== null
                  ? latest_reading.cumulative_value.toLocaleString()
                  : '—'}
              </span>
              <span className="text-sm text-muted-foreground">{unit}</span>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
              Instant
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex items-baseline gap-2">
              <span className="text-3xl font-semibold font-mono">
                {latest_reading.instant_value !== null
                  ? latest_reading.instant_value.toLocaleString()
                  : '—'}
              </span>
              <span className="text-sm text-muted-foreground">
                {mp.utility_class === 'water' ? 'L/h' : 'kW'}
              </span>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Signal sparklines */}
      <Card>
        <CardHeader>
          <CardTitle className="text-sm font-medium">Signal quality (last 24h)</CardTitle>
        </CardHeader>
        <CardContent>
          <SparklineTriplet series={signalHistory?.series ?? []} />
        </CardContent>
      </Card>
    </div>
  )
}
