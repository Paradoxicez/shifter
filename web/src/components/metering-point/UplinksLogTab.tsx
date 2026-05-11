/**
 * UplinksLogTab — Plan 04-09 Task 2
 *
 * TanStack Table-backed uplinks log with:
 *   - 100 rows initial load + "Load more" (+100, cap 500) — D-16
 *   - Quality filter chips via ToggleGroup + URL state ?quality=
 *   - Expandable row showing HexPayloadCell (left) + JsonTree (right)
 *   - Virtualization via @tanstack/react-virtual when rows.length > 200 (D-16, T-04-09-04)
 *   - Empty filter result: "No uplinks match your filters" + "Clear filters" button
 *
 * Columns (9 per UI-SPEC §Uplinks log):
 *   1. Expand chevron
 *   2. Time (HH:mm:ss if today, MMM d HH:mm:ss otherwise)
 *   3. Quality badge
 *   4. Cumulative (right-aligned mono + unit suffix)
 *   5. Instant
 *   6. Battery
 *   7. RSSI
 *   8. SNR
 *   9. fcnt
 */

import { useState, useRef, useCallback, useEffect } from 'react'
import {
  flexRender,
  getCoreRowModel,
  getExpandedRowModel,
  useReactTable,
  type ColumnDef,
  type ExpandedState,
} from '@tanstack/react-table'
import { useVirtualizer } from '@tanstack/react-virtual'
import { format, isToday } from 'date-fns'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useSearchParams } from 'react-router-dom'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { apiFetch } from '@/lib/api'
import { HexPayloadCell } from './HexPayloadCell'
import { JsonTree } from './JsonTree'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface UplinkRow {
  time: string
  cumulative_value: number | null
  instant_value: number | null
  battery_pct: number | null
  rssi: number | null
  snr: number | null
  fcnt: number | null
  quality: string
  raw_payload_hex: string
  decoded_object: Record<string, unknown>
}

interface UplinksResponse {
  uplinks: UplinkRow[]
  has_more: boolean
  next_before: string | null
}

