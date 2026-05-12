/**
 * Absolute-positioned label overlay shown on pin hover.
 * Renders device name + relative last-update time (e.g. "2 min ago").
 */
export function DevicePinLabel({
  deviceName,
  lastSeenAt,
}: {
  deviceName: string
  lastSeenAt: string | null
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
    <div className="absolute left-4 top-0 z-10 pointer-events-none whitespace-nowrap rounded bg-card px-2 py-1 text-xs shadow-sm border">
      <span className="font-medium">{deviceName}</span>
      {lastSeenAt && (
        <span className="text-muted-foreground ml-1">· {formatRelative(lastSeenAt)}</span>
      )}
    </div>
  )
}
