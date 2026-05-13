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
import { Checkbox } from '@/components/ui/checkbox'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { subDays } from 'date-fns'
import { ChevronDown } from 'lucide-react'
import type { AuditFilters } from '@/lib/auditParams'

interface MultiSelectChipProps {
  label: string
  options: string[]
  value: string[]
  onChange: (next: string[]) => void
}

function MultiSelectChip({ label, options, value, onChange }: MultiSelectChipProps) {
  const summary =
    value.length === 0
      ? 'Any'
      : value.length === 1
      ? value[0]
      : `${value.length} selected`
  function toggle(opt: string) {
    onChange(value.includes(opt) ? value.filter((v) => v !== opt) : [...value, opt])
  }
  return (
    <Popover>
      <PopoverTrigger asChild>
        <button
          type="button"
          className="inline-flex items-center gap-2 rounded-full border border-border bg-card px-3 py-1 text-xs hover:bg-muted/40 transition-colors"
          aria-label={`${label} filter`}
        >
          <span className="font-medium text-muted-foreground">{label}:</span>
          <span className="font-medium">{summary}</span>
          <ChevronDown className="h-3 w-3 text-muted-foreground" />
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-56 p-2" align="start">
        {options.length === 0 ? (
          <p className="text-xs text-muted-foreground p-2">No options available</p>
        ) : (
          <div className="flex flex-col gap-1 max-h-64 overflow-y-auto">
            {options.map((opt) => (
              <label
                key={opt}
                className="flex items-center gap-2 px-2 py-1.5 rounded-md hover:bg-muted/50 cursor-pointer text-xs"
              >
                <Checkbox checked={value.includes(opt)} onCheckedChange={() => toggle(opt)} />
                <span className="font-mono">{opt}</span>
              </label>
            ))}
          </div>
        )}
      </PopoverContent>
    </Popover>
  )
}

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
        <MultiSelectChip
          label="Entity type"
          options={availableEntityTypes}
          value={params.entity_type}
          onChange={(next) => setParams({ entity_type: next })}
        />
      )}

      {/* Action multi-select */}
      {availableActions.length > 0 && (
        <MultiSelectChip
          label="Action"
          options={availableActions}
          value={params.action}
          onChange={(next) => setParams({ action: next })}
        />
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
