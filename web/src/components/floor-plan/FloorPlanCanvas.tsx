import { useRef, useCallback } from 'react'
import { DevicePin } from './DevicePin'

export type PlacementView = {
  device_id: string
  device_name: string
  x_frac: number
  y_frac: number
  state: 'healthy' | 'warning' | 'offline'
  utility_class: string
  last_seen_at: string | null
  battery_pct: number | null
  rssi: number | null
}

export function FloorPlanCanvas({
  imageSrc,
  imageW,
  imageH,
  placements,
  placingDeviceID,
  onPlace,
  onNudge,
  onRemoveRequest,
  onOpenDevice,
}: {
  imageSrc: string
  imageW: number
  imageH: number
  placements: PlacementView[]
  placingDeviceID: string | null
  onPlace: (deviceID: string, xFrac: number, yFrac: number) => void
  onNudge: (deviceID: string, xFrac: number, yFrac: number) => void
  onRemoveRequest: (deviceID: string) => void
  onOpenDevice: (deviceID: string) => void
}) {
  const containerRef = useRef<HTMLDivElement>(null)

  const handleClick = useCallback(
    (e: React.PointerEvent<HTMLDivElement>) => {
      if (!placingDeviceID || !containerRef.current) return
      // Ignore clicks that originate on a pin (handled by pin's own handler).
      if ((e.target as HTMLElement).dataset.role === 'device-pin') return
      const rect = containerRef.current.getBoundingClientRect()
      const xFrac = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width))
      const yFrac = Math.max(0, Math.min(1, (e.clientY - rect.top) / rect.height))
      onPlace(placingDeviceID, xFrac, yFrac)
    },
    [placingDeviceID, onPlace],
  )

  return (
    <div
      ref={containerRef}
      className={`relative w-full touch-none select-none ${placingDeviceID ? 'cursor-crosshair' : 'cursor-default'}`}
      onPointerUp={handleClick}
      role={placingDeviceID ? 'application' : 'img'}
      aria-label={placingDeviceID ? 'Floor plan — click to place device' : 'Floor plan'}
      style={{ aspectRatio: `${imageW} / ${imageH}` }}
    >
      <img src={imageSrc} alt="Floor plan" className="w-full block" draggable={false} />
      {placements.map((p) => (
        <DevicePin
          key={p.device_id}
          placement={p}
          containerRef={containerRef}
          onNudge={(xFrac, yFrac) => onNudge(p.device_id, xFrac, yFrac)}
          onRemoveRequest={() => onRemoveRequest(p.device_id)}
          onOpenDevice={() => onOpenDevice(p.device_id)}
        />
      ))}
    </div>
  )
}
