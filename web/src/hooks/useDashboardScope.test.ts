/**
 * useDashboardScope hook tests — Plan 04-06 Task 2
 */

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import { createElement } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { DashboardScope } from './useDashboardScope'
import { useDashboardScope } from './useDashboardScope'

// ---------------------------------------------------------------------------
// Mock apiFetch (preserve ApiError so tests can instantiate it)
// ---------------------------------------------------------------------------

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    apiFetch: vi.fn(),
  }
})

import { ApiError, apiFetch } from '@/lib/api'
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

beforeEach(() => {
  vi.resetAllMocks() // resets call history AND queued mockResolvedValueOnce values
})

afterEach(() => {
  vi.resetAllMocks()
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

    // Verify data is stored under the expected query key
    const cached = queryClient.getQueryData<DashboardScope>(['dashboard', 'scope'])
    expect(cached).toEqual(scope)
  })

  it('staleTime is 5 minutes — second mount does not re-fetch', async () => {
    const scope: DashboardScope = {
      capabilities: 'electricity',
      onboarding: { gateway_count: 2, device_count: 5, uplink_count: 50 },
    }
    // Only set up ONE resolved value — if staleTime works, the second mount
    // will NOT call apiFetch again.
    mockApiFetch.mockResolvedValueOnce(scope)

    const { wrapper, queryClient } = makeWrapper()
    renderHook(() => useDashboardScope(), { wrapper })

    await waitFor(() =>
      expect(queryClient.getQueryState(['dashboard', 'scope'])?.status).toBe('success'),
    )

    // The query should not be stale immediately after fetching
    const state = queryClient.getQueryState(['dashboard', 'scope'])
    expect(state?.isInvalidated).toBe(false)

    // Mount a second hook in the same QueryClient — should use the cached result
    renderHook(() => useDashboardScope(), { wrapper })
    await new Promise((r) => setTimeout(r, 10))

    // apiFetch must only have been called once (staleTime prevents re-fetch)
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
    // Stub window.location.assign since ApiError 401 triggers a redirect in apiFetch,
    // but here we're mocking apiFetch directly so the rejection is raw.
    mockApiFetch.mockRejectedValueOnce(new ApiError(401, 'unauthorized'))

    const { wrapper } = makeWrapper()
    const { result } = renderHook(() => useDashboardScope(), { wrapper })

    await waitFor(() => expect(result.current.isError).toBe(true), { timeout: 3000 })
    expect(result.current.error).toBeInstanceOf(ApiError)
  })
})
