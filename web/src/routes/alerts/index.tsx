/**
 * Plan 06-04 — /alerts route (UI-SPEC §Surface 2).
 *
 * Full alert center page. URL-state filter chips (severity, status, category,
 * target_type, date range) drive the React Query fetch. Per-row Ack +
 * Snooze for admin; viewer is read-only.
 */

import { Link } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useAlertParams } from '@/lib/alertParams'
import { useAckMutation, useAlertsList, useSnoozeMutation } from '@/hooks/useAlerts'
import { useCurrentUser } from '@/lib/use-current-user'
import { toast } from 'sonner'
import { SeverityPill } from '@/components/alerts/SeverityPill'
import { AlertCenterFilters } from './AlertCenterFilters'
import { SnoozeMenu } from './SnoozeMenu'

export default function AlertsPage() {
  const user = useCurrentUser()
  const isAdmin = user?.role === 'admin'
  const [params, setParams] = useAlertParams()
  const { data, isLoading, isError, error } = useAlertsList(params)
  const ack = useAckMutation()
  const snooze = useSnoozeMutation()

  const total = data?.rows.length ?? 0

  const onAck = (id: string) => {
    ack.mutate(
      { id },
      {
        onSuccess: () => toast.success('Alert acknowledged'),
        onError: (err) => toast.error(`Acknowledge failed: ${(err as Error).message}`),
      },
    )
  }

  const onSnooze = (id: string, duration: string) => {
    snooze.mutate(
      { id, duration },
      {
        onSuccess: () =>
          toast.success(
            duration === 'mute' ? 'Alert muted until cleared' : `Snoozed for ${duration}`,
          ),
        onError: (err) => toast.error(`Snooze failed: ${(err as Error).message}`),
      },
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <header className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">Alerts {data ? `(${total})` : ''}</h1>
        <Button
          variant="outline"
          size="sm"
          onClick={() => toast.info('CSV export coming in Plan 06-07')}
        >
          Export CSV
        </Button>
      </header>

      <AlertCenterFilters params={params} setParams={setParams} />

      {isError ? (
        <div className="rounded-md border border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive">
          Failed to load alerts: {(error as Error).message}
        </div>
      ) : isLoading ? (
        <div className="flex flex-col gap-2">
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-12 w-full" />
        </div>
      ) : total === 0 ? (
        <div className="rounded-md border border-border bg-card p-8 text-center text-sm text-muted-foreground">
          No alerts match the current filters.
        </div>
      ) : (
        <ul className="flex flex-col gap-2" aria-label="Alerts list">
          {data?.rows.map((row) => {
            const fired = new Date(row.fired_at)
            return (
              <li
                key={row.id}
                className="flex items-center gap-3 rounded-md border border-border bg-card p-3"
                data-state={row.state}
                data-severity={row.severity}
              >
                <SeverityPill severity={row.severity} />
                <div className="flex-1 min-w-0">
                  <div className="truncate font-medium">{labelFor(row)}</div>
                  <div className="text-xs text-muted-foreground">
                    {row.rule_kind} · {row.target_entity_type} · {fired.toLocaleString()}
                    {row.is_test ? ' · TEST' : null}
                  </div>
                </div>
                <div className="hidden text-xs text-muted-foreground sm:block">{row.state}</div>
                {isAdmin && row.state === 'firing' ? (
                  <div className="flex gap-2">
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => onAck(row.id)}
                      disabled={ack.isPending}
                    >
                      Ack
                    </Button>
                    <SnoozeMenu onSelect={(d) => onSnooze(row.id, d)} disabled={snooze.isPending} />
                  </div>
                ) : null}
              </li>
            )
          })}
        </ul>
      )}

      <div className="text-xs text-muted-foreground">
        <Link to="/settings/alerts" className="underline">
          Manage rules in Settings → Alerts
        </Link>
      </div>
    </div>
  )
}

function labelFor(alert: {
  payload?: unknown
  rule_kind: string
}): string {
  if (alert.payload && typeof alert.payload === 'object' && 'target' in alert.payload) {
    const t = (alert.payload as { target?: { label?: string } }).target
    if (t?.label) return t.label
  }
  return alert.rule_kind
}
