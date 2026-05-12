/**
 * Plan 06-07 — AuditRowExpand
 *
 * Renders the expanded row detail: side-by-side before/after JSON diffs
 * with changed keys highlighted (D-34). Reuses the Phase 4 JsonTree component
 * with the new highlightKeys prop.
 */

import { JsonTree } from '@/components/metering-point/JsonTree'

interface AuditRowExpandProps {
  before: string | null
  after: string | null
}

/**
 * diffKeys returns the set of keys that differ between two parsed JSON objects.
 * A key is "changed" if:
 *   - it exists in one object but not the other (added/removed at key level), or
 *   - it exists in both but the values differ (by JSON serialization).
 */
function diffKeys(a: Record<string, unknown> | null, b: Record<string, unknown> | null): string[] {
  if (!a && !b) return []
  if (!a || !b) return Object.keys(a ?? b ?? {})
  const allKeys = new Set([...Object.keys(a), ...Object.keys(b)])
  const changed: string[] = []
  for (const k of allKeys) {
    if (JSON.stringify(a[k]) !== JSON.stringify(b[k])) {
      changed.push(k)
    }
  }
  return changed
}

function parseJSON(s: string | null): Record<string, unknown> | null {
  if (!s) return null
  try {
    const parsed = JSON.parse(s)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>
    }
    return null
  } catch {
    return null
  }
}

export function AuditRowExpand({ before, after }: AuditRowExpandProps) {
  const beforeObj = parseJSON(before)
  const afterObj = parseJSON(after)
  const changed = diffKeys(beforeObj, afterObj)

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 gap-6 p-4 bg-muted/30 rounded-lg">
      {/* Before panel */}
      <div>
        <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-2">
          Before
        </p>
        {before === null || before === '' ? (
          <span className="text-sm text-muted-foreground italic">(created)</span>
        ) : (
          <JsonTree
            value={beforeObj ?? before}
            defaultOpen
            highlightKeys={changed}
          />
        )}
      </div>

      {/* After panel */}
      <div>
        <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-2">
          After
        </p>
        {after === null || after === '' ? (
          <span className="text-sm text-muted-foreground italic">(removed)</span>
        ) : (
          <JsonTree
            value={afterObj ?? after}
            defaultOpen
            highlightKeys={changed}
          />
        )}
      </div>
    </div>
  )
}
