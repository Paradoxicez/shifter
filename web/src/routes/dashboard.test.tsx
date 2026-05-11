/**
 * DashboardPage route tests — Plan 04-07 Task 2 (TDD)
 *
 * Tests:
 *  - water capability → only water KPI tiles
 *  - electricity capability → only electricity tiles
 *  - both capability → both rows
 *  - onboarding (0,0,0) → EmptyStateOnboarding; KpiGrid NOT rendered
 */

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { createElement } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// ---------------------------------------------------------------------------
// Mock hooks — must be declared before importing the component under test
// ---------------------------------------------------------------------------

vi.mock('@/hooks/useDashboardScope', () => ({
  useDashboardScope: vi.fn(),
}))

vi.mock('@/hooks/useSSE', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/hooks/useSSE')>()
  return {
    ...actual,
    useSSE: vi.fn(() => ({ status: 'open', attempt: 0, lastEventAt: Date.now() })),
  }
})

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return { ...actual, apiFetch: vi.fn() }
})

import { useDashboardScope } from '@/hooks/useDashboardScope'
import { useSSE } from '@/hooks/useSSE'
import { apiFetch } from '@/lib/api'

const mockUseDashboardScope = useDashboardScope as ReturnType<typeof vi.fn>
const mockUseSSE = useSSE as ReturnType<typeof vi.fn>
const mockApiFetch = apiFetch as ReturnType<typeof vi.fn>

// ---------------------------------------------------------------------------
// Data fixtures
// ---------------------------------------------------------------------------

const waterKpi = {
  today_consumption: 47.2,
  today_unit: 'm³',
  instant_total: 12.4,
  instant_unit: 'L/min',
  period_delta_abs: 3.4,
  period_delta_pct: 8.0,
  online_count: 10,
  total_count: 12,
}

const electricityKpi = {
  today_consumption: 120.5,
  today_unit: 'kWh',
  instant_total: 5400,
  instant_unit: 'W',
  period_delta_abs: null,
  period_delta_pct: null,
  online_count: 3,
  total_count: 3,
}

const waterSnapshot = {
  capabilities: 'water',
  generated_at: '2026-05-11T12:00:00Z',
  kpis: { water: waterKpi },
  latest_readings: [
    { metering_point_id: 'mp-001', utility_class: 'water', instant_value: 12.4 },
  ],
}

const electricitySnapshot = {
  capabilities: 'electricity',
  generated_at: '2026-05-11T12:00:00Z',
  kpis: { electricity: electricityKpi },
  latest_readings: [
    { metering_point_id: 'mp-002', utility_class: 'electricity', instant_value: 5400 },
  ],
}

const bothSnapshot = {
  capabilities: 'both',
  generated_at: '2026-05-11T12:00:00Z',
  kpis: { water: waterKpi, electricity: electricityKpi },
  latest_readings: [
    { metering_point_id: 'mp-001', utility_class: 'water', instant_value: 12.4 },
    { metering_point_id: 'mp-002', utility_class: 'electricity', instant_value: 5400 },
  ],
}

// ---------------------------------------------------------------------------
// Render helpers
// ---------------------------------------------------------------------------

function makeQueryClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } })
}

function makeWrapper(queryClient: QueryClient) {
  return ({ children }: { children: React.ReactNode }) =>
    createElement(
      MemoryRouter,
      null,
      createElement(QueryClientProvider, { client: queryClient }, children)
    )
}

async function renderDashboard(queryClient: QueryClient) {
  const { default: DashboardPage } = await import('./dashboard')
  return render(createElement(DashboardPage), { wrapper: makeWrapper(queryClient) })
}

// ---------------------------------------------------------------------------
// Setup / teardown
// ---------------------------------------------------------------------------

beforeEach(() => {
  vi.resetAllMocks()
  mockUseSSE.mockReturnValue({ status: 'open', attempt: 0, lastEventAt: Date.now() })
})

afterEach(() => {
  vi.resetAllMocks()
})

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('DashboardPage — capability gating', () => {
  it('renders water tiles only for water capability', async () => {
    const qc = makeQueryClient()
    // Pre-seed the snapshot into the query cache so the component renders immediately
    qc.setQueryData(['dashboard', 'snapshot'], waterSnapshot)
    mockUseDashboardScope.mockReturnValue({
      data: {
        capabilities: 'water',
        onboarding: { gateway_count: 1, device_count: 1, uplink_count: 10 },
      },
      isLoading: false,
    })
    mockApiFetch.mockResolvedValue(waterSnapshot)

    await renderDashboard(qc)

    await waitFor(() => {
      expect(screen.getByText('Dashboard')).toBeInTheDocument()
    })
    // Water tiles should render
    expect(screen.getAllByText("Today's consumption")).toHaveLength(1)
    expect(screen.getByText('Current flow')).toBeInTheDocument()
    expect(screen.getByText('Period delta')).toBeInTheDocument()
    expect(screen.getByText('Online devices')).toBeInTheDocument()
    // Electricity tile should NOT render
    expect(screen.queryByText('Current load')).not.toBeInTheDocument()
  })

  it('renders electricity tiles only for electricity capability', async () => {
    const qc = makeQueryClient()
    qc.setQueryData(['dashboard', 'snapshot'], electricitySnapshot)
    mockUseDashboardScope.mockReturnValue({
      data: {
        capabilities: 'electricity',
        onboarding: { gateway_count: 1, device_count: 1, uplink_count: 10 },
      },
      isLoading: false,
    })
    mockApiFetch.mockResolvedValue(electricitySnapshot)

    await renderDashboard(qc)

    await waitFor(() => {
      expect(screen.getByText('Dashboard')).toBeInTheDocument()
    })
    // Electricity tile should render
    expect(screen.getByText('Current load')).toBeInTheDocument()
    // Water flow should NOT render
    expect(screen.queryByText('Current flow')).not.toBeInTheDocument()
  })

  it('renders both rows for both capability', async () => {
    const qc = makeQueryClient()
    qc.setQueryData(['dashboard', 'snapshot'], bothSnapshot)
    mockUseDashboardScope.mockReturnValue({
      data: {
        capabilities: 'both',
        onboarding: { gateway_count: 1, device_count: 2, uplink_count: 20 },
      },
      isLoading: false,
    })
    mockApiFetch.mockResolvedValue(bothSnapshot)

    await renderDashboard(qc)

    await waitFor(() => {
      expect(screen.getByText('Dashboard')).toBeInTheDocument()
    })
    // Both utility tiles should render
    expect(screen.getByText('Current flow')).toBeInTheDocument()
    expect(screen.getByText('Current load')).toBeInTheDocument()
    // Both "Today's consumption" tiles
    expect(screen.getAllByText("Today's consumption")).toHaveLength(2)
  })
})

describe('DashboardPage — empty state', () => {
  it('shows EmptyStateOnboarding for (0,0,0) tuple; KpiGrid not rendered', async () => {
    const qc = makeQueryClient()
    mockUseDashboardScope.mockReturnValue({
      data: {
        capabilities: 'both',
        onboarding: { gateway_count: 0, device_count: 0, uplink_count: 0 },
      },
      isLoading: false,
    })

    await renderDashboard(qc)

    await waitFor(() => {
      expect(screen.getByText(/Add your first gateway/i)).toBeInTheDocument()
    })
    // KPI tiles should not render
    expect(screen.queryByText('Current flow')).not.toBeInTheDocument()
    // Dashboard heading should not render (empty state replaces it)
    expect(screen.queryByText('Dashboard')).not.toBeInTheDocument()
  })
})
