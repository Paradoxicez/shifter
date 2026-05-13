/**
 * Plan 06-07 — AuditTable
 *
 * TanStack Table audit log with expand-row diff. Virtualization was removed
 * because variable expand height made transform offsets misalign with the
 * next row's start position; re-add only if profiler shows render-time
 * pressure at realistic operator-install audit volumes.
 */

import { Fragment, useState } from 'react'
import {
  useReactTable,
  getCoreRowModel,
  flexRender,
  type ColumnDef,
} from '@tanstack/react-table'
import { ChevronDown, ChevronRight, Copy } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { toast } from 'sonner'
import { formatDistanceToNow } from 'date-fns'
import type { AuditRow } from '@/hooks/useAudit'
import { AuditRowExpand } from './AuditRowExpand'

interface AuditTableProps {
  rows: AuditRow[]
  hasNextPage?: boolean
  onLoadMore?: () => void
  isLoadingMore?: boolean
}

export function AuditTable({ rows, hasNextPage, onLoadMore, isLoadingMore }: AuditTableProps) {
  const [expandedRows, setExpandedRows] = useState<Set<string>>(new Set())

  function toggleExpand(id: string) {
    setExpandedRows((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const columns: ColumnDef<AuditRow>[] = [
    {
      id: 'time',
      header: 'Time',
      size: 120,
      cell: ({ row }) => (
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="text-xs text-muted-foreground cursor-default">
              {formatDistanceToNow(new Date(row.original.time), { addSuffix: true })}
            </span>
          </TooltipTrigger>
          <TooltipContent>
            <span className="font-mono text-xs">{row.original.time}</span>
          </TooltipContent>
        </Tooltip>
      ),
    },
    {
      id: 'user',
      header: 'User',
      size: 200,
      cell: ({ row }) => (
        <span className="text-sm truncate max-w-[200px]">
          {row.original.user_email || 'system'}
        </span>
      ),
    },
    {
      id: 'action',
      header: 'Action',
      size: 200,
      cell: ({ row }) => (
        <span className="font-mono text-xs">{row.original.action}</span>
      ),
    },
    {
      id: 'entity',
      header: 'Entity',
      size: 240,
      cell: ({ row }) => (
        <span className="font-mono text-xs">
          {row.original.entity_type}:{row.original.entity_id.slice(0, 8)}
        </span>
      ),
    },
    {
      id: 'request_id',
      header: 'Request ID',
      size: 120,
      cell: ({ row }) => {
        const reqId = row.original.request_id
        if (!reqId) return <span className="text-muted-foreground">—</span>
        return (
          <button
            type="button"
            className="font-mono text-xs hover:underline"
            onClick={() => {
              navigator.clipboard.writeText(reqId).then(() => toast.success('Copied'))
            }}
            title="Click to copy"
          >
            <span className="flex items-center gap-1">
              r/{reqId.slice(0, 6)}
              <Copy className="h-3 w-3 text-muted-foreground" />
            </span>
          </button>
        )
      },
    },
    {
      id: 'expand',
      header: '',
      size: 32,
      cell: ({ row }) => {
        // Hide chevron when there is nothing to diff (auth events, etc.).
        const hasDiff = row.original.before !== null || row.original.after !== null
        if (!hasDiff) return null
        return (
          <Button
            variant="ghost"
            size="icon"
            className="h-6 w-6"
            onClick={() => toggleExpand(row.original.id)}
            aria-label={expandedRows.has(row.original.id) ? 'Collapse row' : 'Expand row'}
          >
            {expandedRows.has(row.original.id) ? (
              <ChevronDown className="h-3 w-3" />
            ) : (
              <ChevronRight className="h-3 w-3" />
            )}
          </Button>
        )
      },
    },
  ]

  const table = useReactTable({
    data: rows,
    columns,
    getCoreRowModel: getCoreRowModel(),
  })

  const { rows: tableRows } = table.getRowModel()

  if (rows.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-16 text-muted-foreground">
        <p className="text-sm">No audit rows match the current filters.</p>
        <p className="text-xs mt-1">Try widening the date range or clearing filters.</p>
      </div>
    )
  }

  return (
    <TooltipProvider>
    <div className="flex flex-col gap-2">
      <div
        className="overflow-auto border rounded-lg"
        style={{ maxHeight: '600px' }}
      >
        <table className="w-full text-sm">
          <thead className="sticky top-0 bg-background z-10 border-b">
            {table.getHeaderGroups().map((hg) => (
              <tr key={hg.id}>
                {hg.headers.map((h) => (
                  <th
                    key={h.id}
                    style={{ width: h.column.getSize() }}
                    className="text-left px-3 py-2 text-xs font-medium text-muted-foreground"
                  >
                    {flexRender(h.column.columnDef.header, h.getContext())}
                  </th>
                ))}
              </tr>
            ))}
          </thead>
          <tbody>
            {tableRows.map((row) => {
              const isExpanded = expandedRows.has(row.original.id)
              return (
                <Fragment key={row.id}>
                  <tr className="border-b hover:bg-muted/30 transition-colors">
                    {row.getVisibleCells().map((cell) => (
                      <td
                        key={cell.id}
                        style={{ width: cell.column.getSize() }}
                        className="px-3 py-2 align-middle"
                      >
                        {flexRender(cell.column.columnDef.cell, cell.getContext())}
                      </td>
                    ))}
                  </tr>
                  {isExpanded && (
                    <tr>
                      <td colSpan={columns.length} className="px-3 py-2">
                        <AuditRowExpand
                          before={row.original.before}
                          after={row.original.after}
                        />
                      </td>
                    </tr>
                  )}
                </Fragment>
              )
            })}
          </tbody>
        </table>
      </div>

      {hasNextPage && (
        <div className="flex justify-center">
          <Button
            variant="outline"
            size="sm"
            onClick={onLoadMore}
            disabled={isLoadingMore}
          >
            {isLoadingMore ? 'Loading…' : 'Load more'}
          </Button>
        </div>
      )}
    </div>
    </TooltipProvider>
  )
}
