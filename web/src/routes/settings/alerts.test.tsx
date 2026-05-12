/**
 * Plan 06-04 /settings/alerts tests — rule library + roster.
 */

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'

const { useAlertRulesListMock, useAnomalyRosterMock } = vi.hoisted(() => ({
  useAlertRulesListMock: vi.fn(),
  useAnomalyRosterMock: vi.fn(),
}))
vi.mock('@/hooks/useAlerts', () => ({
  useAlertRulesList: useAlertRulesListMock,
  useAnomalyRoster: useAnomalyRosterMock,
  useCreateRuleMutation: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
  useTestFireMutation: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
}))
vi.mock('@/lib/use-current-user', () => ({
  useCurrentUser: () => ({ id: 'u', role: 'admin' }),
}))

import AlertRulesPage from './alerts'

function render_() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <AlertRulesPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('AlertRulesPage', () => {
  it('renders empty state with Add rule CTA', () => {
    useAlertRulesListMock.mockReturnValue({ data: { rules: [] }, isLoading: false })
    useAnomalyRosterMock.mockReturnValue({ data: { rows: [] }, isLoading: false })
    render_()
    expect(screen.getByText(/No rules yet/i)).toBeInTheDocument()
  })

  it('renders rule library table when rules exist', () => {
    useAlertRulesListMock.mockReturnValue({
      data: {
        rules: [
          {
            id: 'r1',
            rule_kind: 'threshold_instantaneous',
            scope_kind: 'metering_point',
            severity: 'warning',
            cooldown_seconds: 900,
          },
        ],
      },
      isLoading: false,
    })
    useAnomalyRosterMock.mockReturnValue({ data: { rows: [] }, isLoading: false })
    render_()
    expect(screen.getByText('threshold_instantaneous')).toBeInTheDocument()
  })

  it('renders anomaly roster eligible / warming-up counts', () => {
    useAlertRulesListMock.mockReturnValue({ data: { rules: [] }, isLoading: false })
    useAnomalyRosterMock.mockReturnValue({
      data: {
        rows: [
          {
            metering_point_id: 'a',
            metering_point_label: 'MP-A',
            site_label: 'S1',
            days_until_eligible: 0,
          },
          {
            metering_point_id: 'b',
            metering_point_label: 'MP-B',
            site_label: 'S1',
            days_until_eligible: 5,
          },
        ],
      },
      isLoading: false,
    })
    render_()
    // 1 eligible · 1 warming up
    expect(screen.getByText(/meters eligible/i)).toBeInTheDocument()
    expect(screen.getByText(/warming up/i)).toBeInTheDocument()
  })
})
