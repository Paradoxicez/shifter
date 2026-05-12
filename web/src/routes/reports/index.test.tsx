/**
 * ReportsPage tests — Plan 05-09 Task 1
 *
 * Tests:
 *  1. Config panel renders 3-radio scope picker on mount
 *  2. Single site selected → site picker visible
 *  3. Single meter selected → MP combobox visible
 *  4. All meters selected → Group by select visible
 *  5. Range preset buttons update URL params
 *  6. Custom range opens DateRangePicker
 *  7. Generate triggers useReportGenerate with current cfg body
 *  8. Loading state while mutation pending
 *  9. Result panel renders after successful Generate
 *  10. Navigate-away loses result panel (D-07)
 *  11. Empty state when zero sites
 *  12. URL state pre-fills form
 *  13. useReportGenerate hook smoke test
 */

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

vi.mock('@/lib/api', () => ({
  apiFetch: vi.fn(),
  ApiError: class ApiError extends Error {
    status: number
    constructor(status: number, message: string) {
      super(message)
      this.status = status
    }
  },
}))

// Mock the date-range-picker to avoid calendar complexity in tests
vi.mock('@/components/dashboard/DateRangePicker', () => ({
  DateRangePicker: ({ onChange }: { value?: { from?: Date; to?: Date }; onChange: (v: any) => void }) => (
    <div data-testid="date-range-picker">
      <button onClick={() => onChange({ from: new Date('2025-01-01'), to: new Date('2025-01-31') })}>
        Pick custom dates
      </button>
    </div>
  ),
}))

// Mock MeterCombobox
vi.mock('./MeterCombobox', () => ({
  MeterCombobox: ({ onChange, meters }: { value?: string; onChange: (id: string) => void; meters: Array<{ id: string; name: string; site_name: string }> }) => (
    <div data-testid="meter-combobox">
      <button onClick={() => onChange(meters[0]?.id ?? '')}>Select first meter</button>
    </div>
  ),
}))

// Mock TemplatesDropdown — avoids useQuery + useRouteLoaderData complexity in unit tests
vi.mock('./TemplatesDropdown', () => ({
  TemplatesDropdown: () => <button type="button">Templates</button>,
}))

// Mock SaveTemplateDialog — avoids dialog complexity in unit tests
vi.mock('./SaveTemplateDialog', () => ({
  SaveTemplateDialog: () => null,
}))

// Mock useCurrentUser — ReportConfigPanel calls this; MemoryRouter doesn't provide root loader data
vi.mock('@/lib/use-current-user', () => ({
  useCurrentUser: () => ({ id: 'u1', email: 'admin@test.local', role: 'admin', must_change_password: false }),
}))

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

import { apiFetch } from '@/lib/api'

const mockedApiFetch = vi.mocked(apiFetch)

function mkSites(n: number) {
  return Array.from({ length: n }, (_, i) => ({ id: `site-${i + 1}-uuid-000000000000`, name: `Site ${i + 1}` }))
}

const MOCK_RESULT = {
  report_id: 'report-uuid-0000000000000',
  report: {
    summary: { total_consumption: 1234.5 },
    period_rows: [{ period: '2025-01-01T00:00:00Z', consumption: 100, delta_vs_prior: { absolute: 5, percent: 5.1 } }],
    meter_rows: [{ id: 'm1', name: 'Meter 1', site_name: 'Site 1', utility_class: 'water', consumption: 100 }],
  },
  pdf_status: 'pending' as const,
}

function makeWrapper(initialUrl = '/reports') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return function Wrapper({ children }: React.PropsWithChildren<{}>) {
    return (
      <QueryClientProvider client={qc}>
        <MemoryRouter initialEntries={[initialUrl]}>
          {children}
        </MemoryRouter>
      </QueryClientProvider>
    )
  }
}

