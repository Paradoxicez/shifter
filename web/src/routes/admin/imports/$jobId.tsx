import { useQuery } from '@tanstack/react-query'
import { Download } from 'lucide-react'
import { useEffect } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { errorsXLSXDownloadURL, getImportJob, type ImportJob } from '@/lib/imports'
import { useCurrentUser } from '@/lib/use-current-user'
import { OutcomesTable } from '@/routes/devices/bulk-import-dialog'

/**
 * Plan 03-09 §UI-SPEC §Import job detail page (D-36, T-3-90 mitigation).
 *
 *   - text-2xl "Import job {short_id}" + breadcrumb back to /admin/imports.
 *   - Summary card with 4 big-number stats: Total · Created · Skipped · Failed.
 *   - Per-row outcomes table (reuses BulkImport Step 2 component).
 *   - Download errors.xlsx button (shown when invalid_count + failed_count > 0).
 *   - Expired-job banner when status='expired'.
 *
 * Admin-only route guard (server-side RequireAction is authoritative).
 */
export default function ImportJobDetailPage() {
  const { jobId = '' } = useParams<{ jobId: string }>()
  const user = useCurrentUser()
  const navigate = useNavigate()

  useEffect(() => {
    if (user && user.role !== 'admin') {
      navigate('/', { replace: true })
    }
  }, [user, navigate])

  const jobQuery = useQuery({
    queryKey: ['import-job', jobId],
    queryFn: () => getImportJob(jobId, { page: 1, per_page: 100 }),
    enabled: user?.role === 'admin' && !!jobId,
  })

  if (user && user.role !== 'admin') return null

  if (jobQuery.isLoading) {
    return (
      <div className="flex flex-col gap-6 p-6">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-32 w-full" />
        <Skeleton className="h-96 w-full" />
      </div>
    )
  }
  if (jobQuery.isError || !jobQuery.data) {
    return (
      <div className="flex flex-col gap-6 p-6">
        <Alert variant="destructive">
          <AlertDescription>Could not load import job.</AlertDescription>
        </Alert>
        <Button asChild variant="outline">
          <Link to="/admin/imports">Back to imports</Link>
        </Button>
      </div>
    )
  }

  const { job, rows } = jobQuery.data
  const shortId = job.job_id.slice(0, 8)
  const errorTotal = job.failed_count + job.invalid_count
  const expired = job.status === 'expired'

  return (
    <div className="flex flex-col gap-6 p-6">
      <header className="flex flex-col gap-1">
        <p className="text-sm text-muted-foreground">
          <Link to="/admin/imports" className="hover:underline">
            Recent imports
          </Link>{' '}
          ›{' '}
          <span className="font-mono">{shortId}</span>
        </p>
        <div className="flex items-center justify-between">
          <h1 className="text-2xl font-semibold leading-8">
            Import job <span className="font-mono">{shortId}</span>
          </h1>
          <Button
            asChild={errorTotal > 0}
            variant="outline"
            disabled={errorTotal === 0}
          >
            {errorTotal > 0 ? (
              <a href={errorsXLSXDownloadURL(job.job_id)} download>
                <Download className="mr-2 h-4 w-4" />
                Download errors.xlsx
              </a>
            ) : (
              <span>
                <Download className="mr-2 h-4 w-4" />
                Download errors.xlsx
              </span>
            )}
          </Button>
        </div>
      </header>

      {expired ? (
        <Alert>
          <AlertDescription>
            This preview has expired. Re-upload to retry.
          </AlertDescription>
        </Alert>
      ) : null}

      <SummaryCard job={job} />

      <p className="text-sm text-muted-foreground">
        Started {fmt(job.created_at)} ·{' '}
        {job.committed_at ? `Finished ${fmt(job.committed_at)}` : 'Not committed yet'}{' '}
        · File <span className="font-mono">{job.file_name}</span> ({job.file_format})
      </p>

      <OutcomesTable rows={rows} />
    </div>
  )
}

function SummaryCard({ job }: { job: ImportJob }) {
  const cells = [
    { label: 'Total rows', value: job.total_rows, token: 'foreground' as const },
    { label: 'Created', value: job.created_count, token: 'success' as const },
    {
      label: 'Skipped',
      value: job.already_exists_count,
      token: 'info' as const,
    },
    {
      label: 'Failed',
      value: job.failed_count + job.invalid_count,
      token: 'destructive' as const,
    },
  ]
  return (
    <Card>
      <CardContent className="grid grid-cols-2 gap-6 py-4 md:grid-cols-4">
        {cells.map((c) => (
          <div key={c.label} className="flex flex-col gap-1">
            <span
              className="text-2xl font-mono font-semibold"
              style={c.token === 'foreground' ? undefined : { color: `var(--${c.token})` }}
            >
              {c.value}
            </span>
            <span className="text-xs text-muted-foreground">{c.label}</span>
          </div>
        ))}
      </CardContent>
    </Card>
  )
}

function fmt(iso: string | null | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleString()
}
