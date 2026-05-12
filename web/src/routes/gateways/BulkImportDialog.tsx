/**
 * Plan 07-13 Task 3 — BulkImportDialog (3-step gateway bulk import).
 *
 * UI-SPEC Surface 7 verbatim copy:
 *   Step 1: "Import Gateways" / "Upload a CSV file to bulk-import gateways.
 *            Download the template to see required columns." / "Download CSV template"
 *            / "Discard import" / "Validate"
 *   Step 2: "Validation Results" / "{N} gateways ready to import." / "{N} errors" badge
 *            / "Import {N} Gateways"
 *   Commit: "Importing gateways…" → "{N} gateways imported successfully." + "Show details"
 *   Toast success: "Import complete: {N} gateways added."
 *   Toast error:   "Import failed. Check your connection and try again."
 *   Retry button:  "Retry"
 */

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { AlertCircle, ChevronDown, ChevronRight, Download } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
import {
  commitGatewayCSV,
  gatewayImportTemplateURL,
  type CommitResult,
  type ValidateResult,
  validateGatewayCSV,
} from '@/lib/gatewayImport'

export interface BulkImportDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

const MAX_BYTES = 5 << 20 // 5 MiB

type Step = 'upload' | 'results' | 'importing' | 'done'

export function BulkImportDialog({ open, onOpenChange }: BulkImportDialogProps) {
  const qc = useQueryClient()
  const [step, setStep] = useState<Step>('upload')
  const [file, setFile] = useState<File | null>(null)
  const [fileError, setFileError] = useState<string | null>(null)
  const [validateResult, setValidateResult] = useState<ValidateResult | null>(null)
  const [commitResult, setCommitResult] = useState<CommitResult | null>(null)
  const [showDetails, setShowDetails] = useState(false)

  const reset = () => {
    setStep('upload')
    setFile(null)
    setFileError(null)
    setValidateResult(null)
    setCommitResult(null)
    setShowDetails(false)
  }

  const handleOpenChange = (v: boolean) => {
    onOpenChange(v)
    if (!v) reset()
  }

  const validateMutation = useMutation({
    mutationFn: () => {
      if (!file) throw new Error('No file selected')
      return validateGatewayCSV(file)
    },
    onSuccess: (result) => {
      setValidateResult(result)
      setStep('results')
    },
    onError: () => {
      toast.error('Import failed. Check your connection and try again.')
    },
  })

  const commitMutation = useMutation({
    mutationFn: () => {
      if (!file) throw new Error('No file selected')
      return commitGatewayCSV(file)
    },
    onSuccess: (result) => {
      qc.invalidateQueries({ queryKey: ['gateways'] })
      setCommitResult(result)
      setStep('done')
      toast.success(`Import complete: ${result.created} gateways added.`)
    },
    onError: () => {
      toast.error('Import failed. Check your connection and try again.')
    },
  })

  const onFilePicked = (f: File | null) => {
    setFileError(null)
    if (!f) {
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

  const validCount = validateResult?.valid_rows ?? 0
  const errorCount = validateResult?.error_rows ?? 0

  // ---- Footer per step -------------------------------------------------------
  const footer = (() => {
    if (step === 'upload') {
      return (
        <div className="flex w-full items-center justify-between">
          <Button variant="ghost" onClick={() => handleOpenChange(false)}>
            Discard import
          </Button>
          <Button
            onClick={() => validateMutation.mutate()}
            disabled={!file || validateMutation.isPending}
          >
            {validateMutation.isPending ? 'Validating…' : 'Validate'}
          </Button>
        </div>
      )
    }

    if (step === 'results') {
      return (
        <div className="flex w-full items-center justify-between">
          <Button variant="ghost" onClick={() => setStep('upload')}>
            Back
          </Button>
          <div className="flex items-center gap-2">
            {commitMutation.isError ? (
              <Button
                variant="outline"
                onClick={() => {
                  setStep('importing')
                  commitMutation.mutate()
                }}
              >
                Retry
              </Button>
            ) : null}
            <Button
              onClick={() => {
                setStep('importing')
                commitMutation.mutate()
              }}
              disabled={validCount === 0 || commitMutation.isPending}
            >
              Import {validCount} Gateways
            </Button>
          </div>
        </div>
      )
    }

    if (step === 'importing') {
      return (
        <div className="flex w-full justify-end">
          <Button disabled>Importing gateways…</Button>
        </div>
      )
    }

    // done
    return (
      <div className="flex w-full justify-end">
        <Button onClick={() => handleOpenChange(false)}>Close</Button>
      </div>
    )
  })()

  // ---- Title per step -------------------------------------------------------
  const title = step === 'upload' ? 'Import Gateways' : 'Validation Results'

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={handleOpenChange}
      title={title}
      size="lg"
      footer={footer}
    >
      <div className="flex flex-col gap-6">
        {/* Step 1 — Upload */}
        {step === 'upload' ? (
          <div className="flex flex-col gap-4">
            <p className="text-sm text-muted-foreground">
              Upload a CSV file to bulk-import gateways. Download the template to see required columns.
            </p>

            <div className="flex items-center gap-3">
              <Button asChild variant="outline">
                <a href={gatewayImportTemplateURL()} download>
                  <Download className="mr-2 h-4 w-4" />
                  Download CSV template
                </a>
              </Button>
            </div>

            <div className="flex flex-col gap-2">
              <Label htmlFor="gw-bulk-import-file">Choose a CSV file</Label>
              <Input
                id="gw-bulk-import-file"
                type="file"
                accept=".csv"
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
          </div>
        ) : null}

        {/* Step 2 — Validation Results */}
        {step === 'results' && validateResult ? (
          <div className="flex flex-col gap-4">
            <p className="text-sm">
              {validCount} gateways ready to import.
            </p>

            {errorCount > 0 ? (
              <div className="flex items-center gap-2">
                <Badge variant="destructive">
                  {errorCount} {errorCount === 1 ? 'error' : 'errors'}
                </Badge>
                <span className="text-sm text-muted-foreground">
                  These rows will be skipped.
                </span>
              </div>
            ) : null}

            {validateResult.errors.length > 0 ? (
              <div className="rounded-md border">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-16">Row</TableHead>
                      <TableHead>Gateway EUI</TableHead>
                      <TableHead>Error</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {validateResult.errors.map((err) => (
                      <TableRow key={`err-${err.row_number}`}>
                        <TableCell className="font-mono text-xs">{err.row_number}</TableCell>
                        <TableCell className="font-mono text-xs">{err.gateway_eui}</TableCell>
                        <TableCell className="text-sm text-muted-foreground">
                          {err.error_message ?? err.outcome}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            ) : null}
          </div>
        ) : null}

        {/* Importing in progress */}
        {step === 'importing' ? (
          <div className="flex flex-col gap-4">
            <p className="text-sm text-muted-foreground">Importing gateways…</p>
          </div>
        ) : null}

        {/* Done */}
        {step === 'done' && commitResult ? (
          <div className="flex flex-col gap-4">
            <div className="flex items-center gap-2">
              <AlertCircle className="h-4 w-4 text-success" aria-hidden />
              <p className="text-sm font-medium">
                {commitResult.created} gateways imported successfully.
              </p>
            </div>

            <Button
              variant="ghost"
              size="sm"
              className="self-start"
              onClick={() => setShowDetails((v) => !v)}
            >
              {showDetails ? (
                <ChevronDown className="mr-2 h-4 w-4" />
              ) : (
                <ChevronRight className="mr-2 h-4 w-4" />
              )}
              Show details
            </Button>

            {showDetails && commitResult.outcomes.length > 0 ? (
              <div className="rounded-md border">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-16">Row</TableHead>
                      <TableHead>Gateway EUI</TableHead>
                      <TableHead>Outcome</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {commitResult.outcomes.map((o) => (
                      <TableRow key={`outcome-${o.row_number}`}>
                        <TableCell className="font-mono text-xs">{o.row_number}</TableCell>
                        <TableCell className="font-mono text-xs">{o.gateway_eui}</TableCell>
                        <TableCell className="text-sm">{o.outcome}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            ) : null}
          </div>
        ) : null}
      </div>
    </ResponsiveDialog>
  )
}
