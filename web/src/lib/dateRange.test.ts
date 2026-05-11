/**
 * dateRange utility tests — Plan 04-08 Task 1 (TDD RED)
 *
 * Tests:
 *   - computeRangeWindow: correct (start, end, bucketSec) for each preset
 *   - computeRangeWindow throws for invalid custom inputs
 */

import { describe, expect, it } from 'vitest'
import { computeRangeWindow } from './dateRange'

describe('computeRangeWindow — preset cases', () => {
  it('today: start is 00:00 local time today, bucketSec=300', () => {
    const now = new Date('2026-05-11T14:30:00Z')
    const result = computeRangeWindow('today', undefined, undefined, now)
    expect(result.bucketSec).toBe(300)
    expect(result.end.getTime()).toBe(now.getTime())
    // Start should be midnight local time — check it's before or equal to now
    expect(result.start.getTime()).toBeLessThanOrEqual(now.getTime())
    // And it should be at hour 0 locally
    expect(result.start.getHours()).toBe(0)
    expect(result.start.getMinutes()).toBe(0)
    expect(result.start.getSeconds()).toBe(0)
    expect(result.start.getMilliseconds()).toBe(0)
  })

  it('24h: start = now - 24h, bucketSec=300', () => {
    const now = new Date('2026-05-11T14:30:00Z')
    const result = computeRangeWindow('24h', undefined, undefined, now)
    expect(result.bucketSec).toBe(300)
    expect(result.end.getTime()).toBe(now.getTime())
    expect(result.start.getTime()).toBe(now.getTime() - 24 * 3600 * 1000)
  })

  it('7d: bucketSec=3600', () => {
    const now = new Date('2026-05-11T14:30:00Z')
    const result = computeRangeWindow('7d', undefined, undefined, now)
    expect(result.bucketSec).toBe(3600)
    expect(result.end.getTime()).toBe(now.getTime())
    expect(result.start.getTime()).toBe(now.getTime() - 7 * 24 * 3600 * 1000)
  })

  it('30d: bucketSec=14400', () => {
    const now = new Date('2026-05-11T14:30:00Z')
    const result = computeRangeWindow('30d', undefined, undefined, now)
    expect(result.bucketSec).toBe(14400)
    expect(result.end.getTime()).toBe(now.getTime())
    expect(result.start.getTime()).toBe(now.getTime() - 30 * 24 * 3600 * 1000)
  })

  it('custom span <=30d: bucketSec=3600', () => {
    const start = '2026-05-01T00:00:00Z'
    const end = '2026-05-11T00:00:00Z' // 10 days
    const result = computeRangeWindow('custom', start, end)
    expect(result.bucketSec).toBe(3600)
    // toISOString() includes milliseconds (.000Z); compare by getTime() instead
    expect(result.start.getTime()).toBe(new Date(start).getTime())
    expect(result.end.getTime()).toBe(new Date(end).getTime())
  })

  it('custom span >30d: bucketSec=86400', () => {
    const start = '2026-01-01T00:00:00Z'
    const end = '2026-05-01T00:00:00Z' // 120 days
    const result = computeRangeWindow('custom', start, end)
    expect(result.bucketSec).toBe(86400)
  })
})

describe('computeRangeWindow — throw cases', () => {
  it('custom with start > end throws', () => {
    expect(() =>
      computeRangeWindow('custom', '2026-05-11T00:00:00Z', '2026-05-01T00:00:00Z')
    ).toThrow()
  })

  it('custom with span > 1 year throws', () => {
    expect(() =>
      computeRangeWindow('custom', '2024-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
    ).toThrow()
  })

  it('custom without start throws', () => {
    expect(() =>
      computeRangeWindow('custom', undefined, '2026-05-11T00:00:00Z')
    ).toThrow()
  })

  it('custom without end throws', () => {
    expect(() =>
      computeRangeWindow('custom', '2026-05-01T00:00:00Z', undefined)
    ).toThrow()
  })
})
