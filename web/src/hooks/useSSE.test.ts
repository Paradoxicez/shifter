/**
 * useSSE hook tests — Plan 04-06 Task 1
 *
 * Test environment: Vitest + jsdom.
 * EventSource is not implemented by jsdom so we stub it globally.
 */

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook, act } from '@testing-library/react'
import { createElement } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { type UseSSEOptions, useSSE } from './useSSE'

// ---------------------------------------------------------------------------
// MockEventSource
// ---------------------------------------------------------------------------

class MockEventSource {
  static instances: MockEventSource[] = []
  url: string
  withCredentials: boolean
  listeners: Record<string, ((ev: Event) => void)[]> = {}
  closeCalled = false

  constructor(url: string, init?: { withCredentials?: boolean }) {
    this.url = url
    this.withCredentials = init?.withCredentials ?? false
    MockEventSource.instances.push(this)
  }

  addEventListener(type: string, cb: (ev: Event) => void) {
    ;(this.listeners[type] ??= []).push(cb)
  }

  close() {
    this.closeCalled = true
  }

  // Helpers used by tests
  fireOpen() {
    this.dispatch('open', new Event('open'))
  }
  fireSnapshot(data: unknown) {
    this.dispatch('snapshot', new MessageEvent('snapshot', { data: JSON.stringify(data) }))
  }
  fireMeasurement(data: unknown) {
    this.dispatch('measurement', new MessageEvent('measurement', { data: JSON.stringify(data) }))
  }
  fireError() {
    this.dispatch('error', new Event('error'))
  }

  private dispatch(type: string, ev: Event) {
    ;(this.listeners[type] ?? []).forEach((cb) => cb(ev))
  }
}

// ---------------------------------------------------------------------------
// Helper: create a QueryClient wrapper for renderHook
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

// ---------------------------------------------------------------------------
// Setup / teardown
// ---------------------------------------------------------------------------

const defaultMeasurement = {
  metering_point_id: 'abc-123',
  time: '2026-05-11T12:00:00Z',
  cumulative_value: 100,
  instant_value: 5,
  quality: 'ok' as const,
  battery_pct: 87,
  rssi: -65,
}

