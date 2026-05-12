/**
 * Plan 06-04 — Topbar bell button with severity-tinted unread badge.
 *
 * Polls /api/alerts/recent every 30s (via useAlertsRecent in
 * hooks/useAlerts.ts). When critical > 0 → red dot; warning > 0 → amber;
 * info > 0 → blue; all zero → no badge. Severity precedence:
 * critical > warning > info.
 *
 * Click opens the AlertDrawer (Sheet side="right").
 */

import { Bell } from 'lucide-react'
import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { useAlertsRecent } from '@/hooks/useAlerts'
import { AlertDrawer } from './AlertDrawer'

export function AlertBell() {
  const [open, setOpen] = useState(false)
  const { data } = useAlertsRecent()
  const counts = data?.unread_counts ?? { critical: 0, warning: 0, info: 0 }
  const total = counts.critical + counts.warning + counts.info

  const severity: 'critical' | 'warning' | 'info' | null =
    counts.critical > 0 ? 'critical' : counts.warning > 0 ? 'warning' : counts.info > 0 ? 'info' : null

  const dotClass: Record<'critical' | 'warning' | 'info', string> = {
    critical: 'bg-red-500',
    warning: 'bg-amber-500',
    info: 'bg-blue-500',
  }

  return (
    <>
      <Button
        variant="ghost"
        size="icon"
        aria-label={`Alerts (${total} unread, ${counts.critical} critical)`}
        onClick={() => setOpen(true)}
        className="relative"
      >
        <Bell className="h-5 w-5" aria-hidden="true" />
        {severity ? (
          <span
            aria-hidden
            data-severity={severity}
            className={`absolute right-1 top-1 inline-block h-2 w-2 rounded-full ring-2 ring-card ${dotClass[severity]}`}
          />
        ) : null}
      </Button>
      <AlertDrawer open={open} onOpenChange={setOpen} />
    </>
  )
}
