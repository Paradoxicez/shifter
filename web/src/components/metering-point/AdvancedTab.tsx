/**
 * AdvancedTab — Plan 04-09 Task 2
 *
 * Displays the decoded_object + extra JSONB from the latest reading using
 * the custom JsonTree component (D-20).
 *
 * Pending payload alert (forensic-safety rule, D-20):
 *   - When a new measurement event arrives via SSE, the parent route sets
 *     `pendingPayload` state rather than auto-swapping the rendered tree.
 *   - This component renders an info Alert: "Newer payload available · [Refresh]"
 *   - Clicking Refresh calls `clearPending()` which clears the pendingPayload state.
 *     The parent's React-Query invalidation (triggered by the SSE snapshot) will
 *     deliver the new latestReading prop on the next render cycle.
 *
 * Prop contract (single source of truth — Task 3 wires these unchanged):
 *   latestReading: NonNullable<DetailResponse['latest_reading']>
 *   pendingPayload: MeasurementDelta | null
 *   clearPending: () => void
 */

import { useState } from 'react'
import { formatDistanceToNow } from 'date-fns'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import type { MeasurementDelta } from '@/hooks/useSSE'
import { JsonTree } from './JsonTree'
import type { DetailResponse } from './NormalTab'

// ---------------------------------------------------------------------------
// Prop contract (verbatim — Task 3 passes exactly these three props)
// ---------------------------------------------------------------------------

export interface AdvancedTabProps {
  latestReading: NonNullable<DetailResponse['latest_reading']>
  pendingPayload: MeasurementDelta | null
  clearPending: () => void
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function AdvancedTab({ latestReading, pendingPayload, clearPending }: AdvancedTabProps) {
  // displayedReading starts at the server-fetched latestReading.
  // When a new latestReading prop arrives (after the user clicks Refresh and
  // React-Query delivers the refetch), re-sync automatically.
  const [displayedReading, setDisplayedReading] = useState(latestReading)

  // Re-sync when latestReading prop changes (query refetch after snapshot invalidation)
  if (displayedReading !== latestReading && pendingPayload === null) {
    setDisplayedReading(latestReading)
  }

  const onRefresh = () => {
    // The decoded_object/extra will arrive via the detail query refetch that
    // useSSE has already triggered. Just clear the pending banner — new
    // latestReading prop flows in on next render.
    clearPending()
  }

  const lastReceived = formatDistanceToNow(new Date(displayedReading.time), { addSuffix: true })

  return (
    <Card>
      <CardHeader>
        <CardTitle>Decoded payload + extra</CardTitle>
        <CardDescription>Last received {lastReceived}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {pendingPayload !== null && (
          <Alert>
            <AlertDescription className="flex items-center justify-between">
              <span>Newer payload available</span>
              <Button variant="link" size="sm" onClick={onRefresh}>
                Refresh
              </Button>
            </AlertDescription>
          </Alert>
        )}
        <JsonTree
          value={{
            object: displayedReading.decoded_object,
            extra: displayedReading.extra,
          }}
          defaultOpen
        />
      </CardContent>
    </Card>
  )
}
