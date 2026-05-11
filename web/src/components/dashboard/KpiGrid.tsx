/**
 * KpiGrid — Plan 04-07 Task 1
 *
 * Renders 4 KPI tiles for single-utility installs, 8 tiles (water row + electricity row)
 * for 'both'. Responsive: 1-col on <sm, 2-col on sm-md, 4-col on md+.
 *
 * Layout per D-09:
 *   water | 4 tiles | 4 cols, 1 row
 *   electricity | 4 tiles | 4 cols, 1 row
 *   both | 8 tiles | 4 cols, 2 rows (water row first)
 */

import { KpiCard } from './KpiCard'

// Re-exported from Plan 04 endpoint contract
export interface UtilityKPI {
  today_consumption: number
  today_unit: string
  instant_total: number
  instant_unit: string
  period_delta_abs: number | null
  period_delta_pct: number | null
  online_count: number
  total_count: number
}

export interface KpiSnapshot {
  water?: UtilityKPI
  electricity?: UtilityKPI
}

export interface KpiGridProps {
  capabilities: 'water' | 'electricity' | 'both'
  kpis: KpiSnapshot
  /** From SSE-driven live recompute — overrides instant_total for each utility */
  instantOverride?: { water?: number; electricity?: number }
}

function WaterTiles({ kpi, instantOverride }: { kpi: UtilityKPI; instantOverride?: number }) {
  const instantTotal = instantOverride !== undefined ? instantOverride : kpi.instant_total
  return (
    <>
      <KpiCard
        label="Today's consumption"
        utility="water"
        variant="today"
        value={kpi.today_consumption}
        unit={kpi.today_unit}
      />
      <KpiCard
        label="Current flow"
        utility="water"
        variant="instant"
        value={instantTotal}
        unit={kpi.instant_unit}
      />
      <KpiCard
        label="Period delta"
        utility="water"
        variant="delta"
        value={kpi.period_delta_abs}
        unit={kpi.today_unit}
        deltaAbs={kpi.period_delta_abs}
        deltaPct={kpi.period_delta_pct}
      />
      <KpiCard
        label="Online devices"
        utility="water"
        variant="online"
        value={kpi.online_count}
        total={kpi.total_count}
      />
    </>
  )
}

function ElectricityTiles({ kpi, instantOverride }: { kpi: UtilityKPI; instantOverride?: number }) {
  const instantTotal = instantOverride !== undefined ? instantOverride : kpi.instant_total
  return (
    <>
      <KpiCard
        label="Today's consumption"
        utility="electricity"
        variant="today"
        value={kpi.today_consumption}
        unit={kpi.today_unit}
      />
      <KpiCard
        label="Current load"
        utility="electricity"
        variant="instant"
        value={instantTotal}
        unit={kpi.instant_unit}
      />
      <KpiCard
        label="Period delta"
        utility="electricity"
        variant="delta"
        value={kpi.period_delta_abs}
        unit={kpi.today_unit}
        deltaAbs={kpi.period_delta_abs}
        deltaPct={kpi.period_delta_pct}
      />
      <KpiCard
        label="Online devices"
        utility="electricity"
        variant="online"
        value={kpi.online_count}
        total={kpi.total_count}
      />
    </>
  )
}

export function KpiGrid({ capabilities, kpis, instantOverride }: KpiGridProps) {
  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 md:grid-cols-4 md:gap-6">
      {(capabilities === 'water' || capabilities === 'both') && kpis.water && (
        <WaterTiles kpi={kpis.water} instantOverride={instantOverride?.water} />
      )}
      {(capabilities === 'electricity' || capabilities === 'both') && kpis.electricity && (
        <ElectricityTiles kpi={kpis.electricity} instantOverride={instantOverride?.electricity} />
      )}
    </div>
  )
}
