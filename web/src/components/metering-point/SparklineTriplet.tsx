/**
 * SparklineTriplet — Plan 04-09 Task 1
 *
 * Three Recharts AreaCharts side-by-side (battery, RSSI, SNR) — D-17.
 *
 * Fill color per UI-SPEC bands:
 *   Battery: healthy (>30%) → success/10; warning (10–30%) → warning/15; critical (<10%) → destructive/15
 *   RSSI:    healthy (>-105 dBm) → success/10; warning (-115..-105) → warning/15; critical (<-115) → destructive/15
 *   SNR:     healthy (>0 dB) → success/10; warning (-10..0) → warning/15; critical (<-10) → destructive/15
 *
 * Each cell: label uppercase muted + latest value mono + AreaChart h-16
 * data-metric="battery|rssi|snr" + data-band="healthy|warning|critical" for tests.
 */

import { Area, AreaChart, ResponsiveContainer } from 'recharts'

export interface SignalSeriesPoint {
  bucket: string
  battery_pct: number | null
  rssi: number | null
  snr: number | null
}

interface SparklineTripletProps {
  series: SignalSeriesPoint[]
}

// Band classifiers
function batteryBand(v: number): 'critical' | 'warning' | 'healthy' {
  if (v < 10) return 'critical'
  if (v <= 30) return 'warning'
  return 'healthy'
}

function rssiBand(v: number): 'critical' | 'warning' | 'healthy' {
  if (v < -115) return 'critical'
  if (v <= -105) return 'warning'
  return 'healthy'
}

function snrBand(v: number): 'critical' | 'warning' | 'healthy' {
  if (v < -10) return 'critical'
  if (v <= 0) return 'warning'
  return 'healthy'
}

// Fill colors by band
const FILL: Record<'critical' | 'warning' | 'healthy', string> = {
  critical: 'var(--destructive)',
  warning: 'var(--warning)',
  healthy: 'var(--success)',
}

const FILL_OPACITY: Record<'critical' | 'warning' | 'healthy', number> = {
  critical: 0.15,
  warning: 0.15,
  healthy: 0.1,
}

interface SparklineProps {
  label: string
  metric: 'battery' | 'rssi' | 'snr'
  dataKey: string
  data: { bucket: string; value: number | null }[]
  latestValue: number | null
  band: 'critical' | 'warning' | 'healthy'
  unit: string
}

function Sparkline({ label, metric, dataKey, data, latestValue, band, unit }: SparklineProps) {
  return (
    <div data-metric={metric} data-band={band} className="flex flex-col gap-1 flex-1 min-w-0">
      <div className="flex items-center justify-between">
        <span className="text-xs text-muted-foreground uppercase tracking-wide font-medium">
          {label}
        </span>
        <span className="text-xs font-mono font-semibold">
          {latestValue !== null ? `${latestValue}${unit}` : '—'}
        </span>
      </div>
      <div className="h-16 w-full">
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={data} margin={{ top: 2, right: 0, bottom: 0, left: 0 }}>
            <Area
              type="monotone"
              dataKey={dataKey}
              stroke="var(--muted-foreground)"
              strokeWidth={1}
              fill={FILL[band]}
              fillOpacity={FILL_OPACITY[band]}
              dot={false}
              isAnimationActive={false}
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </div>
  )
}

export function SparklineTriplet({ series }: SparklineTripletProps) {
  const latest = series.length > 0 ? series[series.length - 1] : null

  const batteryData = series.map((p) => ({ bucket: p.bucket, value: p.battery_pct }))
  const rssiData = series.map((p) => ({ bucket: p.bucket, value: p.rssi }))
  const snrData = series.map((p) => ({ bucket: p.bucket, value: p.snr }))

  const latestBattery = latest?.battery_pct ?? null
  const latestRssi = latest?.rssi ?? null
  const latestSnr = latest?.snr ?? null

  const bBand = latestBattery !== null ? batteryBand(latestBattery) : 'healthy'
  const rBand = latestRssi !== null ? rssiBand(latestRssi) : 'healthy'
  const sBand = latestSnr !== null ? snrBand(latestSnr) : 'healthy'

  return (
    <div className="flex flex-col gap-4 sm:flex-row sm:gap-6">
      <Sparkline
        label="Battery"
        metric="battery"
        dataKey="value"
        data={batteryData}
        latestValue={latestBattery}
        band={bBand}
        unit="%"
      />
      <Sparkline
        label="RSSI"
        metric="rssi"
        dataKey="value"
        data={rssiData}
        latestValue={latestRssi}
        band={rBand}
        unit=" dBm"
      />
      <Sparkline
        label="SNR"
        metric="snr"
        dataKey="value"
        data={snrData}
        latestValue={latestSnr}
        band={sBand}
        unit=" dB"
      />
    </div>
  )
}
