/**
 * PdfStatusPill — Plan 05-09 Task 2
 *
 * Renders the PDF tile content based on pdf_status from useReportPDFStatus.
 * States: pending/running → spinner + "Generating PDF…"
 *         ready → "Download PDF" button with href to file endpoint
 *         failed → error message
 *         expired → expired notice
 *
 * Accessibility: aria-live="polite" on the spinner state announces the
 * transition to screen readers.
 * Motion: Loader2 uses motion-reduce:animate-none (respects prefers-reduced-motion).
 */

import { AlertCircle, Download, FileText, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import type { PdfStatus } from './useReportPDFStatus'

interface PdfStatusPillProps {
  status: PdfStatus
  reportID: string
}

export function PdfStatusPill({ status, reportID }: PdfStatusPillProps) {
  switch (status) {
    case 'pending':
    case 'running':
      return (
        <div className="flex flex-col items-center gap-2 p-4" aria-live="polite">
          <Loader2 className="h-8 w-8 animate-spin motion-reduce:animate-none text-primary" />
          <div className="text-sm text-muted-foreground">Generating PDF…</div>
        </div>
      )

    case 'ready':
      return (
        <Button asChild className="w-full" variant="default">
          <a href={`/api/reports/${reportID}/file/pdf`} download>
            <Download className="h-4 w-4 mr-2" />
            Download PDF
          </a>
        </Button>
      )

    case 'failed':
      return (
        <div className="flex flex-col items-center gap-2 p-4 text-destructive">
          <AlertCircle className="h-8 w-8" />
          <div className="text-sm">PDF generation failed</div>
        </div>
      )

    case 'expired':
      return (
        <div className="text-sm text-muted-foreground p-4 text-center">
          Report expired
        </div>
      )

    default:
      return <FileText className="h-8 w-8 text-muted-foreground" />
  }
}
