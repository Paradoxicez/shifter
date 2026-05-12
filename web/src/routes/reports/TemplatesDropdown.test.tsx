/**
 * TemplatesDropdown tests — Plan 07-11b Task 1 (TDD RED)
 *
 * Tests:
 *  1. Empty state shows "No templates saved yet. Save the current config to create one."
 *  2. Selecting a template calls onLoadTemplate with the template state
 *  3. Selecting a template shows "Template loaded: {name}" toast
 *  4. Admin sees three-dot Delete action on rows
 *  5. Delete AlertDialog shows template name in body copy
 *  6. Viewer does NOT see three-dot menu on rows
 */

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi, beforeEach } from 'vitest'

// ── Hoist mocks before any imports ──────────────────────────────────────────

const { listTemplatesMock, deleteTemplateMock } = vi.hoisted(() => ({
  listTemplatesMock: vi.fn(),
  deleteTemplateMock: vi.fn(),
}))

vi.mock('@/lib/reportTemplates', () => ({
  listTemplates: listTemplatesMock,
  deleteTemplate: deleteTemplateMock,
}))

vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}))

// ── Component under test ────────────────────────────────────────────────────

import { TemplatesDropdown } from './TemplatesDropdown'
import { toast } from 'sonner'

// ── Helpers ─────────────────────────────────────────────────────────────────

function renderDropdown(props: {
  isAdmin?: boolean
  onLoadTemplate?: (state: object) => void
}) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <TemplatesDropdown
          currentState={{}}
          onLoadTemplate={props.onLoadTemplate ?? vi.fn()}
          isAdmin={props.isAdmin ?? true}
        />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

// ── Tests ────────────────────────────────────────────────────────────────────

describe('TemplatesDropdown', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    deleteTemplateMock.mockResolvedValue(undefined)
  })

  it('shows empty state copy when no templates', async () => {
    listTemplatesMock.mockResolvedValue([])
    renderDropdown({})

    // Open the dropdown
    fireEvent.click(screen.getByRole('button', { name: /Templates/ }))

    await waitFor(() => {
      expect(
        screen.getByText('No templates saved yet. Save the current config to create one.'),
      ).toBeInTheDocument()
    })
  })

  it('calls onLoadTemplate with template state when a template is selected', async () => {
    const onLoad = vi.fn()
    listTemplatesMock.mockResolvedValue([
      { id: 't1', name: 'Monthly per-site', description: '', state: { scope: 'site', range: 'monthly' }, created_at: '', updated_at: '' },
    ])
    renderDropdown({ onLoadTemplate: onLoad })

    fireEvent.click(screen.getByRole('button', { name: /Templates/ }))

    await waitFor(() => {
      expect(screen.getByText('Monthly per-site')).toBeInTheDocument()
    })

    fireEvent.click(screen.getByText('Monthly per-site'))

    expect(onLoad).toHaveBeenCalledWith({ scope: 'site', range: 'monthly' })
  })

  it('shows "Template loaded: {name}" toast on select', async () => {
    listTemplatesMock.mockResolvedValue([
      { id: 't1', name: 'Q1 Report', description: '', state: {}, created_at: '', updated_at: '' },
    ])
    renderDropdown({})

    fireEvent.click(screen.getByRole('button', { name: /Templates/ }))

    await waitFor(() => {
      expect(screen.getByText('Q1 Report')).toBeInTheDocument()
    })

    fireEvent.click(screen.getByText('Q1 Report'))

    expect(toast.success).toHaveBeenCalledWith('Template loaded: Q1 Report')
  })

  it('admin sees three-dot delete action on template rows', async () => {
    listTemplatesMock.mockResolvedValue([
      { id: 't1', name: 'Fleet Monthly', description: '', state: {}, created_at: '', updated_at: '' },
    ])
    renderDropdown({ isAdmin: true })

    fireEvent.click(screen.getByRole('button', { name: /Templates/ }))

    await waitFor(() => {
      expect(screen.getByText('Fleet Monthly')).toBeInTheDocument()
    })

    // Three-dot button should be visible for admin
    const moreButton = screen.getByRole('button', { name: /more|options|delete/i })
    expect(moreButton).toBeInTheDocument()
  })

  it('delete AlertDialog shows template name in body copy', async () => {
    listTemplatesMock.mockResolvedValue([
      { id: 't1', name: 'Annual Overview', description: '', state: {}, created_at: '', updated_at: '' },
    ])
    renderDropdown({ isAdmin: true })

    fireEvent.click(screen.getByRole('button', { name: /Templates/ }))

    await waitFor(() => {
      expect(screen.getByText('Annual Overview')).toBeInTheDocument()
    })

    // Click the more options button
    const moreButton = screen.getByRole('button', { name: /more|options|delete/i })
    fireEvent.click(moreButton)

    // Click delete in the dropdown
    await waitFor(() => {
      const deleteItem = screen.getByText('Delete')
      fireEvent.click(deleteItem)
    })

    // AlertDialog should show the template name in body
    await waitFor(() => {
      expect(screen.getByText(/Annual Overview/)).toBeInTheDocument()
      expect(screen.getByText(/will be permanently deleted. This cannot be undone./)).toBeInTheDocument()
    })
  })

  it('viewer does NOT see three-dot menu on template rows', async () => {
    listTemplatesMock.mockResolvedValue([
      { id: 't1', name: 'Fleet Monthly', description: '', state: {}, created_at: '', updated_at: '' },
    ])
    renderDropdown({ isAdmin: false })

    fireEvent.click(screen.getByRole('button', { name: /Templates/ }))

    await waitFor(() => {
      expect(screen.getByText('Fleet Monthly')).toBeInTheDocument()
    })

    // No three-dot button for viewer
    expect(screen.queryByRole('button', { name: /more|options/i })).not.toBeInTheDocument()
  })
})
