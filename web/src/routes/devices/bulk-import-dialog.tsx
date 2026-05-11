import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  AlertCircle,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Download,
} from 'lucide-react'
import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Stepper } from '@/components/stepper'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ApiError } from '@/lib/api'
import {
  commitImport,
  type ImportJobRow,
  templateDownloadURL,
  uploadImport,
  type UploadResponse,
} from '@/lib/imports'

/**
 * Plan 03-09 §UI-SPEC §Bulk Import dialog (3 steps).
 *
 *   Step 1 — Upload: file picker (.xlsx/.csv) + Download template.
 *   Step 2 — Preview: summary banner + per-row outcomes table with
 *            expandable reason. Outcomes use semantic tokens
 *            (created→success, already_exists→info, invalid→warning,
 *            failed→destructive). DEV-07 idempotency visible.
 *   Step 3 — Commit: button-only "Import N devices" (D-10, NOT
 *            type-to-confirm). On success → navigate to job detail.
 *
 * UX-03 — Shifter vocabulary only (no upstream-broker concepts surface in
 * any operator-visible copy on this dialog).
 */
export interface BulkImportDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

const STEPS = [{ label: 'Upload' }, { label: 'Preview' }, { label: 'Commit' }]
const ACCEPTED_EXT = /\.(xlsx|csv)$/i
const MAX_BYTES = 5 << 20 // 5 MiB (matches backend cap)

