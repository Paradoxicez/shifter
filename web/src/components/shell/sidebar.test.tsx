/**
 * Sidebar component tests — Plan 04-07 Task 2, extended Plan 05-08, Plan 06-04.
 *
 * Verifies Dashboard nav item appears at index 0, and that the Plan 06-04
 * order (Dashboard → Reports → Map → Gateways → Sites → Devices → Profiles
 * → Alerts → Audit (admin) → Settings) is in place.
 */

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/use-current-user', () => ({
  useCurrentUser: () => ({ role: 'admin', email: 'admin@test.com' }),
}))

// Stub the useAlertsRecent hook so the AlertsBadge inside Sidebar resolves
// without a real fetch call.
vi.mock('@/hooks/useAlerts', () => ({
  useAlertsRecent: () => ({ data: undefined }),
}))

import { Sidebar } from './sidebar'

function renderSidebar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <Sidebar />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('Sidebar', () => {
  it('renders Dashboard as the first nav item', () => {
    renderSidebar()
    const navLinks = screen.getAllByRole('link')
    expect(navLinks[0]).toHaveTextContent('Dashboard')
    expect(navLinks[0]).toHaveAttribute('href', '/')
  })

  it('renders all expected nav items including Alerts + Audit (admin)', () => {
    renderSidebar()
    expect(screen.getByRole('link', { name: /Dashboard/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Reports/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /^Map$/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Gateways/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Sites/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Devices/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Profiles/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Alerts/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Audit/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Settings/i })).toBeInTheDocument()
  })

  it('nav order is Dashboard → Reports → Map → Gateways → Sites → Devices → Profiles → Alerts → Audit → Settings (Plan 06-04)', () => {
    renderSidebar()
    const allLinks = screen.getAllByRole('link')
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
        '/alerts',
        '/audit',
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
      'Alerts',
      'Audit',
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

  it('Audit nav link is hidden for viewer (admin-only)', async () => {
    // Re-mock for this test only — viewer should not see Audit.
    vi.resetModules()
    vi.doMock('@/lib/use-current-user', () => ({
      useCurrentUser: () => ({ role: 'viewer', email: 'viewer@test.com' }),
    }))
    vi.doMock('@/hooks/useAlerts', () => ({
      useAlertsRecent: () => ({ data: undefined }),
    }))
    const { Sidebar: ViewerSidebar } = await import('./sidebar')
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { unmount } = render(
      <QueryClientProvider client={qc}>
        <MemoryRouter>
          <ViewerSidebar />
        </MemoryRouter>
      </QueryClientProvider>,
    )
    expect(screen.queryByRole('link', { name: /Audit/i })).toBeNull()
    expect(screen.queryByRole('link', { name: /Alerts/i })).toBeInTheDocument()
    unmount()
    vi.doUnmock('@/lib/use-current-user')
    vi.doUnmock('@/hooks/useAlerts')
  })
})
