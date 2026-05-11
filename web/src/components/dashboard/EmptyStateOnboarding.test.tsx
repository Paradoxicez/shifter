/**
 * EmptyStateOnboarding component tests — Plan 04-07 Task 1 (TDD RED)
 */

import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it } from 'vitest'
import { EmptyStateOnboarding } from './EmptyStateOnboarding'

function wrap(ui: React.ReactNode) {
  return render(<MemoryRouter>{ui}</MemoryRouter>)
}

describe('EmptyStateOnboarding', () => {
  it('shows "Add your first gateway" when gateway_count is 0', () => {
    wrap(
      <EmptyStateOnboarding
        onboarding={{ gateway_count: 0, device_count: 0, uplink_count: 0 }}
      />
    )
    expect(screen.getByText(/Add your first gateway/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Add gateway/i })).toHaveAttribute('href', '/gateways')
  })

  it('shows "Now add your first device" when gateway_count > 0 but device_count is 0', () => {
    wrap(
      <EmptyStateOnboarding
        onboarding={{ gateway_count: 1, device_count: 0, uplink_count: 0 }}
      />
    )
    expect(screen.getByText(/Now add your first device/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Add device/i })).toHaveAttribute('href', '/devices')
  })

  it('shows "Waiting for first uplink" spinner when gateway and device count > 0 but uplink_count is 0', () => {
    wrap(
      <EmptyStateOnboarding
        onboarding={{ gateway_count: 1, device_count: 1, uplink_count: 0 }}
      />
    )
    expect(screen.getByText(/Waiting for first uplink/i)).toBeInTheDocument()
  })
})
