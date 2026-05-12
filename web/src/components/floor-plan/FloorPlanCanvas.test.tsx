import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { FloorPlanCanvas } from './FloorPlanCanvas'

// Minimal placement for tests
const makePlacement = (overrides = {}) => ({
  device_id: 'dev-1',
  device_name: 'Meter A',
  x_frac: 0.5,
  y_frac: 0.4,
  state: 'healthy' as const,
  utility_class: 'water',
  last_seen_at: new Date().toISOString(),
  battery_pct: 80,
  rssi: -90,
  ...overrides,
})

describe('FloorPlanCanvas', () => {
  it('click → fractional coords math (xFrac = (clientX-rect.left)/rect.width)', () => {
    const onPlace = vi.fn()

    // Mock getBoundingClientRect on the container
    const { container } = render(
      <FloorPlanCanvas
        imageSrc="/api/floor-plans/1/image"
        imageW={800}
        imageH={600}
        placements={[]}
        placingDeviceID="dev-abc"
        onPlace={onPlace}
        onNudge={vi.fn()}
        onRemoveRequest={vi.fn()}
        onOpenDevice={vi.fn()}
      />,
    )

    const canvas = container.firstChild as HTMLElement
    // Override getBoundingClientRect so we can calculate expected fractions
    vi.spyOn(canvas, 'getBoundingClientRect').mockReturnValue({
      left: 100, top: 50, width: 400, height: 300,
      right: 500, bottom: 350, x: 100, y: 50, toJSON: () => {},
    } as DOMRect)

    // Simulate pointer click at clientX=300, clientY=200
    // Expected xFrac = (300 - 100) / 400 = 0.5
    // Expected yFrac = (200 - 50) / 300 = 0.5
    fireEvent.pointerUp(canvas, { clientX: 300, clientY: 200 })

    expect(onPlace).toHaveBeenCalledWith('dev-abc', 0.5, 0.5)
  })

  it('does not fire onPlace when placingDeviceID is null', () => {
    const onPlace = vi.fn()
    const { container } = render(
      <FloorPlanCanvas
        imageSrc="/api/floor-plans/1/image"
        imageW={800}
        imageH={600}
        placements={[]}
        placingDeviceID={null}
        onPlace={onPlace}
        onNudge={vi.fn()}
        onRemoveRequest={vi.fn()}
        onOpenDevice={vi.fn()}
      />,
    )
    const canvas = container.firstChild as HTMLElement
    fireEvent.pointerUp(canvas, { clientX: 100, clientY: 100 })
    expect(onPlace).not.toHaveBeenCalled()
  })

  it('renders DevicePin components for each placement', () => {
    render(
      <FloorPlanCanvas
        imageSrc="/api/floor-plans/1/image"
        imageW={800}
        imageH={600}
        placements={[makePlacement({ device_id: 'dev-1', device_name: 'Meter A' })]}
        placingDeviceID={null}
        onPlace={vi.fn()}
        onNudge={vi.fn()}
        onRemoveRequest={vi.fn()}
        onOpenDevice={vi.fn()}
      />,
    )
    // The DevicePin renders a button with aria-label
    expect(screen.getByRole('button', { name: /Meter A/i })).toBeInTheDocument()
  })

  it('applies cursor-crosshair class when placingDeviceID is set', () => {
    const { container } = render(
      <FloorPlanCanvas
        imageSrc="/api/floor-plans/1/image"
        imageW={800}
        imageH={600}
        placements={[]}
        placingDeviceID="dev-x"
        onPlace={vi.fn()}
        onNudge={vi.fn()}
        onRemoveRequest={vi.fn()}
        onOpenDevice={vi.fn()}
      />,
    )
    expect(container.firstChild).toHaveClass('cursor-crosshair')
  })

  it('applies touch-none class on canvas container', () => {
    const { container } = render(
      <FloorPlanCanvas
        imageSrc="/api/floor-plans/1/image"
        imageW={800}
        imageH={600}
        placements={[]}
        placingDeviceID={null}
        onPlace={vi.fn()}
        onNudge={vi.fn()}
        onRemoveRequest={vi.fn()}
        onOpenDevice={vi.fn()}
      />,
    )
    expect(container.firstChild).toHaveClass('touch-none')
  })
})
