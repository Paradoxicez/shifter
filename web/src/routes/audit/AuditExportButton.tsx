/**
 * Plan 06-07 — AuditExportButton
 *
 * Renders the "Export CSV" button. If audit row count <= 50,000 it triggers an
 * inline browser download via window.location. If > 50,000 it calls the async
 * River job path and shows a sonner toast.
 *
 * D-35: 50k inline / >50k async split.
 */

import { Button } from '@/components/ui/button'
import { Download } from 'lucide-react'
import { toast } from 'sonner'
import { useAuditCount, useExportAuditAsync } from '@/hooks/useAudit'
import type { AuditFilters } from '@/lib/auditParams'

const INLINE_CAP = 50_000

interface AuditExportButtonProps {
  params: Pick<AuditFilters, 'from' | 'to' | 'entity_type' | 'action'> & Partial<AuditFilters>
}

export function AuditExportButton({ params }: AuditExportButtonProps) {
  const { data: countData, isLoading: countLoading } = useAuditCount(params)
  const asyncExport = useExportAuditAsync()

  const count = countData?.count ?? 0
  const isInline = count <= INLINE_CAP
  const isLoading = countLoading || asyncExport.isPending

  function buildExportURL(): string {
    const sp = new URLSearchParams()
    if (params.from) sp.set('from', params.from)
    if (params.to) sp.set('to', params.to)
    if (params.entity_type?.length) {
      for (const et of params.entity_type) sp.append('entity_type', et)
    }
    if (params.action?.length) {
      for (const a of params.action) sp.append('action', a)
    }
    if (params.user_id) sp.set('user_id', params.user_id)
    if (params.request_id) sp.set('request_id', params.request_id)
    return `/api/audit/export?${sp.toString()}`
  }

  function handleClick() {
    if (isInline) {
      window.location.href = buildExportURL()
    } else {
      asyncExport.mutate(params, {
        onSuccess: (data) => {
          toast.success(`Full export queued. Job ID: ${data.job_id}. You'll be notified when it's ready.`)
        },
        onError: (err) => {
          toast.error(`Export failed: ${(err as Error).message}`)
        },
      })
    }
  }

  return (
    <Button
      variant="outline"
      size="sm"
      onClick={handleClick}
      disabled={isLoading}
      data-inline={isInline ? 'true' : 'false'}
    >
      <Download className="h-4 w-4 mr-2" />
      {asyncExport.isPending ? 'Queuing…' : 'Export CSV'}
      {!countLoading && count > 0 && (
        <span className="ml-1 text-xs text-muted-foreground">
          ({count > INLINE_CAP ? `${(count / 1000).toFixed(0)}k — async` : count})
        </span>
      )}
    </Button>
  )
}
