import { PowerOff } from 'lucide-react'
import { Button } from '@/components/ui/button'

/**
 * Plan 03-09 §UI-SPEC §Bulk-action bar — sticky strip below toolbar.
 *
 *   Sticky h-12 bg-secondary border-y px-4 flex items-center justify-between
 *
 * Hidden when no rows selected. Admin-only — viewer never sees this surface
 * (defense-in-depth; server is authoritative).
 */
export interface BulkActionBarProps {
  selectedIDs: string[]
  onClearSelection: () => void
  onDecommission: () => void
}

export function BulkActionBar({
  selectedIDs,
  onClearSelection,
  onDecommission,
}: BulkActionBarProps) {
  if (selectedIDs.length === 0) return null
  const n = selectedIDs.length
  return (
    <div
      role="region"
      aria-label="Bulk actions"
      className="sticky top-0 z-10 flex h-12 items-center justify-between gap-3 border-y bg-secondary px-4"
    >
      <span className="text-sm font-semibold">{n} selected</span>
      <div className="flex items-center gap-2">
        <Button variant="destructive" size="sm" onClick={onDecommission}>
          <PowerOff className="mr-2 h-4 w-4" />
          Decommission {n} {n === 1 ? 'device' : 'devices'}
        </Button>
        <Button variant="outline" size="sm" disabled>
          Export selected
        </Button>
        <Button variant="ghost" size="sm" onClick={onClearSelection}>
          Clear selection
        </Button>
      </div>
    </div>
  )
}
