/**
 * VendorCatalogCard tests — Plan 07-05 Task 2 (TDD GREEN).
 *
 * Tests:
 *  1. renders DataTable with Vendor | Family | Capability | Version | Installed | Devices using | Status columns
 *  2. hides Family + Version columns at viewport ≤ 768px
 *  3. shows Update to v{X.Y.Z} button when status = update-available
 */

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { createElement } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

vi.mock('@/lib/catalog', () => ({
  fetchCatalog: vi.fn(),
}))

vi.mock('@/routes/settings/CatalogUpdateModal', () => ({
  CatalogUpdateModal: () => null,
}))

import { fetchCatalog } from '@/lib/catalog'
const mockFetchCatalog = fetchCatalog as ReturnType<typeof vi.fn>

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const installedEntry = {
  slug: 'axioma_w1',
  name: 'Axioma Qalcosonic W1',
  vendor: 'Axioma',
  family: 'Qalcosonic W1',
  capabilities: ['cumulative', 'flow_rate', 'leak_detection'],
  version: '1.0.0',
  codec_js_path: 'axioma/w1.js',
  counter_modulus: 0,
  mac_version: '1.0.0',
  region: null,
  expected_uplink_interval_seconds: 3600,
  offline_threshold_multiplier: 2,
  anomaly_compatibility: 'full',
  battery_curve: 'linear_pct',
  vendor_has_separate_meter_serial: false,
}

const updateAvailableEntry = {
  slug: 'itron_kinmy',
  name: 'Itron KINMY LoRa Module',
  vendor: 'Itron',
  family: 'KINMY LoRa Module',
  capabilities: ['cumulative', 'flow_rate'],
  version: '1.1.0',
  codec_js_path: 'itron/kinmy.js',
  counter_modulus: 0,
  mac_version: '1.0.0',
  region: null,
  expected_uplink_interval_seconds: 3600,
  offline_threshold_multiplier: 2,
  anomaly_compatibility: 'limited',
  battery_curve: 'li_socl2_3v6',
  vendor_has_separate_meter_serial: false,
}

const installedProfile = {
  profile_id: 'aaaa-bbbb-cccc-dddd',
  slug: 'axioma_w1',
  installed_version: '1.0.0',
  status: 'installed' as const,
  devices_using_count: 3,
  customer_edited: false,
  codec_js_synced_at: '2026-05-13T00:00:00Z',
}

const updateAvailableProfile = {
  profile_id: 'eeee-ffff-gggg-hhhh',
  slug: 'itron_kinmy',
  installed_version: '1.0.0',
  status: 'update-available' as const,
  devices_using_count: 5,
  customer_edited: false,
  codec_js_synced_at: '2026-05-13T00:00:00Z',
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
  const { VendorCatalogCard } = await import('@/routes/settings/VendorCatalogCard')
  return render(createElement(VendorCatalogCard), { wrapper: makeWrapper(queryClient) })
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

describe('VendorCatalogCard (Surface 1, D-23..D-25, D-40)', () => {
  it('renders DataTable with Vendor | Family | Capability | Version | Installed | Devices using | Status columns', async () => {
    mockFetchCatalog.mockResolvedValue({
      entries: [installedEntry],
      profiles: [installedProfile],
    })

    const qc = makeQueryClient()
    await renderCard(qc)

    // Wait for data to load
    await waitFor(() => {
      expect(screen.getByText('Axioma')).toBeInTheDocument()
    })

    // All 7 column headers must be present
    // Column headers are <th> elements; use getAllByText for any that may be ambiguous
    expect(screen.getAllByText('Vendor').length).toBeGreaterThanOrEqual(1)
    // Family header is present (may be hidden via CSS class but still in DOM)
    expect(screen.getByRole('columnheader', { name: 'Family' })).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: 'Capability' })).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: 'Version' })).toBeInTheDocument()
    // "Installed" header — scope to th to avoid collision with status badge
    expect(screen.getByRole('columnheader', { name: 'Installed' })).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: 'Devices using' })).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: 'Status' })).toBeInTheDocument()

    // DataTable accessibility attributes
    expect(screen.getByRole('grid', { name: 'Vendor profiles' })).toBeInTheDocument()
  })

  it('hides Family + Version columns at viewport ≤ 768px', async () => {
    // Set narrow viewport
    Object.defineProperty(window, 'innerWidth', { writable: true, configurable: true, value: 375 })

    mockFetchCatalog.mockResolvedValue({
      entries: [installedEntry],
      profiles: [installedProfile],
    })

    const qc = makeQueryClient()
    await renderCard(qc)

    await waitFor(() => {
      expect(screen.getByText('Axioma')).toBeInTheDocument()
    })

    // The hidden class is applied via Tailwind's `hidden sm:table-cell`.
    // In the test environment (jsdom with no real CSS), we verify the DOM elements
    // have the correct hidden class applied by the DataTable component.
    const familyHeader = screen.getByText('Family').closest('th')
    const versionHeader = screen.getByText('Version').closest('th')

    expect(familyHeader).toHaveClass('hidden')
    expect(versionHeader).toHaveClass('hidden')

    // Reset viewport
    Object.defineProperty(window, 'innerWidth', { writable: true, configurable: true, value: 1024 })
  })

  it('shows Update to v{X.Y.Z} button when status = update-available', async () => {
    mockFetchCatalog.mockResolvedValue({
      entries: [updateAvailableEntry],
      profiles: [updateAvailableProfile],
    })

    const qc = makeQueryClient()
    await renderCard(qc)

    // Wait for row to render
    await waitFor(() => {
      expect(screen.getByText('Itron')).toBeInTheDocument()
    })

    // Update button: aria-label is the accessible name (used by getByRole)
    const updateButton = screen.getByRole('button', {
      name: 'Update Itron KINMY LoRa Module to version 1.1.0',
    })
    expect(updateButton).toBeInTheDocument()

    // Button text content (visible label)
    expect(updateButton).toHaveTextContent('Update to v1.1.0')

    // Correct aria-label on Update button
    expect(updateButton).toHaveAttribute(
      'aria-label',
      'Update Itron KINMY LoRa Module to version 1.1.0'
    )

    // Status badge text
    expect(screen.getByText(/Update available v1\.0\.0 → v1\.1\.0/)).toBeInTheDocument()
  })
})
