/**
 * useFloorPlanHealth — Phase 5 Plan 10
 *
 * Per-floor-plan SSE subscription: listens to mp:<uuid> events for each placed
 * device, computes D-22 device health state client-side.
 *
 * Extends the Phase 4 useSSE hook by subscribing to mp:<uuid> topics for
 * each placed device's metering point (RESEARCH §Hub Extension Option B —
 * client-side state computation, no server-side migration required).
 */

import { useEffect, useReducer } from 'react'
import { useSSE } from '@/hooks/useSSE'
import type { MeasurementDelta } from '@/hooks/useSSE'

export type DeviceHealthState = 'healthy' | 'warning' | 'offline'

type PlacementHealth = {
  device_id: string
  metering_point_id: string
  last_seen_at: string | null
  battery_pct: number | null
  rssi: number | null
  expected_interval_s: number
}

function computeState(p: PlacementHealth): DeviceHealthState {
  if (!p.last_seen_at) return 'offline'
  const stale = Date.now() - new Date(p.last_seen_at).getTime() > 2 * p.expected_interval_s * 1000
  if (stale) return 'offline'
  if ((p.battery_pct ?? 100) <= 20) return 'warning'
  if ((p.rssi ?? 0) < -110) return 'warning'
  return 'healthy'
}

type Action =
  | { type: 'snapshot'; placements: PlacementHealth[] }
  | { type: 'measurement'; mpID: string; battery_pct: number | null; rssi: number | null; time: string }

function reducer(
  state: Record<string, PlacementHealth>,
  action: Action,
): Record<string, PlacementHealth> {
  switch (action.type) {
    case 'snapshot':
      return Object.fromEntries(action.placements.map((p) => [p.metering_point_id, p]))
    case 'measurement': {
      if (!state[action.mpID]) return state
      const next: PlacementHealth = {
        ...state[action.mpID],
        last_seen_at: action.time,
        battery_pct: action.battery_pct,
        rssi: action.rssi,
      }
      return { ...state, [action.mpID]: next }
    }
  }
}

/**
 * Returns a map of device_id → DeviceHealthState that updates live as SSE
 * measurement events arrive.
 *
 * @param placements - Placement rows from the floor-plan API, each with a
 *   `metering_point_id` field so we can subscribe to `mp:<uuid>` topics.
 */
export function useFloorPlanHealth(placements: PlacementHealth[]): Record<string, DeviceHealthState> {
  const [state, dispatch] = useReducer(reducer, {})

  // Snapshot: reset state when the placements list changes (floor plan switch, etc.)
  useEffect(() => {
    dispatch({ type: 'snapshot', placements })
  }, [placements])

  // Subscribe to mp:<uuid> topics for each placed device's metering point
  useSSE({
    topics: placements.map((p) => `mp:${p.metering_point_id}`),
    onMeasurement: (delta: MeasurementDelta) => {
      dispatch({
        type: 'measurement',
        mpID: delta.metering_point_id,
        battery_pct: delta.battery_pct,
        rssi: delta.rssi,
        time: delta.time,
      })
    },
  })

  return Object.fromEntries(
    Object.values(state).map((p) => [p.device_id, computeState(p)]),
  )
}
