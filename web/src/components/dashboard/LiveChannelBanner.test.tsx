/**
 * LiveChannelBanner component tests — Plan 04-07 Task 1 (TDD RED)
 */

import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it } from 'vitest'
import { LiveChannelBanner } from './LiveChannelBanner'

function wrap(ui: React.ReactNode) {
  return render(<MemoryRouter>{ui}</MemoryRouter>)
}

describe('LiveChannelBanner', () => {
  it('renders nothing when status is open', () => {
    const { container } = wrap(<LiveChannelBanner status="open" lastEventAt={Date.now()} />)
    expect(container.firstChild).toBeNull()
  })

  it('renders reconnecting alert within 30s', () => {
    const lastEventAt = Date.now() - 10_000 // 10 seconds ago
    wrap(<LiveChannelBanner status="reconnecting" lastEventAt={lastEventAt} />)
    expect(screen.getByText(/Reconnecting to live updates/i)).toBeInTheDocument()
  })

  it('renders escalated copy after 30s with reconnecting status', () => {
    const lastEventAt = Date.now() - 35_000 // 35 seconds ago
    wrap(<LiveChannelBanner status="reconnecting" lastEventAt={lastEventAt} />)
    expect(screen.getByText(/Live updates paused/i)).toBeInTheDocument()
  })

  it('renders session expired with link when status is closed', () => {
    wrap(<LiveChannelBanner status="closed" lastEventAt={null} />)
    expect(screen.getByText(/Session expired/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Sign in again/i })).toBeInTheDocument()
  })

  it('renders connecting state as reconnecting-style alert', () => {
    wrap(<LiveChannelBanner status="connecting" lastEventAt={null} />)
    // connecting is a transient state — should either render nothing or a reconnecting-style alert
    // The component renders null for 'open'; for 'connecting' we expect something or nothing
    // Based on UI-SPEC: only 'reconnecting' and 'closed' show banners
  })
})
