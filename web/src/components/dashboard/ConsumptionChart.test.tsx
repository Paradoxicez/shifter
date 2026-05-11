/**
 * ConsumptionChart tests — Plan 04-08 Task 2 (TDD RED)
 *
 * Tests:
 *   - Renders Recharts AreaChart wrapped in ChartContainer
 *   - liveMode=true → ReferenceDot (pulse dot) present
 *   - liveMode=false → no pulse dot
 *   - animate-pulse + motion-reduce:animate-none class present in live mode
 */

import { render } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it } from 'vitest'
import { ConsumptionChart } from './ConsumptionChart'

const sampleData = [
  { bucket: '2026-05-11T00:00:00Z', cumulative_delta: 1.2 },
  { bucket: '2026-05-11T00:05:00Z', cumulative_delta: 0.8 },
  { bucket: '2026-05-11T00:10:00Z', cumulative_delta: 1.0 },
]

function renderChart(liveMode: boolean) {
  return render(
    <MemoryRouter>
      <ConsumptionChart
        data={sampleData}
        utility="water"
        liveMode={liveMode}
        bucketSec={300}
      />
    </MemoryRouter>
  )
}

describe('ConsumptionChart', () => {
  it('renders without crashing with data', () => {
    const { container } = renderChart(false)
    // Recharts renders an SVG
    expect(container.querySelector('svg')).toBeTruthy()
  })

  it('renders with empty data without crashing', () => {
    const { container } = render(
      <MemoryRouter>
        <ConsumptionChart data={[]} utility="water" liveMode={false} bucketSec={300} />
      </MemoryRouter>
    )
    expect(container).toBeTruthy()
  })

  it('liveMode=true → pulse marker element present in DOM', () => {
    const { container } = renderChart(true)
    // The live mode marker should have animate-pulse class somewhere
    const pulseEl = container.querySelector('.animate-pulse')
    expect(pulseEl).toBeTruthy()
  })

  it('liveMode=false → no pulse marker element', () => {
    const { container } = renderChart(false)
    const pulseEl = container.querySelector('.animate-pulse')
    expect(pulseEl).toBeNull()
  })

  it('liveMode=true → pulse marker has motion-reduce:animate-none class', () => {
    const { container } = renderChart(true)
    const pulseEl = container.querySelector('.animate-pulse')
    // SVG elements have className as SVGAnimatedString — use getAttribute('class')
    const classAttr = pulseEl?.getAttribute('class') ?? ''
    expect(classAttr).toContain('motion-reduce:animate-none')
  })
})
