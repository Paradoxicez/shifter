/**
 * SparklineTriplet tests — Plan 04-09 Task 1
 *
 * TDD RED: write failing tests for SparklineTriplet component.
 */

import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { SparklineTriplet } from './SparklineTriplet'

type SeriesPoint = { bucket: string; battery_pct: number | null; rssi: number | null; snr: number | null }

function makePoint(overrides: Partial<SeriesPoint> = {}): SeriesPoint {
  return {
    bucket: '2026-05-11T14:00:00Z',
    battery_pct: 80,
    rssi: -65,
    snr: 8.5,
    ...overrides,
  }
}

describe('SparklineTriplet', () => {
  it('renders battery label', () => {
    render(<SparklineTriplet series={[makePoint()]} />)
    expect(screen.getByText(/BATTERY/i)).toBeTruthy()
  })

  it('renders RSSI label', () => {
    render(<SparklineTriplet series={[makePoint()]} />)
    expect(screen.getByText(/RSSI/i)).toBeTruthy()
  })

  it('renders SNR label', () => {
    render(<SparklineTriplet series={[makePoint()]} />)
    expect(screen.getByText(/SNR/i)).toBeTruthy()
  })

  it('renders battery critical fill class when battery<10', () => {
    const { container } = render(
      <SparklineTriplet series={[makePoint({ battery_pct: 5 })]} />
    )
    // The fill color is applied via inline style or data attribute
    const batterySection = container.querySelector('[data-metric="battery"]')
    expect(batterySection?.getAttribute('data-band')).toBe('critical')
  })

  it('renders battery warning fill class when battery 10-30', () => {
    const { container } = render(
      <SparklineTriplet series={[makePoint({ battery_pct: 20 })]} />
    )
    const batterySection = container.querySelector('[data-metric="battery"]')
    expect(batterySection?.getAttribute('data-band')).toBe('warning')
  })

  it('renders battery healthy fill class when battery>30', () => {
    const { container } = render(
      <SparklineTriplet series={[makePoint({ battery_pct: 80 })]} />
    )
    const batterySection = container.querySelector('[data-metric="battery"]')
    expect(batterySection?.getAttribute('data-band')).toBe('healthy')
  })

  it('shows latest battery value numerically', () => {
    render(<SparklineTriplet series={[makePoint({ battery_pct: 87 })]} />)
    expect(screen.getByText('87%')).toBeTruthy()
  })

  it('shows latest RSSI value numerically', () => {
    render(<SparklineTriplet series={[makePoint({ rssi: -65 })]} />)
    expect(screen.getByText(/-65\s*dBm/)).toBeTruthy()
  })

  it('shows latest SNR value numerically', () => {
    render(<SparklineTriplet series={[makePoint({ snr: 8.5 })]} />)
    expect(screen.getByText(/8\.5\s*dB/)).toBeTruthy()
  })

  it('renders without crashing when series is empty', () => {
    const { container } = render(<SparklineTriplet series={[]} />)
    expect(container.firstChild).toBeTruthy()
  })
})
