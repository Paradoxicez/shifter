// pdfjs-dist v5 is ESM-only and dropped the UMD pdf.worker.min.js build. The
// only Vite-compatible way to load the worker is via the import.meta.url URL
// pattern — letting Vite emit the worker as a build asset. Import this file
// ONCE at the entrypoint of every component that calls pdfjsLib.getDocument()
// (or import it from a shared "pdfToPng" helper that imports this).
//
// Documented in Phase 5 RESEARCH §pdf.js + §Common Pitfalls #3.

import * as pdfjsLib from 'pdfjs-dist'

pdfjsLib.GlobalWorkerOptions.workerSrc = new URL(
  'pdfjs-dist/build/pdf.worker.min.mjs',
  import.meta.url,
).toString()

export { pdfjsLib }
