/**
 * ReportPeriodTable — Plan 05-09 Task 2
 *
 * Period breakdown table: Period / Consumption / Δ prior / Δ YoY.
 *
 * D-03 silent fallback: "Δ YoY" column shown only when ANY row has delta_vs_yoy
 * populated — no warning, no empty column header.
 *
 * Uses shadcn Table primitives (no TanStack Table dependency for this simple table).
 */

import { format, parseISO } from 'date-fns'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface PeriodRow {
  period: string
  consumption: number
  delta_vs_prior?: { absolute: number; percent: number }
  delta_vs_yoy?: { absolute: number; percent: number }
}

interface ReportPeriodTableProps {
  rows: PeriodRow[]
  unitLabel?: string
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatPeriod(period: string): string {
  try {
    return format(parseISO(period), 'PPP')
  } catch {
    return period
  }
}

function DeltaBadge({ delta }: { delta: { absolute: number; percent: number } }) {
  const sign = delta.percent >= 0 ? '+' : ''
  const positive = delta.percent >= 0
  return (
    <Badge
      variant="outline"
      className={
        positive
          ? 'text-green-600 border-green-200 bg-green-50 dark:text-green-400 dark:border-green-800 dark:bg-green-950'
          : 'text-red-600 border-red-200 bg-red-50 dark:text-red-400 dark:border-red-800 dark:bg-red-950'
      }
    >
      {sign}{delta.percent.toFixed(1)}%
    </Badge>
  )
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function ReportPeriodTable({ rows, unitLabel = '' }: ReportPeriodTableProps) {
  if (rows.length === 0) return null

  // D-03: only show YoY column when ANY row has delta_vs_yoy populated
  const showYoY = rows.some((r) => r.delta_vs_yoy !== null && r.delta_vs_yoy !== undefined)

  return (
    <div className="rounded-md border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Period</TableHead>
            <TableHead className="text-right">Consumption</TableHead>
            <TableHead className="text-right">vs Prior</TableHead>
            {showYoY && <TableHead className="text-right">vs YoY</TableHead>}
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((row, i) => (
            <TableRow key={i}>
              <TableCell className="font-medium">{formatPeriod(row.period)}</TableCell>
              <TableCell className="text-right tabular-nums">
                {row.consumption.toFixed(3)}{unitLabel ? ` ${unitLabel}` : ''}
              </TableCell>
              <TableCell className="text-right">
                {row.delta_vs_prior ? (
                  <DeltaBadge delta={row.delta_vs_prior} />
                ) : (
                  <span className="text-muted-foreground text-xs">—</span>
                )}
              </TableCell>
              {showYoY && (
                <TableCell className="text-right">
                  {row.delta_vs_yoy ? (
                    <DeltaBadge delta={row.delta_vs_yoy} />
                  ) : (
                    <span className="text-muted-foreground text-xs">—</span>
                  )}
                </TableCell>
              )}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
