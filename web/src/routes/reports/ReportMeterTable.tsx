/**
 * ReportMeterTable — Plan 05-09 Task 2
 *
 * Per-meter breakdown table: Meter / Site / Utility / Total.
 * Rendered only when scope != 'meter' (controlled by ReportResultPanel).
 *
 * Uses shadcn Table primitives.
 */

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

interface MeterRow {
  id: string
  name: string
  site_name: string
  utility_class: string
  consumption: number
}

interface ReportMeterTableProps {
  rows: MeterRow[]
  unitLabel?: string
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function ReportMeterTable({ rows, unitLabel = '' }: ReportMeterTableProps) {
  if (rows.length === 0) return null

  return (
    <div className="rounded-md border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Meter</TableHead>
            <TableHead>Site</TableHead>
            <TableHead>Utility</TableHead>
            <TableHead className="text-right">Total</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((row) => (
            <TableRow key={row.id}>
              <TableCell className="font-medium">{row.name}</TableCell>
              <TableCell className="text-muted-foreground">{row.site_name}</TableCell>
              <TableCell>
                <Badge variant="outline" className="capitalize">
                  {row.utility_class}
                </Badge>
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {row.consumption.toFixed(3)}{unitLabel ? ` ${unitLabel}` : ''}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
