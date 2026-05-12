/**
 * Plan 07-13 Task 3 (TDD RED → GREEN): BulkImportDialog gateway bulk-import.
 *
 * Tests:
 *   - step 1 → 2 flow with valid CSV (validate returns 3 valid rows)
 *   - error count badge renders for invalid rows
 *   - Import button label includes row count: "Import 3 Gateways"
 *   - success toast "Import complete: 3 gateways added."
 *   - error toast "Import failed. Check your connection and try again." on commit network failure
 */

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { BulkImportDialog } from './BulkImportDialog'

vi.mock('@/lib/gatewayImport', async (orig) => {
  const real = await orig<typeof import('@/lib/gatewayImport')>()
  return {
    ...real,
    validateGatewayCSV: vi.fn(),
    commitGatewayCSV: vi.fn(),
  }
})

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn() },
}))

function renderDialog(props?: { open?: boolean; onOpenChange?: (v: boolean) => void }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const onOpenChange = vi.fn()
  return {
    qc,
    onOpenChange,
    ...render(
      <QueryClientProvider client={qc}>
        <BulkImportDialog open={props?.open ?? true} onOpenChange={props?.onOpenChange ?? onOpenChange} />
      </QueryClientProvider>,
    ),
  }
}

describe('BulkImportDialog (Plan 07-13)', () => {
  beforeEach(() => {
    Object.defineProperty(window, 'innerWidth', {
      writable: true,
      configurable: true,
      value: 1280,
    })
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  it('step 1 → 2 flow with valid CSV — validate returns 3 valid rows', async () => {
    const { validateGatewayCSV } = await import('@/lib/gatewayImport')
    ;(validateGatewayCSV as ReturnType<typeof vi.fn>).mockResolvedValue({
      valid_rows: 3,
      error_rows: 0,
      errors: [],
    })

    renderDialog()

    // Step 1: verify heading and template link present
    expect(screen.getByRole('heading', { name: /Import Gateways/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Download CSV template/i })).toBeInTheDocument()

    // Pick a file
    const fileInput = screen.getByLabelText(/Choose a CSV file/i) as HTMLInputElement
    const csv = new File(['gateway_eui,name\naabbccddeeff0001,GW1'], 'gateways.csv', {
      type: 'text/csv',
    })
    await userEvent.upload(fileInput, csv)

    // Click Validate
    await userEvent.click(screen.getByRole('button', { name: /^Validate$/i }))

    // Step 2: Validation Results heading
    await waitFor(() =>
      expect(screen.getByRole('heading', { name: /Validation Results/i })).toBeInTheDocument(),
    )
    // Success summary text
    expect(screen.getByText(/3 gateways ready to import\./i)).toBeInTheDocument()
  })

  it('error count badge renders for invalid rows', async () => {
    const { validateGatewayCSV } = await import('@/lib/gatewayImport')
    ;(validateGatewayCSV as ReturnType<typeof vi.fn>).mockResolvedValue({
      valid_rows: 2,
      error_rows: 1,
      errors: [
        {
          row_number: 2,
          gateway_eui: 'not-valid-eui',
          outcome: 'error',
          error_message: 'invalid EUI format',
        },
      ],
    })

    renderDialog()

    const fileInput = screen.getByLabelText(/Choose a CSV file/i) as HTMLInputElement
    await userEvent.upload(
      fileInput,
      new File(['gateway_eui,name\nbad,GW1'], 'gateways.csv', { type: 'text/csv' }),
    )
    await userEvent.click(screen.getByRole('button', { name: /^Validate$/i }))

    await waitFor(() =>
      expect(screen.getByRole('heading', { name: /Validation Results/i })).toBeInTheDocument(),
    )
    // Error badge shows count
    expect(screen.getByText(/1 errors?/i)).toBeInTheDocument()
  })

  it('Import button label includes row count: "Import 3 Gateways"', async () => {
    const { validateGatewayCSV } = await import('@/lib/gatewayImport')
    ;(validateGatewayCSV as ReturnType<typeof vi.fn>).mockResolvedValue({
      valid_rows: 3,
      error_rows: 0,
      errors: [],
    })

    renderDialog()

    const fileInput = screen.getByLabelText(/Choose a CSV file/i) as HTMLInputElement
    await userEvent.upload(
      fileInput,
      new File(['csv'], 'gateways.csv', { type: 'text/csv' }),
    )
    await userEvent.click(screen.getByRole('button', { name: /^Validate$/i }))

    await waitFor(() =>
      expect(screen.getByRole('heading', { name: /Validation Results/i })).toBeInTheDocument(),
    )

    // Import button shows row count
    expect(screen.getByRole('button', { name: /Import 3 Gateways/i })).toBeInTheDocument()
  })

  it('success toast "Import complete: 3 gateways added."', async () => {
    const { validateGatewayCSV, commitGatewayCSV } = await import('@/lib/gatewayImport')
    const { toast } = await import('sonner')
    ;(validateGatewayCSV as ReturnType<typeof vi.fn>).mockResolvedValue({
      valid_rows: 3,
      error_rows: 0,
      errors: [],
    })
    ;(commitGatewayCSV as ReturnType<typeof vi.fn>).mockResolvedValue({
      created: 3,
      updated: 0,
      skipped: 0,
      outcomes: [],
    })

    renderDialog()

    const fileInput = screen.getByLabelText(/Choose a CSV file/i) as HTMLInputElement
    await userEvent.upload(
      fileInput,
      new File(['csv'], 'gateways.csv', { type: 'text/csv' }),
    )
    await userEvent.click(screen.getByRole('button', { name: /^Validate$/i }))

    await waitFor(() =>
      expect(screen.getByRole('heading', { name: /Validation Results/i })).toBeInTheDocument(),
    )

    await userEvent.click(screen.getByRole('button', { name: /Import 3 Gateways/i }))

    await waitFor(() =>
      expect(toast.success).toHaveBeenCalledWith('Import complete: 3 gateways added.'),
    )

    // After success, shows result heading
    await waitFor(() =>
      expect(screen.getByText(/3 gateways imported successfully\./i)).toBeInTheDocument(),
    )
  })

  it('error toast on commit network failure', async () => {
    const { validateGatewayCSV, commitGatewayCSV } = await import('@/lib/gatewayImport')
    const { toast } = await import('sonner')
    ;(validateGatewayCSV as ReturnType<typeof vi.fn>).mockResolvedValue({
      valid_rows: 2,
      error_rows: 0,
      errors: [],
    })
    ;(commitGatewayCSV as ReturnType<typeof vi.fn>).mockRejectedValue(new Error('network failure'))

    renderDialog()

    const fileInput = screen.getByLabelText(/Choose a CSV file/i) as HTMLInputElement
    await userEvent.upload(
      fileInput,
      new File(['csv'], 'gateways.csv', { type: 'text/csv' }),
    )
    await userEvent.click(screen.getByRole('button', { name: /^Validate$/i }))

    await waitFor(() =>
      expect(screen.getByRole('heading', { name: /Validation Results/i })).toBeInTheDocument(),
    )

    await userEvent.click(screen.getByRole('button', { name: /Import 2 Gateways/i }))

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(
        'Import failed. Check your connection and try again.',
      ),
    )
  })
})