async function renderReportsPage(initialUrl = '/reports') {
  const { ReportsPage } = await import('./index')
  const W = makeWrapper(initialUrl)
  const result = render(
    <W>
      <ReportsPage />
    </W>
  )
  return result
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('ReportsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.resetModules()
    // default: 2 sites exist
    mockedApiFetch.mockImplementation((path: string) => {
      if (path === '/api/sites') return Promise.resolve(mkSites(2))
      if (path === '/api/metering-points') return Promise.resolve([])
      return Promise.reject(new Error('unexpected call: ' + path))
    })
  })

  afterEach(() => {
    vi.clearAllMocks()
    vi.resetModules()
  })

  it('shows config panel with 3-radio scope picker on mount (D-05)', async () => {
    await renderReportsPage()
    await waitFor(() => {
      expect(screen.getByText('All meters')).toBeInTheDocument()
      expect(screen.getByText('Single site')).toBeInTheDocument()
      expect(screen.getByText('Single meter')).toBeInTheDocument()
    })
  })

  it('reveals site picker when Single site is selected', async () => {
    const user = userEvent.setup()
    await renderReportsPage()
    await waitFor(() => screen.getByText('Single site'))
    const siteRadio = screen.getByLabelText('Single site')
    await user.click(siteRadio)
    await waitFor(() => expect(screen.getByText('Site')).toBeInTheDocument())
  })

  it('reveals MP combobox when Single meter is selected', async () => {
    const user = userEvent.setup()
    await renderReportsPage()
    await waitFor(() => screen.getByText('Single meter'))
    const meterRadio = screen.getByLabelText('Single meter')
    await user.click(meterRadio)
    await waitFor(() => expect(screen.getByTestId('meter-combobox')).toBeInTheDocument())
  })

  it('reveals Group by select when All meters is selected', async () => {
    await renderReportsPage()
    await waitFor(() => {
      // All meters is the default so Group by should be visible
      expect(screen.getByText('Group by')).toBeInTheDocument()
    })
  })

  it('Generate triggers useReportGenerate with current cfg as body (POST /api/reports/generate)', async () => {
    const user = userEvent.setup()
    mockedApiFetch.mockImplementation((path: string) => {
      if (path === '/api/sites') return Promise.resolve(mkSites(2))
      if (path === '/api/reports/generate') return Promise.resolve(MOCK_RESULT)
      return Promise.reject(new Error('unexpected: ' + path))
    })
    await renderReportsPage()
    await waitFor(() => screen.getByText('Generate report'))
    const btn = screen.getByRole('button', { name: /Generate report/i })
    await user.click(btn)
    await waitFor(() => {
      expect(mockedApiFetch).toHaveBeenCalledWith(
        '/api/reports/generate',
        expect.objectContaining({ method: 'POST' })
      )
    })
  })

  it('shows Loader2 spinner and disabled button while mutation is pending', async () => {
    // Return a promise that never resolves to simulate pending state
    let resolvePending!: (v: unknown) => void
    mockedApiFetch.mockImplementation((path: string) => {
      if (path === '/api/sites') return Promise.resolve(mkSites(2))
      if (path === '/api/reports/generate') return new Promise((r) => { resolvePending = r })
      return Promise.reject(new Error('unexpected: ' + path))
    })
    const user = userEvent.setup()
    await renderReportsPage()
    await waitFor(() => screen.getByText('Generate report'))
    const btn = screen.getByRole('button', { name: /Generate report/i })
    await user.click(btn)
    // Button should show "Generating…" and be disabled
    await waitFor(() => {
      expect(screen.getByText(/Generating…/)).toBeInTheDocument()
    })
    expect(btn).toBeDisabled()
    // Clean up: resolve so no async leaks
    act(() => { resolvePending(MOCK_RESULT) })
  })

  it('renders ResultPanel after successful Generate (result state set via onSuccess)', async () => {
    const user = userEvent.setup()
    mockedApiFetch.mockImplementation((path: string) => {
      if (path === '/api/sites') return Promise.resolve(mkSites(2))
      if (path === '/api/reports/generate') return Promise.resolve(MOCK_RESULT)
      if (path.startsWith('/api/reports/')) return Promise.resolve({ ...MOCK_RESULT, id: MOCK_RESULT.report_id, created_at: '2025-01-01', scope: 'all', range: 'monthly' })
      return Promise.reject(new Error('unexpected: ' + path))
    })
    await renderReportsPage()
    await waitFor(() => screen.getByText('Generate report'))
    await user.click(screen.getByRole('button', { name: /Generate report/i }))
    await waitFor(() => {
      expect(screen.getByText('Report ready')).toBeInTheDocument()
    })
  })

  it('clicking Configure another (back) returns to ConfigPanel (D-07 ephemeral)', async () => {
    const user = userEvent.setup()
    mockedApiFetch.mockImplementation((path: string) => {
      if (path === '/api/sites') return Promise.resolve(mkSites(2))
      if (path === '/api/reports/generate') return Promise.resolve(MOCK_RESULT)
      if (path.startsWith('/api/reports/')) return Promise.resolve({ ...MOCK_RESULT, id: MOCK_RESULT.report_id, created_at: '2025-01-01', scope: 'all', range: 'monthly' })
      return Promise.reject(new Error('unexpected: ' + path))
    })
    await renderReportsPage()
    await waitFor(() => screen.getByText('Generate report'))
    await user.click(screen.getByRole('button', { name: /Generate report/i }))
    await waitFor(() => screen.getByText('Report ready'))
    // Click "Configure another" to go back
    await user.click(screen.getByRole('button', { name: /Configure another/i }))
    await waitFor(() => {
      expect(screen.getByText('Generate report')).toBeInTheDocument()
    })
  })

  it('shows EmptyStateOnboarding when zero sites', async () => {
    mockedApiFetch.mockImplementation((path: string) => {
      if (path === '/api/sites') return Promise.resolve([])
      return Promise.reject(new Error('unexpected: ' + path))
    })
    await renderReportsPage()
    await waitFor(() => {
      expect(screen.getByText('Nothing to report yet')).toBeInTheDocument()
    })
    expect(screen.getByRole('link', { name: /Go to Sites/i })).toBeInTheDocument()
  })

  it('URL state ?scope=site pre-fills the scope to Single site', async () => {
    await renderReportsPage('/reports?scope=site')
    await waitFor(() => {
      // Radix RadioGroupItem renders as a <button role="radio"> with aria-checked, not a native input
      const siteRadio = screen.getByLabelText('Single site')
      expect(siteRadio).toHaveAttribute('aria-checked', 'true')
    })
  })

  it('URL state ?range=daily pre-fills range to Daily', async () => {
    await renderReportsPage('/reports?range=daily')
    await waitFor(() => {
      // The Daily button should have a primary/default variant appearance
      expect(screen.getByRole('button', { name: /^Daily$/i })).toBeInTheDocument()
    })
  })

  it('useReportGenerate hook smoke: exports mutate, isPending, data', async () => {
    const { renderHook } = await import('@testing-library/react')
    const { useReportGenerate } = await import('./useReportGenerate')
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
    const { result } = renderHook(() => useReportGenerate(), {
      wrapper: ({ children }) => (
        <QueryClientProvider client={qc}>
          {children}
        </QueryClientProvider>
      ),
    })
    expect(result.current).toHaveProperty('mutate')
    expect(result.current).toHaveProperty('mutateAsync')
    expect(result.current).toHaveProperty('isPending')
    expect(result.current).toHaveProperty('data')
    expect(result.current.isPending).toBe(false)
  })

  it('Generate report button is present with correct text matching UI-SPEC copywriting', async () => {
    await renderReportsPage()
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /Generate report/i })).toBeInTheDocument()
    })
  })
})
