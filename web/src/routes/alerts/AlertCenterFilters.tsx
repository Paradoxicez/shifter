/**
 * Plan 06-04 — filter chips for the /alerts page (UI-SPEC §Surface 2).
 *
 * Chips: Severity, Status, Category, Target type. Each chip is a select
 * (Popover-style would be nicer per the UI-SPEC but a native select keeps
 * the surface small for v1 — the URL-state contract is identical).
 */

import { Button } from '@/components/ui/button'
import type { AlertsParams } from '@/lib/alertParams'

export interface AlertCenterFiltersProps {
  params: AlertsParams
  setParams: (next: Partial<AlertsParams>) => void
}

const SEVERITY_OPTIONS: Array<AlertsParams['severity']> = ['all', 'critical', 'warning', 'info']
const STATUS_OPTIONS: Array<AlertsParams['status']> = [
  'open',
  'firing',
  'acknowledged',
  'snoozed',
  'cleared',
  'all',
]
const CATEGORY_OPTIONS: Array<AlertsParams['category']> = ['all', 'threshold', 'offline', 'anomaly']
const TARGET_OPTIONS: Array<AlertsParams['target_type']> = [
  'all',
  'metering_point',
  'site',
  'gateway',
]

export function AlertCenterFilters({ params, setParams }: AlertCenterFiltersProps) {
  const clearAll = () => {
    setParams({ severity: 'all', status: 'open', category: 'all', target_type: 'all' })
  }
  return (
    <div className="flex flex-wrap items-center gap-2" role="toolbar" aria-label="Alert filters">
      <FilterChip
        label="Severity"
        value={params.severity}
        options={SEVERITY_OPTIONS}
        onChange={(v) => setParams({ severity: v as AlertsParams['severity'] })}
      />
      <FilterChip
        label="Status"
        value={params.status}
        options={STATUS_OPTIONS}
        onChange={(v) => setParams({ status: v as AlertsParams['status'] })}
      />
      <FilterChip
        label="Category"
        value={params.category}
        options={CATEGORY_OPTIONS}
        onChange={(v) => setParams({ category: v as AlertsParams['category'] })}
      />
      <FilterChip
        label="Target"
        value={params.target_type}
        options={TARGET_OPTIONS}
        onChange={(v) => setParams({ target_type: v as AlertsParams['target_type'] })}
      />
      <Button variant="link" size="sm" onClick={clearAll}>
        Clear all
      </Button>
    </div>
  )
}

function FilterChip({
  label,
  value,
  options,
  onChange,
}: {
  label: string
  value: string
  options: readonly string[]
  onChange: (v: string) => void
}) {
  return (
    <label className="inline-flex items-center gap-2 rounded-full border border-border bg-card px-3 py-1 text-xs">
      <span className="font-medium text-muted-foreground">{label}:</span>
      <select
        className="bg-transparent text-xs font-medium focus:outline-none"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        aria-label={label}
      >
        {options.map((opt) => (
          <option key={opt} value={opt}>
            {opt}
          </option>
        ))}
      </select>
    </label>
  )
}
