/**
 * Sidebar component tests — Plan 04-07 Task 2
 *
 * Verifies Dashboard nav item appears at index 0.
 */

import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/use-current-user', () => ({
  useCurrentUser: () => ({ role: 'admin', email: 'admin@test.com' }),
}))

import { Sidebar } from './sidebar'

describe('Sidebar', () => {
  it('renders Dashboard as the first nav item', () => {
    render(
      <MemoryRouter>
        <Sidebar />
      </MemoryRouter>
    )

    const navLinks = screen.getAllByRole('link')
    // First link should be Dashboard (to="/")
    expect(navLinks[0]).toHaveTextContent('Dashboard')
    expect(navLinks[0]).toHaveAttribute('href', '/')
  })

  it('renders all expected nav items', () => {
    render(
      <MemoryRouter>
        <Sidebar />
      </MemoryRouter>
    )

    expect(screen.getByRole('link', { name: /Dashboard/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Gateways/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Sites/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Devices/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Settings/i })).toBeInTheDocument()
  })
})