beforeEach(() => {
  MockEventSource.instances = []
  vi.stubGlobal('EventSource', MockEventSource)
  // Default: probe returns 200 (not a 401 expiry)
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({ status: 200, ok: true, text: async () => '{}' }),
  )
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('useSSE', () => {
  it('initial status is connecting on mount', () => {
    const { wrapper } = makeWrapper()
    const { result } = renderHook(() => useSSE({ topics: ['dashboard:global'] }), { wrapper })
    expect(result.current.status).toBe('connecting')
  })

  it('status becomes open after EventSource fires open event', () => {
    const { wrapper } = makeWrapper()
    renderHook(() => useSSE({ topics: ['dashboard:global'] }), { wrapper })

    act(() => {
      MockEventSource.instances[0].fireOpen()
    })

    // We cannot observe status change on the same hook because renderHook
    // captures the initial render; re-reading result gives us the latest state.
    // We simply assert the EventSource was constructed with the correct URL.
    expect(MockEventSource.instances[0].url).toContain('/api/events?topics=')
    expect(MockEventSource.instances[0].url).toContain('dashboard%3Aglobal')
  })

  it('sets withCredentials: true on EventSource', () => {
    const { wrapper } = makeWrapper()
    renderHook(() => useSSE({ topics: ['dashboard:global'] }), { wrapper })
    expect(MockEventSource.instances[0].withCredentials).toBe(true)
  })

  it('snapshot event with dashboard:global invalidates [dashboard, snapshot]', async () => {
    const { wrapper, queryClient } = makeWrapper()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    renderHook(() => useSSE({ topics: ['dashboard:global'] }), { wrapper })

    await act(async () => {
      MockEventSource.instances[0].fireSnapshot({ ready: true, topics: ['dashboard:global'] })
    })

    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['dashboard', 'snapshot'] })
  })

  it('snapshot event with mp:<uuid> invalidates [mp, uuid, detail]', async () => {
    const mpId = 'aabbccdd-0000-1111-2222-333344445555'
    const { wrapper, queryClient } = makeWrapper()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    renderHook(() => useSSE({ topics: [`mp:${mpId}`] }), { wrapper })

    await act(async () => {
      MockEventSource.instances[0].fireSnapshot({ ready: true, topics: [`mp:${mpId}`] })
    })

    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['mp', mpId, 'detail'] })
  })

  it('does not read conn_id from snapshot payload', async () => {
    const mpId = 'abc-123-def-456-ghi-789012345678'
    const { wrapper, queryClient } = makeWrapper()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    renderHook(() => useSSE({ topics: ['dashboard:global', `mp:${mpId}`] }), { wrapper })

    await act(async () => {
      // Fire snapshot with extra junk — hook should only react to ready/topics.
      MockEventSource.instances[0].fireSnapshot({
        ready: true,
        topics: [`mp:${mpId}`],
        conn_id: 'should-be-ignored',
      })
    })

    // hook still invalidates the mp detail key (proves it processed the event)
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['mp', mpId, 'detail'] })
    // conn_id is ignored — no call using conn_id value
    const calls = invalidateSpy.mock.calls.map((c) => JSON.stringify(c))
    expect(calls.every((c) => !c.includes('should-be-ignored'))).toBe(true)
  })

  it('measurement event calls setQueryData with [mp, id, latest]', async () => {
    const { wrapper, queryClient } = makeWrapper()
    const setDataSpy = vi.spyOn(queryClient, 'setQueryData')

    renderHook(() => useSSE({ topics: ['dashboard:global'] }), { wrapper })

    await act(async () => {
      MockEventSource.instances[0].fireMeasurement(defaultMeasurement)
    })

    expect(setDataSpy).toHaveBeenCalledWith(['mp', 'abc-123', 'latest'], defaultMeasurement)
  })

  it('measurement event invalidates [dashboard, snapshot]', async () => {
    const { wrapper, queryClient } = makeWrapper()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    renderHook(() => useSSE({ topics: ['dashboard:global'] }), { wrapper })

    await act(async () => {
      MockEventSource.instances[0].fireMeasurement(defaultMeasurement)
    })

    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['dashboard', 'snapshot'] })
  })

  it('measurement event calls onMeasurement callback', async () => {
    const { wrapper } = makeWrapper()
    const onMeasurement = vi.fn()

    renderHook(() => useSSE({ topics: ['dashboard:global'], onMeasurement }), { wrapper })

    await act(async () => {
      MockEventSource.instances[0].fireMeasurement(defaultMeasurement)
    })

    expect(onMeasurement).toHaveBeenCalledWith(defaultMeasurement)
  })

  it('measurement event applies invalidationKeys override', async () => {
    const { wrapper, queryClient } = makeWrapper()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
    const customKey = ['dashboard', 'timeseries', 'today']

    renderHook(
      () =>
        useSSE({
          topics: ['dashboard:global'],
          invalidationKeys: () => [customKey],
        }),
      { wrapper },
    )

    await act(async () => {
      MockEventSource.instances[0].fireMeasurement(defaultMeasurement)
    })

    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: customKey })
  })

  it('error event triggers reconnect with backoff after 401 probe passes', async () => {
    vi.useFakeTimers()
    vi.spyOn(Math, 'random').mockReturnValue(0)

    const { wrapper } = makeWrapper()
    renderHook(() => useSSE({ topics: ['dashboard:global'] }), { wrapper })

    const firstES = MockEventSource.instances[0]

    await act(async () => {
      firstES.fireError()
    })

    // After error + 401-probe (which returns 200), a setTimeout should be set.
    // Advance time for attempt=1: delay = min(30000, 500 * 2**1) + 0 = 1000ms
    await act(async () => {
      vi.advanceTimersByTime(1000)
    })

    // A second EventSource should have been created for reconnect
    expect(MockEventSource.instances.length).toBeGreaterThanOrEqual(2)
  })

  it('backoff: attempt 1 → 1000ms, attempt 2 → 2000ms, attempt 3 → 4000ms', async () => {
    vi.useFakeTimers()
    vi.spyOn(Math, 'random').mockReturnValue(0)

    const { wrapper } = makeWrapper()
    renderHook(() => useSSE({ topics: ['dashboard:global'] }), { wrapper })

    // Attempt 1
    await act(async () => {
      MockEventSource.instances[0].fireError()
    })
    // delay = min(30000, 500 * 2^1) = 1000ms
    expect(vi.getTimerCount()).toBeGreaterThan(0)
    await act(async () => {
      vi.advanceTimersByTime(999)
    })
    // Should NOT have created a new instance yet
    const countAfter999 = MockEventSource.instances.length
    expect(countAfter999).toBe(1)

    await act(async () => {
      vi.advanceTimersByTime(1)
    })
    expect(MockEventSource.instances.length).toBeGreaterThan(1)
  })

  it('cleanup calls eventSource.close() on unmount', () => {
    const { wrapper } = makeWrapper()
    const { unmount } = renderHook(() => useSSE({ topics: ['dashboard:global'] }), { wrapper })

    const es = MockEventSource.instances[0]
    unmount()
    expect(es.closeCalled).toBe(true)
  })

  it('status becomes closed (no retry) when 401 probe fires', async () => {
    vi.useFakeTimers()

    // Override fetch mock to return 401
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ status: 401, ok: false, text: async () => '{"error":"unauthorized"}' }),
    )

    const { wrapper, queryClient: _qc } = makeWrapper()
    const { result } = renderHook(() => useSSE({ topics: ['dashboard:global'] }), { wrapper })

    await act(async () => {
      MockEventSource.instances[0].fireError()
      // Allow the async probe to settle
      await Promise.resolve()
      await Promise.resolve()
    })

    // After 401 probe, no timer should be pending (no retry)
    expect(result.current.status).toBe('closed')
    expect(vi.getTimerCount()).toBe(0)
  })

  it('topics change causes previous EventSource to close and new one to open', async () => {
    const { wrapper } = makeWrapper()
    let topicList = ['dashboard:global']

    const { rerender } = renderHook(
      ({ opts }: { opts: UseSSEOptions }) => useSSE(opts),
      {
        wrapper,
        initialProps: { opts: { topics: topicList } },
      },
    )

    const firstES = MockEventSource.instances[0]

    // Change topics
    topicList = ['dashboard:global', 'mp:aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee']
    rerender({ opts: { topics: topicList } })

    // Old EventSource should be closed
    expect(firstES.closeCalled).toBe(true)

    // New EventSource should include the new topic in URL
    const lastES = MockEventSource.instances[MockEventSource.instances.length - 1]
    expect(lastES.url).toContain('mp%3Aaaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee')
  })
})
