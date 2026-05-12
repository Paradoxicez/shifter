/**
 * MapPicker — Plan 05-08 Task 2 (GW-04)
 *
 * Reusable modal map picker for selecting a lat/lng point.
 * Used by the gateway create/edit dialog.
 *
 * Clicking anywhere on the map emits {lat, lng} and closes the dialog.
 * Clicking a site marker emits that site's lat/lng (reuses the marker layer).
 *
 * Data: fetches /api/map/data when open (enabled: open) to show context markers.
 * The MapView onSiteClick prop handles site-marker clicks in picker mode.
 * ClickEmitter renders inside MapContainer to capture raw map clicks.
 */

import { useMapEvents } from 'react-leaflet'
import { useQuery } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { MapView, type MapSite, type MapGateway } from './MapView'

/**
 * ClickEmitter — must be rendered inside a MapContainer.
 * Captures Leaflet map click events and calls onPick.
 */
function ClickEmitter({ onPick }: { onPick: (lat: number, lng: number) => void }) {
  useMapEvents({
    click: (e) => onPick(e.latlng.lat, e.latlng.lng),
  })
  return null
}

export interface MapPickerProps {
  open: boolean
  onOpenChange: (v: boolean) => void
  onPick: (lat: number, lng: number) => void
  initialLat?: number
  initialLng?: number
}

export function MapPicker({
  open,
  onOpenChange,
  onPick,
}: MapPickerProps) {
  const { data } = useQuery({
    queryKey: ['map', 'picker-data'],
    queryFn: () =>
      apiFetch<{ sites: MapSite[]; gateways: MapGateway[] }>('/api/map/data'),
    enabled: open, // only fetch when dialog opens
  })

  const handlePick = (lat: number, lng: number) => {
    onPick(lat, lng)
    onOpenChange(false)
  }

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Pick location on map"
    >
      <div className="h-[60vh] relative">
        <MapView
          sites={data?.sites ?? []}
          gateways={data?.gateways ?? []}
          capabilities="both"
          onSiteClick={handlePick}
        >
          {/* ClickEmitter sits inside MapContainer via the children slot (MapView prop) */}
          <ClickEmitter onPick={handlePick} />
        </MapView>
      </div>
      <div className="flex justify-end gap-2 pt-4">
        <Button variant="outline" onClick={() => onOpenChange(false)}>
          Cancel
        </Button>
      </div>
    </ResponsiveDialog>
  )
}
