/**
 * AdvancedTab tests — Plan 04-09 Task 2
 *
 * TDD RED: write failing tests before implementation.
 */

import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { AdvancedTab } from './AdvancedTab'
import type { MeasurementDelta } from '@/hooks/useSSE'

const latestReading = {
  time: '2026-05-11T14:30:00Z',
  cumulative_value: 1234.56,
  instant_value: 4.2,
  quality: 'ok' as const,
  battery_pct: 87,
  rssi: -65,
  snr: 8.5,
  fcnt: 428,
  decoded_object: { cumulative_l: 1234560, battery_pct: 87 },
  extra: { debug_temp: 23 },
  raw_payload_hex: '0a 1f 3c',
}

const pendingDelta: MeasurementDelta = {
  metering_point_id: 'mp-1',
  time: '2026-05-11T15:00:00Z',
  cumulative_value: 1300,
  instant_value: 5.2,
  quality: 'ok',
  battery_pct: 85,
  rssi: -70,
}

describe('AdvancedTab', () => {
  it('renders JsonTree with the root object and extra sections visible', () => {
    render(
      <AdvancedTab
        latestReading={latestReading}
        pendingPayload={null}
        clearPending={vi.fn()}
      />
    )
    // The root JsonTree opens by default (defaultOpen=true).
    // "extra" auto-opens (name === 'extra'), so debug_temp from extra is visible.
    // "object" at depth=1 stays collapsed — its children (cumulative_l, battery_pct)
    // are not visible until expanded. Check for "extra" section contents instead.
    expect(screen.getByText(/debug_temp/)).toBeTruthy()
    // The "object" key itself (collapsed count label) is also visible
    expect(screen.getByText(/object/)).toBeTruthy()
  })

  it('does NOT show "Newer payload available" alert when pendingPayload is null', () => {
    render(
      <AdvancedTab
        latestReading={latestReading}
        pendingPayload={null}
        clearPending={vi.fn()}
      />
    )
    expect(screen.queryByText(/Newer payload available/i)).toBeNull()
  })

  it('shows "Newer payload available" alert when pendingPayload is non-null', () => {
    render(
      <AdvancedTab
        latestReading={latestReading}
        pendingPayload={pendingDelta}
        clearPending={vi.fn()}
      />
    )
    expect(screen.getByText(/Newer payload available/i)).toBeTruthy()
  })

  it('calls clearPending when Refresh button clicked', () => {
    const clearPending = vi.fn()
    render(
      <AdvancedTab
        latestReading={latestReading}
        pendingPayload={pendingDelta}
        clearPending={clearPending}
      />
    )
    const refreshButton = screen.getByRole('button', { name: /Refresh/i })
    fireEvent.click(refreshButton)
    expect(clearPending).toHaveBeenCalledOnce()
  })

  it('renders the "extra" key section', () => {
    render(
      <AdvancedTab
        latestReading={latestReading}
        pendingPayload={null}
        clearPending={vi.fn()}
      />
    )
    // "extra" key should be visible (auto-opens)
    expect(screen.getByText(/debug_temp/)).toBeTruthy()
  })
})
