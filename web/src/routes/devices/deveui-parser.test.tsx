import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DevEUIParser } from './deveui-parser'

vi.mock('@/lib/api', async (orig) => {
  const real = await orig<typeof import('@/lib/api')>()
  return {
    ...real,
    apiFetch: vi.fn(),
  }
})

function renderParser(onPick = vi.fn()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return {
    onPick,
    ...render(
      <QueryClientProvider client={qc}>
        <DevEUIParser onPick={onPick} />
      </QueryClientProvider>,
    ),
  }
}

describe('DevEUIParser (Plan 02-14 / D-11)', () => {
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

  it('renders both interpretations (MSB-first + LSB-first)', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      msb: '0102030405060708',
      lsb: '0807060504030201',
      msb_vendor: 'unknown',
      lsb_vendor: 'unknown',
    })

    renderParser()
    await userEvent.type(screen.getByLabelText('DevEUI'), '0102030405060708')

    await waitFor(() => {
      expect(screen.getByText(/MSB-first:/)).toBeInTheDocument()
    })
    expect(screen.getByText(/01:02:03:04:05:06:07:08/)).toBeInTheDocument()
    expect(screen.getByText(/LSB-first:/)).toBeInTheDocument()
    expect(screen.getByText(/08:07:06:05:04:03:02:01/)).toBeInTheDocument()
  })

  it('strips hyphens, colons, and spaces before parsing', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      msb: '0102030405060708',
      lsb: '0807060504030201',
      msb_vendor: 'unknown',
      lsb_vendor: 'unknown',
    })

    renderParser()
    const input = screen.getByLabelText('DevEUI')
    await userEvent.type(input, '01:02:03-04 05:06:07:08')
    await userEvent.tab()

    // After blur, the input value is the normalized 16-hex form.
    await waitFor(() => {
      expect((input as HTMLInputElement).value).toBe('0102030405060708')
    })
    // And the preview rows still render with the colon-formatted display.
    expect(screen.getByText(/01:02:03:04:05:06:07:08/)).toBeInTheDocument()
  })

  it('shows vendor OUI hint when backend matches (e.g. Axioma)', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      msb: '70b3d57ed0011223',
      lsb: '231201d07ed5b370',
      msb_vendor: 'Axioma',
      lsb_vendor: 'unknown',
    })

    renderParser()
    await userEvent.type(screen.getByLabelText('DevEUI'), '70b3d57ed0011223')

    await waitFor(() => {
      expect(screen.getByText(/Vendor: Axioma/)).toBeInTheDocument()
    })
  })

  it('emits lowercase 16-hex no-separator on confirm', async () => {
    const { apiFetch } = await import('@/lib/api')
    ;(apiFetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      msb: '0102030405060708',
      lsb: '0807060504030201',
      msb_vendor: 'unknown',
      lsb_vendor: 'unknown',
    })

    const onPick = vi.fn()
    renderParser(onPick)
    await userEvent.type(screen.getByLabelText('DevEUI'), '0102030405060708')

    await waitFor(() => {
      expect(screen.getByLabelText(/LSB-first/i)).toBeInTheDocument()
    })

    // Click the LSB radio interpretation, then click confirm.
    await userEvent.click(screen.getByLabelText(/LSB-first/i))
    await userEvent.click(screen.getByRole('button', { name: /Use this DevEUI/i }))

    expect(onPick).toHaveBeenCalledWith('0807060504030201')
  })
})
