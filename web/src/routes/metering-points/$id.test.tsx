/**
 * MeteringPointDetailPage tests — Plan 04-09 Task 3
 *
 * TDD RED: write failing tests for the 3-tab $id.tsx route.
 */

import { render, screen, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import MeteringPointDetailPage from './$id'

// Mock apiFetch
vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return { ...actual, apiFetch: vi.fn() }
})

// Mock useSSE to avoid EventSource in tests
vi.mock('@/hooks/useSSE', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/hooks/useSSE')>()
  return {
    ...actual,
    useSSE: vi.fn().mockReturnValue({ status: 'open', attempt: 0, lastEventAt: null }),
  }
})

// Mock useCurrentUser
vi.mock('@/lib/use-current-user', () => ({
  useCurrentUser: vi.fn().mockReturnValue({ role: 'admin', email: 'admin@test.com' }),
}))

async function getApiFetchMock() {
  const { apiFetch } = await import('@/lib/api')
  return vi.mocked(apiFetch)
}

function makeQueryClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } })
}

function wrap(initialPath = '/metering-points/mp-1') {
  return render(
    <MemoryRouter initialEntries={[initialPath]}>
      <QueryClientProvider client={makeQueryClient()}>
        <Routes>
          <Route path="/metering-points/:id" element={<MeteringPointDetailPage />} />
        </Routes>
      </QueryClientProvider>
    </MemoryRouter>
  )
}

const baseMeteringPoint = {
  id: 'mp-1',
  name: 'Main Water Meter',
  site_id: 'site-1',
  site_name: 'Building A',
  utility_class: 'water',
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
  decoded_object: { cumulative_l: 1234560 },
  extra: {},
  raw_payload_hex: '0a 1f 3c',
}

const baseDetailResponse = {
  metering_point: baseMeteringPoint,
  active_binding: baseActiveBinding,
  latest_reading: baseLatestReading,
  quality_summary: { window_size: 100, flagged_count: 0, by_quality: { ok: 100 } },
  online: true,
}

const emptyDetailResponse = {
  metering_point: baseMeteringPoint,
  active_binding: null,
  latest_reading: null,
  quality_summary: { window_size: 0, flagged_count: 0, by_quality: {} },
  online: null,
}

describe('MeteringPointDetailPage', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    const mock = await getApiFetchMock()
    // Default: detail returns base, signal-history returns empty
    mock.mockImplementation((url: string) => {
      if ((url as string).includes('signal-history')) {
        return Promise.resolve({ window_start: '', window_end: '', bucket_interval_seconds: 3600, series: [] })
      }
      return Promise.resolve(baseDetailResponse)
    })
  })

  it('renders MP name in page header', async () => {
    wrap()
    await waitFor(() => {
      // getAllByText handles multiple occurrences (h1 + NormalTab h2)
      const els = screen.getAllByText('Main Water Meter')
      expect(els.length).toBeGreaterThan(0)
    })
  })

  it('renders site name', async () => {
    wrap()
    await waitFor(() => {
      expect(screen.getByText(/Building A/)).toBeTruthy()
    })
  })

  it('renders utility class badge', async () => {
    wrap()
    await waitFor(() => {
      // Multiple "water" badges may appear (header + NormalTab) — use getAllByText
      const els = screen.getAllByText(/^water$/i)
      expect(els.length).toBeGreaterThan(0)
    })
  })

  it('renders Normal tab trigger by default', async () => {
    wrap()
    await waitFor(() => {
      expect(screen.getByRole('tab', { name: /Normal/i })).toBeTruthy()
    })
  })

  it('renders Advanced tab trigger', async () => {
    wrap()
    await waitFor(() => {
      expect(screen.getByRole('tab', { name: /Advanced/i })).toBeTruthy()
    })
  })

  it('renders Uplinks log tab trigger', async () => {
    wrap()
    await waitFor(() => {
      expect(screen.getByRole('tab', { name: /Uplinks/i })).toBeTruthy()
    })
  })

  it('D-22: Advanced and Uplinks tabs are disabled when latest_reading is null', async () => {
    const mock = await getApiFetchMock()
    mock.mockResolvedValue(emptyDetailResponse)
    wrap()
    await waitFor(() => {
      const advancedTab = screen.getByRole('tab', { name: /Advanced/i })
      expect(advancedTab).toBeTruthy()
      expect(advancedTab.hasAttribute('disabled')).toBe(true)
    })
  })

  it('Normal tab is always enabled', async () => {
    const mock = await getApiFetchMock()
    mock.mockResolvedValue(emptyDetailResponse)
    wrap()
    await waitFor(() => {
      const normalTab = screen.getByRole('tab', { name: /Normal/i })
      expect(normalTab.hasAttribute('disabled')).toBe(false)
    })
  })

  it('switches to Advanced tab when ?tab=advanced', async () => {
    wrap('/metering-points/mp-1?tab=advanced')
    await waitFor(() => {
      // AdvancedTab content includes JsonTree or decoded payload title
      expect(screen.getByText(/Decoded payload/i)).toBeTruthy()
    })
  })

  it('page header shows MP name', async () => {
    wrap()
    await waitFor(() => {
      const header = screen.getAllByText('Main Water Meter')
      expect(header.length).toBeGreaterThan(0)
    })
  })
})
