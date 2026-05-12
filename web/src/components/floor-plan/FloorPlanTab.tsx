import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Layers, Plus } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { apiFetch } from '@/lib/api'
import { FloorPlanSelector } from './FloorPlanSelector'
import { FloorPlanCanvas, type PlacementView } from './FloorPlanCanvas'
import { DeviceSidebar } from './DeviceSidebar'
import { UploadFloorPlanDialog } from './UploadFloorPlanDialog'
import { ReplaceImageDialog } from './ReplaceImageDialog'
import { RemovePinAlertDialog } from './RemovePinAlertDialog'
import { useFloorPlanHealth } from '@/lib/hooks/useFloorPlanHealth'

// ---------------------------------------------------------------------------
// Types matching the server's ListPlacementsByPlan response shape
// ---------------------------------------------------------------------------

type FloorPlan = {
  id: string
  label: string
  sort_order: number
  image_w: number
  image_h: number
}

type PlacementRaw = {
  device_id: string
  device_name: string
  metering_point_id: string
  x_frac: number
  y_frac: number
  utility_class: string
  last_seen_at: string | null
  battery_pct: number | null
  rssi: number | null
  expected_interval_s: number
}

type SiteDevice = {
  id: string
  name: string
  utility_class: string
}

// ---------------------------------------------------------------------------
// Empty state
// ---------------------------------------------------------------------------

