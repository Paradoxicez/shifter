import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { DevicePin } from './DevicePin'
import { MemoryRouter } from 'react-router-dom'
import React from 'react'

const makePlacement = (overrides = {}) => ({
  device_id: 'dev-1',
  device_name: 'Meter A',
  x_frac: 0.5,
  y_frac: 0.4,
  state: 'healthy' as 'healthy' | 'warning' | 'offline',
  utility_class: 'water',
  last_seen_at: new Date().toISOString(),
  battery_pct: 80,
  rssi: -90,
  ...overrides,
})

const containerRef = { current: document.createElement('div') } as React.RefObject<HTMLDivElement>

describe('DevicePin', () => {
  it('renders green (bg-success) for healthy state (D-22)', () => {
    render(
      <MemoryRouter>
        <DevicePin
          placement={makePlacement({ state: 'healthy' })}
          containerRef={containerRef}
          onNudge={vi.fn()}
          onRemoveRequest={vi.fn()}
          onOpenDevice={vi.fn()}
        />
      </MemoryRouter>,
    )
    const pin = screen.getByRole('button', { name: /Meter A/i })
    expect(pin).toHaveClass('bg-success')
  })

  it('renders yellow (bg-warning) for warning state (D-22)', () => {
    render(
      <MemoryRouter>
        <DevicePin
          placement={makePlacement({ state: 'warning' })}
          containerRef={containerRef}
          onNudge={vi.fn()}
          onRemoveRequest={vi.fn()}
          onOpenDevice={vi.fn()}
        />
      </MemoryRouter>,
    )
    const pin = screen.getByRole('button', { name: /Meter A/i })
    expect(pin).toHaveClass('bg-warning')
  })

  it('renders red (bg-destructive) for offline state (D-22)', () => {
    render(
      <MemoryRouter>
        <DevicePin
          placement={makePlacement({ state: 'offline' })}
          containerRef={containerRef}
          onNudge={vi.fn()}
          onRemoveRequest={vi.fn()}
          onOpenDevice={vi.fn()}
        />
      </MemoryRouter>,
    )
    const pin = screen.getByRole('button', { name: /Meter A/i })
    expect(pin).toHaveClass('bg-destructive')
  })

  it('positions via left:%/top:% from x_frac/y_frac', () => {
    render(
      <MemoryRouter>
        <DevicePin
          placement={makePlacement({ x_frac: 0.25, y_frac: 0.75 })}
          containerRef={containerRef}
          onNudge={vi.fn()}
          onRemoveRequest={vi.fn()}
          onOpenDevice={vi.fn()}
        />
      </MemoryRouter>,
    )
    const pin = screen.getByRole('button', { name: /Meter A/i })
    expect(pin).toHaveStyle({ left: '25%', top: '75%' })
  })

  it('has ring-2 ring-white and rounded-full classes', () => {
    render(
      <MemoryRouter>
        <DevicePin
          placement={makePlacement()}
          containerRef={containerRef}
          onNudge={vi.fn()}
          onRemoveRequest={vi.fn()}
          onOpenDevice={vi.fn()}
        />
      </MemoryRouter>,
    )
    const pin = screen.getByRole('button', { name: /Meter A/i })
    expect(pin).toHaveClass('ring-2')
    expect(pin).toHaveClass('ring-white')
    expect(pin).toHaveClass('rounded-full')
  })
})
