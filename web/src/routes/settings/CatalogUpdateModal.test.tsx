/**
 * CatalogUpdateModal tests — Plan 07-06 Task 3 (TDD).
 *
 * Tests:
 *  1. renders one diff row per changed field
 *  2. default toggle for customer-edited field is "Use mine" and shows Edited badge
 *  3. Apply Update submits accepted_fields excluding "Use mine" rows + shows success toast on response
 */

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { createElement } from 'react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

vi.mock('@/lib/catalog', () => ({
  fetchCatalogEntry: vi.fn(),
  applyCatalogUpdate: vi.fn(),
}))

vi.mock('@/lib/profiles', () => ({
  getProfileWithMappings: vi.fn(),
}))

vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}))

import { fetchCatalogEntry, applyCatalogUpdate } from '@/lib/catalog'
import { getProfileWithMappings } from '@/lib/profiles'
import { toast } from 'sonner'

const mockFetchCatalogEntry = fetchCatalogEntry as ReturnType<typeof vi.fn>
const mockApplyCatalogUpdate = applyCatalogUpdate as ReturnType<typeof vi.fn>
const mockGetProfileWithMappings = getProfileWithMappings as ReturnType<typeof vi.fn>
const mockToast = toast as unknown as { success: ReturnType<typeof vi.fn>; error: ReturnType<typeof vi.fn> }

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const catalogEntry = {
  slug: 'axioma_w1',
  name: 'Axioma Qalcosonic W1',
  vendor: 'Axioma',
  family: 'Qalcosonic W1',
  capabilities: ['cumulative', 'flow_rate'],
  version: '1.1.0',
  codec_js_path: 'axioma/w1.js',
  codec_js: '// v1.1.0 codec\nfunction decodeUplink(input) { return { data: {} }; }',
  counter_modulus: 0,
  mac_version: '1.0.0',
  region: 'EU868',
  expected_uplink_interval_seconds: 3600,
  offline_threshold_multiplier: 2,
  anomaly_compatibility: 'full',
  battery_curve: 'linear_pct',
  vendor_has_separate_meter_serial: false,
}

const installedProfile = {
  profile: {
    id: 'profile-uuid-1234',
    slug: 'axioma_w1',
    name: 'Axioma Qalcosonic W1',
    vendor: 'Axioma',
    family: 'Qalcosonic W1',
    capabilities: ['cumulative', 'flow_rate'],
    counter_modulus: 0,
    mac_version: '1.0.0',
    region: null, // differs from catalog: 'EU868'
    codec_js: '// v1.0.0 codec\nfunction decodeUplink(input) { return {}; }', // differs
    codec_js_synced_at: '2026-05-13T00:00:00Z',
  },
  mappings: [],
}

const baseRow = {
  slug: 'axioma_w1',
  vendor: 'Axioma',
  family: 'Qalcosonic W1',
  capabilities: ['cumulative', 'flow_rate'],
  catalogVersion: '1.1.0',
  installedVersion: '1.0.0',
  status: 'update-available' as const,
  devicesUsingCount: 3,
  profileId: 'profile-uuid-1234',
  customerEdited: false,
  codecJsSyncedAt: '2026-05-13T00:00:00Z',
}

// ---------------------------------------------------------------------------
// Render helpers
// ---------------------------------------------------------------------------

function makeQueryClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } })
}

