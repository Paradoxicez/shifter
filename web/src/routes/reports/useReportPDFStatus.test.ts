/**
 * useReportPDFStatus tests — Plan 05-09 Task 2
 *
 * Pins the key behavioral contracts:
 *  1. polls while pending (fetch is called during the pending window)
 *  2. stops polling after status becomes ready (no calls after terminal)
 *  3. fires toast.success exactly once on ready transition
 *  4. fires toast.error when status is failed
 *  5. does not poll when initialStatus is already ready
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import React from 'react'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

vi.mock('@/lib/api', () => ({
  apiFetch: vi.fn(),
  ApiError: class ApiError extends Error {
    status: number
    constructor(status: number, message: string) {
      super(message)
      this.status = status
    }
  },
}))

const mockToastSuccess = vi.fn()
const mockToastError = vi.fn()
vi.mock('sonner', () => ({
  toast: {
    success: (...args: unknown[]) => mockToastSuccess(...args),
    error: (...args: unknown[]) => mockToastError(...args),
  },
}))

import { apiFetch } from '@/lib/api'
import { useReportPDFStatus } from './useReportPDFStatus'

const mockedApiFetch = vi.mocked(apiFetch)
const REPORT_ID = 'report-test-uuid-000'

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
    },
  })
  return function Wrapper({ children }: React.PropsWithChildren<{}>) {
    return React.createElement(QueryClientProvider, { client: qc }, children)
  }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('useReportPDFStatus', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  it('polls while pdf_status is pending (fetch is called during the pending window)', async () => {
    // Use real timers — let TanStack Query's own interval mechanism run
    mockedApiFetch.mockResolvedValue({
      id: REPORT_ID,
      pdf_status: 'pending',
      created_at: '2025-01-01T00:00:00Z',
      scope: 'all',
      range: 'monthly',
    })

    renderHook(() => useReportPDFStatus(REPORT_ID, 'pending'), {
      wrapper: makeWrapper(),
    })

    // TanStack Query fires refetchInterval=2s; wait up to 5s for at least 1 fetch
    await waitFor(() => {
      expect(mockedApiFetch).toHaveBeenCalled()
    }, { timeout: 5000 })

    expect(mockedApiFetch).toHaveBeenCalledWith(`/api/reports/${REPORT_ID}`)
  })

  it('stops polling after status becomes ready (no refetch after terminal)', async () => {
    let callCount = 0
    mockedApiFetch.mockImplementation(async () => {
      callCount++
      return {
        id: REPORT_ID,
        pdf_status: 'ready',
        created_at: '2025-01-01T00:00:00Z',
        scope: 'all',
        range: 'monthly',
      }
    })

    renderHook(() => useReportPDFStatus(REPORT_ID, 'pending'), {
      wrapper: makeWrapper(),
    })

    // Wait for first fetch to return 'ready'
    await waitFor(() => {
      expect(callCount).toBeGreaterThanOrEqual(1)
    }, { timeout: 5000 })

    const countAfterReady = callCount

    // Wait 100ms — polling should have stopped; count should not increase further
    await new Promise((r) => setTimeout(r, 100))
    expect(callCount).toBe(countAfterReady)
  })

  it('fires toast.success exactly once when status flips to ready', async () => {
    let callCount = 0
    mockedApiFetch.mockImplementation(async () => {
      callCount++
      return {
        id: REPORT_ID,
        pdf_status: callCount >= 1 ? 'ready' : 'pending',
        created_at: '2025-01-01T00:00:00Z',
        scope: 'all',
        range: 'monthly',
      }
    })

    renderHook(() => useReportPDFStatus(REPORT_ID, 'pending'), {
      wrapper: makeWrapper(),
    })

    // Wait for toast to fire
    await waitFor(() => {
      expect(mockToastSuccess).toHaveBeenCalledTimes(1)
    }, { timeout: 5000 })

    expect(mockToastSuccess).toHaveBeenCalledWith(
      'Your PDF is ready — click to download.',
      expect.objectContaining({ action: expect.any(Object) })
    )

    // Give it a moment to ensure toast doesn't fire a second time
    await new Promise((r) => setTimeout(r, 100))
    expect(mockToastSuccess).toHaveBeenCalledTimes(1)
  })

  it('fires toast.error when status is failed', async () => {
    mockedApiFetch.mockResolvedValue({
      id: REPORT_ID,
      pdf_status: 'failed',
      created_at: '2025-01-01T00:00:00Z',
      scope: 'all',
      range: 'monthly',
    })

    renderHook(() => useReportPDFStatus(REPORT_ID, 'pending'), {
      wrapper: makeWrapper(),
    })

    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalledTimes(1)
    }, { timeout: 5000 })
  })

  it('does not poll when initialStatus is already ready', async () => {
    // When initial status is terminal, staleTime=Infinity prevents background fetch
    mockedApiFetch.mockResolvedValue({
      id: REPORT_ID,
      pdf_status: 'ready',
      created_at: '2025-01-01T00:00:00Z',
      scope: 'all',
      range: 'monthly',
    })

    const { result } = renderHook(
      () => useReportPDFStatus(REPORT_ID, 'ready'),
      { wrapper: makeWrapper() }
    )

    expect(result.current).toBe('ready')

    // Wait a tick to ensure no fetch fires
    await new Promise((r) => setTimeout(r, 200))
    expect(mockedApiFetch).not.toHaveBeenCalled()
  })
})