interface UplinksLogTabProps {
  meteringPointId: string
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const QUALITY_OPTIONS = ['ok', 'decode_fail', 'missing_canonical', 'out_of_range', 'duplicate_fcnt']
const VIRTUALIZE_THRESHOLD = 200
const ROW_CAP = 500
const PAGE_SIZE = 100

const QUALITY_COLORS: Record<string, string> = {
  ok: 'bg-success/20 text-success border-success/30',
  decode_fail: 'bg-destructive/20 text-destructive border-destructive/30',
  missing_canonical: 'bg-warning/20 text-warning border-warning/30',
  out_of_range: 'bg-warning/20 text-warning border-warning/30',
  duplicate_fcnt: 'bg-muted text-muted-foreground',
}

function formatTime(iso: string): string {
  const d = new Date(iso)
  return isToday(d) ? format(d, 'HH:mm:ss') : format(d, 'MMM d HH:mm:ss')
}

function QualityCell({ quality }: { quality: string }) {
  const cls = QUALITY_COLORS[quality] ?? 'bg-muted text-muted-foreground'
  return (
    <Badge variant="outline" className={`text-xs ${cls}`}>
      {quality.replace(/_/g, ' ')}
    </Badge>
  )
}

// ---------------------------------------------------------------------------
// Column definitions
// ---------------------------------------------------------------------------

function buildColumns(): ColumnDef<UplinkRow>[] {
  return [
    {
      id: 'expander',
      header: '',
      size: 32,
      cell: ({ row }) => (
        <button
          type="button"
          onClick={row.getToggleExpandedHandler()}
          aria-expanded={row.getIsExpanded()}
          className="flex items-center justify-center w-6 h-6"
        >
          {row.getIsExpanded() ? (
            <ChevronDown className="h-3 w-3 text-muted-foreground" />
          ) : (
            <ChevronRight className="h-3 w-3 text-muted-foreground" />
          )}
        </button>
      ),
    },
    {
      accessorKey: 'time',
      header: 'Time',
      cell: ({ getValue }) => {
        const iso = getValue<string>()
        return (
          <span title={iso} className="text-xs font-mono text-muted-foreground">
            {formatTime(iso)}
          </span>
        )
      },
    },
    {
      accessorKey: 'quality',
      header: 'Quality',
      cell: ({ getValue }) => <QualityCell quality={getValue<string>()} />,
    },
    {
      accessorKey: 'cumulative_value',
      header: () => <span className="text-right w-full block">Cumulative</span>,
      cell: ({ getValue }) => {
        const v = getValue<number | null>()
        return (
          <span className="text-xs font-mono text-right block">
            {v !== null ? v.toLocaleString() : '—'}
          </span>
        )
      },
    },
    {
      accessorKey: 'instant_value',
      header: 'Instant',
      cell: ({ getValue }) => {
        const v = getValue<number | null>()
        return <span className="text-xs font-mono">{v !== null ? v : '—'}</span>
      },
    },
    {
      accessorKey: 'battery_pct',
      header: 'Battery',
      cell: ({ getValue }) => {
        const v = getValue<number | null>()
        return <span className="text-xs font-mono">{v !== null ? `${v}%` : '—'}</span>
      },
    },
    {
      accessorKey: 'rssi',
      header: 'RSSI',
      cell: ({ getValue }) => {
        const v = getValue<number | null>()
        const cls = v !== null && v < -105 ? 'text-destructive' : ''
        return <span className={`text-xs font-mono ${cls}`}>{v !== null ? `${v} dBm` : '—'}</span>
      },
    },
    {
      accessorKey: 'snr',
      header: 'SNR',
      cell: ({ getValue }) => {
        const v = getValue<number | null>()
        const cls = v !== null && v < 0 ? 'text-warning' : ''
        return <span className={`text-xs font-mono ${cls}`}>{v !== null ? `${v} dB` : '—'}</span>
      },
    },
    {
      accessorKey: 'fcnt',
      header: () => <span className="text-right w-full block">fcnt</span>,
      cell: ({ getValue }) => {
        const v = getValue<number | null>()
        return (
          <span className="text-xs font-mono text-right block">
            {v !== null ? v : '—'}
          </span>
        )
      },
    },
  ]
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function UplinksLogTab({ meteringPointId }: UplinksLogTabProps) {
  const [params, setParams] = useSearchParams()
  const qualityParam = params.get('quality') ?? ''
  const activeQualities = qualityParam ? qualityParam.split(',').filter(Boolean) : []

  const [rows, setRows] = useState<UplinkRow[]>([])
  const [hasMore, setHasMore] = useState(false)
  const [nextBefore, setNextBefore] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [expanded, setExpanded] = useState<ExpandedState>({})

  // Build quality filter query string
  const qualityQS = activeQualities.length > 0
    ? `&quality=${activeQualities.join(',')}`
    : ''

  const fetchUplinks = useCallback(async (before: string | null, replace: boolean) => {
    const beforeQS = before ? `&before=${encodeURIComponent(before)}` : ''
    const url = `/api/metering-points/${meteringPointId}/uplinks?limit=${PAGE_SIZE}${beforeQS}${qualityQS}`
    try {
      const data = await apiFetch<UplinksResponse>(url)
      setRows((prev) => {
        const next = replace ? data.uplinks : [...prev, ...data.uplinks]
        // Apply row cap
        return next.slice(0, ROW_CAP)
      })
      setHasMore(data.has_more)
      setNextBefore(data.next_before)
    } finally {
      setLoading(false)
      setLoadingMore(false)
    }
  }, [meteringPointId, qualityQS])

  // Fetch on mount and when quality filter changes
  useEffect(() => {
    setLoading(true)
    setRows([])
    fetchUplinks(null, true)
  }, [fetchUplinks])

  const handleLoadMore = () => {
    if (!hasMore || loadingMore || rows.length >= ROW_CAP) return
    setLoadingMore(true)
    fetchUplinks(nextBefore, false)
  }

  const clearFilters = () => {
    setParams((p) => {
      p.delete('quality')
      return p
    })
  }

  const handleQualityToggle = (values: string[]) => {
    setParams((p) => {
      if (values.length === 0) {
        p.delete('quality')
      } else {
        p.set('quality', values.join(','))
      }
      return p
    })
  }

  const columns = buildColumns()

  const table = useReactTable({
    data: rows,
    columns,
    state: { expanded },
    onExpandedChange: setExpanded,
    getCoreRowModel: getCoreRowModel(),
    getExpandedRowModel: getExpandedRowModel(),
    getRowCanExpand: () => true,
  })

  const tableRows = table.getRowModel().rows

  // Virtualization
  const parentRef = useRef<HTMLDivElement>(null)
  const shouldVirtualize = rows.length > VIRTUALIZE_THRESHOLD

  const virtualizer = useVirtualizer({
    count: tableRows.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 40,
    overscan: 10,
    enabled: shouldVirtualize,
  })

  const virtualItems = shouldVirtualize ? virtualizer.getVirtualItems() : []
  const totalSize = shouldVirtualize ? virtualizer.getTotalSize() : 0

  return (
    <div className="space-y-4">
      {/* Quality filter chips — always visible */}
      <ToggleGroup
        type="multiple"
        value={activeQualities}
        onValueChange={handleQualityToggle}
        variant="outline"
        className="flex-wrap"
      >
        {QUALITY_OPTIONS.map((q) => (
          <ToggleGroupItem key={q} value={q} className="text-xs">
            {q.replace(/_/g, ' ')}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>

      {/* Loading state */}
      {loading && (
        <div className="p-4 text-sm text-muted-foreground">Loading uplinks…</div>
      )}

      {/* Empty state */}
      {!loading && rows.length === 0 && (
        <div className="flex flex-col items-center gap-4 py-12 text-center">
          <p className="text-sm text-muted-foreground">No uplinks match your filters</p>
          {activeQualities.length > 0 && (
            <Button variant="outline" size="sm" onClick={clearFilters}>
              Clear filters
            </Button>
          )}
        </div>
      )}

      {/* Table */}
      {!loading && rows.length > 0 && (
      <div
        ref={parentRef}
        className="rounded-md border overflow-auto"
        style={{ maxHeight: shouldVirtualize ? '600px' : undefined }}
        data-testid={shouldVirtualize ? 'virtualized' : undefined}
      >
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map((hg) => (
              <TableRow key={hg.id}>
                {hg.headers.map((h) => (
                  <TableHead key={h.id} style={{ width: h.getSize() }}>
                    {flexRender(h.column.columnDef.header, h.getContext())}
                  </TableHead>
                ))}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {shouldVirtualize ? (
              <>
                <tr style={{ height: virtualItems[0]?.start ?? 0 }} />
                {virtualItems.map((vi) => {
                  const row = tableRows[vi.index]
                  if (!row) return null
                  return (
                    <>
                      <TableRow key={row.id} data-index={vi.index}>
                        {row.getVisibleCells().map((cell) => (
                          <TableCell key={cell.id}>
                            {flexRender(cell.column.columnDef.cell, cell.getContext())}
                          </TableCell>
                        ))}
                      </TableRow>
                      {row.getIsExpanded() && (
                        <TableRow key={`${row.id}-expanded`}>
                          <TableCell colSpan={columns.length}>
                            <ExpandedRowContent row={row.original} />
                          </TableCell>
                        </TableRow>
                      )}
                    </>
                  )
                })}
                <tr style={{ height: Math.max(0, totalSize - (virtualItems[virtualItems.length - 1]?.end ?? 0)) }} />
              </>
            ) : (
              tableRows.map((row) => (
                <>
                  <TableRow key={row.id}>
                    {row.getVisibleCells().map((cell) => (
                      <TableCell key={cell.id}>
                        {flexRender(cell.column.columnDef.cell, cell.getContext())}
                      </TableCell>
                    ))}
                  </TableRow>
                  {row.getIsExpanded() && (
                    <TableRow key={`${row.id}-expanded`}>
                      <TableCell colSpan={columns.length}>
                        <ExpandedRowContent row={row.original} />
                      </TableCell>
                    </TableRow>
                  )}
                </>
              ))
            )}
          </TableBody>
        </Table>
      </div>
      )}

      {/* Load more / cap indicator — only when table is visible */}
      {!loading && rows.length > 0 && (
        rows.length >= ROW_CAP ? (
          <p className="text-center text-xs text-muted-foreground">
            Showing 500 rows — use date range to narrow results
          </p>
        ) : hasMore ? (
          <div className="flex justify-center">
            <Button variant="outline" size="sm" onClick={handleLoadMore} disabled={loadingMore}>
              {loadingMore ? 'Loading…' : 'Load more'}
            </Button>
          </div>
        ) : null
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Expanded row
// ---------------------------------------------------------------------------

function ExpandedRowContent({ row }: { row: UplinkRow }) {
  return (
    <div className="grid grid-cols-1 gap-4 p-2 md:grid-cols-2">
      <HexPayloadCell hex={row.raw_payload_hex} />
      <div>
        <p className="text-xs text-muted-foreground uppercase tracking-wide font-medium mb-1">
          Decoded
        </p>
        <JsonTree value={row.decoded_object} defaultOpen />
      </div>
    </div>
  )
}
