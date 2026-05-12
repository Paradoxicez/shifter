/**
 * Plan 06-04 AnomalyStateCard tests — three states + viewer read-only.
 */

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { useMPAnomalyStateMock, useToggleMock } = vi.hoisted(() => ({
  useMPAnomalyStateMock: vi.fn(),
  useToggleMock: vi.fn(),
}))

vi.mock('@/hooks/useAlerts', () => ({
  useMPAnomalyState: useMPAnomalyStateMock,
  useToggleMPAnomalyRuleMutation: () => useToggleMock(),
}))

const { useCurrentUserMock } = vi.hoisted(() => ({ useCurrentUserMock: vi.fn() }))
vi.mock('@/lib/use-current-user', () => ({
  useCurrentUser: useCurrentUserMock,
}))

import { AnomalyStateCard } from './AnomalyStateCard'

function render_(mpId = 'mp-1') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <AnomalyStateCard meteringPointId={mpId} />
    </QueryClientProvider>,
  )
}

describe('AnomalyStateCard', () => {
  beforeEach(() => {
    useCurrentUserMock.mockReturnValue({ id: 'u', role: 'admin' })
    useToggleMock.mockReturnValue({ mutate: vi.fn(), isPending: false })
  })

  it('renders warming_up state when not eligible', () => {
    useMPAnomalyStateMock.mockReturnValue({
      data: { eligible: false, days_until_eligible: 16, rules: [] },
      isLoading: false,
    })
    const { container } = render_()
    expect(container.querySelector('[data-state="warming_up"]')).not.toBeNull()
    expect(screen.getByText(/Building baseline/i)).toBeInTheDocument()
  })

  it('renders eligible_inactive state when all rules disabled', () => {
    useMPAnomalyStateMock.mockReturnValue({
      data: {
        eligible: true,
        days_until_eligible: 0,
        rules: [
          { rule_kind: 'anomaly_p95', enabled: false },
          { rule_kind: 'anomaly_iqr', enabled: false },
          { rule_kind: 'anomaly_quiet_hour', enabled: false },
        ],
      },
      isLoading: false,
    })
    const { container } = render_()
    expect(container.querySelector('[data-state="eligible_inactive"]')).not.toBeNull()
  })

  it('renders active state when ≥1 rule enabled', () => {
    useMPAnomalyStateMock.mockReturnValue({
      data: {
        eligible: true,
        days_until_eligible: 0,
        rules: [
          { rule_kind: 'anomaly_p95', enabled: true, severity: 'warning' },
          { rule_kind: 'anomaly_iqr', enabled: false },
          { rule_kind: 'anomaly_quiet_hour', enabled: true, severity: 'critical' },
        ],
      },
      isLoading: false,
    })
    const { container } = render_()
    expect(container.querySelector('[data-state="active"]')).not.toBeNull()
  })

  it('viewer role disables toggles', () => {
    useCurrentUserMock.mockReturnValue({ id: 'u', role: 'viewer' })
    useMPAnomalyStateMock.mockReturnValue({
      data: {
        eligible: true,
        days_until_eligible: 0,
        rules: [{ rule_kind: 'anomaly_p95', enabled: true }],
      },
      isLoading: false,
    })
    render_()
    const sw = screen.getByRole('switch', { name: /anomaly_p95.*read-only/i })
    expect(sw).toBeDisabled()
  })
})

