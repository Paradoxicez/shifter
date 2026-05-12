/**
 * SaveTemplateDialog tests — Plan 07-11b Task 1 (TDD RED)
 *
 * Tests:
 *  1. Empty name shows "Name is required."
 *  2. 409 response (duplicate name) shows "A template with this name already exists."
 *  3. Successful save shows "Template saved: {name}" toast
 *  4. Cancel button renders with "Discard template"
 */

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi, beforeEach } from 'vitest'

// ── Hoist mocks before any imports ──────────────────────────────────────────

const { createTemplateMock } = vi.hoisted(() => ({
  createTemplateMock: vi.fn(),
}))

vi.mock('@/lib/reportTemplates', () => ({
  createTemplate: createTemplateMock,
  listTemplates: vi.fn().mockResolvedValue([]),
  getTemplate: vi.fn(),
  updateTemplate: vi.fn(),
  deleteTemplate: vi.fn(),
}))

vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}))

// ── Component under test ────────────────────────────────────────────────────

import { SaveTemplateDialog } from './SaveTemplateDialog'
import { toast } from 'sonner'
import { ApiError } from '@/lib/api'

// ── Helpers ─────────────────────────────────────────────────────────────────

function renderDialog(props: {
  open?: boolean
  onOpenChange?: (open: boolean) => void
  currentState?: object
  onSaved?: () => void
}) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <SaveTemplateDialog
          open={props.open ?? true}
          onOpenChange={props.onOpenChange ?? vi.fn()}
          currentState={props.currentState ?? {}}
          onSaved={props.onSaved ?? vi.fn()}
        />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

// ── Tests ────────────────────────────────────────────────────────────────────

describe('SaveTemplateDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows "Name is required." when name is empty on submit', async () => {
    renderDialog({})

    // Dialog should be open
    await waitFor(() => {
      expect(screen.getByText('Save as Template')).toBeInTheDocument()
    })

    // Click Save Template without entering a name
    fireEvent.click(screen.getByRole('button', { name: 'Save Template' }))

    await waitFor(() => {
      expect(screen.getByText('Name is required.')).toBeInTheDocument()
    })
  })

  it('shows "A template with this name already exists." on 409', async () => {
    createTemplateMock.mockRejectedValue(new ApiError(409, 'duplicate'))
    renderDialog({})

    await waitFor(() => {
      expect(screen.getByText('Save as Template')).toBeInTheDocument()
    })

    // Enter a name and submit
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Existing Template' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save Template' }))

    await waitFor(() => {
      expect(screen.getByText('A template with this name already exists.')).toBeInTheDocument()
    })
  })

  it('shows "Template saved: {name}" toast on success', async () => {
    createTemplateMock.mockResolvedValue({
      id: 't1',
      name: 'My Template',
      description: '',
      state: {},
      created_at: '',
      updated_at: '',
    })
    renderDialog({})

    await waitFor(() => {
      expect(screen.getByText('Save as Template')).toBeInTheDocument()
    })

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'My Template' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save Template' }))

    await waitFor(() => {
      expect(toast.success).toHaveBeenCalledWith('Template saved: My Template')
    })
  })

  it('shows "Discard template" cancel button', async () => {
    renderDialog({})

    await waitFor(() => {
      expect(screen.getByText('Save as Template')).toBeInTheDocument()
    })

    expect(screen.getByRole('button', { name: 'Discard template' })).toBeInTheDocument()
  })
})
