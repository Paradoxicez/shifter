/**
 * ConsumptionChart — Plan 04-08 Task 2
 *
 * Recharts AreaChart wrapped in ChartContainer displaying cumulative
 * consumption deltas over time buckets.
 *
 * UI-SPEC §Color §Chart:
 *   - stroke="var(--primary)"
 *   - fill="var(--primary)" with fillOpacity={0.1}
 *   - grid stroke="var(--border)"
 *
 * Live-mode pulse marker (D-13):
 *   - Only when liveMode=true (preset is 'today' or '24h')
 *   - Rendered as a div overlay at rightmost data point
 *   - animate-pulse motion-reduce:animate-none (respects prefers-reduced-motion)
 */

import { Area, AreaChart, CartesianGrid, ReferenceDot, XAxis, YAxis } from 'recharts'
import { format } from 'date-fns'
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from '@/components/ui/chart'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface DataPoint {
  bucket: string
  cumulative_delta: number
}

interface ConsumptionChartProps {
  data: DataPoint[]
  utility: 'water' | 'electricity'
  liveMode: boolean
  bucketSec: number
}

// ---------------------------------------------------------------------------
// Chart config
// ---------------------------------------------------------------------------

const chartConfig: ChartConfig = {
  cumulative_delta: { label: 'Consumption', color: 'var(--primary)' },
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function ConsumptionChart({ data, liveMode, bucketSec }: ConsumptionChartProps) {
  const xFormatter = (v: string) => {
    const d = new Date(v)
    if (bucketSec < 3600) return format(d, 'HH:mm')  // 5-min buckets
    if (bucketSec < 86400) return format(d, 'HH:mm') // hourly
    return format(d, 'MMM d')                         // daily
  }

  const lastPoint = data.length > 0 ? data[data.length - 1] : null

  // Empty state: no data for this range — render placeholder instead of blank chart.
  if (data.length === 0) {
    return (
      <div className="h-48 sm:h-56 md:h-64 lg:h-72 flex items-center justify-center">
        <p className="text-sm text-muted-foreground">No data for this range</p>
      </div>
    )
  }

  return (
    <div className="relative">
      <ChartContainer config={chartConfig} className="h-48 sm:h-56 md:h-64 lg:h-72 w-full">
        <AreaChart data={data} margin={{ top: 8, right: 8, bottom: 0, left: 0 }}>
          <CartesianGrid stroke="var(--border)" vertical={false} />
          <XAxis
            dataKey="bucket"
            tickFormatter={xFormatter}
            tick={{ fill: 'var(--muted-foreground)', fontSize: 12 }}
            tickLine={false}
            axisLine={false}
          />
          <YAxis
            tick={{ fill: 'var(--muted-foreground)', fontSize: 12 }}
            tickLine={false}
            axisLine={false}
            width={40}
          />
          <ChartTooltip content={<ChartTooltipContent />} />
          <Area
            type="monotone"
            dataKey="cumulative_delta"
            stroke="var(--primary)"
            fill="var(--primary)"
            fillOpacity={0.1}
            strokeWidth={2}
            dot={false}
            activeDot={{ r: 4 }}
          />
          {liveMode && lastPoint && (
            <ReferenceDot
              x={lastPoint.bucket}
              y={lastPoint.cumulative_delta}
              r={4}
              fill="var(--primary)"
              stroke="var(--primary)"
              className="animate-pulse motion-reduce:animate-none"
            />
          )}
        </AreaChart>
      </ChartContainer>
    </div>
  )
}
