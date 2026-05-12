/**
 * Sidebar component tests — Plan 04-07 Task 2, extended Plan 05-08
 *
 * Verifies Dashboard nav item appears at index 0, and that Reports + Map
 * were added between Dashboard and Gateways (Plan 05-08).
 */

import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/use-current-user', () => ({
  useCurrentUser: () => ({ role: 'admin', email: 'admin@test.com' }),
}))

import { Sidebar } from './sidebar'

function renderSidebar() {
  return render(
    <MemoryRouter>
      <Sidebar />
    </MemoryRouter>
  )
}

describe('Sidebar', () => {
  it('renders Dashboard as the first nav item', () => {
    renderSidebar()
    const navLinks = screen.getAllByRole('link')
    // First link should be Dashboard (to="/")
    expect(navLinks[0]).toHaveTextContent('Dashboard')
    expect(navLinks[0]).toHaveAttribute('href', '/')
  })

  it('renders all expected nav items', () => {
    renderSidebar()
    expect(screen.getByRole('link', { name: /Dashboard/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Reports/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /^Map$/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Gateways/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Sites/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Devices/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Settings/i })).toBeInTheDocument()
  })

  it('nav order is Dashboard → Reports → Map → Gateways → Sites → Devices → Profiles → Settings', () => {
    renderSidebar()
    // Get the main NAV links (excludes admin section)
    const allLinks = screen.getAllByRole('link')
    // Filter to only main nav links (by href)
    const mainLinks = allLinks.filter((link) => {
      const href = link.getAttribute('href') ?? ''
      return [
        '/',
        '/reports',
        '/map',
        '/gateways',
        '/sites',
        '/devices',
        '/profiles',
        '/settings',
      ].includes(href)
    })
    const labels = mainLinks.map((l) => l.textContent?.trim())
    expect(labels).toEqual([
      'Dashboard',
      'Reports',
      'Map',
      'Gateways',
      'Sites',
      'Devices',
      'Profiles',
      'Settings',
    ])
  })

  it('includes /reports and /map nav links (Plan 05-08)', () => {
    renderSidebar()
    const reportsLink = screen.getByRole('link', { name: /Reports/i })
    const mapLink = screen.getByRole('link', { name: /^Map$/i })
    expect(reportsLink).toHaveAttribute('href', '/reports')
    expect(mapLink).toHaveAttribute('href', '/map')
  })
})
