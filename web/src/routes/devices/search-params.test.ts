import { act, renderHook } from '@testing-library/react'
import React from 'react'
import { createMemoryRouter, RouterProvider } from 'react-router-dom'
import { describe, expect, it } from 'vitest'
import {
  devicesSearchSchema,
  devicesSearchToQuery,
  useDevicesSearch,
} from './search-params'

/**
 * Plan 03-09 Task 1 — URL state hook contract.
 *
 * Direct schema coverage (no React) for the zod gate, then hook coverage via
 * a MemoryRouter that lets us inspect the URL after each setSearch call.
 */

describe('devicesSearchSchema', () => {
  it('TestZodSchema_DefaultsApplied — empty params apply defaults', () => {
    const parsed = devicesSearchSchema.parse({})
    expect(parsed).toEqual({
      site: [],
      status: undefined,
      last_seen: 'all',
      q: '',
      page: 1,
      per_page: 50,
      sort: '-last_seen',
    })
  })

  it('TestZodSchema_RejectsInvalidStatus — status=banana fails safeParse', () => {
    const parsed = devicesSearchSchema.safeParse({ status: 'banana' })
    expect(parsed.success).toBe(false)
  })

  it('TestZodSchema_AcceptsMultiSite — array of uuids passes', () => {
    // Zod v4 enforces strict RFC 9562 UUID variant + version bits — use v4 UUIDs.
    const a = '11111111-1111-4111-8111-111111111111'
    const b = '22222222-2222-4222-8222-222222222222'
    const parsed = devicesSearchSchema.parse({ site: [a, b] })
    expect(parsed.site).toEqual([a, b])
  })

  it('TestZodSchema_RejectsInvalidUUID — site=notvalid fails safeParse', () => {
    const parsed = devicesSearchSchema.safeParse({ site: ['notvalid'] })
    expect(parsed.success).toBe(false)
  })
})

/**
 * Mounts a probe component inside a MemoryRouter that calls
 * `useDevicesSearch()` and exposes the current tuple + router URL via a
 * closure. Avoids JSX so this file can stay `.test.ts` per the plan's
 * files_modified manifest.
 */
function makeHarness(initialPath: string) {
  let exposed: ReturnType<typeof useDevicesSearch> | null = null

  const Probe: React.FC = () => {
    exposed = useDevicesSearch()
    return null
  }

  const router = createMemoryRouter(
    [{ path: '/devices', element: React.createElement(Probe) }],
    { initialEntries: [initialPath] },
  )

  const utils = renderHook(() => null, {
    wrapper: () => React.createElement(RouterProvider, { router }),
  })

  return {
    get search() {
      if (!exposed) throw new Error('Probe did not mount')
      return exposed
    },
    get url() {
      return router.state.location.pathname + router.state.location.search
    },
    unmount: utils.unmount,
  }
}

describe('useDevicesSearch (Plan 03-09)', () => {
  it('TestUseDevicesSearch_UpdatePushesHistory — setSearch({page:2}) writes URL', () => {
    const h = makeHarness('/devices')
    expect(h.search[0].page).toBe(1)

    act(() => {
      h.search[1]({ page: 2 })
    })

    expect(h.url).toBe('/devices?page=2')
    expect(h.search[0].page).toBe(2)
  })

  it('TestUseDevicesSearch_MultiValueArray — site array uses append, not comma-join', () => {
    const a = '11111111-1111-4111-8111-111111111111'
    const b = '22222222-2222-4222-8222-222222222222'
    const h = makeHarness('/devices')

    act(() => {
      h.search[1]({ site: [a, b] })
    })

    expect(h.url).toBe(`/devices?site=${a}&site=${b}`)
    expect(h.search[0].site).toEqual([a, b])
  })

  it('TestUseDevicesSearch_ClearsFalsyParams — empty q removes the param', () => {
    const h = makeHarness('/devices?q=meter-1')
    expect(h.search[0].q).toBe('meter-1')

    act(() => {
      h.search[1]({ q: '' })
    })

    expect(h.url).toBe('/devices')
    expect(h.search[0].q).toBe('')
  })

  it('TestUseDevicesSearch_DeepLinkRestore — initial URL parses into typed state', () => {
    const site = '33333333-3333-4333-8333-333333333333'
    const h = makeHarness(
      `/devices?site=${site}&status=active&last_seen=24h&sort=name`,
    )
    expect(h.search[0].site).toEqual([site])
    expect(h.search[0].status).toBe('active')
    expect(h.search[0].last_seen).toBe('24h')
    expect(h.search[0].sort).toBe('name')
  })

  it('TestUseDevicesSearch_BadParamsFallBackToDefaults — invalid status silently dropped', () => {
    const h = makeHarness('/devices?status=banana')
    expect(h.search[0]).toEqual({
      site: [],
      status: undefined,
      last_seen: 'all',
      q: '',
      page: 1,
      per_page: 50,
      sort: '-last_seen',
    })
  })
})

describe('devicesSearchToQuery (canonical wire mapping)', () => {
  it('omits defaults', () => {
    const sp = devicesSearchToQuery(devicesSearchSchema.parse({}))
    expect(sp.toString()).toBe('')
  })

  it('encodes multi-site as append', () => {
    const a = '11111111-1111-4111-8111-111111111111'
    const b = '22222222-2222-4222-8222-222222222222'
    const sp = devicesSearchToQuery(
      devicesSearchSchema.parse({ site: [a, b], status: 'active' }),
    )
    expect(sp.getAll('site')).toEqual([a, b])
    expect(sp.get('status')).toBe('active')
  })
})
