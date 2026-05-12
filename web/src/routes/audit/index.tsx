/**
 * Plan 06-07 — /audit route (UI-SPEC §Surface 6).
 *
 * Admin-only audit log browser. Viewers are redirected to "/".
 * URL-state filter chips (from, to, entity_type, action, user_id, request_id)
 * drive the React Query infinite scroll fetch.
 *
 * D-31: admin-only — viewer role → redirect to "/".
 * D-37: No live-tail. Manual Refresh button + refetchOnWindowFocus:false.
 */

import { Navigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useCurrentUser } from '@/lib/use-current-user'
import { useAuditList, useAuditDistincts } from '@/hooks/useAudit'
import { useAuditParams } from '@/lib/auditParams'
import { AuditFilterChips } from './AuditFilterChips'
import { AuditTable } from './AuditTable'
import { AuditExportButton } from './AuditExportButton'

export default function AuditPage() {
  const user = useCurrentUser()

  // D-31: admin-only guard
  if (user?.role !== 'admin') {
    return <Navigate to="/" replace />
  }

  return <AuditPageContent />
}

function AuditPageContent() {
  const [params, setParams] = useAuditParams()
  const {
    data,
    isLoading,
    isError,
    error,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    refetch,
    isRefetching,
  } = useAuditList(params)
  const { data: distincts } = useAuditDistincts()

  // Flatten all pages into a single row list.
  const rows = data?.pages.flatMap((p) => p.rows) ?? []
  const total = data?.pages[0]?.total ?? 0

  return (
    <div className="flex flex-col gap-4">
      <header className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">
          Audit log{total > 0 ? ` (${total.toLocaleString()})` : ''}
        </h1>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => refetch()}
            disabled={isRefetching}
          >
            {isRefetching ? 'Refreshing…' : 'Refresh'}
          </Button>
          <AuditExportButton params={params} />
        </div>
      </header>

      <AuditFilterChips
        params={params}
        setParams={setParams}
        availableActions={distincts?.actions ?? []}
        availableEntityTypes={distincts?.entity_types ?? []}
      />

      {isError ? (
        <div className="rounded-md border border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive">
          Failed to load audit log: {(error as Error).message}
        </div>
      ) : isLoading ? (
        <div className="flex flex-col gap-2">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
        </div>
      ) : (
        <AuditTable
          rows={rows}
          hasNextPage={hasNextPage}
          onLoadMore={() => fetchNextPage()}
          isLoadingMore={isFetchingNextPage}
        />
      )}
    </div>
  )
}