async function renderModal(
  queryClient: QueryClient,
  rowOverride?: Partial<typeof baseRow>,
) {
  const { CatalogUpdateModal } = await import('@/routes/settings/CatalogUpdateModal')
  const row = { ...baseRow, ...rowOverride }
  const onClose = vi.fn()
  return {
    onClose,
    ...render(
      createElement(
        QueryClientProvider,
        { client: queryClient },
        createElement(CatalogUpdateModal, { row, onClose })
      )
    ),
  }
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

describe('CatalogUpdateModal (Surface 3, V2-VEND-01)', () => {
  it('renders one diff row per changed field', async () => {
    mockFetchCatalogEntry.mockResolvedValue(catalogEntry)
    mockGetProfileWithMappings.mockResolvedValue(installedProfile)

    const qc = makeQueryClient()
    await renderModal(qc)

    // Dialog title
    await waitFor(() => {
      expect(screen.getByText('Update Axioma Qalcosonic W1')).toBeInTheDocument()
    })

    // Subtitle showing version range
    await waitFor(() => {
      expect(screen.getByText(/v1\.0\.0.*v1\.1\.0/)).toBeInTheDocument()
    })

    // There should be diff rows for codec_js and region (2 changed fields)
    await waitFor(() => {
      expect(screen.getByText('Codec')).toBeInTheDocument()
      expect(screen.getByText('Region')).toBeInTheDocument()
    })

    // Each row has "Use mine" and "Use catalog" toggles
    const useMineButtons = screen.getAllByRole('radio', { name: /Use my value for/i })
    expect(useMineButtons.length).toBeGreaterThan(0)

    const useCatalogButtons = screen.getAllByRole('radio', { name: /Use catalog value for/i })
    expect(useCatalogButtons.length).toBeGreaterThan(0)
  })

  it('default toggle for customer-edited field is "Use mine" and shows Edited badge', async () => {
    mockFetchCatalogEntry.mockResolvedValue(catalogEntry)
    mockGetProfileWithMappings.mockResolvedValue(installedProfile)

    const qc = makeQueryClient()
    // customerEdited=true → all fields default to "Use mine" + show Edited badge
    await renderModal(qc, { customerEdited: true })

    await waitFor(() => {
      expect(screen.getByText('Update Axioma Qalcosonic W1')).toBeInTheDocument()
    })

    // All toggles should default to "Use mine" when customerEdited=true
    await waitFor(() => {
      const useMineItems = screen.getAllByRole('radio', { name: /Use my value for/i })
      // All Use mine toggles should be checked (aria-checked="true")
      expect(useMineItems.length).toBeGreaterThan(0)
    })

    // "Edited" badges should appear (one per changed field)
    await waitFor(() => {
      const editedBadges = screen.getAllByText('Edited')
      expect(editedBadges.length).toBeGreaterThan(0)
    })

    // Verify aria-label on the Edited badge
    await waitFor(() => {
      const editedBadge = screen.getAllByText('Edited')[0]
      expect(editedBadge.closest('[aria-label]')?.getAttribute('aria-label')).toBe(
        'You have edited this field'
      )
    })
  })

  it('Apply Update submits accepted_fields excluding "Use mine" rows + shows success toast on response', async () => {
    mockFetchCatalogEntry.mockResolvedValue(catalogEntry)
    mockGetProfileWithMappings.mockResolvedValue(installedProfile)
    mockApplyCatalogUpdate.mockResolvedValue({
      updated_at: '2026-05-13T07:00:00Z',
      new_version: '1.1.0',
    })

    const qc = makeQueryClient()
    const { onClose } = await renderModal(qc)

    // Wait for diff to load
    await waitFor(() => {
      expect(screen.getByText('Codec')).toBeInTheDocument()
    })

    // Toggle "Codec" field to "Use mine" — it should be excluded from accepted_fields
    const useMineForCodec = screen.getByRole('radio', { name: /Use my value for Codec/i })
    fireEvent.click(useMineForCodec)

    // Click Apply Update
    const applyBtn = screen.getByRole('button', { name: /Apply Update/i })
    fireEvent.click(applyBtn)

    await waitFor(() => {
      expect(mockApplyCatalogUpdate).toHaveBeenCalledWith(
        'profile-uuid-1234',
        expect.objectContaining({
          target_version: '1.1.0',
          // codec_js excluded because "Use mine" was selected
          accepted_fields: expect.not.arrayContaining(['codec_js']),
        })
      )
    })

    await waitFor(() => {
      expect(mockToast.success).toHaveBeenCalledWith('Profile updated to v1.1.0')
    })

    await waitFor(() => {
      expect(onClose).toHaveBeenCalled()
    })
  })
})
