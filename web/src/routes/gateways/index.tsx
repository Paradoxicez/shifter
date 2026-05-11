import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
  type ColumnDef,
} from '@tanstack/react-table'
import {
  Archive,
  ArchiveRestore,
  MoreHorizontal,
  Pencil,
  Plus,
  PowerOff,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { Area, AreaChart } from 'recharts'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { ChartContainer, type ChartConfig } from '@/components/ui/chart'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Toggle } from '@/components/ui/toggle'
import { listGateways, restoreGateway, type Gateway } from '@/lib/gateways'
import { AddGatewayDialog } from './add-gateway-dialog'
import { DecommissionGatewayDialog } from './decommission-gateway-dialog'

/**
 * Choose a CSS variable token based on uplink success ratio.
 * UI-SPEC §Gateway list — RX/TX cell:
 *   ≥ 95% → --success
 *   70–94% → --warning
 *   < 70%  → --destructive
 * When no TX traffic was attempted, default to --success (nothing failing).
 */
export function statusTokenFor(g: Gateway): 'success' | 'warning' | 'destructive' {
  if (!g.stats_tx_24h || g.stats_tx_24h === 0) return 'success'
  const ratio = (g.stats_tx_ok_24h ?? 0) / g.stats_tx_24h
  if (ratio >= 0.95) return 'success'
  if (ratio >= 0.7) return 'warning'
  return 'destructive'
}

/**
 * Compact 24-bucket area chart of hourly RX counts. Uses the shadcn
 * ChartContainer + Recharts AreaChart and renders only with CSS variable
 * tokens — never with hex literals.
 *
 * Allowed fill tokens (per UI-SPEC §Status Encoding):
 *   - var(--success)      uplink success ≥ 95%
 *   - var(--warning)      uplink success 70–94%
 *   - var(--destructive)  uplink success <70%
 */
function Sparkline({
  data,
  fillToken,
}: {
  data: number[]
  fillToken: 'success' | 'warning' | 'destructive'
}) {
  const fill = `var(--${fillToken})`
  const chartData = data.map((v, i) => ({ hour: i, count: v }))
  const config: ChartConfig = { count: { label: 'RX', color: fill } }
  return (
    <ChartContainer config={config} className="h-9 w-32 aspect-auto">
      <AreaChart data={chartData} margin={{ top: 2, right: 2, bottom: 2, left: 2 }}>
        <Area
          type="monotone"
          dataKey="count"
          stroke={fill}
          fill={fill}
          fillOpacity={0.2}
          strokeWidth={1.5}
          isAnimationActive={false}
          dot={false}
        />
      </AreaChart>
    </ChartContainer>
  )
}

function StatusDot({ state }: { state: Gateway['state'] }) {
  const token =
    state === 'ONLINE' ? 'success' : state === 'OFFLINE' ? 'destructive' : 'muted-foreground'
  return (
    <span
      aria-label={state}
      className="inline-block h-2.5 w-2.5 rounded-full"
      style={{ backgroundColor: `var(--${token})` }}
    />
  )
}

function relativeFromNow(iso: string | null): string {
  if (!iso) return '—'
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return '—'
  const diff = Date.now() - t
  const min = Math.floor(diff / 60_000)
  if (min < 1) return 'just now'
  if (min < 60) return `${min} min ago`
  const hr = Math.floor(min / 60)
  if (hr < 24) return `${hr} hr ago`
  const day = Math.floor(hr / 24)
  return `${day} d ago`
}

/**
 * UI-SPEC §Gateways list page (GW-01, GW-03, D-02, D-32):
 *   - Page header: text-2xl "Gateways" + Plus-leading "Add gateway" button.
 *   - Toolbar: Search input (max-w-sm) + "Show archived" Toggle.
 *   - Table columns: Name (link) | Gateway ID (mono) | Region | Last seen |
 *     24h activity (RX/TX/success%) | Trend (sparkline) | Status dot | Actions.
 *   - Sort: status desc then last_seen desc (handled server-side in v2).
 *   - Empty state: "No gateways yet" + Add gateway CTA.
 *   - Loading: 5 Skeleton rows.
 *   - Show-archived ON: archived rows append at bottom with opacity-60 styling;
 *     only Restore appears in the overflow menu.
 */
