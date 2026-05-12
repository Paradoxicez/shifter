/**
 * Plan 06-04 — Slide-over alert drawer (UI-SPEC §Surface 1).
 *
 * shadcn Sheet (side="right"). Shows up to 10 most-recent active alerts
 * sorted by severity DESC then fired_at DESC. Each row has:
 *   - severity dot + 2-line content (rule kind → target → fired_at)
 *   - Ack button + Snooze dropdown (admin only — viewer sees neither)
 *
 * Footer link "See all alerts →" routes to /alerts.
 */

import { Link } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import {
  SeverityDot,
  type Severity,
} from '@/components/alerts/SeverityPill'
import { useAckMutation, useAlertsRecent, useSnoozeMutation, type AlertDTO } from '@/hooks/useAlerts'
import { useCurrentUser } from '@/lib/use-current-user'
import { toast } from 'sonner'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

export interface AlertDrawerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function AlertDrawer({ open, onOpenChange }: AlertDrawerProps) {
  const { data, isLoading } = useAlertsRecent()
  const rows = data?.rows ?? []

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="flex flex-col sm:max-w-md">
        <SheetHeader>
          <SheetTitle>Alerts</SheetTitle>
          <SheetDescription>
            {rows.length === 0
              ? 'No active alerts'
              : `${rows.length} active alert${rows.length === 1 ? '' : 's'}`}
          </SheetDescription>
        </SheetHeader>

        <div className="flex-1 overflow-y-auto px-4">
          {isLoading ? (
            <div className="py-8 text-center text-sm text-muted-foreground">Loading…</div>
          ) : rows.length === 0 ? (
            <div className="py-8 text-center text-sm text-muted-foreground">
              No active alerts. Operator is on top of things.
            </div>
          ) : (
            <ul className="flex flex-col gap-2 pb-4">
              {rows.map((row) => (
                <DrawerRow key={row.id} alert={row} />
              ))}
            </ul>
          )}
        </div>

        <SheetFooter>
          <Button asChild variant="link" className="w-full">
            <Link to="/alerts" onClick={() => onOpenChange(false)}>
              See all alerts →
            </Link>
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}

function DrawerRow({ alert }: { alert: AlertDTO }) {
  const user = useCurrentUser()
  const isAdmin = user?.role === 'admin'
  const ack = useAckMutation()
  const snooze = useSnoozeMutation()

  const onAck = () => {
    ack.mutate(
      { id: alert.id },
      {
        onSuccess: () => toast.success('Alert acknowledged'),
        onError: (err) => toast.error(`Could not acknowledge: ${(err as Error).message}`),
      },
    )
  }

  const onSnooze = (duration: string) => {
    snooze.mutate(
      { id: alert.id, duration },
      {
        onSuccess: () =>
          toast.success(
            duration === 'mute' ? 'Alert muted until cleared' : `Snoozed for ${duration}`,
          ),
        onError: (err) => toast.error(`Could not snooze: ${(err as Error).message}`),
      },
    )
  }

  const fired = new Date(alert.fired_at)

  return (
    <li
      className="rounded-md border border-border bg-card p-3 text-sm"
      data-state={alert.state}
      data-severity={alert.severity}
    >
      <div className="flex items-center gap-2">
        <SeverityDot severity={alert.severity as Severity} />
        <span className="flex-1 truncate font-medium">{labelFor(alert)}</span>
        {alert.is_test ? (
          <span className="rounded-full bg-blue-500/15 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-blue-600">
            TEST
          </span>
        ) : null}
      </div>
      <div className="mt-1 text-xs text-muted-foreground">
        {alert.target_entity_type} · {relativeTime(fired)}
      </div>
      {isAdmin ? (
        <div className="mt-2 flex gap-2">
          <Button size="sm" variant="outline" onClick={onAck} disabled={ack.isPending}>
            Ack
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button size="sm" variant="outline" disabled={snooze.isPending}>
                Snooze ▾
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent>
              <DropdownMenuItem onSelect={() => onSnooze('1h')}>Snooze 1 hour</DropdownMenuItem>
              <DropdownMenuItem onSelect={() => onSnooze('8h')}>Snooze 8 hours</DropdownMenuItem>
              <DropdownMenuItem onSelect={() => onSnooze('24h')}>Snooze 24 hours</DropdownMenuItem>
              <DropdownMenuItem onSelect={() => onSnooze('7d')}>Snooze 7 days</DropdownMenuItem>
              <DropdownMenuItem onSelect={() => onSnooze('mute')}>
                Mute until I clear
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      ) : null}
    </li>
  )
}

function labelFor(alert: AlertDTO): string {
  // Payload is the D-12 canonical shape — pull target.label if present.
  if (alert.payload && typeof alert.payload === 'object' && 'target' in alert.payload) {
    const target = (alert.payload as { target?: { label?: string } }).target
    if (target?.label) return target.label
  }
  return alert.rule_kind
}

function relativeTime(d: Date): string {
  const diff = Date.now() - d.getTime()
  const s = Math.floor(diff / 1000)
  if (s < 60) return `${s}s ago`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  const days = Math.floor(h / 24)
  return `${days}d ago`
}