export function BulkImportDialog({ open, onOpenChange }: BulkImportDialogProps) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [step, setStep] = useState(0)
  const [file, setFile] = useState<File | null>(null)
  const [fileError, setFileError] = useState<string | null>(null)
  const [uploadError, setUploadError] = useState<string | null>(null)
  const [commitError, setCommitError] = useState<string | null>(null)
  const [preview, setPreview] = useState<UploadResponse | null>(null)

  const reset = () => {
    setStep(0)
    setFile(null)
    setFileError(null)
    setUploadError(null)
    setCommitError(null)
    setPreview(null)
  }

  const handleOpenChange = (v: boolean) => {
    onOpenChange(v)
    if (!v) reset()
  }

  const uploadMutation = useMutation({
    mutationFn: () => {
      if (!file) throw new Error('No file selected')
      return uploadImport(file)
    },
    onSuccess: (resp) => {
      setPreview(resp)
      setStep(1)
    },
    onError: (err: unknown) => {
      const msg =
        err instanceof ApiError ? err.message : 'Could not upload the file.'
      setUploadError(msg)
    },
  })

  const commitMutation = useMutation({
    mutationFn: () => {
      if (!preview) throw new Error('No preview to commit')
      return commitImport(preview.job_id)
    },
    onSuccess: (summary) => {
      qc.invalidateQueries({ queryKey: ['devices'] })
      qc.invalidateQueries({ queryKey: ['import-jobs'] })
      toast.success(
        `Imported ${summary.created} ${summary.created === 1 ? 'device' : 'devices'}. ` +
          `Skipped ${summary.already_exists}. ${summary.failed} failed.`,
      )
      handleOpenChange(false)
      navigate(`/admin/imports/${summary.job_id}`)
    },
    onError: (err: unknown) => {
      const msg =
        err instanceof ApiError ? err.message : 'Commit failed. No devices created.'
      setCommitError(msg)
    },
  })

  const onFilePicked = (f: File | null) => {
    setFileError(null)
    setUploadError(null)
    if (!f) {
      setFile(null)
      return
    }
    if (!ACCEPTED_EXT.test(f.name)) {
      setFileError('Supported formats: XLSX, CSV.')
      setFile(null)
      return
    }
    if (f.size > MAX_BYTES) {
      setFileError(`Max file size is 5 MB. Yours is ${(f.size / 1024 / 1024).toFixed(1)} MB.`)
      setFile(null)
      return
    }
    setFile(f)
  }

  // ---- Footer per step --------------------------------------------------
  const footer = (() => {
    if (step === 0) {
      return (
        <div className="flex w-full items-center justify-between">
          <Button variant="ghost" onClick={() => handleOpenChange(false)}>
            Cancel
          </Button>
          <span className="text-sm text-muted-foreground">
            Step 1 of {STEPS.length}
          </span>
          <Button
            onClick={() => uploadMutation.mutate()}
            disabled={!file || uploadMutation.isPending}
          >
            {uploadMutation.isPending ? 'Validating…' : 'Validate file'}
          </Button>
        </div>
      )
    }
    if (step === 1) {
      const validCount = preview?.valid_count ?? 0
      return (
        <div className="flex w-full items-center justify-between">
          <Button variant="ghost" onClick={() => setStep(0)}>
            Back
          </Button>
          <span className="text-sm text-muted-foreground">
            Step 2 of {STEPS.length}
          </span>
          <Button onClick={() => setStep(2)} disabled={validCount === 0}>
            Continue to commit
          </Button>
        </div>
      )
    }
    const n = preview?.valid_count ?? 0
    return (
      <div className="flex w-full items-center justify-between">
        <Button variant="ghost" onClick={() => setStep(1)}>
          Back
        </Button>
        <span className="text-sm text-muted-foreground">
          Step 3 of {STEPS.length}
        </span>
        <Button
          onClick={() => commitMutation.mutate()}
          disabled={commitMutation.isPending || n === 0}
        >
          {commitMutation.isPending ? 'Importing…' : `Import ${n} devices`}
        </Button>
      </div>
    )
  })()

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={handleOpenChange}
      title="Bulk import devices"
      size="lg"
      footer={footer}
    >
      <div className="flex flex-col gap-6">
        <Stepper steps={STEPS} currentIndex={step} />

        {/* Step 1 — Upload */}
        {step === 0 ? (
          <div className="flex flex-col gap-4">
            <h2 className="text-lg font-semibold leading-7">
              Upload your device list
            </h2>
            <p className="text-sm text-muted-foreground">
              We&rsquo;ll validate every row before anything is created. Nothing
              changes until you confirm.
            </p>

            <Alert>
              <AlertDescription>
                XLSX is recommended — it handles Thai text and non-ASCII names
                correctly. CSV must be UTF-8.
              </AlertDescription>
            </Alert>

            <div className="flex items-center gap-3">
              <Button asChild variant="outline">
                <a href={templateDownloadURL()} download>
                  <Download className="mr-2 h-4 w-4" />
                  Download template (XLSX)
                </a>
              </Button>
            </div>

            <div className="flex flex-col gap-2">
              <Label htmlFor="bulk-import-file">Choose a file (XLSX or CSV)</Label>
              <Input
                id="bulk-import-file"
                type="file"
                accept=".xlsx,.csv"
                onChange={(e) => onFilePicked(e.target.files?.[0] ?? null)}
              />
              {file ? (
                <p className="text-xs text-muted-foreground">
                  Selected: <span className="font-mono">{file.name}</span> (
                  {(file.size / 1024).toFixed(1)} KB)
                </p>
              ) : null}
            </div>

            {fileError ? (
              <Alert variant="destructive">
                <AlertDescription>{fileError}</AlertDescription>
              </Alert>
            ) : null}
            {uploadError ? (
              <Alert variant="destructive">
                <AlertDescription>{uploadError}</AlertDescription>
              </Alert>
            ) : null}
          </div>
        ) : null}

        {/* Step 2 — Preview */}
        {step === 1 && preview ? (
          <PreviewStep preview={preview} />
        ) : null}

        {/* Step 3 — Commit confirm */}
        {step === 2 && preview ? (
          <div className="flex flex-col gap-4">
            <h2 className="text-lg font-semibold leading-7">Commit the import</h2>
            <p className="text-sm text-muted-foreground">
              This will create <strong>{preview.valid_count}</strong> devices.{' '}
              <strong>{preview.already_exists_count}</strong> already-existing
              rows will be skipped. <strong>{preview.invalid_count}</strong>{' '}
              errors won&rsquo;t be touched.
            </p>
            <Card>
              <CardContent className="flex flex-col gap-1 py-4 text-sm">
                <SummaryLine label="Will create" value={preview.valid_count} token="success" />
                <SummaryLine
                  label="Already exists (skip)"
                  value={preview.already_exists_count}
                  token="info"
                />
                <SummaryLine
                  label="Errors"
                  value={preview.invalid_count}
                  token="destructive"
                />
              </CardContent>
            </Card>
            <p className="text-xs text-muted-foreground">
              Re-running this same file is safe — Shifter uses DevEUI as the
              idempotency key.
            </p>
            {commitError ? (
              <Alert variant="destructive">
                <AlertDescription>{commitError}</AlertDescription>
              </Alert>
            ) : null}
          </div>
        ) : null}
      </div>
    </ResponsiveDialog>
  )
}

// ---------------------------------------------------------------------------

