import { useCallback, useRef } from 'react'
import { Link } from 'react-router-dom'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Button } from '@/components/ui/button'
import { useDebounceCallback } from '@/lib/hooks/useDebounceCallback'
import type { PlacementView } from './FloorPlanCanvas'

function formatRelative(iso: string): string {
  const diffMs = Date.now() - new Date(iso).getTime()
  const diffMin = Math.floor(diffMs / 60_000)
  if (diffMin < 1) return 'just now'
  if (diffMin < 60) return `${diffMin} min ago`
  const diffH = Math.floor(diffMin / 60)
  if (diffH < 24) return `${diffH}h ago`
  return `${Math.floor(diffH / 24)}d ago`
}

export function DevicePin({
  placement,
  containerRef,
  onNudge,
  onRemoveRequest,
  onOpenDevice,
}: {
  placement: PlacementView
  containerRef: React.RefObject<HTMLDivElement | null>
  onNudge: (xFrac: number, yFrac: number) => void
  onRemoveRequest: () => void
  onOpenDevice: () => void
}) {
  const draggingRef = useRef(false)
  const debouncedNudge = useDebounceCallback(onNudge, 300)

  const stateClass = (
    {
      healthy: 'bg-success',
      warning: 'bg-warning',
      offline: 'bg-destructive',
    } as const
  )[placement.state]

  const handlePointerDown = useCallback(
    (e: React.PointerEvent) => {
      e.preventDefault()
      e.stopPropagation()
      ;(e.target as HTMLElement).setPointerCapture(e.pointerId)
      draggingRef.current = false

      const rect = containerRef.current?.getBoundingClientRect()
      if (!rect) return

      const onMove = (move: PointerEvent) => {
        draggingRef.current = true
        const xFrac = Math.max(0, Math.min(1, (move.clientX - rect.left) / rect.width))
        const yFrac = Math.max(0, Math.min(1, (move.clientY - rect.top) / rect.height))
        debouncedNudge(xFrac, yFrac)
      }
      const onUp = () => {
        window.removeEventListener('pointermove', onMove)
        window.removeEventListener('pointerup', onUp)
      }
      window.addEventListener('pointermove', onMove)
      window.addEventListener('pointerup', onUp)
    },
    [containerRef, debouncedNudge],
  )

  return (
    <Popover>
      <PopoverTrigger asChild>
        <div
          data-role="device-pin"
          role="button"
          tabIndex={0}
          aria-label={`Device ${placement.device_name}, ${placement.state}`}
          className={`absolute h-3 w-3 rounded-full ring-2 ring-white shadow-sm -translate-x-1/2 -translate-y-1/2 cursor-pointer ${stateClass}`}
          style={{ left: `${placement.x_frac * 100}%`, top: `${placement.y_frac * 100}%` }}
          onPointerDown={handlePointerDown}
          onContextMenu={(e) => {
            e.preventDefault()
            onRemoveRequest()
          }}
        />
      </PopoverTrigger>
      <PopoverContent className="w-64">
        <div className="space-y-2">
          <div className="font-semibold">{placement.device_name}</div>
          <div className="text-xs text-muted-foreground">{placement.utility_class}</div>
          <dl className="text-sm space-y-0.5">
            <div className="flex justify-between">
              <dt className="text-muted-foreground">Last seen</dt>
              <dd>{placement.last_seen_at ? formatRelative(placement.last_seen_at) : '—'}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-muted-foreground">Battery</dt>
              <dd>{placement.battery_pct != null ? `${placement.battery_pct}%` : '—'}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-muted-foreground">RSSI</dt>
              <dd>{placement.rssi != null ? `${placement.rssi} dBm` : '—'}</dd>
            </div>
          </dl>
          <Button asChild className="w-full" size="sm" onClick={onOpenDevice}>
            <Link to={`/devices/${placement.device_id}`}>Open device</Link>
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )
}
