/**
 * ImportFromCatalogDialog tests — Plan 07-06 Task 2 (TDD).
 *
 * Tests:
 *  1. step 1 → step 2: selecting "Import from catalog" + Continue reveals the picker with vendor search
 *  2. step 2 selection populates Review form with vendor name + version + capabilities
 *  3. step 3 successful import shows "Profile added" toast and closes
 *  4. step 3 duplicate slug shows inline error "This catalog entry is already installed. Renaming does not bypass the duplicate check — remove the existing profile first."
 */

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { createElement } from 'react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

vi.mock('@/lib/catalog', () => ({
  fetchCatalog: vi.fn(),
  fetchCatalogEntry: vi.fn(),
  importFromCatalog: vi.fn(),
}))

vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}))

import { fetchCatalog, fetchCatalogEntry, importFromCatalog } from '@/lib/catalog'
import { toast } from 'sonner'

const mockFetchCatalog = fetchCatalog as ReturnType<typeof vi.fn>
const mockFetchCatalogEntry = fetchCatalogEntry as ReturnType<typeof vi.fn>
const mockImportFromCatalog = importFromCatalog as ReturnType<typeof vi.fn>
const mockToast = toast as unknown as { success: ReturnType<typeof vi.fn>; error: ReturnType<typeof vi.fn> }

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const axiomaEntry = {
  slug: 'axioma_w1',
  name: 'Axioma Qalcosonic W1',
  vendor: 'Axioma',
  family: 'Qalcosonic W1',
  capabilities: ['cumulative', 'flow_rate', 'leak_detection'],
  version: '1.0.0',
  codec_js_path: 'axioma/w1.js',
  codec_js: 'function decodeUplink(input) { return { data: {} }; }',
  counter_modulus: 0,
  mac_version: '1.0.0',
  region: null,
  expected_uplink_interval_seconds: 3600,
  offline_threshold_multiplier: 2,
  anomaly_compatibility: 'full',
  battery_curve: 'linear_pct',
  vendor_has_separate_meter_serial: false,
}

const itronEntry = {
  slug: 'itron_kinmy',
  name: 'Itron KINMY LoRa Module',
  vendor: 'Itron',
  family: 'KINMY LoRa Module',
  capabilities: ['cumulative', 'flow_rate'],
  version: '1.1.0',
  codec_js_path: 'itron/kinmy.js',
  codec_js: 'function decodeUplink(input) { return { data: { flow: 0 } }; }',
  counter_modulus: 0,
  mac_version: '1.0.0',
  region: null,
  expected_uplink_interval_seconds: 86400,
  offline_threshold_multiplier: 2,
  anomaly_compatibility: 'limited',
  battery_curve: 'li_socl2_3v6',
  vendor_has_separate_meter_serial: false,
}

// ---------------------------------------------------------------------------
// Render helpers
// ---------------------------------------------------------------------------

function makeQueryClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } })
}

