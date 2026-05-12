/**
 * Plan 06-04 AlertBell tests.
 */

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'

const { useAlertsRecentMock } = vi.hoisted(() => ({ useAlertsRecentMock: vi.fn() }))
vi.mock('@/hooks/useAlerts', () => ({
  useAlertsRecent: useAlertsRecentMock,
  useAckMutation: () => ({ mutate: vi.fn(), isPending: false }),
  useSnoozeMutation: () => ({ mutate: vi.fn(), isPending: false }),
}))
vi.mock('@/lib/use-current-user', () => ({
  useCurrentUser: () => ({ id: 'u', email: 'a@b', role: 'admin', must_change_password: false }),
}))

import { AlertBell } from './AlertBell'

function renderBell() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <AlertBell />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('AlertBell', () => {
  it('renders critical dot when critical_count > 0', () => {
    useAlertsRecentMock.mockReturnValue({
      data: { rows: [], unread_counts: { critical: 2, warning: 0, info: 0 } },
    })
    renderBell()
    const dot = document.querySelector('[data-severity="critical"]')
    expect(dot).not.toBeNull()
  })

  it('renders warning dot when only warning > 0', () => {
    useAlertsRecentMock.mockReturnValue({
      data: { rows: [], unread_counts: { critical: 0, warning: 1, info: 0 } },
    })
    renderBell()
    const dot = document.querySelector('[data-severity="warning"]')
    expect(dot).not.toBeNull()
  })

  it('hides dot when all counts are zero', () => {
    useAlertsRecentMock.mockReturnValue({
      data: { rows: [], unread_counts: { critical: 0, warning: 0, info: 0 } },
    })
    renderBell()
    // Only the Bell icon should be present, no severity-tinted dot.
    expect(document.querySelector('[data-severity]')).toBeNull()
  })

  it('aria-label includes critical count', () => {
    useAlertsRecentMock.mockReturnValue({
      data: { rows: [], unread_counts: { critical: 3, warning: 1, info: 0 } },
    })
    renderBell()
    const btn = screen.getByRole('button', { name: /Alerts \(4 unread, 3 critical\)/i })
    expect(btn).toBeInTheDocument()
  })
})
