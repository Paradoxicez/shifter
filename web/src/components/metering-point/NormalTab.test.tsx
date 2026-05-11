/**
 * NormalTab tests — Plan 04-09 Task 2
 *
 * TDD RED: write failing tests before implementation.
 */

import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { NormalTab } from './NormalTab'

// Mock apiFetch so queries don't hit network
vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return { ...actual, apiFetch: vi.fn().mockResolvedValue({ series: [] }) }
})

// Mock useCurrentUser — default to admin for most tests
vi.mock('@/lib/use-current-user', () => ({
  useCurrentUser: vi.fn().mockReturnValue({ id: 'admin-id', role: 'admin', email: 'admin@test.com', must_change_password: false }),
}))

function makeQueryClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } })
}

function wrap(ui: React.ReactElement) {
  return render(
    <MemoryRouter>
      <QueryClientProvider client={makeQueryClient()}>{ui}</QueryClientProvider>
    </MemoryRouter>
  )
}

const baseMeteringPoint = {
  id: 'mp-1',
  name: 'Main Water Meter',
  site_id: 'site-1',
  site_name: 'Building A',
  utility_class: 'water' as const,
  location_description: null,
}

const baseActiveBinding = {
  device_id: 'dev-1',
  dev_eui: 'aabbccdd11223344',
  device_profile_name: 'Axioma W1',
  valid_from: '2026-05-01T08:00:00Z',
}

const baseLatestReading = {
  time: '2026-05-11T14:30:00Z',
  cumulative_value: 1234.56,
  instant_value: 4.2,
  quality: 'ok',
  battery_pct: 87,
  rssi: -65,
  snr: 8.5,
  fcnt: 428,
  decoded_object: {},
  extra: {},
  raw_payload_hex: '0a 1f 3c',
}

const baseQualitySummary = {
  window_size: 100,
  flagged_count: 0,
  by_quality: { ok: 100 },
}

const baseDetail = {
  metering_point: baseMeteringPoint,
  active_binding: baseActiveBinding,
  latest_reading: baseLatestReading,
  quality_summary: baseQualitySummary,
  online: true,
}

describe('NormalTab', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    // Re-setup useCurrentUser after reset
    const { useCurrentUser } = await import('@/lib/use-current-user')
    vi.mocked(useCurrentUser).mockReturnValue({ id: 'admin-id', role: 'admin', email: 'admin@test.com', must_change_password: false })
  })

  it('renders MP name', () => {
    wrap(<NormalTab detail={baseDetail} signalHistory={undefined} online={true} />)
    expect(screen.getByText('Main Water Meter')).toBeTruthy()
  })

  it('renders cumulative card when latest_reading present', () => {
    wrap(<NormalTab detail={baseDetail} signalHistory={undefined} online={true} />)
    // Cumulative card heading should appear
    expect(screen.getByText(/Cumulative/i)).toBeTruthy()
  })

  it('renders SparklineTriplet labels when latest_reading present', () => {
    wrap(<NormalTab detail={baseDetail} signalHistory={undefined} online={true} />)
    expect(screen.getByText(/BATTERY/i)).toBeTruthy()
  })

  it('renders "No device bound" text when latest_reading is null (D-22)', () => {
    const emptyDetail = {
      ...baseDetail,
      active_binding: null,
      latest_reading: null,
      online: null,
    }
    wrap(<NormalTab detail={emptyDetail} signalHistory={undefined} online={null} />)
    expect(screen.getByText(/No device bound/i)).toBeTruthy()
  })

  it('renders "Add device" CTA for admin when latest_reading is null (D-22)', () => {
    const emptyDetail = {
      ...baseDetail,
      active_binding: null,
      latest_reading: null,
      online: null,
    }
    wrap(<NormalTab detail={emptyDetail} signalHistory={undefined} online={null} />)
    expect(screen.getByText(/Add device/i)).toBeTruthy()
  })
})
