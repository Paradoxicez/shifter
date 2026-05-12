/**
 * Settings page + DataRetentionCard + EditRetentionDialog tests — Plan 05-11 Task 2 (TDD)
 * Extended for Phase 6 additions in Plan 06-10 (alerts_days, audit_log_days, BackupStatusCard).
 *
 * Tests:
 *  1. DataRetentionCard renders 7 rows: raw, hourly, daily, monthly, alerts, audit_log, yearly
 *  2. Yearly row shows "Never expires" when yearly_days is null
 *  3. Admin sees 6 Edit buttons (one per editable row; yearly is read-only)
 *  4. Viewer sees no Edit buttons (AUTH-06 frontend hide)
 *  5. Clicking Edit on a row opens EditRetentionDialog pre-filled with current value
 *  6. Out-of-range value shows inline zod error on submit
 *  7. Save fires PATCH /api/settings/retention; on 200 shows toast + closes dialog
 *  8. BackupStatusCard renders freshness dot (Plan 06-10)
 *  9. BackupStatusCard shows never_run state correctly
 * 10. Threshold form shows save button (admin only)
 * 11. Viewer does not see threshold form
 * 12. BackupFreshnessDot color logic (unit tests — no DOM)
 */

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createElement } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// ---------------------------------------------------------------------------
// Mocks — declared before component imports
// ---------------------------------------------------------------------------

vi.mock('@/lib/use-current-user', () => ({
  useCurrentUser: vi.fn(),
}))

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return { ...actual, apiFetch: vi.fn() }
})

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  Toaster: () => null,
}))

import { useCurrentUser } from '@/lib/use-current-user'
import { apiFetch } from '@/lib/api'
import { toast } from 'sonner'

const mockUseCurrentUser = useCurrentUser as ReturnType<typeof vi.fn>
const mockApiFetch = apiFetch as ReturnType<typeof vi.fn>
const mockToast = toast as unknown as { success: ReturnType<typeof vi.fn>; error: ReturnType<typeof vi.fn> }

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const retentionData = {
  raw_days: 90,
  hourly_days: 365,
  daily_days: 1825,
  monthly_days: 7300,
  yearly_days: null,
  // Phase 6 additions (Plan 06-10):
  alerts_days: 90,
  audit_log_days: 365,
  updated_at: '2026-05-01T00:00:00Z',
}

const retentionDataWithYearly = {
  ...retentionData,
  yearly_days: 3650,
}

const backupStatusNeverRun = {
  never_run: true,
  last: undefined,
  recent: [],
  warn_threshold_hours: 24,
  crit_threshold_hours: 168,
  destination_dir: '/var/lib/shifter/backups',
}

