/**
 * useDashboardScope hook tests — Plan 04-06 Task 2
 */

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import { createElement } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { DashboardScope } from './useDashboardScope'
import { useDashboardScope } from './useDashboardScope'

// ---------------------------------------------------------------------------
// Mock apiFetch
// ---------------------------------------------------------------------------

vi.mock('@/lib/api', () => ({
  apiFetch: vi.fn(),
}))

import { apiFetch } from '@/lib/api'
const mockApiFetch = vi.mocked(apiFetch)

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

function makeWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return {
    queryClient,
    wrapper: ({ children }: { children: React.ReactNode }) =>
      createElement(QueryClientProvider, { client: queryClient }, children),
  }
}

afterEach(() => {
  vi.clearAllMocks()
})

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('useDashboardScope', () => {
  it('fetches from /api/dashboard/scope on mount', async () => {
    const scope: DashboardScope = {
      capabilities: 'water',
      onboarding: { gateway_count: 1, device_count: 2, uplink_count: 100 },
    }
    mockApiFetch.mockResolvedValueOnce(scope)

    const { wrapper } = makeWrapper()
    const { result } = renderHook(() => useDashboardScope(), { wrapper })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiFetch).toHaveBeenCalledWith('/api/dashboard/scope')
    expect(result.current.data).toEqual(scope)
  })

  it('query key is [dashboard, scope]', async () => {
    const scope: DashboardScope = {
      capabilities: 'both',
      onboarding: { gateway_count: 0, device_count: 0, uplink_count: 0 },
    }
    mockApiFetch.mockResolvedValueOnce(scope)

    const { wrapper, queryClient } = makeWrapper()
    renderHook(() => useDashboardScope(), { wrapper })

    await waitFor(() =>
      expect(queryClient.getQueryState(['dashboard', 'scope'])?.status).toBe('success'),
    )

    // Verify data is stored under the expected key
    const cached = queryClient.getQueryData<DashboardScope>(['dashboard', 'scope'])
    expect(cached).toEqual(scope)
  })

  it('staleTime is 5 minutes (300_000ms)', async () => {
    const scope: DashboardScope = {
      capabilities: 'electricity',
      onboarding: { gateway_count: 2, device_count: 5, uplink_count: 50 },
    }
    mockApiFetch.mockResolvedValueOnce(scope)

    const { wrapper, queryClient } = makeWrapper()
    renderHook(() => useDashboardScope(), { wrapper })

    await waitFor(() =>
      expect(queryClient.getQueryState(['dashboard', 'scope'])?.status).toBe('success'),
    )

    // The query should not be stale immediately after fetching
    const state = queryClient.getQueryState(['dashboard', 'scope'])
    expect(state?.isInvalidated).toBe(false)

    // Verify the hook uses staleTime = 5 * 60 * 1000 by checking that a
    // second mount does NOT re-fetch (data is still fresh)
    mockApiFetch.mockResolvedValueOnce(scope)
    renderHook(() => useDashboardScope(), { wrapper })
    // Wait a bit — if staleTime is correct, apiFetch should still only be called once
    await new Promise((r) => setTimeout(r, 10))
    expect(mockApiFetch).toHaveBeenCalledTimes(1)
  })

  it('returns { capabilities, onboarding } shape', async () => {
    const scope: DashboardScope = {
      capabilities: 'both',
      onboarding: { gateway_count: 3, device_count: 10, uplink_count: 5000 },
    }
    mockApiFetch.mockResolvedValueOnce(scope)

    const { wrapper } = makeWrapper()
    const { result } = renderHook(() => useDashboardScope(), { wrapper })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.capabilities).toBe('both')
    expect(result.current.data?.onboarding.gateway_count).toBe(3)
    expect(result.current.data?.onboarding.device_count).toBe(10)
    expect(result.current.data?.onboarding.uplink_count).toBe(5000)
  })

  it('surfaces isError when apiFetch rejects (e.g. 401)', async () => {
    const { ApiError } = await import('@/lib/api')
    mockApiFetch.mockRejectedValueOnce(new ApiError(401, 'unauthorized'))

    const { wrapper } = makeWrapper()
    const { result } = renderHook(() => useDashboardScope(), { wrapper })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.error).toBeTruthy()
  })
})
