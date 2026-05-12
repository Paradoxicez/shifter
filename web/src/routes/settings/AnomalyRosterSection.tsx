/**
 * Plan 06-04 — Anomaly cold-start roster section for Settings → Alerts.
 *
 * Shows "X meters eligible · Y warming up · [Show roster ▾]" header.
 * Expanding the roster shows two groups (Eligible, Warming up); each row
 * links to /metering-points/{id}.
 */

import { useState } from 'react'
import { ChevronRight } from 'lucide-react'
import { Link } from 'react-router-dom'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { useAnomalyRoster } from '@/hooks/useAlerts'

export function AnomalyRosterSection() {
  const [expanded, setExpanded] = useState(false)
  const { data, isLoading } = useAnomalyRoster()
  const rows = data?.rows ?? []

  const eligible = rows.filter((r) => r.days_until_eligible === 0)
  const warming = rows.filter((r) => r.days_until_eligible > 0)

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <CardTitle>Anomaly detection</CardTitle>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => setExpanded((v) => !v)}
          aria-expanded={expanded}
        >
          {expanded ? 'Hide roster' : 'Show roster'} ▾
        </Button>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="text-sm text-muted-foreground">Loading roster…</div>
        ) : (
          <>
            <div className="text-sm">
              <span className="font-medium">{eligible.length}</span> meters eligible ·{' '}
              <span className="font-medium">{warming.length}</span> warming up
            </div>

            {expanded ? (
              <div className="mt-4 space-y-4">
                {eligible.length > 0 ? (
                  <section>
                    <h4 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                      Eligible ({eligible.length})
                    </h4>
                    <ul className="mt-2 divide-y divide-border">
                      {eligible.map((row) => (
                        <RosterRow key={row.metering_point_id} row={row} />
                      ))}
                    </ul>
                  </section>
                ) : null}
                {warming.length > 0 ? (
                  <section>
                    <h4 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                      Warming up ({warming.length})
                    </h4>
                    <ul className="mt-2 divide-y divide-border">
                      {warming.map((row) => (
                        <RosterRow key={row.metering_point_id} row={row} />
                      ))}
                    </ul>
                  </section>
                ) : null}
              </div>
            ) : null}
          </>
        )}
      </CardContent>
    </Card>
  )
}

function RosterRow({
  row,
}: {
  row: { metering_point_id: string; metering_point_label: string; site_label: string; days_until_eligible: number }
}) {
  return (
    <li className="flex items-center justify-between py-2 text-sm">
      <Link
        to={`/metering-points/${row.metering_point_id}`}
        className="flex-1 underline-offset-2 hover:underline"
      >
        {row.metering_point_label}
      </Link>
      <span className="text-xs text-muted-foreground">
        {row.site_label || '—'} ·{' '}
        {row.days_until_eligible === 0 ? 'Eligible' : `${row.days_until_eligible}d left`}
      </span>
      <ChevronRight className="ml-2 h-4 w-4 text-muted-foreground" aria-hidden />
    </li>
  )
}
