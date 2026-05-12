/**
 * Popover content for a DevicePin.
 * Shows: cumulative value, last reading, battery %, signal RSSI + "Open device" link.
 * This is rendered inside Popover from DevicePin — exported for testing.
 */
import { Link } from 'react-router-dom'
import { Button } from '@/components/ui/button'

export function DevicePinPopover({
  deviceId,
  deviceName,
  utilityClass,
  lastSeenAt,
  batteryPct,
  rssi,
  onOpenDevice,
}: {
  deviceId: string
  deviceName: string
  utilityClass: string
  lastSeenAt: string | null
  batteryPct: number | null
  rssi: number | null
  onOpenDevice: () => void
}) {
  function formatRelative(iso: string): string {
    const diffMs = Date.now() - new Date(iso).getTime()
    const diffMin = Math.floor(diffMs / 60_000)
    if (diffMin < 1) return 'just now'
    if (diffMin < 60) return `${diffMin} min ago`
    const diffH = Math.floor(diffMin / 60)
    if (diffH < 24) return `${diffH}h ago`
    return `${Math.floor(diffH / 24)}d ago`
  }

  return (
    <div className="space-y-2">
      <div className="font-semibold">{deviceName}</div>
      <div className="text-xs text-muted-foreground">{utilityClass}</div>
      <dl className="text-sm space-y-0.5">
        <div className="flex justify-between">
          <dt className="text-muted-foreground">Last seen</dt>
          <dd>{lastSeenAt ? formatRelative(lastSeenAt) : '—'}</dd>
        </div>
        <div className="flex justify-between">
          <dt className="text-muted-foreground">Battery</dt>
          <dd>{batteryPct != null ? `${batteryPct}%` : '—'}</dd>
        </div>
        <div className="flex justify-between">
          <dt className="text-muted-foreground">RSSI</dt>
          <dd>{rssi != null ? `${rssi} dBm` : '—'}</dd>
        </div>
      </dl>
      <Button asChild className="w-full" size="sm" onClick={onOpenDevice}>
        <Link to={`/devices/${deviceId}`}>Open device</Link>
      </Button>
    </div>
  )
}
