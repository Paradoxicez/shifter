/**
 * useDashboardScope — Phase 4 Plan 06
 *
 * React-Query wrapper for GET /api/dashboard/scope.
 * Returns capabilities + onboarding counts for the install.
 * Plans 07 and 09 use this to gate which KPI tiles and nav items to render.
 *
 * staleTime: 5 minutes — capabilities and onboarding counts are install-level
 * configuration that changes rarely (adding a gateway / first device).
 * The 5-minute cache means rapid navigation between dashboard tabs doesn't
 * hammer the endpoint.
 */

import { useQuery } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'

export type DashboardCapabilities = 'water' | 'electricity' | 'both'

export interface DashboardScope {
  capabilities: DashboardCapabilities
  onboarding: {
    gateway_count: number
    device_count: number
    uplink_count: number
  }
}

export function useDashboardScope() {
  return useQuery<DashboardScope>({
    queryKey: ['dashboard', 'scope'],
    queryFn: () => apiFetch<DashboardScope>('/api/dashboard/scope'),
    staleTime: 5 * 60_000, // 5 minutes
  })
}
