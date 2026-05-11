/**
 * JsonTree — Plan 04-09 Task 1
 *
 * Custom recursive collapsible JSON tree (D-20 — NOT react-json-view).
 *
 * Security notes (T-04-09-02):
 *   - All rendering uses React text nodes (auto-escapes); no dangerouslySetInnerHTML anywhere.
 * Denial-of-service mitigation (T-04-09-03):
 *   - Recursion capped at MAX_DEPTH=32; deeper nodes render "…truncated" placeholder.
 *
 * Styling per UI-SPEC §Advanced-tab JSON:
 *   - key: text-sm font-mono font-semibold text-foreground
 *   - string: text-success font-mono text-sm
 *   - number: text-info font-mono text-sm
 *   - boolean: text-warning font-mono text-sm
 *   - null: text-muted-foreground italic font-mono text-sm
 *   - Indent: paddingLeft: 16 * (depth + 1) (inline style)
 *   - Chevron: ChevronRight collapsed, ChevronDown expanded
 *   - Node count: text-xs text-muted-foreground "(N items)"
 *   - depth < 1 OR name === 'extra' → defaultOpen=true
 */

import { useState } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'

const MAX_DEPTH = 32

interface JsonTreeProps {
  value: unknown
  name?: string
  depth?: number
  defaultOpen?: boolean
}

function JsonKey({ children }: { children: React.ReactNode }) {
  return <span className="text-sm font-mono font-semibold text-foreground">{children}</span>
}

export function JsonTree({ value, name, depth = 0, defaultOpen = false }: JsonTreeProps) {
  // Auto-open logic: name === 'extra' always opens; caller passes defaultOpen=true for root (depth=0)
  const shouldDefaultOpen = defaultOpen || name === 'extra'
  const [open, setOpen] = useState(shouldDefaultOpen)

  // T-04-09-03: cap recursion depth
  if (depth > MAX_DEPTH) {
    return <span className="text-muted-foreground italic text-sm font-mono">…truncated</span>
  }

  // Primitive renders
  if (value === null) {
    return (
      <div className="text-sm font-mono flex items-center gap-1">
        {name !== undefined && <><JsonKey>{name}</JsonKey><span className="text-muted-foreground">:</span></>}
        <span className="text-muted-foreground italic font-mono text-sm">null</span>
      </div>
    )
  }

  if (typeof value === 'string') {
    return (
      <div className="text-sm font-mono flex items-center gap-1">
        {name !== undefined && <><JsonKey>{name}</JsonKey><span className="text-muted-foreground">:</span></>}
        <span className="text-success font-mono text-sm">{`"${value}"`}</span>
      </div>
    )
  }

  if (typeof value === 'number') {
    return (
      <div className="text-sm font-mono flex items-center gap-1">
        {name !== undefined && <><JsonKey>{name}</JsonKey><span className="text-muted-foreground">:</span></>}
        <span className="text-info font-mono text-sm">{String(value)}</span>
      </div>
    )
  }

  if (typeof value === 'boolean') {
    return (
      <div className="text-sm font-mono flex items-center gap-1">
        {name !== undefined && <><JsonKey>{name}</JsonKey><span className="text-muted-foreground">:</span></>}
        <span className="text-warning font-mono text-sm">{String(value)}</span>
      </div>
    )
  }

  // Object / Array
  const isArray = Array.isArray(value)
  const entries: [string, unknown][] = isArray
    ? (value as unknown[]).map((v, i) => [String(i), v])
    : Object.entries(value as Record<string, unknown>)

  const count = entries.length
  const countLabel = `(${count} ${count === 1 ? 'item' : 'items'})`

  return (
    <div>
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
        className="flex items-center gap-1 text-sm font-mono"
      >
        {open ? (
          <ChevronDown className="h-3 w-3 text-muted-foreground" />
        ) : (
          <ChevronRight className="h-3 w-3 text-muted-foreground" />
        )}
        {name !== undefined && <JsonKey>{name}</JsonKey>}
        <span className="text-xs text-muted-foreground">{countLabel}</span>
      </button>
      {open && (
        <div style={{ paddingLeft: 16 * (depth + 1) }}>
          {entries.map(([k, v]) => (
            <JsonTree
              key={k}
              name={k}
              value={v}
              depth={depth + 1}
              // Auto-open 'extra' at depth=1 (child of root, where the name is 'extra')
              defaultOpen={k === 'extra'}
            />
          ))}
        </div>
      )}
    </div>
  )
}
