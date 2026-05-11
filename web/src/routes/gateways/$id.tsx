import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArchiveRestore, Pencil, PowerOff } from 'lucide-react'
import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { Area, AreaChart } from 'recharts'
import { toast } from 'sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ChartContainer, type ChartConfig } from '@/components/ui/chart'
import { Skeleton } from '@/components/ui/skeleton'
import { getGateway, restoreGateway, type Gateway } from '@/lib/gateways'
import { AddGatewayDialog } from './add-gateway-dialog'
import { DecommissionGatewayDialog } from './decommission-gateway-dialog'

function statusTokenFor(g: Gateway): 'success' | 'warning' | 'destructive' {
  if (!g.stats_tx_24h || g.stats_tx_24h === 0) return 'success'
  const ratio = (g.stats_tx_ok_24h ?? 0) / g.stats_tx_24h
  if (ratio >= 0.95) return 'success'
  if (ratio >= 0.7) return 'warning'
  return 'destructive'
}

/**
 * Full-card sparkline (w-full h-24) used on the detail page; renders only
 * with CSS variable tokens — never hex literals. Allowed tokens:
 * var(--success), var(--warning), var(--destructive).
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
    <ChartContainer config={config} className="h-24 w-full aspect-auto">
      <AreaChart data={chartData} margin={{ top: 4, right: 4, bottom: 4, left: 4 }}>
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
  if (!iso) return 'never'
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return 'unknown'
  const diff = Date.now() - t
  const min = Math.floor(diff / 60_000)
  if (min < 1) return 'just now'
  if (min < 60) return `${min} min ago`
  const hr = Math.floor(min / 60)
  if (hr < 24) return `${hr} hr ago`
  const day = Math.floor(hr / 24)
  return `${day} d ago`
}

function Stat({
  label,
  value,
  tone,
}: {
  label: string
  value: string | number
  tone?: 'success' | 'warning' | 'destructive'
}) {
  return (
    <div className="flex flex-col gap-1">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span
        className="text-2xl font-semibold font-mono"
        style={tone ? { color: `var(--${tone})` } : undefined}
      >
        {value}
      </span>
    </div>
  )
}

/**
 * UI-SPEC §Gateway detail page (`/gateways/:id`):
 *   - Header: name (text-2xl) + gateway_id (text-sm font-mono) + status row
 *     ("Online · last uplink 3 min ago") + Edit + Decommission/Restore CTAs.
 *   - Identity card: Region, Coordinates (mono), Description, "Pick on map"
 *     disabled with v5 tooltip.
 *   - 24h stats card: RX / TX / Success% Stats + sparkline.
 *
 * Data: useQuery(['gateway', id]) → getGateway(id). 404 → "Gateway not found".
 */
