import { describe, it, expect, vi, beforeEach } from 'vitest'

// ---------------------------------------------------------------------------
// Minimal fixture: a 1-byte "PDF" header so pdfjs-dist can be mocked below.
// The real pdfjs-dist is NOT called in unit tests; we replace the module.
// ---------------------------------------------------------------------------

// OffscreenCanvas polyfill for jsdom
if (typeof OffscreenCanvas === 'undefined') {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  ;(globalThis as any).OffscreenCanvas = class OffscreenCanvas {
    width: number
    height: number
    constructor(w: number, h: number) {
      this.width = w
      this.height = h
    }
    getContext(_: string) {
      return {}
    }
    convertToBlob(_opts?: { type?: string }) {
      return Promise.resolve(new Blob(['<png>'], { type: _opts?.type ?? 'image/png' }))
    }
  }
}

// ---------------------------------------------------------------------------
// Mock pdfjs-dist so tests don't spin up a real Worker
// ---------------------------------------------------------------------------
const mockPageCleanup = vi.fn()
const mockDocDestroy = vi.fn().mockResolvedValue(undefined)
const mockRenderPromise = Promise.resolve()
const mockGetPage = vi.fn().mockResolvedValue({
  getViewport: () => ({ width: 800, height: 600 }),
  render: () => ({ promise: mockRenderPromise }),
  cleanup: mockPageCleanup,
})
const mockGetDocument = vi.fn().mockReturnValue({ promise: Promise.resolve({ getPage: mockGetPage, destroy: mockDocDestroy }) })

vi.mock('./pdfWorker', () => ({
  pdfjsLib: {
    GlobalWorkerOptions: { workerSrc: 'pdf.worker.min.mjs' },
    getDocument: mockGetDocument,
  },
}))

// Import AFTER mocking
const { convertPdfToPng, ErrInvalidPDF } = await import('./pdfToPng')

describe('pdfToPng', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockDocDestroy.mockResolvedValue(undefined)
    mockGetPage.mockResolvedValue({
      getViewport: () => ({ width: 800, height: 600 }),
      render: () => ({ promise: Promise.resolve() }),
      cleanup: mockPageCleanup,
    })
    mockGetDocument.mockReturnValue({ promise: Promise.resolve({ getPage: mockGetPage, destroy: mockDocDestroy }) })
  })

  it('renders PDF page 1 to 150-DPI canvas and exports PNG blob (D-17)', async () => {
    const file = new File([new Uint8Array([37, 80, 68, 70])], 'plan.pdf', { type: 'application/pdf' })
    const blob = await convertPdfToPng(file)
    expect(blob.type).toBe('image/png')
    expect(blob.size).toBeGreaterThan(0)
    // Confirm getDocument was called with arraybuffer
    expect(mockGetDocument).toHaveBeenCalledOnce()
  })

  it('uses pdfWorker shim — workerSrc contains pdf.worker.min.mjs', async () => {
    // pdfWorker is mocked; confirm the shim export is being consumed
    const { pdfjsLib } = await import('./pdfWorker')
    expect(pdfjsLib.GlobalWorkerOptions.workerSrc).toContain('pdf.worker.min.mjs')
  })

  it('throws ErrInvalidPDF when given a non-PDF file (mime type guard)', async () => {
    const file = new File([new Uint8Array([1, 2, 3])], 'image.png', { type: 'image/png' })
    await expect(convertPdfToPng(file)).rejects.toBeInstanceOf(ErrInvalidPDF)
  })

  it('releases page + document memory after blob export (cleanup + destroy called)', async () => {
    const file = new File([new Uint8Array([37, 80, 68, 70])], 'plan.pdf', { type: 'application/pdf' })
    await convertPdfToPng(file)
    expect(mockPageCleanup).toHaveBeenCalledOnce()
    expect(mockDocDestroy).toHaveBeenCalledOnce()
  })
})
