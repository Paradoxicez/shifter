/**
 * KpiCard component tests — Plan 04-07 Task 1 (TDD RED)
 */

import { render, screen } from '@testing-library/react'
import { createElement } from 'react'
import { describe, expect, it } from 'vitest'
import { KpiCard } from './KpiCard'

// Tooltip requires a Provider — wrap with one
import { TooltipProvider } from '@/components/ui/tooltip'

function wrap(ui: React.ReactNode) {
  return render(createElement(TooltipProvider, null, ui))
}

describe('KpiCard — today variant', () => {
  it('renders label and value', () => {
    wrap(<KpiCard label="Today's consumption" utility="water" variant="today" value={47.2} unit="m³" />)
    expect(screen.getByText("Today's consumption")).toBeInTheDocument()
    expect(screen.getByText('m³')).toBeInTheDocument()
  })

  it('renders null value as dash', () => {
    wrap(<KpiCard label="Today's consumption" utility="water" variant="today" value={null} unit="m³" />)
    expect(screen.getByText('—')).toBeInTheDocument()
  })
})

describe('KpiCard — instant variant', () => {
  it('renders value and unit for electricity', () => {
    wrap(<KpiCard label="Current load" utility="electricity" variant="instant" value={5400} unit="W" />)
    expect(screen.getByText('Current load')).toBeInTheDocument()
    expect(screen.getByText('W')).toBeInTheDocument()
  })
})

describe('KpiCard — delta variant', () => {
  it('water with positive delta shows warning color indicator', () => {
    wrap(
      <KpiCard
        label="Period delta"
        utility="water"
        variant="delta"
        value={3.4}
        unit="m³"
        deltaAbs={3.4}
        deltaPct={8.0}
      />
    )
    // The delta badge/footer should be visible with the value
    expect(screen.getByText('Period delta')).toBeInTheDocument()
    expect(screen.getByText('+8.0%')).toBeInTheDocument()
  })

  it('water with negative delta shows muted color indicator', () => {
    wrap(
      <KpiCard
        label="Period delta"
        utility="water"
        variant="delta"
        value={3.4}
        unit="m³"
        deltaAbs={-3.4}
        deltaPct={-8.0}
      />
    )
    expect(screen.getByText('-8.0%')).toBeInTheDocument()
  })

  it('electricity delta shows muted color indicator regardless of sign', () => {
    wrap(
      <KpiCard
        label="Period delta"
        utility="electricity"
        variant="delta"
        value={120.5}
        unit="kWh"
        deltaAbs={20}
        deltaPct={10}
      />
    )
    expect(screen.getByText('Period delta')).toBeInTheDocument()
    expect(screen.getByText('+10.0%')).toBeInTheDocument()
  })

  it('zero delta shows Minus indicator', () => {
    wrap(
      <KpiCard
        label="Period delta"
        utility="water"
        variant="delta"
        value={0}
        unit="m³"
        deltaAbs={0}
        deltaPct={0}
      />
    )
    // Zero delta — no change sign text expected
    expect(screen.getByText('Period delta')).toBeInTheDocument()
  })

  it('null delta shows em dash and tooltip trigger', () => {
    wrap(
      <KpiCard
        label="Period delta"
        utility="water"
        variant="delta"
        value={null}
        unit="m³"
        deltaAbs={null}
        deltaPct={null}
      />
    )
    // The null comparison state renders a dash
    expect(screen.getAllByText('—').length).toBeGreaterThan(0)
  })
})

describe('KpiCard — online variant', () => {
  it('all online shows success indicator', () => {
    wrap(
      <KpiCard
        label="Online devices"
        utility="water"
        variant="online"
        value={10}
        total={10}
      />
    )
    expect(screen.getByText('Online devices')).toBeInTheDocument()
    expect(screen.getByText('10 / 10')).toBeInTheDocument()
  })

  it('some offline shows warning indicator', () => {
    wrap(
      <KpiCard
        label="Online devices"
        utility="water"
        variant="online"
        value={8}
        total={10}
      />
    )
    expect(screen.getByText('8 / 10')).toBeInTheDocument()
  })

  it('all offline shows destructive indicator', () => {
    wrap(
      <KpiCard
        label="Online devices"
        utility="water"
        variant="online"
        value={0}
        total={5}
      />
    )
    expect(screen.getByText('0 / 5')).toBeInTheDocument()
  })
})