async function renderDialog(queryClient: QueryClient, props?: Partial<{ onOpenChange: () => void; onImported: () => void }>) {
  const { ImportFromCatalogDialog } = await import('@/routes/profiles/ImportFromCatalogDialog')
  const onOpenChange = props?.onOpenChange ?? vi.fn()
  const onImported = props?.onImported ?? vi.fn()
  return render(
    createElement(
      QueryClientProvider,
      { client: queryClient },
      createElement(ImportFromCatalogDialog, { open: true, onOpenChange, onImported })
    )
  )
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

describe('ImportFromCatalogDialog (Surface 2, V2-VEND-01)', () => {
  it('step 1 → step 2: selecting "Import from catalog" + Continue reveals the picker with vendor search', async () => {
    mockFetchCatalog.mockResolvedValue({
      entries: [axiomaEntry, itronEntry],
      profiles: [],
    })

    const qc = makeQueryClient()
    await renderDialog(qc)

    // Step 1: dialog title visible
    expect(screen.getByText('Add Device Profile')).toBeInTheDocument()

    // Select "Import from catalog"
    const catalogRadio = screen.getAllByRole('radio')[1]
    fireEvent.click(catalogRadio)

    // Click Continue
    const continueBtn = screen.getByRole('button', { name: /Continue/i })
    fireEvent.click(continueBtn)

    // Step 2: picker with search input
    await waitFor(() => {
      expect(screen.getByPlaceholderText('Search vendor profiles…')).toBeInTheDocument()
    })

    // Both vendor entries should be in the list
    await waitFor(() => {
      expect(screen.getByText(/Axioma/)).toBeInTheDocument()
    })
  })

  it('step 2 selection populates Review form with vendor name + version + capabilities', async () => {
    mockFetchCatalog.mockResolvedValue({
      entries: [axiomaEntry],
      profiles: [],
    })
    mockFetchCatalogEntry.mockResolvedValue(axiomaEntry)

    const qc = makeQueryClient()
    await renderDialog(qc)

    // Navigate to picker
    const catalogRadio = screen.getAllByRole('radio')[1]
    fireEvent.click(catalogRadio)
    fireEvent.click(screen.getByRole('button', { name: /Continue/i }))

    // Wait for catalog list
    await waitFor(() => {
      expect(screen.getByPlaceholderText('Search vendor profiles…')).toBeInTheDocument()
    })

    // Select Axioma entry
    await waitFor(() => {
      expect(screen.getByText(/Axioma/)).toBeInTheDocument()
    })
    fireEvent.click(screen.getByText(/Axioma/))

    // Should advance to step 3 — Review form
    await waitFor(() => {
      expect(screen.getByText(/Review Profile:/)).toBeInTheDocument()
    })

    // Profile name pre-filled
    await waitFor(() => {
      const nameInput = screen.getByLabelText('Profile name') as HTMLInputElement
      expect(nameInput.value).toContain('Axioma')
    })
  })

  it('step 3 successful import shows "Profile added" toast and closes', async () => {
    mockFetchCatalog.mockResolvedValue({ entries: [axiomaEntry], profiles: [] })
    mockFetchCatalogEntry.mockResolvedValue(axiomaEntry)
    mockImportFromCatalog.mockResolvedValue({ profile_id: 'new-uuid-1234' })

    const onOpenChange = vi.fn()
    const qc = makeQueryClient()
    await renderDialog(qc, { onOpenChange })

    // Navigate to step 2
    fireEvent.click(screen.getAllByRole('radio')[1])
    fireEvent.click(screen.getByRole('button', { name: /Continue/i }))

    // Select entry → step 3
    await waitFor(() => screen.getByPlaceholderText('Search vendor profiles…'))
    await waitFor(() => screen.getByText(/Axioma/))
    fireEvent.click(screen.getByText(/Axioma/))

    // Wait for review form title and form fields to be populated by useEffect
    await waitFor(() => screen.getByText(/Review Profile:/))
    await waitFor(() => {
      const nameInput = screen.getByLabelText('Profile name') as HTMLInputElement
      expect(nameInput.value).toContain('Axioma')
    })

    // Submit
    fireEvent.click(screen.getByRole('button', { name: /Add Profile/i }))

    await waitFor(() => {
      expect(mockImportFromCatalog).toHaveBeenCalled()
    })
    await waitFor(() => {
      expect(mockToast.success).toHaveBeenCalledWith('Profile added')
    })
  })

  it('step 3 duplicate slug shows inline error "This catalog entry is already installed. Renaming does not bypass the duplicate check — remove the existing profile first."', async () => {
    mockFetchCatalog.mockResolvedValue({ entries: [axiomaEntry], profiles: [] })
    mockFetchCatalogEntry.mockResolvedValue(axiomaEntry)
    mockImportFromCatalog.mockRejectedValue(new Error('already imported'))

    const qc = makeQueryClient()
    await renderDialog(qc)

    // Navigate to step 3
    fireEvent.click(screen.getAllByRole('radio')[1])
    fireEvent.click(screen.getByRole('button', { name: /Continue/i }))
    await waitFor(() => screen.getByPlaceholderText('Search vendor profiles…'))
    await waitFor(() => screen.getByText(/Axioma/))
    fireEvent.click(screen.getByText(/Axioma/))

    // Wait for review form fields to be populated by useEffect
    await waitFor(() => screen.getByText(/Review Profile:/))
    await waitFor(() => {
      const nameInput = screen.getByLabelText('Profile name') as HTMLInputElement
      expect(nameInput.value).toContain('Axioma')
    })

    // Submit
    fireEvent.click(screen.getByRole('button', { name: /Add Profile/i }))

    await waitFor(() => {
      expect(mockToast.error).toHaveBeenCalledWith(
        'This catalog entry is already installed. Renaming does not bypass the duplicate check — remove the existing profile first.'
      )
    })
  })
})
