/**
 * Plan 07-10 Task 3 — AddRuleDialog backtest + profile-aware rule kind filter tests.
 *
 * Tests:
 *  1. "Test against last 30 days" button renders; click calls runBacktest; fires count shown
 *  2. Zero-fires result appends "Defaults may be well-tuned for this meter." copy
 *  3. limited profile hides anomaly_quiet_hour from rule kind dropdown
 *  4. unsupported profile shows "Anomaly detection is not available for this device type." banner
 */

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'

// ── Hoist mocks before any imports ──────────────────────────────────────────

const { runBacktestMock } = vi.hoisted(() => ({
  runBacktestMock: vi.fn(),
}))

vi.mock('@/lib/backtest', () => ({
  runBacktest: runBacktestMock,
}))

vi.mock('@/hooks/useAlerts', () => ({
  useCreateRuleMutation: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
  useTestFireMutation: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
}))

vi.mock('@/lib/use-current-user', () => ({
  useCurrentUser: () => ({ id: 'u', role: 'admin' }),
}))

// ── Component under test ────────────────────────────────────────────────────

import { AddRuleDialog } from './AddRuleDialog'

// ── Helpers ─────────────────────────────────────────────────────────────────

function renderDialog(props: {
  open?: boolean
  meteringPointId?: string
  anomalyCompatibility?: 'full' | 'limited' | 'unsupported'
}) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <AddRuleDialog
          open={props.open ?? true}
          onOpenChange={vi.fn()}
          meteringPointId={props.meteringPointId ?? 'mp-123'}
          anomalyCompatibility={props.anomalyCompatibility ?? 'full'}
        />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

// ── Tests ────────────────────────────────────────────────────────────────────

describe('AddRuleDialog — backtest button', () => {
  it('renders "Test against last 30 days" button', () => {
    renderDialog({})
    expect(screen.getByRole('button', { name: /Test against last 30 days/i })).toBeInTheDocument()
  })

  it('calls runBacktest and renders fires count on click', async () => {
    runBacktestMock.mockResolvedValueOnce({
      fires_count: 7,
      daily_fires: Array.from({ length: 30 }, (_, i) => ({
        day: `2026-01-${String(i + 1).padStart(2, '0')}`,
        count: i < 7 ? 1 : 0,
      })),
    })

    renderDialog({ meteringPointId: 'mp-abc' })

    const btn = screen.getByRole('button', { name: /Test against last 30 days/i })
    fireEvent.click(btn)

    await waitFor(() => {
      expect(screen.getByText(/7 fires in the last 30 days/i)).toBeInTheDocument()
    })
    expect(runBacktestMock).toHaveBeenCalled()
  })

  it('shows "Defaults may be well-tuned for this meter." when fires_count is 0', async () => {
    runBacktestMock.mockResolvedValueOnce({
      fires_count: 0,
      daily_fires: Array.from({ length: 30 }, (_, i) => ({
        day: `2026-01-${String(i + 1).padStart(2, '0')}`,
        count: 0,
      })),
    })

    renderDialog({ meteringPointId: 'mp-zero' })

    const btn = screen.getByRole('button', { name: /Test against last 30 days/i })
    fireEvent.click(btn)

    await waitFor(() => {
      expect(
        screen.getByText(/Defaults may be well-tuned for this meter/i),
      ).toBeInTheDocument()
    })
  })
})

describe('AddRuleDialog — profile-aware rule kind filter', () => {
  it('hides anomaly_quiet_hour for limited profile', () => {
    renderDialog({ anomalyCompatibility: 'limited' })
    // anomaly_p95 and anomaly_iqr should still be present in the dropdown
    // anomaly_quiet_hour should NOT be in the DOM
    expect(screen.queryByText('anomaly_quiet_hour')).not.toBeInTheDocument()
  })

  it('shows "Anomaly detection is not available" banner for unsupported profile', () => {
    renderDialog({ anomalyCompatibility: 'unsupported' })
    expect(
      screen.getByText(/Anomaly detection is not available for this device type/i),
    ).toBeInTheDocument()
  })
})
