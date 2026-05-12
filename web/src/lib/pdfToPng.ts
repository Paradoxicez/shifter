// Client-side PDF → PNG conversion (D-17). The server NEVER sees PDF bytes.
//
// Pitfall #3 (RESEARCH): pdfjs-dist v5 is ESM-only. The worker is set ONCE via
// the side-effect import of './pdfWorker' (plan 05-01). Importing this file
// pulls in the worker shim.

import { pdfjsLib } from './pdfWorker'

export class ErrInvalidPDF extends Error {
  constructor() {
    super('not a valid PDF')
  }
}

export class ErrPDFTooLarge extends Error {
  constructor(public dim: number) {
    super(`PDF page renders to ${dim}px, exceeds 8192 cap`)
  }
}

const TARGET_DPI = 150
const PDF_NATIVE_DPI = 72
const SCALE = TARGET_DPI / PDF_NATIVE_DPI // 150 / 72 ≈ 2.083
const MAX_DIM = 8192

export async function convertPdfToPng(file: File): Promise<Blob> {
  if (file.type !== 'application/pdf') throw new ErrInvalidPDF()

  const buf = await file.arrayBuffer()
  const pdf = await pdfjsLib.getDocument({ data: buf }).promise

  try {
    const page = await pdf.getPage(1)
    const viewport = page.getViewport({ scale: SCALE })

    // Enforce 8192² cap client-side (D-19): server will also reject but we
    // shouldn't waste user bandwidth on a doomed upload.
    if (viewport.width > MAX_DIM || viewport.height > MAX_DIM) {
      page.cleanup()
      throw new ErrPDFTooLarge(Math.max(viewport.width, viewport.height))
    }

    const canvas = new OffscreenCanvas(
      Math.floor(viewport.width),
      Math.floor(viewport.height),
    )
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const ctx = canvas.getContext('2d') as any
    if (!ctx) throw new Error('canvas 2d context unavailable')

    // pdfjs-dist v5: canvas is required; pass null when using canvasContext directly
    // (OffscreenCanvas is not HTMLCanvasElement, so we supply canvasContext + canvas:null)
    await page.render({ canvas: null, canvasContext: ctx, viewport }).promise

    page.cleanup()
    return await canvas.convertToBlob({ type: 'image/png' })
  } finally {
    await pdf.destroy()
  }
}
