/**
 * Plan 06-04 AlertWorkerBanner tests.
 */

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/use-current-user', () => ({
  useCurrentUser: () => ({ role: 'admin' }),
}))

const fetchMock = vi.hoisted(() => vi.fn())
vi.mock('@/lib/api', () => ({
  apiFetch: fetchMock,
}))

import { AlertWorkerBanner } from './AlertWorkerBanner'

function renderBanner() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <AlertWorkerBanner />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('AlertWorkerBanner', () => {
  it('hides when no workers are degraded', async () => {
    fetchMock.mockResolvedValueOnce({
      alert_workers: [{ worker_kind: 'threshold_instantaneous', degraded: false }],
    })
    const { container } = renderBanner()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })
    expect(container.querySelector('[role="alert"]')).toBeNull()
  })

  it('renders banner when at least one worker is degraded', async () => {
    fetchMock.mockResolvedValueOnce({
      alert_workers: [{ worker_kind: 'threshold_hourly', degraded: true }],
    })
    const { findAllByRole } = renderBanner()
    const banners = await findAllByRole('alert')
    expect(banners.length).toBeGreaterThan(0)
    expect(banners[0].textContent).toMatch(/degraded/i)
  })
})
