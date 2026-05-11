/**
 * KpiCard — Plan 04-07 Task 1
 *
 * Single KPI metric tile. Supports 4 variants: today, instant, delta, online.
 * Color contract per UI-SPEC §KPI tile color contract:
 *   - water + positive delta → text-warning (potential leak signal)
 *   - electricity any delta → text-muted-foreground (normal fluctuation)
 *   - zero → Minus icon + muted
 *   - null comparison → em-dash + tooltip "Comparison available after 24 hours"
 */

import {
  AlertCircle,
  CheckCircle2,
  Minus,
  TrendingDown,
  TrendingUp,
  XCircle,
} from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

export interface KpiCardProps {
  label: string
  utility: 'water' | 'electricity'
  variant: 'today' | 'instant' | 'delta' | 'online'
  value: number | null
  unit?: string
  /** delta variant only */
  deltaAbs?: number | null
  deltaPct?: number | null
  /** online variant only — total device count */
  total?: number
  /** E2E selector — data-kpi="today|instant|delta|online" */
  'data-kpi'?: string
}

// ---------------------------------------------------------------------------
// Delta footer
// ---------------------------------------------------------------------------

interface DeltaFooterProps {
  utility: 'water' | 'electricity'
  deltaAbs: number | null | undefined
  deltaPct: number | null | undefined
}

function DeltaFooter({ utility, deltaAbs, deltaPct }: DeltaFooterProps) {
  // No comparison data yet
  if (deltaAbs == null && deltaPct == null) {
    return (
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="cursor-default text-sm font-mono text-muted-foreground" aria-label="No comparison available">
              —
            </span>
          </TooltipTrigger>
          <TooltipContent>Comparison available after 24 hours of history.</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    )
  }

  const abs = deltaAbs ?? 0
  const pct = deltaPct ?? 0

  // Zero delta
  if (abs === 0 && pct === 0) {
    return (
      <span className="flex items-center gap-1 text-sm font-mono text-muted-foreground">
        <Minus className="h-3 w-3" aria-hidden="true" />
        <span>0%</span>
      </span>
    )
  }

  const isPositive = pct > 0
  const sign = isPositive ? '+' : ''
  const pctText = `${sign}${pct.toFixed(1)}%`

  // Color: water + positive → warning; everything else → muted
  const isWarning = utility === 'water' && isPositive

  const colorClass = isWarning ? 'text-warning' : 'text-muted-foreground'

  const Icon = isPositive ? TrendingUp : TrendingDown

  return (
    <span className={cn('flex items-center gap-1 text-sm font-mono', colorClass)}>
      <Icon className="h-3 w-3" aria-hidden="true" />
      <span>{pctText}</span>
    </span>
  )
}

// ---------------------------------------------------------------------------
// Online footer
// ---------------------------------------------------------------------------

interface OnlineFooterProps {
  value: number | null
  total: number | undefined
}

function OnlineFooter({ value, total }: OnlineFooterProps) {
  const online = value ?? 0
  const totalCount = total ?? 0

  const countText = `${online} / ${totalCount}`

  let Icon = CheckCircle2
  let colorClass = 'text-success'

  if (totalCount === 0 || online === 0) {
    Icon = XCircle
    colorClass = 'text-destructive'
  } else if (online < totalCount) {
    Icon = AlertCircle
    colorClass = 'text-warning'
  }

  return (
    <span className={cn('flex items-center gap-1 text-sm font-mono', colorClass)}>
      <Icon className="h-3 w-3" aria-hidden="true" />
      <span>{countText}</span>
    </span>
  )
}

// ---------------------------------------------------------------------------
// KpiCard
// ---------------------------------------------------------------------------

export function KpiCard({ label, utility, variant, value, unit, deltaAbs, deltaPct, total, 'data-kpi': dataKpi }: KpiCardProps) {
  const displayValue = value !== null && value !== undefined
    ? new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(value)
    : '—'

  return (
    <Card className="gap-3" data-kpi={dataKpi ?? variant}>
      <CardHeader className="pb-0">
        <CardTitle className="text-sm font-semibold text-muted-foreground">{label}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-1">
        <div className="flex items-baseline gap-1">
          <span className="text-2xl font-semibold leading-8">{displayValue}</span>
          {unit && value !== null && (
            <span className="text-sm font-mono text-muted-foreground">{unit}</span>
          )}
        </div>

        {/* Footer row per variant */}
        {variant === 'delta' && (
          <DeltaFooter utility={utility} deltaAbs={deltaAbs} deltaPct={deltaPct} />
        )}
        {variant === 'online' && (
          <OnlineFooter value={value} total={total} />
        )}
      </CardContent>
    </Card>
  )
}
