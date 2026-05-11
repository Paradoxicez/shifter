/**
 * LiveChannelBanner — Plan 04-07 Task 1
 *
 * SSE connection status banner per UI-SPEC §State Conventions:
 *   - status='open'            → render nothing (no green banner)
 *   - status='connecting'      → render nothing (transient)
 *   - status='reconnecting' ≤30s → Alert warning "Reconnecting to live updates…"
 *   - status='reconnecting' >30s → "Live updates paused" + last-known time
 *   - status='closed' (401)    → "Session expired" + link to /login
 *
 * Icon uses animate-spin with motion-reduce:animate-none per UI-SPEC.
 */

import { RotateCw } from 'lucide-react'
import { Link } from 'react-router-dom'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import type { SSEStatus } from '@/hooks/useSSE'

export interface LiveChannelBannerProps {
  status: SSEStatus
  lastEventAt: number | null
}

export function LiveChannelBanner({ status, lastEventAt }: LiveChannelBannerProps) {
  if (status === 'open' || status === 'connecting') {
    return null
  }

  if (status === 'closed') {
    return (
      <Alert variant="destructive">
        <AlertTitle>Session expired</AlertTitle>
        <AlertDescription>
          Your session has ended.{' '}
          <Link to="/login" className="underline font-medium">
            Sign in again
          </Link>{' '}
          to resume live updates.
        </AlertDescription>
      </Alert>
    )
  }

  // status === 'reconnecting'
  const now = Date.now()
  const elapsed = lastEventAt != null ? now - lastEventAt : null
  const isLongGap = elapsed != null && elapsed > 30_000

  if (isLongGap && lastEventAt != null) {
    const lastTime = new Date(lastEventAt).toLocaleTimeString()
    return (
      <Alert>
        <RotateCw className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
        <AlertTitle>Live updates paused</AlertTitle>
        <AlertDescription>
          <span className="font-mono">Showing last-known values from {lastTime}</span>
        </AlertDescription>
      </Alert>
    )
  }

  // Reconnecting within 30s
  const secondsAgo = elapsed != null ? Math.floor(elapsed / 1000) : null
  return (
    <Alert>
      <RotateCw className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
      <AlertTitle>Reconnecting to live updates…</AlertTitle>
      {secondsAgo != null && (
        <AlertDescription>
          <span className="font-mono">Last snapshot {secondsAgo} sec ago</span>
        </AlertDescription>
      )}
    </Alert>
  )
}
