/**
 * Plan 06-04 — degraded-state banner that auto-renders when the alert
 * worker is unhealthy (D-22).
 *
 * Polls /api/health/detailed every 60s; surfaces an Alert (warning variant)
 * when any alert_worker_state row has degraded=true. Admin-only "View
 * details" link points to /settings/alerts (the Plan 06-10 surface).
 *
 * role="alert" so screen readers announce. sticky positioning so the
 * banner stays visible while operator scrolls inside the shell.
 */

import { useQuery } from '@tanstack/react-query'
import { AlertTriangle } from 'lucide-react'
import { Link } from 'react-router-dom'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { apiFetch } from '@/lib/api'
import { useCurrentUser } from '@/lib/use-current-user'

interface AlertWorkerDetail {
  worker_kind: string
  degraded: boolean
  last_error?: string
  last_run_at?: string
}

interface HealthDetailedResponse {
  alert_workers?: AlertWorkerDetail[]
}

export function AlertWorkerBanner() {
  const user = useCurrentUser()
  const isAdmin = user?.role === 'admin'

  const { data } = useQuery({
    queryKey: ['health', 'detailed'] as const,
    queryFn: () => apiFetch<HealthDetailedResponse>('/api/health/detailed'),
    refetchInterval: 60_000,
    refetchOnWindowFocus: false,
    retry: 0,
    // Don't blow up if the endpoint 403s for viewer — banner remains hidden.
    throwOnError: false,
  })

  const degraded = (data?.alert_workers ?? []).filter((w) => w.degraded)
  if (degraded.length === 0) return null

  return (
    <div
      role="alert"
      className="sticky top-14 z-30 border-b border-amber-500/30 bg-amber-50/80 px-4 py-2 text-amber-900 backdrop-blur md:px-6"
    >
      <Alert variant="default" className="border-0 bg-transparent p-0">
        <AlertTriangle className="h-4 w-4" />
        <AlertTitle>Alert evaluation is degraded</AlertTitle>
        <AlertDescription>
          {degraded.length === 1
            ? `${degraded[0].worker_kind} worker is unhealthy`
            : `${degraded.length} alert workers are unhealthy`}
          {isAdmin ? (
            <>
              {' · '}
              <Link to="/settings/alerts" className="font-medium underline">
                View details
              </Link>
            </>
          ) : null}
        </AlertDescription>
      </Alert>
    </div>
  )
}
