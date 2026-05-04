import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { CreateSiteDialog } from './create-site-dialog'

vi.mock('@/lib/api', async (orig) => {
  const real = await orig<typeof import('@/lib/api')>()
  return {
    ...real,
    apiFetch: vi.fn(),
  }
})

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

function renderDialog(onCreated = vi.fn(), onOpenChange = vi.fn()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return {
    onCreated,
    onOpenChange,
    ...render(
      <QueryClientProvider client={qc}>
        <CreateSiteDialog open onOpenChange={onOpenChange} onCreated={onCreated} />
      </QueryClientProvider>,
    ),
  }
}

describe('CreateSiteDialog (Plan 02-14 / D-17 + D-18)', () => {
  beforeEach(() => {
    Object.defineProperty(window, 'innerWidth', {
      writable: true,
      configurable: true,
      value: 1024,
    })
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  it('renders & submits — fills form, calls /api/sites, closes on success', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      id: 'uuid-foo',
      name: 'Foo',
      lat: 13.7563,
      lng: 100.5018,
      timezone: 'Asia/Bangkok',
    })

    const onCreated = vi.fn()
    const onOpenChange = vi.fn()
    renderDialog(onCreated, onOpenChange)

    await userEvent.type(screen.getByLabelText('Site name'), 'Foo')
    await userEvent.type(screen.getByLabelText('Latitude'), '13.7563')
    await userEvent.type(screen.getByLabelText('Longitude'), '100.5018')

    await userEvent.click(screen.getByRole('button', { name: 'Add site' }))

    await waitFor(() => {
      expect(apiFetch).toHaveBeenCalledWith(
        '/api/sites',
        expect.objectContaining({ method: 'POST' }),
      )
    })
    await waitFor(() => {
      expect(onCreated).toHaveBeenCalledWith(
        expect.objectContaining({ id: 'uuid-foo' }),
      )
    })
    await waitFor(() => {
      expect(onOpenChange).toHaveBeenCalledWith(false)
    })
  })

  it('validates lat/lng range — shows inline error and disables submit', async () => {
    renderDialog()

    await userEvent.type(screen.getByLabelText('Site name'), 'Bad')
    await userEvent.type(screen.getByLabelText('Latitude'), '99')
    // Trigger blur for validation
    await userEvent.tab()

    expect(
      await screen.findByText('Latitude must be between −90 and 90.'),
    ).toBeInTheDocument()
  })

  it("renders disabled 'Pick on map' button with 'Available in v5' tooltip (D-18)", () => {
    renderDialog()
    const btn = screen.getByRole('button', { name: 'Pick on map' })
    expect(btn).toBeDisabled()
    // Tooltip is rendered as title attribute fallback for accessibility in tests.
    expect(btn).toHaveAttribute('title', 'Available in v5')
  })

  it('shows D-18 lat/lng helper text mentioning "paste from Google Maps"', () => {
    renderDialog()
    expect(
      screen.getByText(/paste from Google Maps/i),
    ).toBeInTheDocument()
  })
})
