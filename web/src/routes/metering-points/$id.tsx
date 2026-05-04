import { useQuery } from '@tanstack/react-query'
import { Archive, MoreHorizontal, Pencil, PowerOff, Plus, Replace } from 'lucide-react'
import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { AddDeviceDialog } from '@/routes/devices/add-device-dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { getMPDetail } from '@/lib/metering-points'
import { SwapMeterDialog } from './swap-meter-dialog'

/**
 * UI-SPEC §Metering Point detail page (DATA-01..04):
 *   - Header: MP name + breadcrumb "Sites › {site} › {MP}" + Edit + overflow
 *     (Archive, Swap meter, Decommission device).
 *   - Latest reading card: cumulative big number (mono) + unit + last uplink relative.
 *   - Active binding card: Device + DevEUI mono + Profile + Capabilities chips
 *     + Reading offset mono + valid_from + Swap meter primary CTA.
 *   - Empty binding state: "No device bound" + Add device CTA opens AddDeviceDialog
 *     with this MP pre-selected (initialMPId prop).
 */
export default function MeteringPointDetailPage() {
  const { id } = useParams<{ id: string }>()
  const [swapOpen, setSwapOpen] = useState(false)
  const [addDeviceOpen, setAddDeviceOpen] = useState(false)

  const detailQuery = useQuery({
    queryKey: ['mp-detail', id],
    queryFn: () => getMPDetail(id ?? ''),
    enabled: Boolean(id),
  })

  const detail = detailQuery.data
  if (!detail) {
    return <div className="p-6 text-sm text-muted-foreground">Loading…</div>
  }

  const minutesAgo = (() => {
    if (!detail.latest_measurement?.time) return null
    const t = new Date(detail.latest_measurement.time).getTime()
    return Math.max(0, Math.round((Date.now() - t) / 60000))
  })()

  const unit = detail.utility_class === 'water' ? 'm³' : 'kWh'

  return (
    <div className="flex flex-col gap-6 p-6">
      <header className="flex items-start justify-between">
        <div>
          <p className="text-sm text-muted-foreground">
            <Link to="/sites" className="hover:underline">
              Sites
            </Link>{' '}
            ›{' '}
            <Link to={`/sites/${detail.site.id}`} className="hover:underline">
              {detail.site.name}
            </Link>{' '}
            › {detail.name}
          </p>
          <h1 className="text-2xl font-semibold leading-8">{detail.name}</h1>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="ghost">
            <Pencil className="mr-2 h-4 w-4" />
            Edit metering point
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="sm" aria-label="MP actions">
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={() => setSwapOpen(true)} disabled={!detail.active_binding}>
                <Replace className="mr-2 h-4 w-4" />
                Swap meter
              </DropdownMenuItem>
              <DropdownMenuItem disabled={!detail.active_binding}>
                <PowerOff className="mr-2 h-4 w-4" />
                Decommission device
              </DropdownMenuItem>
              <DropdownMenuItem>
                <Archive className="mr-2 h-4 w-4" />
                Archive metering point
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </header>

      <Card>
        <CardHeader>
          <CardTitle>Latest reading</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-1">
          {detail.latest_measurement ? (
            <>
              <div className="flex items-baseline gap-2">
                <span className="text-2xl font-semibold leading-8 font-mono">
                  {detail.latest_measurement.cumulative_value ?? '—'}
                </span>
                <span className="text-sm text-muted-foreground">{unit}</span>
              </div>
              <p className="text-xs text-muted-foreground">
                Last uplink: {minutesAgo ?? '?'} min ago
              </p>
            </>
          ) : (
            <p className="text-sm text-muted-foreground">Awaiting first uplink.</p>
          )}
        </CardContent>
      </Card>

      {detail.active_binding ? (
        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <CardTitle>Active binding</CardTitle>
            <Button onClick={() => setSwapOpen(true)}>
              <Replace className="mr-2 h-4 w-4" />
              Swap meter
            </Button>
          </CardHeader>
          <CardContent className="flex flex-col gap-2 text-sm">
            <div>
              <span className="font-semibold">Device:</span> {detail.active_binding.device.name}
            </div>
            <div>
              <span className="font-semibold">DevEUI:</span>{' '}
              <span className="font-mono">{detail.active_binding.device.dev_eui}</span>
            </div>
            <div>
              <span className="font-semibold">Profile:</span>{' '}
              {detail.active_binding.device_profile.name}
            </div>
            <div className="flex items-center gap-2">
              <span className="font-semibold">Capabilities:</span>
              {detail.active_binding.device_profile.capabilities.map((c) => (
                <Badge key={c} variant="secondary" className="text-xs uppercase">
                  {c}
                </Badge>
              ))}
            </div>
            <div>
              <span className="font-semibold">Reading offset:</span>{' '}
              <span className="font-mono">{detail.active_binding.reading_offset}</span>
            </div>
            <div>
              <span className="font-semibold">Valid from:</span>{' '}
              <span className="font-mono">{detail.active_binding.valid_from}</span>
            </div>
          </CardContent>
        </Card>
      ) : (
        <Card>
          <CardHeader>
            <CardTitle>Active binding</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col items-center gap-3 py-6 text-center">
            <p className="text-sm font-semibold">No device bound</p>
            <p className="text-sm text-muted-foreground">
              Add a device to start receiving telemetry on this metering point.
            </p>
            <Button onClick={() => setAddDeviceOpen(true)}>
              <Plus className="mr-2 h-4 w-4" />
              Add device
            </Button>
          </CardContent>
        </Card>
      )}

      {detail.latest_measurement?.quality && detail.latest_measurement.quality !== 'ok' ? (
        <Alert variant="default">
          <AlertDescription>
            Latest uplink flagged: {detail.latest_measurement.quality}.
          </AlertDescription>
        </Alert>
      ) : null}

      <SwapMeterDialog
        open={swapOpen}
        onOpenChange={setSwapOpen}
        meteringPointId={detail.id}
        meteringPointName={detail.name}
      />
      <AddDeviceDialog
        open={addDeviceOpen}
        onOpenChange={setAddDeviceOpen}
        initialMPId={detail.id}
      />
    </div>
  )
}