export default function GatewayDetailPage() {
  const { id = '' } = useParams<{ id: string }>()
  const qc = useQueryClient()
  const [editOpen, setEditOpen] = useState(false)
  const [decommissionOpen, setDecommissionOpen] = useState(false)

  const gatewayQuery = useQuery({
    queryKey: ['gateway', id],
    queryFn: () => getGateway(id),
    enabled: Boolean(id),
    retry: false,
  })

  const restoreMutation = useMutation({
    mutationFn: () => restoreGateway(id),
    onSuccess: (restored) => {
      qc.invalidateQueries({ queryKey: ['gateway', id] })
      qc.invalidateQueries({ queryKey: ['gateways'] })
      toast.success(`Gateway ${restored.name} restored.`)
    },
    onError: () => {
      toast.error('Could not restore gateway')
    },
  })

  if (gatewayQuery.isLoading) {
    return (
      <div className="flex flex-col gap-6 p-6">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-40 w-full" />
        <Skeleton className="h-40 w-full" />
      </div>
    )
  }

  if (gatewayQuery.error || !gatewayQuery.data) {
    return (
      <div className="flex flex-col items-center gap-4 rounded-md border p-12 text-center m-6">
        <h1 className="text-2xl font-semibold leading-8">Gateway not found</h1>
        <p className="text-sm text-muted-foreground">
          The gateway you requested doesn&rsquo;t exist or you don&rsquo;t have access.
        </p>
        <Button asChild variant="outline">
          <Link to="/gateways">Back to gateways</Link>
        </Button>
      </div>
    )
  }

  const g = gatewayQuery.data
  const isArchived = g.archived_at !== null
  const successPct =
    g.stats_tx_24h && g.stats_tx_24h > 0
      ? Math.round(((g.stats_tx_ok_24h ?? 0) / g.stats_tx_24h) * 100)
      : null
  const token = statusTokenFor(g)

  return (
    <div className="flex flex-col gap-6 p-6">
      <header className="flex items-start justify-between gap-4">
        <div className="flex flex-col gap-1">
          <h1 className="text-2xl font-semibold leading-8">{g.name}</h1>
          <p className="text-sm font-mono text-muted-foreground">{g.gateway_id}</p>
          <div className="mt-1 flex items-center gap-2">
            <StatusDot state={g.state} />
            <span className="text-sm">
              {g.state === 'ONLINE'
                ? `Online · last uplink ${relativeFromNow(g.last_seen_at)}`
                : g.state === 'OFFLINE'
                  ? `Offline · last uplink ${relativeFromNow(g.last_seen_at)}`
                  : 'Never seen'}
            </span>
          </div>
        </div>
        <div className="flex items-center gap-2">
          {!isArchived ? (
            <>
              <Button variant="ghost" onClick={() => setEditOpen(true)}>
                <Pencil className="mr-2 h-4 w-4" />
                Edit gateway
              </Button>
              <Button variant="destructive" onClick={() => setDecommissionOpen(true)}>
                <PowerOff className="mr-2 h-4 w-4" />
                Decommission
              </Button>
            </>
          ) : (
            <Button
              variant="outline"
              disabled={restoreMutation.isPending}
              onClick={() => restoreMutation.mutate()}
            >
              <ArchiveRestore className="mr-2 h-4 w-4" />
              {restoreMutation.isPending ? 'Restoring…' : 'Restore'}
            </Button>
          )}
        </div>
      </header>

      {isArchived ? (
        <Alert>
          <AlertTitle>This gateway is archived.</AlertTitle>
          <AlertDescription>
            Archived on {g.archived_at?.slice(0, 10) ?? 'unknown'} —{' '}
            {g.archived_reason ?? 'operator decommission'}.
          </AlertDescription>
        </Alert>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>Identity</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3 text-sm">
          <div>
            <span className="font-semibold">Region:</span>{' '}
            <span>{g.region}</span>
          </div>
          <div>
            <span className="font-semibold">Coordinates:</span>{' '}
            {g.lat != null && g.lng != null ? (
              <span className="font-mono">
                {g.lat}, {g.lng}
              </span>
            ) : (
              <span className="text-muted-foreground">—</span>
            )}
          </div>
          {g.description ? (
            <div>
              <span className="font-semibold">Description:</span>{' '}
              <span className="text-muted-foreground">{g.description}</span>
            </div>
          ) : null}
          {Object.keys(g.tags ?? {}).length > 0 ? (
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-semibold">Tags:</span>
              {Object.entries(g.tags).map(([k, v]) => (
                <span
                  key={k}
                  className="rounded-md bg-secondary px-2 py-0.5 text-xs"
                >
                  {v ? `${k}=${v}` : k}
                </span>
              ))}
            </div>
          ) : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Last 24 hours</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-6">
          <div className="grid grid-cols-3 gap-6">
            <Stat label="RX uplinks" value={g.stats_rx_24h ?? '—'} />
            <Stat label="TX downlinks" value={g.stats_tx_24h ?? '—'} />
            <Stat
              label="Uplink success"
              value={successPct != null ? `${successPct}%` : '—'}
              tone={token}
            />
          </div>
          {g.stats_sparkline ? (
            <Sparkline data={g.stats_sparkline.rx} fillToken={token} />
          ) : null}
          {g.stats_refreshed_at ? (
            <p className="text-xs text-muted-foreground self-end">
              Updated {relativeFromNow(g.stats_refreshed_at)}
            </p>
          ) : null}
        </CardContent>
      </Card>

      <AddGatewayDialog
        open={editOpen}
        onOpenChange={setEditOpen}
        gateway={g}
      />
      <DecommissionGatewayDialog
        open={decommissionOpen}
        onOpenChange={setDecommissionOpen}
        gateway={g}
      />
    </div>
  )
}
