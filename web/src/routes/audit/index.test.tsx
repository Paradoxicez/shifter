/**
 * Plan 06-07 — Task 3 unit tests for /audit route components.
 *
 * Tests follow the established Phase 6 Vitest + React Testing Library pattern.
 * These are the RED phase tests — they will fail until the components are created.
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

function makeQC() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } })
}

const adminUser = { id: 'u1', email: 'admin@example.com', role: 'admin' as const, must_change_password: false }
const viewerUser = { id: 'u2', email: 'viewer@example.com', role: 'viewer' as const, must_change_password: false }

// Mock useRouteLoaderData so useCurrentUser returns our test user.
vi.mock('react-router-dom', async (importOriginal) => {
  const real = await importOriginal<typeof import('react-router-dom')>()
  return {
    ...real,
    useRouteLoaderData: vi.fn().mockReturnValue({ user: adminUser }),
    Navigate: vi.fn(({ to }: { to: string }) => <div data-testid="navigate" data-to={to} />),
  }
})

// Mock audit API calls.
vi.mock('@/hooks/useAudit', () => ({
  useAuditList: vi.fn().mockReturnValue({
    data: { pages: [{ rows: [], next_cursor: null, total: 0 }] },
    isLoading: false,
    isError: false,
    fetchNextPage: vi.fn(),
    hasNextPage: false,
  }),
  useAuditCount: vi.fn().mockReturnValue({ data: { count: 0 }, isLoading: false }),
  useAuditDistincts: vi.fn().mockReturnValue({ data: { actions: [], entity_types: [] }, isLoading: false }),
  useExportAuditAsync: vi.fn().mockReturnValue({ mutate: vi.fn(), isPending: false }),
}))

// ─────────────────────────────────────────────────────────────────────────────
// auditParams (lib/auditParams.ts)
// ─────────────────────────────────────────────────────────────────────────────

describe('auditParams', () => {
  it('AuditParams_DefaultLast7Days: empty URL params → from = now()-7d, to = now()', async () => {
    const { auditParams } = await import('@/lib/auditParams')
    const result = auditParams.parse({})
    const now = new Date()
    const from = new Date(result.from)
    const to = new Date(result.to)
    const diffMs = now.getTime() - from.getTime()
    const sevenDaysMs = 7 * 24 * 60 * 60 * 1000
    // from should be approximately 7 days ago (within 5 seconds tolerance)
    expect(diffMs).toBeGreaterThan(sevenDaysMs - 5000)
    expect(diffMs).toBeLessThan(sevenDaysMs + 5000)
    // to should be approximately now
    expect(to.getTime()).toBeGreaterThan(now.getTime() - 5000)
  })

  it('AuditParams_DeepLink: URL params are preserved', async () => {
    const { auditParams } = await import('@/lib/auditParams')
    const result = auditParams.parse({
      from: '2026-04-01T00:00:00Z',
      to: '2026-05-01T00:00:00Z',
      entity_type: ['user'],
    })
    expect(result.from).toBe('2026-04-01T00:00:00Z')
    expect(result.to).toBe('2026-05-01T00:00:00Z')
    expect(result.entity_type).toEqual(['user'])
  })

  it('AuditParams_InvalidCatchFallback: invalid datetime falls back to default', async () => {
    const { auditParams } = await import('@/lib/auditParams')
    const result = auditParams.parse({ from: 'not-a-date', to: 'also-bad' })
    // .catch() should produce valid ISO dates
    expect(() => new Date(result.from)).not.toThrow()
    expect(() => new Date(result.to)).not.toThrow()
  })
})

// ─────────────────────────────────────────────────────────────────────────────
// JsonTree highlightKeys prop (Plan 06-07 extension)
// ─────────────────────────────────────────────────────────────────────────────

describe('JsonTree_HighlightKeysProp', () => {
  it('applies bg-warning/20 class to matching keys', async () => {
    const { JsonTree } = await import('@/components/metering-point/JsonTree')
    const { container } = render(
      <JsonTree
        value={{ name: 'old', status: 'active' }}
        highlightKeys={['name']}
        defaultOpen
      />,
    )
    // The "name" key row should have the highlight class.
    const highlighted = container.querySelector('[data-highlight="true"]')
    expect(highlighted).toBeTruthy()
  })

  it('does not apply highlight class to non-matching keys', async () => {
    const { JsonTree } = await import('@/components/metering-point/JsonTree')
    const { container } = render(
      <JsonTree
        value={{ name: 'old', status: 'active' }}
        highlightKeys={['name']}
        defaultOpen
      />,
    )
    // "status" key should NOT be highlighted
    const allHighlighted = container.querySelectorAll('[data-highlight="true"]')
    expect(allHighlighted.length).toBe(1)
  })
})

// ─────────────────────────────────────────────────────────────────────────────
// AuditRowExpand
// ─────────────────────────────────────────────────────────────────────────────

describe('AuditRowExpand', () => {
  it('AuditRowExpand_HighlightChangedKeys: changed key highlighted in both panels', async () => {
    const { AuditRowExpand } = await import('./AuditRowExpand')
    const { container } = render(
      <AuditRowExpand
        before='{"name":"old","status":"active"}'
        after='{"name":"new","status":"active"}'
      />,
    )
    // "name" is changed; "status" is not. Both panels should highlight "name".
    const highlighted = container.querySelectorAll('[data-highlight="true"]')
    expect(highlighted.length).toBeGreaterThanOrEqual(2)
  })

  it('AuditRowExpand_HandlesCreatedRemoved: null before shows "(created)"', async () => {
    const { AuditRowExpand } = await import('./AuditRowExpand')
    render(
      <AuditRowExpand before={null} after='{"name":"new"}' />,
    )
    expect(screen.getByText('(created)')).toBeTruthy()
  })

  it('AuditRowExpand_HandlesCreatedRemoved: null after shows "(removed)"', async () => {
    const { AuditRowExpand } = await import('./AuditRowExpand')
    render(
      <AuditRowExpand before='{"name":"old"}' after={null} />,
    )
    expect(screen.getByText('(removed)')).toBeTruthy()
  })
})

// ─────────────────────────────────────────────────────────────────────────────
// AuditPage admin guard (D-31)
// ─────────────────────────────────────────────────────────────────────────────

describe('AuditPage', () => {
  it('ViewerRedirects: viewer user → Navigate to="/"', async () => {
    const { useRouteLoaderData } = await import('react-router-dom')
    vi.mocked(useRouteLoaderData).mockReturnValue({ user: viewerUser })

    const AuditPage = (await import('./index')).default
    render(
      <QueryClientProvider client={makeQC()}>
        <MemoryRouter initialEntries={['/audit']}>
          <Routes>
            <Route path="/audit" element={<AuditPage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    )
    // Viewer should see the Navigate component (redirected to /)
    const nav = screen.getByTestId('navigate')
    expect(nav.getAttribute('data-to')).toBe('/')

    // Reset mock
    vi.mocked(useRouteLoaderData).mockReturnValue({ user: adminUser })
  })

  it('admin sees Audit log heading', async () => {
    const { useRouteLoaderData } = await import('react-router-dom')
    vi.mocked(useRouteLoaderData).mockReturnValue({ user: adminUser })

    const AuditPage = (await import('./index')).default
    render(
      <QueryClientProvider client={makeQC()}>
        <MemoryRouter initialEntries={['/audit']}>
          <Routes>
            <Route path="/audit" element={<AuditPage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    )
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /Audit log/i })).toBeTruthy()
    })
  })
})

// ─────────────────────────────────────────────────────────────────────────────
// AuditExportButton
// ─────────────────────────────────────────────────────────────────────────────

describe('AuditExportButton', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('ExportButton_InlineLEQ50k: count <= 50000 → inline download href', async () => {
    const { useAuditCount } = await import('@/hooks/useAudit')
    vi.mocked(useAuditCount).mockReturnValue({
      data: { count: 1234 },
      isLoading: false,
    } as ReturnType<typeof useAuditCount>)

    const { AuditExportButton } = await import('./AuditExportButton')
    render(
      <AuditExportButton params={{ from: '2026-01-01T00:00:00Z', to: '2026-01-08T00:00:00Z', entity_type: [], action: [] }} />,
    )
    const btn = screen.getByRole('button', { name: /Export CSV/i })
    expect(btn).toBeTruthy()
    // Inline export: button should link to /api/audit/export (not trigger async)
    expect(btn.getAttribute('data-inline')).toBe('true')
  })

  it('ExportButton_AsyncGT50k: count > 50000 → calls async mutation', async () => {
    const { useAuditCount, useExportAuditAsync } = await import('@/hooks/useAudit')
    const mockMutate = vi.fn()
    vi.mocked(useAuditCount).mockReturnValue({
      data: { count: 60000 },
      isLoading: false,
    } as ReturnType<typeof useAuditCount>)
    vi.mocked(useExportAuditAsync).mockReturnValue({
      mutate: mockMutate,
      isPending: false,
    } as ReturnType<typeof useExportAuditAsync>)

    const { AuditExportButton } = await import('./AuditExportButton')
    render(
      <AuditExportButton params={{ from: '2026-01-01T00:00:00Z', to: '2026-01-08T00:00:00Z', entity_type: [], action: [] }} />,
    )
    const btn = screen.getByRole('button', { name: /Export CSV/i })
    fireEvent.click(btn)
    expect(mockMutate).toHaveBeenCalledOnce()
  })
})
