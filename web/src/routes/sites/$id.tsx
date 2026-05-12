import { useQuery } from '@tanstack/react-query'
import { Archive, Map as MapIcon, MoreHorizontal, Pencil, Plus } from 'lucide-react'
import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { getSite } from '@/lib/sites'
import { apiFetch } from '@/lib/api'
import { FloorPlanTab } from '@/components/floor-plan/FloorPlanTab'

/**
 * UI-SPEC §Site detail page:
 *   - Header with name + address + Edit ghost + overflow (Archive).
 *   - Identity card: Timezone + Coordinates (mono) + Description +
 *     disabled "Pick on map" with tooltip "Available in v5".
 *   - Metering Points section with "Add metering point" CTA + table.
 *   - Devices section (read-only).
 *   - Floor plan tab (D-23): default tab when floor_plan rows exist;
 *     otherwise Overview default.
 *
 * Phase 2 ships the shells for MP / Devices sections; the full nested
 * tables hook into Plan 02-12's wiring (and Phase 3+ owns Devices
 * promotion). For now, we render empty-state copy from UI-SPEC.
 */
export default function SiteDetailPage() {
  const { id } = useParams<{ id: string }>()
  const siteQuery = useQuery({
    queryKey: ['site', id],
    queryFn: () => getSite(id ?? ''),
    enabled: Boolean(id),
  })

  // D-23: check if any floor plans exist to determine default tab
  const { data: floorPlans = [] } = useQuery<{ id: string }[]>({
    queryKey: ['floor-plans', id],
    queryFn: () => apiFetch<{ id: string }[]>(`/api/sites/${id}/floor-plans`),
    enabled: Boolean(id),
  })

  const [, setMpDialogOpen] = useState(false)

  const site = siteQuery.data

  if (!site) {
    return <div className="p-6 text-sm text-muted-foreground">Loading…</div>
  }

  // D-23: default to floor-plan tab when plans exist, otherwise overview
  const defaultTab = floorPlans.length > 0 ? 'floor-plan' : 'overview'

  return (
    <div className="flex flex-col gap-6 p-6">
      <header className="flex items-start justify-between">
        <div>
          <h1 className="text-2xl font-semibold leading-8">{site.name}</h1>
          {site.address ? (
            <p className="mt-1 text-sm text-muted-foreground leading-6">
              {site.address}
            </p>
          ) : null}
        </div>
        <div className="flex items-center gap-2">
          <Button variant="ghost">
            <Pencil className="mr-2 h-4 w-4" />
            Edit site
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="sm" aria-label="Site actions">
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem>
                <Archive className="mr-2 h-4 w-4" />
                Archive site
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </header>

      <Tabs defaultValue={defaultTab}>
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="floor-plan">Floor plan</TabsTrigger>
        </TabsList>

        <TabsContent value="overview">
          <div className="flex flex-col gap-6 mt-4">
            <Card>
              <CardHeader>
                <CardTitle>Identity</CardTitle>
              </CardHeader>
              <CardContent className="flex flex-col gap-2 text-sm">
                <div>
                  <span className="font-semibold">Timezone:</span>{' '}
                  <span className="font-mono">{site.timezone}</span>
                </div>
                <div>
                  <span className="font-semibold">Coordinates:</span>{' '}
                  {site.lat != null && site.lng != null ? (
                    <span className="font-mono">
                      {site.lat}, {site.lng}
                    </span>
                  ) : (
                    <span className="text-muted-foreground">—</span>
                  )}
                </div>
                {site.description ? (
                  <div>
                    <span className="font-semibold">Description:</span>{' '}
                    <span className="text-muted-foreground">{site.description}</span>
                  </div>
                ) : null}
                <div>
                  <Button
                    variant="outline"
                    disabled
                    title="Available in v5"
                    aria-label="Pick on map"
                    type="button"
                  >
                    <MapIcon className="mr-2 h-4 w-4" />
                    Pick on map
                  </Button>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="flex flex-row items-center justify-between">
                <CardTitle>Metering points</CardTitle>
                <Button onClick={() => setMpDialogOpen(true)}>
                  <Plus className="mr-2 h-4 w-4" />
                  Add metering point
                </Button>
              </CardHeader>
              <CardContent>
                <div className="flex flex-col items-center gap-2 py-6 text-center text-sm text-muted-foreground">
                  <p className="font-semibold text-foreground">No metering points yet</p>
                  <p>
                    A metering point is what telemetry attaches to. Add one to start receiving
                    readings.
                  </p>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Devices on this site</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="flex flex-col items-center gap-2 py-6 text-center text-sm text-muted-foreground">
                  <p className="font-semibold text-foreground">No devices on this site yet</p>
                  <p>
                    Add a device and bind it to a metering point on this site to start receiving
                    telemetry. Visit{' '}
                    <Link to="/devices" className="text-primary hover:underline">
                      Devices
                    </Link>{' '}
                    to add one.
                  </p>
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="floor-plan" className="mt-0">
          <FloorPlanTab siteID={id!} />
        </TabsContent>
      </Tabs>
    </div>
  )
}
