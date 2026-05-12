/**
 * Plan 06-07 — AuditFilterChips
 *
 * Filter chips for the /audit page (UI-SPEC §Surface 6).
 * Chips: date range from/to (ISO text inputs), entity_type multi-select,
 * action multi-select, user_id text, request_id text.
 *
 * Follows the AlertCenterFilters pattern from Plan 06-04.
 */

import { Button } from '@/components/ui/button'
import { subDays } from 'date-fns'
import type { AuditFilters } from '@/lib/auditParams'

export interface AuditFilterChipsProps {
  params: AuditFilters
  setParams: (next: Partial<AuditFilters>) => void
  /** Available action and entity_type values from /api/audit/distincts */
  availableActions?: string[]
  availableEntityTypes?: string[]
}

function defaultFrom() {
  return subDays(new Date(), 7).toISOString().slice(0, 16)
}
function defaultTo() {
  return new Date().toISOString().slice(0, 16)
}

export function AuditFilterChips({
  params,
  setParams,
  availableActions = [],
  availableEntityTypes = [],
}: AuditFilterChipsProps) {
  function clearAll() {
    setParams({
      from: subDays(new Date(), 7).toISOString(),
      to: new Date().toISOString(),
      entity_type: [],
      action: [],
      user_id: undefined,
      request_id: undefined,
    })
  }

  return (
    <div className="flex flex-wrap items-center gap-2" role="toolbar" aria-label="Audit filters">
      {/* Date range — from */}
      <label className="inline-flex items-center gap-2 rounded-full border border-border bg-card px-3 py-1 text-xs">
        <span className="font-medium text-muted-foreground">From:</span>
        <input
          type="datetime-local"
          className="bg-transparent text-xs font-medium focus:outline-none"
          aria-label="From date"
          defaultValue={params.from ? params.from.slice(0, 16) : defaultFrom()}
          onBlur={(e) => {
            const v = e.target.value
            if (v) setParams({ from: new Date(v).toISOString() })
          }}
        />
      </label>

      {/* Date range — to */}
      <label className="inline-flex items-center gap-2 rounded-full border border-border bg-card px-3 py-1 text-xs">
        <span className="font-medium text-muted-foreground">To:</span>
        <input
          type="datetime-local"
          className="bg-transparent text-xs font-medium focus:outline-none"
          aria-label="To date"
          defaultValue={params.to ? params.to.slice(0, 16) : defaultTo()}
          onBlur={(e) => {
            const v = e.target.value
            if (v) setParams({ to: new Date(v).toISOString() })
          }}
        />
      </label>

      {/* Entity type multi-select */}
      {availableEntityTypes.length > 0 && (
        <label className="inline-flex items-center gap-2 rounded-full border border-border bg-card px-3 py-1 text-xs">
          <span className="font-medium text-muted-foreground">Entity type:</span>
          <select
            multiple
            className="bg-transparent text-xs font-medium focus:outline-none max-h-[80px]"
            aria-label="Entity type"
            value={params.entity_type}
            onChange={(e) => {
              const selected = Array.from(e.target.selectedOptions, (o) => o.value)
              setParams({ entity_type: selected })
            }}
          >
            {availableEntityTypes.map((et) => (
              <option key={et} value={et}>
                {et}
              </option>
            ))}
          </select>
        </label>
      )}

      {/* Action multi-select */}
      {availableActions.length > 0 && (
        <label className="inline-flex items-center gap-2 rounded-full border border-border bg-card px-3 py-1 text-xs">
          <span className="font-medium text-muted-foreground">Action:</span>
          <select
            multiple
            className="bg-transparent text-xs font-medium focus:outline-none max-h-[80px]"
            aria-label="Action"
            value={params.action}
            onChange={(e) => {
              const selected = Array.from(e.target.selectedOptions, (o) => o.value)
              setParams({ action: selected })
            }}
          >
            {availableActions.map((a) => (
              <option key={a} value={a}>
                {a}
              </option>
            ))}
          </select>
        </label>
      )}

      {/* Last 7 days chip (always shown) */}
      <button
        type="button"
        className="inline-flex items-center gap-1 rounded-full border border-border bg-card px-3 py-1 text-xs font-medium hover:bg-muted/40 transition-colors"
        aria-label="Last 7 days"
        onClick={() =>
          setParams({
            from: subDays(new Date(), 7).toISOString(),
            to: new Date().toISOString(),
          })
        }
      >
        Last 7 days
      </button>

      <Button variant="link" size="sm" onClick={clearAll}>
        Clear all
      </Button>
    </div>
  )
}