const backupStatusOk = {
  never_run: false,
  last: {
    id: 'abc-123',
    file_name: 'backup-2026-05-01.tar.gz',
    sha256: 'deadbeefdeadbeef',
    started_at: '2026-05-01T02:00:00Z',
    finished_at: '2026-05-01T02:05:00Z',
    status: 'completed',
    trigger_kind: 'cron',
    size_bytes: 1048576,
    age_seconds: 3600,
  },
  recent: [
    {
      id: 'abc-123',
      file_name: 'backup-2026-05-01.tar.gz',
      sha256: 'deadbeefdeadbeef',
      started_at: '2026-05-01T02:00:00Z',
      finished_at: '2026-05-01T02:05:00Z',
      status: 'completed',
      trigger_kind: 'cron',
      size_bytes: 1048576,
      age_seconds: 3600,
    },
  ],
  warn_threshold_hours: 24,
  crit_threshold_hours: 168,
  destination_dir: '/var/lib/shifter/backups',
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

async function renderCard(queryClient: QueryClient) {
  const { DataRetentionCard } = await import('@/components/settings/DataRetentionCard')
  return render(createElement(DataRetentionCard), { wrapper: makeWrapper(queryClient) })
}

// ---------------------------------------------------------------------------
// Setup / teardown
// ---------------------------------------------------------------------------

beforeEach(() => {
  vi.resetAllMocks()
})

afterEach(() => {
  vi.resetAllMocks()
})

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('DataRetentionCard — render', () => {
  it('Test 1: renders 7 retention rows (raw, hourly, daily, monthly, alerts, audit_log, yearly)', async () => {
    mockUseCurrentUser.mockReturnValue({ role: 'admin', email: 'a@test.com' })
    mockApiFetch.mockResolvedValue(retentionData)

    const qc = makeQueryClient()
    await renderCard(qc)

    await waitFor(() => {
      expect(screen.getByText('Raw measurements')).toBeInTheDocument()
    })

    expect(screen.getByText('Hourly aggregate')).toBeInTheDocument()
    expect(screen.getByText('Daily aggregate')).toBeInTheDocument()
    expect(screen.getByText('Monthly aggregate')).toBeInTheDocument()
    expect(screen.getByText('Alerts')).toBeInTheDocument()
    expect(screen.getByText('Audit log')).toBeInTheDocument()
    expect(screen.getByText('Yearly aggregate')).toBeInTheDocument()
  })

  it('Test 2: yearly row shows "Never expires" when yearly_days is null', async () => {
    mockUseCurrentUser.mockReturnValue({ role: 'admin', email: 'a@test.com' })
    mockApiFetch.mockResolvedValue(retentionData) // yearly_days: null

    const qc = makeQueryClient()
    await renderCard(qc)

    await waitFor(() => {
      expect(screen.getByText('Never expires')).toBeInTheDocument()
    })
  })

  it('Test 2b: yearly row shows formatted value when yearly_days is set', async () => {
    mockUseCurrentUser.mockReturnValue({ role: 'admin', email: 'a@test.com' })
    mockApiFetch.mockResolvedValue(retentionDataWithYearly) // yearly_days: 3650

    const qc = makeQueryClient()
    await renderCard(qc)

    await waitFor(() => {
      // 3650 days / 365 = 10 years
      expect(screen.getByTestId('retention-row-yearly_days')).toHaveTextContent('10 years')
    })
  })

  it('Test 3: admin sees 6 Edit buttons (one per editable row; yearly is read-only)', async () => {
    mockUseCurrentUser.mockReturnValue({ role: 'admin', email: 'admin@test.com' })
    mockApiFetch.mockResolvedValue(retentionData)

    const qc = makeQueryClient()
    await renderCard(qc)

    await waitFor(() => {
      expect(screen.getByText('Raw measurements')).toBeInTheDocument()
    })

    const editButtons = screen.getAllByRole('button', { name: /^Edit /i })
    expect(editButtons).toHaveLength(6) // raw, hourly, daily, monthly, alerts, audit_log — NOT yearly
  })

  it('Test 4: viewer sees no Edit buttons (AUTH-06 frontend hide)', async () => {
    mockUseCurrentUser.mockReturnValue({ role: 'viewer', email: 'viewer@test.com' })
    mockApiFetch.mockResolvedValue(retentionData)

    const qc = makeQueryClient()
    await renderCard(qc)

    await waitFor(() => {
      expect(screen.getByText('Raw measurements')).toBeInTheDocument()
    })

    expect(screen.queryAllByRole('button', { name: /^Edit /i })).toHaveLength(0)
  })
})

describe('EditRetentionDialog — interaction', () => {
  it('Test 5: clicking Edit on raw row opens EditRetentionDialog pre-filled with 90', async () => {
    mockUseCurrentUser.mockReturnValue({ role: 'admin', email: 'admin@test.com' })
    mockApiFetch.mockResolvedValue(retentionData)

    const qc = makeQueryClient()
    await renderCard(qc)

    await waitFor(() => {
      expect(screen.getByText('Raw measurements')).toBeInTheDocument()
    })

    // Click Edit on Raw measurements row
    const editBtn = screen.getByRole('button', { name: 'Edit Raw measurements' })
    await userEvent.click(editBtn)

    // Dialog should open with title
    await waitFor(() => {
      expect(screen.getByText('Edit Raw measurements')).toBeInTheDocument()
    })

    // Input should be pre-filled with current value (90)
    const input = screen.getByRole('spinbutton')
    expect(input).toHaveValue(90)
  })

  it('Test 6: out-of-range value shows inline zod error on submit', async () => {
    // Render EditRetentionDialog directly (avoids Radix portal interaction in the card test).
    const { EditRetentionDialog } = await import('@/components/settings/EditRetentionDialog')
    const { RETENTION_LEVELS } = await import('@/components/settings/DataRetentionCard')

    const rawLevel = RETENTION_LEVELS.find((l) => l.key === 'raw_days')!
    const onOpenChange = vi.fn()
    const qc = makeQueryClient()

    render(
      createElement(
        QueryClientProvider,
        { client: qc },
        createElement(EditRetentionDialog, {
          level: rawLevel,
          initialValue: 90,
          open: true,
          onOpenChange,
        })
      )
    )

    // Dialog title must be visible
    await waitFor(() => {
      expect(screen.getByText('Edit Raw measurements')).toBeInTheDocument()
    })

    // Set an out-of-range value via fireEvent (bypasses pointer-events: none from Radix overlay)
    const input = screen.getByRole('spinbutton')
    fireEvent.change(input, { target: { value: '10' } })

    // Submit the form via fireEvent.submit to bypass pointer-events check
    const form = document.getElementById('edit-retention-form')!
    fireEvent.submit(form)

    // Zod error must appear
    await waitFor(() => {
      expect(screen.getByText(/Must be at least 30/i)).toBeInTheDocument()
    })

    // PATCH must NOT have been called
    expect(mockApiFetch).not.toHaveBeenCalledWith(
      '/api/settings/retention',
      expect.objectContaining({ method: 'PATCH' })
    )
  })

  it('Test 7: Save fires PATCH /api/settings/retention; on 200 shows toast + closes dialog', async () => {
    mockUseCurrentUser.mockReturnValue({ role: 'admin', email: 'admin@test.com' })
    // First call: GET retention data; second call (after save): re-fetch after invalidation
    mockApiFetch
      .mockResolvedValueOnce(retentionData)
      .mockResolvedValueOnce({ ...retentionData, raw_days: 60 }) // PATCH response
      .mockResolvedValueOnce({ ...retentionData, raw_days: 60 }) // refetch after invalidation

    const qc = makeQueryClient()
    await renderCard(qc)

    await waitFor(() => {
      expect(screen.getByText('Raw measurements')).toBeInTheDocument()
    })

    await userEvent.click(screen.getByRole('button', { name: 'Edit Raw measurements' }))

    await waitFor(() => {
      expect(screen.getByText('Edit Raw measurements')).toBeInTheDocument()
    })

    const input = screen.getByRole('spinbutton')
    await userEvent.clear(input)
    await userEvent.type(input, '60')

    const saveBtn = screen.getByRole('button', { name: 'Save retention settings' })
    await userEvent.click(saveBtn)

    // PATCH should have been called with {raw_days: 60}
    await waitFor(() => {
      expect(mockApiFetch).toHaveBeenCalledWith(
        '/api/settings/retention',
        expect.objectContaining({
          method: 'PATCH',
          body: JSON.stringify({ raw_days: 60 }),
        })
      )
    })

    // Success toast should fire
    await waitFor(() => {
      expect(mockToast.success).toHaveBeenCalledWith('Retention settings saved.')
    })

    // Dialog should close (Edit button visible again in card)
    await waitFor(() => {
      expect(screen.queryByText('Edit Raw measurements')).not.toBeInTheDocument()
    })
  })
})

describe('DataRetentionCard — value display', () => {
  it('displays formatted values correctly for each level', async () => {
    mockUseCurrentUser.mockReturnValue({ role: 'viewer', email: 'viewer@test.com' })
    mockApiFetch.mockResolvedValue({
      raw_days: 90,
      hourly_days: 365,
      daily_days: 1825,
      monthly_days: 7300,
      yearly_days: null,
      alerts_days: 90,
      audit_log_days: 365,
      updated_at: '2026-05-01T00:00:00Z',
    })

    const qc = makeQueryClient()
    await renderCard(qc)

    await waitFor(() => {
      // raw_days=90 (unit:'days') → "90 days" (scoped to raw row)
      const rawRow = screen.getByTestId('retention-row-raw_days')
      expect(rawRow).toHaveTextContent('90 days')
      // hourly_days=365 (unit:'days') → "365 days" (scoped to hourly row)
      const hourlyRow = screen.getByTestId('retention-row-hourly_days')
      expect(hourlyRow).toHaveTextContent('365 days')
      // daily_days=1825 (unit:'days') → "1825 days"
      expect(screen.getByText('1825 days')).toBeInTheDocument()
      // monthly_days=7300 (unit:'years') → 7300/365 ≈ 20 → "20 years"
      expect(screen.getByText('20 years')).toBeInTheDocument()
      // yearly_days=null → "Never expires"
      expect(screen.getByText('Never expires')).toBeInTheDocument()
    })
  })
})

// ---------------------------------------------------------------------------
// BackupFreshnessDot — pure logic tests (no DOM needed)
// ---------------------------------------------------------------------------

describe('BackupFreshnessDot — computeFreshnessStatus', () => {
  it('Test 8a: never_run=true returns crit', async () => {
    const { computeFreshnessStatus } = await import(
      '@/components/settings/BackupFreshnessDot'
    )
    expect(
      computeFreshnessStatus({ neverRun: true, ageSeconds: null, warnThresholdHours: 24, critThresholdHours: 168 })
    ).toBe('crit')
  })

  it('Test 8b: age <= warnHours returns ok', async () => {
    const { computeFreshnessStatus } = await import(
      '@/components/settings/BackupFreshnessDot'
    )
    // 12 hours ago (43200s), warn=24h, crit=168h → ok
    expect(
      computeFreshnessStatus({ neverRun: false, ageSeconds: 43200, warnThresholdHours: 24, critThresholdHours: 168 })
    ).toBe('ok')
  })

  it('Test 8c: warn < age <= crit returns warn', async () => {
    const { computeFreshnessStatus } = await import(
      '@/components/settings/BackupFreshnessDot'
    )
    // 48 hours ago (172800s), warn=24h, crit=168h → warn
    expect(
      computeFreshnessStatus({ neverRun: false, ageSeconds: 172800, warnThresholdHours: 24, critThresholdHours: 168 })
    ).toBe('warn')
  })

  it('Test 8d: age > critHours returns crit', async () => {
    const { computeFreshnessStatus } = await import(
      '@/components/settings/BackupFreshnessDot'
    )
    // 200 hours ago (720000s), warn=24h, crit=168h → crit
    expect(
      computeFreshnessStatus({ neverRun: false, ageSeconds: 720000, warnThresholdHours: 24, critThresholdHours: 168 })
    ).toBe('crit')
  })
})

// ---------------------------------------------------------------------------
// BackupStatusCard — render tests
// ---------------------------------------------------------------------------

describe('BackupStatusCard — render', () => {
  async function renderBackupCard(queryClient: QueryClient) {
    const { BackupStatusCard } = await import('@/components/settings/BackupStatusCard')
    return render(createElement(BackupStatusCard), { wrapper: makeWrapper(queryClient) })
  }

  it('Test 9: shows never_run state — "Never run" age text + red dot', async () => {
    mockUseCurrentUser.mockReturnValue({ role: 'admin', email: 'admin@test.com' })
    mockApiFetch.mockResolvedValue(backupStatusNeverRun)

    const qc = makeQueryClient()
    await renderBackupCard(qc)

    await waitFor(() => {
      expect(screen.getByTestId('backup-age-text')).toHaveTextContent('Never run')
    })
    const dot = screen.getByTestId('backup-freshness-dot')
    expect(dot).toHaveAttribute('data-status', 'crit')
  })

  it('Test 10: shows ok state — age text + green dot', async () => {
    mockUseCurrentUser.mockReturnValue({ role: 'admin', email: 'admin@test.com' })
    mockApiFetch.mockResolvedValue(backupStatusOk)

    const qc = makeQueryClient()
    await renderBackupCard(qc)

    await waitFor(() => {
      // age_seconds=3600 = 1 hour ago
      expect(screen.getByTestId('backup-age-text')).toHaveTextContent('1 h ago')
    })
    const dot = screen.getByTestId('backup-freshness-dot')
    expect(dot).toHaveAttribute('data-status', 'ok')
  })

  it('Test 11: admin sees threshold form', async () => {
    mockUseCurrentUser.mockReturnValue({ role: 'admin', email: 'admin@test.com' })
    mockApiFetch.mockResolvedValue(backupStatusOk)

    const qc = makeQueryClient()
    await renderBackupCard(qc)

    await waitFor(() => {
      expect(screen.getByTestId('backup-threshold-form')).toBeInTheDocument()
    })
  })

  it('Test 12: viewer does not see threshold form', async () => {
    mockUseCurrentUser.mockReturnValue({ role: 'viewer', email: 'viewer@test.com' })
    mockApiFetch.mockResolvedValue(backupStatusOk)

    const qc = makeQueryClient()
    await renderBackupCard(qc)

    await waitFor(() => {
      expect(screen.getByTestId('backup-freshness-dot')).toBeInTheDocument()
    })
    expect(screen.queryByTestId('backup-threshold-form')).not.toBeInTheDocument()
  })
})