export default function GatewaysPage() {
  const qc = useQueryClient()
  const [includeArchived, setIncludeArchived] = useState(false)
  const [search, setSearch] = useState('')
  const [createOpen, setCreateOpen] = useState(false)
  const [editGateway, setEditGateway] = useState<Gateway | null>(null)
  const [decommissionGateway, setDecommissionGateway] = useState<Gateway | null>(null)

  const gatewaysQuery = useQuery({
    queryKey: ['gateways', { includeArchived }],
    queryFn: () => listGateways({ includeArchived }),
  })

  const filtered = useMemo(() => {
    const rows = gatewaysQuery.data?.items ?? []
    if (!search) return rows
    const needle = search.toLowerCase()
    return rows.filter(
      (g) =>
        g.name.toLowerCase().includes(needle) ||
        g.gateway_id.toLowerCase().includes(needle),
    )
  }, [gatewaysQuery.data, search])

  const restoreMutation = useMutation({
    mutationFn: (id: string) => restoreGateway(id),
    onSuccess: (restored) => {
      qc.invalidateQueries({ queryKey: ['gateways'] })
      toast.success(`Gateway ${restored.name} restored.`)
    },
    onError: () => {
      toast.error('Could not restore gateway')
    },
  })

  const columns: ColumnDef<Gateway>[] = [
    {
      accessorKey: 'name',
      header: 'Name',
      cell: ({ row }) => (
        <Link
          to={`/gateways/${row.original.id}`}
          className="text-sm font-semibold text-primary hover:underline"
        >
          {row.original.name}
        </Link>
      ),
    },
    {
      accessorKey: 'gateway_id',
      header: 'Gateway ID',
      cell: ({ row }) => (
        <span className="text-sm font-mono">{row.original.gateway_id}</span>
      ),
    },
    {
      accessorKey: 'region',
      header: 'Region',
      cell: ({ row }) => <span className="text-sm">{row.original.region}</span>,
    },
    {
      accessorKey: 'last_seen_at',
      header: 'Last seen',
      cell: ({ row }) => (
        <span className="text-xs text-muted-foreground">
          {relativeFromNow(row.original.last_seen_at)}
        </span>
      ),
    },
    {
      id: 'activity',
      header: '24h activity',
      cell: ({ row }) => {
        const g = row.original
        const successPct =
          g.stats_tx_24h && g.stats_tx_24h > 0
            ? Math.round(((g.stats_tx_ok_24h ?? 0) / g.stats_tx_24h) * 100)
            : null
        const token = statusTokenFor(g)
        return (
          <div className="flex flex-col gap-1">
            <div className="flex items-center gap-2">
              <span className="text-xs text-muted-foreground w-8">RX 24h</span>
              <span className="text-sm font-mono font-semibold">
                {g.stats_rx_24h ?? '—'}
              </span>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-xs text-muted-foreground w-8">TX 24h</span>
              <span className="text-sm font-mono font-semibold">
                {g.stats_tx_24h ?? '—'}
              </span>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-xs text-muted-foreground w-8">OK</span>
              <span
                className="text-sm font-mono font-semibold"
                style={{ color: `var(--${token})` }}
              >
                {successPct != null ? `${successPct}%` : '—'}
              </span>
            </div>
          </div>
        )
      },
    },
    {
      id: 'sparkline',
      header: 'Trend',
      cell: ({ row }) => {
        const g = row.original
        const data = g.stats_sparkline?.rx ?? new Array(24).fill(0)
        return <Sparkline data={data} fillToken={statusTokenFor(g)} />
      },
    },
    {
      id: 'status',
      header: 'Status',
      cell: ({ row }) => (
        <div className="flex items-center gap-2">
          <StatusDot state={row.original.state} />
          <span className="text-sm">
            {row.original.archived_at ? 'Archived' : row.original.state.toLowerCase()}
          </span>
        </div>
      ),
    },
    {
      id: 'actions',
      header: '',
      cell: ({ row }) => {
        const g = row.original
        const isArchived = g.archived_at !== null
        return (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="sm" aria-label="Row actions">
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {!isArchived ? (
                <>
                  <DropdownMenuItem onClick={() => setEditGateway(g)}>
                    <Pencil className="mr-2 h-4 w-4" />
                    Edit
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onClick={() => setDecommissionGateway(g)}
                    className="text-destructive focus:text-destructive"
                  >
                    <PowerOff className="mr-2 h-4 w-4" />
                    Decommission
                  </DropdownMenuItem>
                </>
              ) : (
                <DropdownMenuItem onClick={() => restoreMutation.mutate(g.id)}>
                  <ArchiveRestore className="mr-2 h-4 w-4" />
                  Restore
                </DropdownMenuItem>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        )
      },
    },
  ]

  const table = useReactTable({
    data: filtered,
    columns,
    getCoreRowModel: getCoreRowModel(),
  })

  const isLoading = gatewaysQuery.isLoading
  const isEmpty = !isLoading && filtered.length === 0 && !search

  return (
    <div className="flex flex-col gap-6 p-6">
      <header className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold leading-8">Gateways</h1>
        <Button onClick={() => setCreateOpen(true)}>
          <Plus className="mr-2 h-4 w-4" /> Add gateway
        </Button>
      </header>

      <div className="flex items-center gap-4">
        <Input
          className="max-w-sm"
          placeholder="Search gateways by name or ID"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <Toggle
          variant="outline"
          aria-label="Show archived"
          pressed={includeArchived}
          onPressedChange={setIncludeArchived}
          className="ml-auto"
        >
          <Archive className="mr-2 h-4 w-4" />
          Show archived
        </Toggle>
      </div>

      {isEmpty ? (
        <div className="flex flex-col items-center gap-4 rounded-md border p-12 text-center">
          <h2 className="text-lg font-semibold">No gateways yet</h2>
          <p className="text-sm text-muted-foreground">
            Add the first LoRaWAN gateway so devices can uplink to Shifter.
          </p>
          <Button onClick={() => setCreateOpen(true)}>
            <Plus className="mr-2 h-4 w-4" /> Add gateway
          </Button>
        </div>
      ) : (
        <div className="rounded-md border">
          <Table>
            <TableHeader>
              {table.getHeaderGroups().map((hg) => (
                <TableRow key={hg.id}>
                  {hg.headers.map((h) => (
                    <TableHead key={h.id}>
                      {flexRender(h.column.columnDef.header, h.getContext())}
                    </TableHead>
                  ))}
                </TableRow>
              ))}
            </TableHeader>
            <TableBody>
              {isLoading
                ? Array.from({ length: 5 }).map((_, i) => (
                    <TableRow key={`sk-${i}`}>
                      {columns.map((_c, ci) => (
                        <TableCell key={ci}>
                          <Skeleton className="h-4 w-full" />
                        </TableCell>
                      ))}
                    </TableRow>
                  ))
                : table.getRowModel().rows.map((row) => {
                    const archived = row.original.archived_at !== null
                    return (
                      <TableRow
                        key={row.id}
                        className={archived ? 'opacity-60' : undefined}
                      >
                        {row.getVisibleCells().map((c) => (
                          <TableCell key={c.id}>
                            {flexRender(c.column.columnDef.cell, c.getContext())}
                          </TableCell>
                        ))}
                      </TableRow>
                    )
                  })}
            </TableBody>
          </Table>
        </div>
      )}

      <AddGatewayDialog open={createOpen} onOpenChange={setCreateOpen} />
      {editGateway ? (
        <AddGatewayDialog
          open
          onOpenChange={(v) => {
            if (!v) setEditGateway(null)
          }}
          gateway={editGateway}
        />
      ) : null}
      {decommissionGateway ? (
        <DecommissionGatewayDialog
          open
          onOpenChange={(v) => {
            if (!v) setDecommissionGateway(null)
          }}
          gateway={decommissionGateway}
        />
      ) : null}
    </div>
  )
}
