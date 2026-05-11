import { useQuery } from '@tanstack/react-query'
import { useEffect } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  type ImportJob,
  listImportJobs,
} from '@/lib/imports'
import { useCurrentUser } from '@/lib/use-current-user'

/**
 * Plan 03-09 §UI-SPEC §Imports list page (`/admin/imports`).
 *
 *   - text-2xl "Recent imports" header + sub-description.
 *   - TanStack-friendly table with: Job ID (mono link), Started, Finished,
 *     Status chip, Totals ({created}/{skipped}/{failed}), Actor.
 *   - Default sort: started desc (backend default).
 *   - Admin-only (D-36 + T-3-90 mitigation). Non-admin → redirect home.
 */
export default function AdminImportsPage() {
  const user = useCurrentUser()
  const navigate = useNavigate()
  useEffect(() => {
    if (user && user.role !== 'admin') {
      navigate('/', { replace: true })
    }
  }, [user, navigate])

  const jobsQuery = useQuery({
    queryKey: ['import-jobs', { page: 1, per_page: 50 }],
    queryFn: () => listImportJobs({ page: 1, per_page: 50 }),
    enabled: user?.role === 'admin',
  })

  if (user && user.role !== 'admin') {
    return null
  }

  const jobs = jobsQuery.data?.jobs ?? []
  const isLoading = jobsQuery.isLoading
  const isEmpty = !isLoading && jobs.length === 0

  return (
    <div className="flex flex-col gap-6 p-6">
      <header className="flex flex-col gap-1">
        <h1 className="text-2xl font-semibold leading-8">Recent imports</h1>
        <p className="text-sm text-muted-foreground">
          Bulk imports from the last 90 days. Older imports are pruned.
        </p>
      </header>

      {isEmpty ? (
        <div className="flex flex-col items-center gap-4 rounded-md border p-12 text-center">
          <h2 className="text-lg font-semibold">No imports yet</h2>
          <p className="text-sm text-muted-foreground">
            Run your first bulk import from the Devices page.
          </p>
          <Button asChild variant="outline">
            <Link to="/devices">Go to Devices</Link>
          </Button>
        </div>
      ) : (
        <div className="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Job ID</TableHead>
                <TableHead>Started</TableHead>
                <TableHead>Finished</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Totals (created / skipped / failed)</TableHead>
                <TableHead>File</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading
                ? Array.from({ length: 5 }).map((_, i) => (
                    <TableRow key={`sk-${i}`}>
                      {Array.from({ length: 6 }).map((__, ci) => (
                        <TableCell key={ci}>
                          <Skeleton className="h-4 w-full" />
                        </TableCell>
                      ))}
                    </TableRow>
                  ))
                : jobs.map((job) => <JobRow key={job.id} job={job} />)}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  )
}

function JobRow({ job }: { job: ImportJob }) {
  const shortId = job.job_id.slice(0, 8)
  return (
    <TableRow>
      <TableCell>
        <Link
          to={`/admin/imports/${job.job_id}`}
          className="text-xs font-mono text-primary hover:underline"
        >
          {shortId}
        </Link>
      </TableCell>
      <TableCell className="text-xs">{fmt(job.created_at)}</TableCell>
      <TableCell className="text-xs">{fmt(job.committed_at)}</TableCell>
      <TableCell>
        <StatusChip status={job.status} />
      </TableCell>
      <TableCell className="text-sm font-mono">
        <span style={{ color: 'var(--success)' }}>{job.created_count}</span>{' '}
        / <span style={{ color: 'var(--info)' }}>{job.already_exists_count}</span>{' '}
        /{' '}
        <span style={{ color: 'var(--destructive)' }}>
          {job.failed_count + job.invalid_count}
        </span>
      </TableCell>
      <TableCell className="text-xs text-muted-foreground truncate max-w-[24ch]">
        {job.file_name}
      </TableCell>
    </TableRow>
  )
}

function StatusChip({ status }: { status: ImportJob['status'] }) {
  const token: 'success' | 'info' | 'warning' | 'destructive' | 'muted' = (() => {
    switch (status) {
      case 'committed':
        return 'success'
      case 'preview':
        return 'info'
      case 'committing':
        return 'info'
      case 'expired':
        return 'muted'
      case 'failed':
        return 'destructive'
      default:
        return 'muted'
    }
  })()
  if (token === 'muted') {
    return (
      <Badge variant="secondary" className="text-xs">
        {status}
      </Badge>
    )
  }
  return (
    <Badge
      variant="outline"
      className="text-xs"
      style={{ color: `var(--${token})`, borderColor: `var(--${token})` }}
    >
      {status}
    </Badge>
  )
}

function fmt(iso: string | null | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleString()
}
