/**
 * Plan 06-04 /alerts page tests — URL-state filter chips drive the query.
 */

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'

const { useAlertsListMock } = vi.hoisted(() => ({ useAlertsListMock: vi.fn() }))
vi.mock('@/hooks/useAlerts', () => ({
  useAlertsList: useAlertsListMock,
  useAckMutation: () => ({ mutate: vi.fn(), isPending: false }),
  useSnoozeMutation: () => ({ mutate: vi.fn(), isPending: false }),
}))
vi.mock('@/lib/use-current-user', () => ({
  useCurrentUser: () => ({ id: 'u', role: 'admin' }),
}))

import AlertsPage from './index'

function render_(url = '/alerts') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[url]}>
        <AlertsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('AlertsPage', () => {
  it('renders empty state when no alerts', () => {
    useAlertsListMock.mockReturnValue({
      data: { rows: [], unread_counts: { critical: 0, warning: 0, info: 0 } },
      isLoading: false,
      isError: false,
    })
    render_()
    expect(screen.getByText(/No alerts match/i)).toBeInTheDocument()
  })

  it('renders alert rows', () => {
    useAlertsListMock.mockReturnValue({
      data: {
        rows: [
          {
            id: 'a1',
            rule_id: 'r1',
            rule_kind: 'threshold_instantaneous',
            severity: 'critical',
            state: 'firing',
            payload: { target: { label: 'MP-42' } },
            target_entity_type: 'metering_point',
            target_entity_id: 'mp-42',
            is_test: false,
            fired_at: new Date().toISOString(),
            muted: false,
          },
        ],
        unread_counts: { critical: 1, warning: 0, info: 0 },
      },
      isLoading: false,
      isError: false,
    })
    render_()
    expect(screen.getByText('MP-42')).toBeInTheDocument()
    expect(screen.getByText(/Alerts \(1\)/i)).toBeInTheDocument()
  })

  it('reads severity from URL search params', () => {
    useAlertsListMock.mockReturnValue({
      data: { rows: [], unread_counts: { critical: 0, warning: 0, info: 0 } },
      isLoading: false,
      isError: false,
    })
    render_('/alerts?severity=critical&status=open')
    // Verify the hook was called with the parsed severity.
    expect(useAlertsListMock).toHaveBeenCalled()
    const lastCall = useAlertsListMock.mock.calls[useAlertsListMock.mock.calls.length - 1]
    expect(lastCall[0].severity).toBe('critical')
    expect(lastCall[0].status).toBe('open')
  })
})
