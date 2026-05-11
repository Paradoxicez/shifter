/**
 * useSSE — Phase 4 Plan 06
 *
 * Owns the per-tab EventSource connection to GET /api/events.
 *
 * Topic-set changes are handled by close + reconnect-with-new-?topics=.
 * Phase 4 ships no subscribe/unsubscribe HTTP API — Plan 03 is SSE-only.
 * Every reconnect emits a fresh snapshot via D-03, which keeps the React-Query
 * invalidation path uniform across network loss and topic-set changes.
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type SSEStatus = 'connecting' | 'open' | 'reconnecting' | 'closed'

export interface MeasurementDelta {
  metering_point_id: string
  time: string
  cumulative_value: number | null
  instant_value: number | null
  quality: 'ok' | 'decode_fail' | 'missing_canonical' | 'out_of_range' | 'duplicate_fcnt'
  battery_pct: number | null
  rssi: number | null
}

export interface SnapshotEvent {
  ready: boolean
  topics: string[]
  // Wire format: {ready, topics} only — no client correlation id in Phase 4.
}

export interface UseSSEOptions {
  topics: string[]
  /**
   * Called on every 'measurement' event (after JSON parse).
   * The hook itself also invalidates React-Query keys; callers can layer extra behavior.
   */
  onMeasurement?: (delta: MeasurementDelta) => void
  /**
   * Map a measurement event to additional query keys to invalidate.
   * Default invalidation keys are always applied; this adds more.
   */
  invalidationKeys?: (delta: MeasurementDelta) => Array<string | (string | number)[]>
}

export interface UseSSEResult {
  status: SSEStatus
  /** Current reconnect attempt count (0 when open) */
  attempt: number
  /** ms timestamp of last received event (for banner sublabel) */
  lastEventAt: number | null
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

export function useSSE(opts: UseSSEOptions): UseSSEResult {
  const queryClient = useQueryClient()
  const [status, setStatus] = useState<SSEStatus>('connecting')
  const [attempt, setAttempt] = useState(0)
  const [lastEventAt, setLastEventAt] = useState<number | null>(null)

  const esRef = useRef<EventSource | null>(null)
  // Track attempt in a ref so the async error handler closure reads the current value
  const attemptRef = useRef(0)

  // Topics serialized for dependency stability — sort so order doesn't trigger reconnects.
  const topicsKey = opts.topics.slice().sort().join(',')

  // Keep the latest callbacks in refs so the effect doesn't recreate the EventSource
  // every time a callback reference changes (common with inline arrow functions).
  const onMeasurementRef = useRef(opts.onMeasurement)
  const invalidationKeysRef = useRef(opts.invalidationKeys)
  useEffect(() => {
    onMeasurementRef.current = opts.onMeasurement
    invalidationKeysRef.current = opts.invalidationKeys
  })

  const connect = useCallback(() => {
    if (!topicsKey) return

    // Encode sorted topic list into query string
    const url = `/api/events?topics=${encodeURIComponent(topicsKey)}`
    const es = new EventSource(url, { withCredentials: true })
    esRef.current = es
    setStatus('connecting')

    es.addEventListener('open', () => {
      attemptRef.current = 0
      setAttempt(0)
      setStatus('open')
    })

    es.addEventListener('snapshot', (ev) => {
      try {
        const snap = JSON.parse((ev as MessageEvent).data) as SnapshotEvent
        // D-03: always-full-snapshot — invalidate dashboard snapshot key on every snapshot event
        queryClient.invalidateQueries({ queryKey: ['dashboard', 'snapshot'] })
        // For each mp:<uuid> topic in the snapshot, invalidate the per-MP detail key
        for (const t of snap.topics ?? []) {
          if (t.startsWith('mp:')) {
            // Strip optional ':uplinks' suffix to get the raw UUID
            const id = t.slice(3).split(':')[0]
            queryClient.invalidateQueries({ queryKey: ['mp', id, 'detail'] })
          }
        }
        setLastEventAt(Date.now())
      } catch (err) {
        console.warn('useSSE: failed to parse snapshot event', err)
      }
    })

    es.addEventListener('measurement', (ev) => {
      try {
        const delta = JSON.parse((ev as MessageEvent).data) as MeasurementDelta
        // Default invalidation: write latest reading + refresh dashboard KPI tiles
        queryClient.setQueryData(['mp', delta.metering_point_id, 'latest'], delta)
        queryClient.invalidateQueries({ queryKey: ['dashboard', 'snapshot'] })
        // Caller-provided additional invalidation keys (e.g. timeseries for current range)
        const extraKeys = invalidationKeysRef.current?.(delta)
        if (extraKeys) {
          for (const k of extraKeys) {
            queryClient.invalidateQueries({ queryKey: Array.isArray(k) ? k : [k] })
          }
        }
        onMeasurementRef.current?.(delta)
        setLastEventAt(Date.now())
      } catch (err) {
        console.warn('useSSE: failed to parse measurement event', err)
      }
    })

    es.addEventListener('error', async () => {
      es.close()
      esRef.current = null
      setStatus('reconnecting')

      // Auth-expired heuristic — EventSource can't expose HTTP status codes.
      // Fire a probe fetch: if 401 the session expired, stop retrying.
      try {
        const probe = await fetch('/api/account/me', { credentials: 'include' })
        if (probe.status === 401) {
          setStatus('closed')
          return
        }
      } catch {
        // Network error on probe — treat as transient and continue with backoff
      }

      // D-04 backoff: min(30s, 0.5s * 2^n) + jitter
      // Jitter prevents synchronized retries across multiple browser tabs.
      const next = attemptRef.current + 1
      attemptRef.current = next
      setAttempt(next)
      const delay = Math.min(30_000, 500 * 2 ** next) + Math.random() * 1000
      window.setTimeout(connect, delay)
    })
  }, [topicsKey, queryClient]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    connect()
    return () => {
      esRef.current?.close()
      esRef.current = null
      setStatus('closed')
    }
  }, [connect])

  return { status, attempt, lastEventAt }
}