function NoFloorPlansEmpty({
  siteID,
  onUpload,
}: {
  siteID: string
  onUpload: () => void
}) {
  void siteID
  return (
    <div className="flex flex-1 items-center justify-center p-8">
      <div className="max-w-md text-center space-y-4">
        <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-full bg-primary/10">
          <Layers className="h-8 w-8 text-primary" />
        </div>
        <h3 className="text-lg font-semibold">No floor plans uploaded</h3>
        <p className="text-sm text-muted-foreground">
          Upload a PNG, JPG, or PDF to start placing devices.
        </p>
        <Button onClick={onUpload}>
          <Plus className="mr-2 h-4 w-4" />
          Upload floor plan
        </Button>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Main tab
// ---------------------------------------------------------------------------

export function FloorPlanTab({ siteID }: { siteID: string }) {
  const queryClient = useQueryClient()
  const [activePlanID, setActivePlanID] = useState<string | null>(null)
  const [placingDeviceID, setPlacingDeviceID] = useState<string | null>(null)
  const [removeDeviceID, setRemoveDeviceID] = useState<string | null>(null)
  const [uploadOpen, setUploadOpen] = useState(false)
  const [replaceOpen, setReplaceOpen] = useState(false)

  // Fetch floor plans for this site
  const { data: plans = [] } = useQuery<FloorPlan[]>({
    queryKey: ['floor-plans', siteID],
    queryFn: () => apiFetch<FloorPlan[]>(`/api/sites/${siteID}/floor-plans`),
  })

  const activePlan = plans.find((p) => p.id === activePlanID) ?? plans[0] ?? null

  // Fetch placements for the active floor plan
  const { data: placements = [] } = useQuery<PlacementRaw[]>({
    queryKey: ['placements', activePlan?.id],
    queryFn: () =>
      activePlan
        ? apiFetch<PlacementRaw[]>(`/api/floor-plans/${activePlan.id}/placements`)
        : Promise.resolve([]),
    enabled: !!activePlan,
  })

  // Fetch devices on this site (for the sidebar unplaced list)
  const { data: siteDevices = [] } = useQuery<SiteDevice[]>({
    queryKey: ['site-devices', siteID],
    queryFn: () => apiFetch<SiteDevice[]>(`/api/sites/${siteID}/devices`),
  })

  // Compute live health states via SSE
  const states = useFloorPlanHealth(placements)

  // Build placement views with computed health state
  const placedIDs = new Set(placements.map((p) => p.device_id))
  const unplacedDevices = siteDevices.filter((d) => !placedIDs.has(d.id))
  const placementViews: PlacementView[] = placements.map((p) => ({
    ...p,
    state: states[p.device_id] ?? 'healthy',
  }))

  // Mutations
  const placeMutation = useMutation({
    mutationFn: (vars: { deviceID: string; xFrac: number; yFrac: number }) =>
      apiFetch(`/api/floor-plans/${activePlan!.id}/placements`, {
        method: 'POST',
        body: JSON.stringify({ device_id: vars.deviceID, x_frac: vars.xFrac, y_frac: vars.yFrac }),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['placements', activePlan?.id] })
      setPlacingDeviceID(null)
    },
  })

  const nudgeMutation = useMutation({
    mutationFn: (vars: { deviceID: string; xFrac: number; yFrac: number }) =>
      apiFetch(`/api/floor-plans/${activePlan!.id}/placements/${vars.deviceID}`, {
        method: 'PATCH',
        body: JSON.stringify({ x_frac: vars.xFrac, y_frac: vars.yFrac }),
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['placements', activePlan?.id] }),
  })

  const removeMutation = useMutation({
    mutationFn: (deviceID: string) =>
      apiFetch(`/api/floor-plans/${activePlan!.id}/placements/${deviceID}`, { method: 'DELETE' }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['placements', activePlan?.id] })
      setRemoveDeviceID(null)
    },
  })

  const replaceImageMutation = useMutation({
    mutationFn: async (file: File | Blob) => {
      const form = new FormData()
      form.append('file', file, file instanceof File ? file.name : `plan-${Date.now()}.png`)
      return apiFetch(`/api/floor-plans/${activePlan!.id}`, { method: 'PATCH', body: form })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['floor-plans', siteID] })
      toast.success('Image replaced. Review pin positions.')
      setReplaceOpen(false)
    },
  })

  if (plans.length === 0) {
    return (
      <>
        <NoFloorPlansEmpty siteID={siteID} onUpload={() => setUploadOpen(true)} />
        <UploadFloorPlanDialog siteID={siteID} open={uploadOpen} onOpenChange={setUploadOpen} />
      </>
    )
  }

  return (
    <div className="flex h-[calc(100vh-10rem)] overflow-hidden">
      <DeviceSidebar
        unplaced={unplacedDevices}
        placed={placementViews.map((p) => ({
          id: p.device_id,
          name: p.device_name,
          utility_class: p.utility_class,
          state: p.state,
        }))}
        placingDeviceID={placingDeviceID}
        onSelectDevice={(id) =>
          setPlacingDeviceID((prev) => (prev === id ? null : id))
        }
      />
      <div className="flex-1 flex flex-col min-w-0 overflow-hidden">
        <div className="flex items-center gap-2 px-3 py-2 border-b bg-card shrink-0">
          <FloorPlanSelector
            plans={plans}
            activeID={activePlan?.id ?? null}
            onChange={setActivePlanID}
          />
          <div className="ml-auto flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={() => setReplaceOpen(true)}>
              Replace image
            </Button>
            <Button variant="outline" size="sm" onClick={() => setUploadOpen(true)}>
              <Plus className="mr-1 h-3 w-3" />
              Add floor plan
            </Button>
          </div>
        </div>
        {activePlan && (
          <div className="flex-1 overflow-auto p-2">
            <FloorPlanCanvas
              imageSrc={`/api/floor-plans/${activePlan.id}/image`}
              imageW={activePlan.image_w}
              imageH={activePlan.image_h}
              placements={placementViews}
              placingDeviceID={placingDeviceID}
              onPlace={(deviceID, xFrac, yFrac) =>
                placeMutation.mutate({ deviceID, xFrac, yFrac })
              }
              onNudge={(deviceID, xFrac, yFrac) =>
                nudgeMutation.mutate({ deviceID, xFrac, yFrac })
              }
              onRemoveRequest={setRemoveDeviceID}
              onOpenDevice={(deviceID) => {
                window.location.assign(`/devices/${deviceID}`)
              }}
            />
          </div>
        )}
      </div>

      {/* Dialogs */}
      <UploadFloorPlanDialog siteID={siteID} open={uploadOpen} onOpenChange={setUploadOpen} />
      <ReplaceImageDialog
        pinCount={placements.length}
        open={replaceOpen}
        onOpenChange={setReplaceOpen}
        onReplace={(file) => replaceImageMutation.mutate(file)}
      />
      <RemovePinAlertDialog
        deviceName={
          placementViews.find((p) => p.device_id === removeDeviceID)?.device_name ?? ''
        }
        open={!!removeDeviceID}
        onOpenChange={(o) => !o && setRemoveDeviceID(null)}
        onConfirm={() => removeDeviceID && removeMutation.mutate(removeDeviceID)}
      />
    </div>
  )
}
