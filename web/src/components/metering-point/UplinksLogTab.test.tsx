/**
 * UplinksLogTab tests — Plan 04-09 Task 2
 */

import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { UplinksLogTab } from './UplinksLogTab'

// vi.mock is hoisted — cannot reference external const in factory
vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return { ...actual, apiFetch: vi.fn() }
})

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

function makeUplink(idx = 0) {
  return {
    time: `2026-05-11T14:${String(idx % 60).padStart(2, '0')}:00Z`,
    quality: 'ok',
    cumulative_value: 1234.56 + idx,
    instant_value: 4.2,
    battery_pct: 87,
    rssi: -65,
    snr: 8.5,
    fcnt: 100 - idx,
    raw_payload_hex: '0a 1f 3c',
    decoded_object: {},
  }
}

function makeUplinksResponse(count: number, hasMore = false) {
  return {
    uplinks: Array.from({ length: count }, (_, i) => makeUplink(i)),
    has_more: hasMore,
    next_before: hasMore ? '2026-05-11T13:30:00Z' : null,
  }
}

// Helper to access the mocked apiFetch
async function getApiFetchMock() {
  const { apiFetch } = await import('@/lib/api')
  return vi.mocked(apiFetch)
}

describe('UplinksLogTab', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    const mock = await getApiFetchMock()
    mock.mockResolvedValue(makeUplinksResponse(5))
  })

  it('renders table with uplinks', async () => {
    wrap(<UplinksLogTab meteringPointId="mp-1" />)
    await waitFor(() => {
      // Should show at least one "ok" quality badge
      expect(screen.getAllByText(/^ok$/).length).toBeGreaterThan(0)
    })
  })

  it('shows "Load more" button when has_more=true', async () => {
    const mock = await getApiFetchMock()
    mock.mockResolvedValue(makeUplinksResponse(100, true))
    wrap(<UplinksLogTab meteringPointId="mp-1" />)
    await waitFor(() => {
      expect(screen.getByText(/Load more/i)).toBeTruthy()
    })
  })

  it('hides "Load more" button when has_more=false', async () => {
    wrap(<UplinksLogTab meteringPointId="mp-1" />)
    await waitFor(() => {
      expect(screen.queryByText(/Load more/i)).toBeNull()
    })
  })

  it('renders quality filter chips', async () => {
    wrap(<UplinksLogTab meteringPointId="mp-1" />)
    // Quality filter chips render immediately (no fetch needed)
    expect(screen.getByText(/decode fail/i)).toBeTruthy()
  })

  it('shows "No uplinks match your filters" when response is empty', async () => {
    const mock = await getApiFetchMock()
    mock.mockResolvedValue({ uplinks: [], has_more: false, next_before: null })
    wrap(<UplinksLogTab meteringPointId="mp-1" />)
    await waitFor(() => {
      expect(screen.getByText(/No uplinks match/i)).toBeTruthy()
    })
  })

  it('shows cap text at 500 rows', async () => {
    const mock = await getApiFetchMock()
    mock.mockResolvedValue(makeUplinksResponse(500, true))
    wrap(<UplinksLogTab meteringPointId="mp-1" />)
    await waitFor(() => {
      expect(screen.getByText(/Showing 500/i)).toBeTruthy()
    })
  })

  it('clicking Load more fetches more rows', async () => {
    const mock = await getApiFetchMock()
    mock.mockResolvedValueOnce(makeUplinksResponse(100, true))
    mock.mockResolvedValueOnce(makeUplinksResponse(50, false))
    wrap(<UplinksLogTab meteringPointId="mp-1" />)
    await waitFor(() => {
      expect(screen.getByText(/Load more/i)).toBeTruthy()
    })
    fireEvent.click(screen.getByText(/Load more/i))
    await waitFor(() => {
      expect(mock).toHaveBeenCalledTimes(2)
    })
  })
})
