/**
 * MapPage — Plan 05-08
 *
 * /map route: full-bleed Leaflet map showing all sites + gateways.
 *
 * Data: GET /api/map/data (shipped in Plan 05-04)
 * Empty state: MapEmptyState card → navigate to /sites?action=create
 * Capabilities: from useDashboardScope() (Plan 04-06) for popup gating
 */

import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { Skeleton } from '@/components/ui/skeleton'
import { apiFetch } from '@/lib/api'
import { useDashboardScope } from '@/hooks/useDashboardScope'
import { MapView, type MapSite, type MapGateway } from '@/components/map/MapView'
import { MapEmptyState } from '@/components/map/MapEmptyState'

export function MapPage() {
  const scope = useDashboardScope()
  const navigate = useNavigate()

  const { data, isLoading } = useQuery({
    queryKey: ['map', 'data'],
    queryFn: () =>
      apiFetch<{ sites: MapSite[]; gateways: MapGateway[] }>('/api/map/data'),
  })

  if (isLoading) {
    return <Skeleton className="h-[calc(100vh-3.5rem)] w-full" />
  }

  const sites = data?.sites ?? []
  const gateways = data?.gateways ?? []

  if (sites.length === 0 && gateways.length === 0) {
    return (
      <MapEmptyState onAddSite={() => navigate('/sites?action=create')} />
    )
  }

  const capabilities = scope.data?.capabilities ?? 'both'

  return (
    <MapView
      sites={sites}
      gateways={gateways}
      capabilities={capabilities}
    />
  )
}

export default MapPage
