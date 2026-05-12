/**
 * useReportPDFStatus — Plan 05-09 Task 2
 *
 * Polls GET /api/reports/:id every 2s while pdf_status is 'pending' or 'running'.
 * Stops automatically when status reaches 'ready', 'failed', or 'expired'.
 * Fires a Sonner toast once when status transitions to 'ready' (with click-to-download action).
 * Fires a Sonner toast.error once when status becomes 'failed'.
 *
 * Backend contract (plan 05-06 StatusHandler, StatusResponse shape):
 *   { id, pdf_status, pdf_path?, created_at, scope, range }
 */

import { useEffect, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'
import { apiFetch } from '@/lib/api'

// ---------------------------------------------------------------------------
// Types — matches plan 05-06 StatusResponse shape verbatim
// ---------------------------------------------------------------------------

export type PdfStatus = 'pending' | 'running' | 'ready' | 'failed' | 'expired'

export type ReportStatus = {
  id: string
  pdf_status: PdfStatus
  pdf_path?: string
  created_at: string
  scope: 'all' | 'site' | 'meter'
  range: 'daily' | 'monthly' | 'yearly' | 'custom'
}

function isTerminal(status: PdfStatus): boolean {
  return status === 'ready' || status === 'failed' || status === 'expired'
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

export function useReportPDFStatus(reportID: string, initialStatus: PdfStatus): PdfStatus {
  // Track the last status seen so we fire toasts only on transition, never on
  // re-render of the same status (prevents duplicate toasts on React StrictMode
  // double-invocation).
  const lastNotified = useRef<PdfStatus>(initialStatus)

  const query = useQuery<ReportStatus>({
    queryKey: ['report-status', reportID],
    queryFn: () => apiFetch<ReportStatus>(`/api/reports/${reportID}`),
    // Poll every 2s only while the job is in-flight; stop when terminal
    refetchInterval: (q) => {
      const s = q.state.data?.pdf_status ?? initialStatus
      return s === 'pending' || s === 'running' ? 2000 : false
    },
    // Seed with the status we already know from the generate response, so the
    // tile can show the spinner immediately without waiting for the first poll.
    initialData: {
      id: reportID,
      pdf_status: initialStatus,
      created_at: new Date().toISOString(),
      scope: 'all' as const,
      range: 'monthly' as const,
    },
    // Mark initial data as fresh for terminal statuses so TanStack Query does
    // not fire a background refetch immediately. For pending/running, set to 0
    // so the first poll fires on schedule (after 2s via refetchInterval).
    initialDataUpdatedAt: isTerminal(initialStatus) ? Date.now() : 0,
    staleTime: isTerminal(initialStatus) ? Infinity : 0,
    // Don't refetch on window focus when we've already stopped polling
    refetchOnWindowFocus: false,
  })

  const currentStatus = query.data?.pdf_status ?? initialStatus

  useEffect(() => {
    if (currentStatus === 'ready' && lastNotified.current !== 'ready') {
      lastNotified.current = 'ready'
      toast.success('Your PDF is ready — click to download.', {
        action: {
          label: 'Download',
          onClick: () => {
            window.location.href = `/api/reports/${reportID}/file/pdf`
          },
        },
      })
    }

    if (currentStatus === 'failed' && lastNotified.current !== 'failed') {
      lastNotified.current = 'failed'
      toast.error('PDF generation failed. Try again or contact support.')
    }
  }, [currentStatus, reportID])

  return currentStatus
}
