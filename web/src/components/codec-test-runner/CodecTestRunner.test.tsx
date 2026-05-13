import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { CodecTestRunner } from './CodecTestRunner'
import * as codecTestApi from '@/lib/codecTest'

// Mock the API module
vi.mock('@/lib/codecTest', () => ({
  runCodecTest: vi.fn(),
}))

// Mock clipboard
Object.defineProperty(navigator, 'clipboard', {
  value: { writeText: vi.fn().mockResolvedValue(undefined) },
  writable: true,
})

function renderWithQueryClient(ui: React.ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>)
}

describe('CodecTestRunner panel (Surface 4, D-05..D-08, D-26..D-28)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders Test Codec panel collapsed by default', () => {
    renderWithQueryClient(<CodecTestRunner profileId="prof-abc-123" />)

    // Panel header should be visible
    expect(screen.getByText('Test Codec')).toBeInTheDocument()

    // The output region and input fields should not be rendered until expanded
    expect(screen.queryByLabelText('fPort (optional)')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Payload (hex bytes)')).not.toBeInTheDocument()
    expect(screen.queryByText('Run Test')).not.toBeInTheDocument()
  })

  it('shows decoded JSON tree on successful test', async () => {
    const mockRunCodecTest = vi.mocked(codecTestApi.runCodecTest)
    mockRunCodecTest.mockResolvedValue({
      decoded_json: { cumulative: 123.456 },
      canonical_mapping: { cumulative_value: 123.456 },
    })

    renderWithQueryClient(<CodecTestRunner profileId="prof-abc-123" />)

    // Expand the panel
    fireEvent.click(screen.getByText('Test Codec'))

    // Fill in hex payload
    const hexInput = screen.getByLabelText('Payload (hex bytes)')
    fireEvent.change(hexInput, { target: { value: '6F 01 23 45' } })

    // Click Run Test
    fireEvent.click(screen.getByText('Run Test'))

    // Should show Decoded JSON tab as active with the data
    await waitFor(() => {
      expect(screen.getByRole('tab', { name: 'Decoded JSON' })).toBeInTheDocument()
    })
    await waitFor(() => {
      // JsonTree renders keys, so "cumulative" should appear
      expect(screen.getByText('cumulative')).toBeInTheDocument()
    })
  })

  it('renders error panel with line/col on goja exception', async () => {
    const mockRunCodecTest = vi.mocked(codecTestApi.runCodecTest)
    mockRunCodecTest.mockResolvedValue({
      error_message: 'SyntaxError: unexpected token',
      error_line: 5,
      error_col: 12,
      error_stack: 'SyntaxError: unexpected token\n  at eval:5:12',
    })

    renderWithQueryClient(<CodecTestRunner profileId="prof-abc-123" />)

    // Expand the panel
    fireEvent.click(screen.getByText('Test Codec'))

    // Fill in hex payload
    const hexInput = screen.getByLabelText('Payload (hex bytes)')
    fireEvent.change(hexInput, { target: { value: 'DE AD BE EF' } })

    // Click Run Test
    fireEvent.click(screen.getByText('Run Test'))

    // Should show error panel with line/col info
    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument()
    })
    await waitFor(() => {
      expect(screen.getByText(/at line 5, column 12/)).toBeInTheDocument()
    })
    // Copy Stack Trace button should be visible
    expect(screen.getByText('Copy Stack Trace')).toBeInTheDocument()
  })
})
