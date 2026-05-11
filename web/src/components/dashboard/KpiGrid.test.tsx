/**
 * KpiGrid component tests — Plan 04-07 Task 1
 */

import { render, screen } from '@testing-library/react'
import { createElement } from 'react'
import { describe, expect, it } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import { KpiGrid, type KpiSnapshot } from './KpiGrid'

function wrap(ui: React.ReactNode) {
  return render(createElement(TooltipProvider, null, ui))
}

const waterKpi = {
  today_consumption: 47.2,
  today_unit: 'm³',
  instant_total: 12.4,
  instant_unit: 'L/min',
  period_delta_abs: 3.4,
  period_delta_pct: 8.0,
  online_count: 10,
  total_count: 12,
}

const electricityKpi = {
  today_consumption: 120.5,
  today_unit: 'kWh',
  instant_total: 5400,
  instant_unit: 'W',
  period_delta_abs: null,
  period_delta_pct: null,
  online_count: 3,
  total_count: 3,
}

describe('KpiGrid', () => {
  it('renders 4 water tiles for water-only capability', () => {
    const kpis: KpiSnapshot = { water: waterKpi }
    wrap(<KpiGrid capabilities="water" kpis={kpis} />)
    // Water labels
    expect(screen.getAllByText("Today's consumption")).toHaveLength(1)
    expect(screen.getByText('Current flow')).toBeInTheDocument()
    expect(screen.getByText('Period delta')).toBeInTheDocument()
    expect(screen.getByText('Online devices')).toBeInTheDocument()
    // No electricity label
    expect(screen.queryByText('Current load')).not.toBeInTheDocument()
  })

  it('renders 4 electricity tiles for electricity-only capability', () => {
    const kpis: KpiSnapshot = { electricity: electricityKpi }
    wrap(<KpiGrid capabilities="electricity" kpis={kpis} />)
    // Electricity labels
    expect(screen.getAllByText("Today's consumption")).toHaveLength(1)
    expect(screen.getByText('Current load')).toBeInTheDocument()
    // No water flow label
    expect(screen.queryByText('Current flow')).not.toBeInTheDocument()
  })

  it('renders 8 tiles (4 water + 4 electricity) for both capability', () => {
    const kpis: KpiSnapshot = { water: waterKpi, electricity: electricityKpi }
    wrap(<KpiGrid capabilities="both" kpis={kpis} />)
    // Both today's consumption tiles
    expect(screen.getAllByText("Today's consumption")).toHaveLength(2)
    // Water flow + electricity load
    expect(screen.getByText('Current flow')).toBeInTheDocument()
    expect(screen.getByText('Current load')).toBeInTheDocument()
  })

  it('uses instantOverride for water instant_total', () => {
    const kpis: KpiSnapshot = { water: waterKpi }
    wrap(<KpiGrid capabilities="water" kpis={kpis} instantOverride={{ water: 99.9 }} />)
    expect(screen.getByText('99.9')).toBeInTheDocument()
  })
})
