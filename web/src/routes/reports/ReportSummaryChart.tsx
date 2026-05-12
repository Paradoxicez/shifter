/**
 * ReportSummaryChart — Plan 05-09 Task 2
 *
 * Renders one BarChart per utility class present in period_rows data.
 * Capability-gated: single-capability install shows 1 chart; both shows 2.
 * The data itself already excludes absent capabilities (plan 05-03 assembler).
 *
 * Mirrors dashboard ConsumptionChart styling (ChartContainer + Recharts,
 * stroke=var(--primary), fill=var(--primary) at 0.1 opacity, grid=var(--border)).
 */

import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from 'recharts'
import { format, parseISO } from 'date-fns'
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from '@/components/ui/chart'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface PeriodRow {
  period: string
  consumption: number
  delta_vs_prior?: { absolute: number; percent: number }
  delta_vs_yoy?: { absolute: number; percent: number }
}

interface ReportSummaryChartProps {
  periodRows: PeriodRow[]
  /** If provided, only charts for listed utility classes are rendered.
   *  When undefined, a single generic chart is rendered (scope=meter or scope=site). */
  utilityClasses?: string[]
}

// ---------------------------------------------------------------------------
// Chart config
// ---------------------------------------------------------------------------

const chartConfig: ChartConfig = {
  consumption: { label: 'Consumption', color: 'var(--primary)' },
}

// ---------------------------------------------------------------------------
// Single chart
// ---------------------------------------------------------------------------

function PeriodBarChart({ rows, title }: { rows: PeriodRow[]; title: string }) {
  const data = rows.map((r) => ({
    period: r.period,
    consumption: r.consumption,
  }))

  const xFormatter = (v: string) => {
    try {
      return format(parseISO(v), 'MMM d')
    } catch {
      return v
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{title}</CardTitle>
      </CardHeader>
      <CardContent>
        <ChartContainer config={chartConfig} className="h-48 sm:h-56 w-full">
          <BarChart data={data} margin={{ top: 8, right: 8, bottom: 0, left: 0 }}>
            <CartesianGrid stroke="var(--border)" vertical={false} />
            <XAxis
              dataKey="period"
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
            <Bar
              dataKey="consumption"
              fill="var(--primary)"
              fillOpacity={0.8}
              radius={[2, 2, 0, 0]}
            />
          </BarChart>
        </ChartContainer>
      </CardContent>
    </Card>
  )
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function ReportSummaryChart({ periodRows, utilityClasses }: ReportSummaryChartProps) {
  if (periodRows.length === 0) return null

  // When utility classes specified (scope=all with capabilities), render one chart per class
  if (utilityClasses && utilityClasses.length > 1) {
    return (
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        {utilityClasses.map((uc) => (
          <PeriodBarChart
            key={uc}
            rows={periodRows}
            title={uc.charAt(0).toUpperCase() + uc.slice(1) + ' consumption'}
          />
        ))}
      </div>
    )
  }

  const title =
    utilityClasses && utilityClasses.length === 1
      ? utilityClasses[0].charAt(0).toUpperCase() + utilityClasses[0].slice(1) + ' consumption'
      : 'Consumption'

  return <PeriodBarChart rows={periodRows} title={title} />
}