function PreviewStep({ preview }: { preview: UploadResponse }) {
  return (
    <div className="flex flex-col gap-4">
      <h2 className="text-lg font-semibold leading-7">Review what will happen</h2>
      <p className="text-sm text-muted-foreground">
        Validation is complete. Nothing has been created yet.
      </p>

      <Card>
        <CardContent className="flex flex-wrap items-center gap-6 py-4">
          <SummaryDot
            label="Will create"
            count={preview.valid_count}
            token="success"
          />
          <SummaryDot
            label="Already exists (skip)"
            count={preview.already_exists_count}
            token="info"
          />
          <SummaryDot
            label="Errors"
            count={preview.invalid_count}
            token="destructive"
          />
          {preview.invalid_count > 0 ? (
            <Button asChild size="sm" variant="outline" className="ml-auto">
              <a
                href={`/api/imports/${encodeURIComponent(preview.job_id)}/errors.xlsx`}
                download
              >
                <Download className="mr-2 h-4 w-4" />
                Download errors.xlsx
              </a>
            </Button>
          ) : null}
        </CardContent>
      </Card>

      <OutcomesTable rows={preview.outcomes} />
    </div>
  )
}

function SummaryDot({
  label,
  count,
  token,
}: {
  label: string
  count: number
  token: 'success' | 'info' | 'destructive'
}) {
  return (
    <div className="flex items-center gap-2 text-sm">
      <span
        aria-hidden="true"
        className="inline-block h-2.5 w-2.5 rounded-full"
        style={{ backgroundColor: `var(--${token})` }}
      />
      <span className="text-muted-foreground">{label}</span>
      <span className="font-semibold font-mono">{count}</span>
    </div>
  )
}

function SummaryLine({
  label,
  value,
  token,
}: {
  label: string
  value: number
  token: 'success' | 'info' | 'destructive'
}) {
  return (
    <div className="flex items-center justify-between">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-mono" style={{ color: `var(--${token})` }}>
        {value}
      </span>
    </div>
  )
}

export function OutcomeChip({ status }: { status: ImportJobRow['status'] }) {
  // Plan 03-09 §Outcome chip → semantic token map.
  const token: 'success' | 'info' | 'warning' | 'destructive' = (() => {
    switch (status) {
      case 'created':
      case 'valid':
        return 'success'
      case 'already_exists':
        return 'info'
      case 'invalid':
        return 'warning'
      case 'failed':
        return 'destructive'
      default:
        return 'warning'
    }
  })()
  return (
    <Badge
      variant="outline"
      className="text-xs"
      style={{
        color: `var(--${token})`,
        borderColor: `var(--${token})`,
      }}
    >
      {status}
    </Badge>
  )
}

export function OutcomesTable({ rows }: { rows: ImportJobRow[] }) {
  const [expanded, setExpanded] = useState<Record<number, boolean>>({})
  return (
    <div className="rounded-md border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-10" />
            <TableHead className="w-16">Row</TableHead>
            <TableHead>Outcome</TableHead>
            <TableHead>DevEUI</TableHead>
            <TableHead>Name</TableHead>
            <TableHead>Reason</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.length === 0 ? (
            <TableRow>
              <TableCell colSpan={6} className="text-center text-sm text-muted-foreground py-6">
                <AlertCircle className="inline-block mr-2 h-4 w-4" />
                No outcomes to show.
              </TableCell>
            </TableRow>
          ) : (
            rows.flatMap((r) => {
              const isOpen = !!expanded[r.row_index]
              const raw = r.raw ?? {}
              const dev_eui = String(raw.dev_eui ?? '—')
              const name = String(raw.name ?? '—')
              const out = [
                <TableRow key={`row-${r.row_index}`}>
                  <TableCell>
                    {r.reason ? (
                      <button
                        type="button"
                        aria-label={isOpen ? 'Collapse reason' : 'Expand reason'}
                        onClick={() =>
                          setExpanded((p) => ({ ...p, [r.row_index]: !isOpen }))
                        }
                      >
                        {isOpen ? (
                          <ChevronDown className="h-4 w-4" />
                        ) : (
                          <ChevronRight className="h-4 w-4" />
                        )}
                      </button>
                    ) : (
                      <CheckCircle2 className="h-4 w-4 text-muted-foreground" />
                    )}
                  </TableCell>
                  <TableCell className="text-xs font-mono">{r.row_index}</TableCell>
                  <TableCell>
                    <OutcomeChip status={r.status} />
                  </TableCell>
                  <TableCell className="font-mono text-xs">{dev_eui}</TableCell>
                  <TableCell className="text-sm">{name}</TableCell>
                  <TableCell className="text-xs text-muted-foreground max-w-[40ch] truncate">
                    {r.reason ?? '—'}
                  </TableCell>
                </TableRow>,
              ]
              if (isOpen && r.reason) {
                out.push(
                  <TableRow key={`row-${r.row_index}-expanded`} className="bg-muted/40">
                    <TableCell colSpan={6} className="text-sm py-3 pl-12">
                      {r.reason}
                    </TableCell>
                  </TableRow>,
                )
              }
              return out
            })
          )}
        </TableBody>
      </Table>
    </div>
  )
}
