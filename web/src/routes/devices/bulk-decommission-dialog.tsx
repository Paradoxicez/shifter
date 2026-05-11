import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { toast } from 'sonner'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ApiError } from '@/lib/api'
import {
  bulkDecommissionDevices,
  type BulkDecommissionResponse,
} from '@/lib/devices'

/**
 * Plan 03-09 §UI-SPEC §Bulk-decommission destructive dialog (D-17).
 *
 * Mirrors the single decommission AlertDialog (Plan 03-08
 * decommission-gateway-dialog.tsx) — button-only confirm, NO type-to-confirm
 * per D-10 (mirrors D-15 single-device pattern).
 *
 * Partial-success outcome → toast.warning with summary string. All-fail →
 * toast.error. All-success → toast.success.
 *
 * Wire format: POST /api/devices/bulk-decommission with body
 * `{ device_ids: string[], reason: string }` (per Plan 03-06 backend handler).
 *
 * Triggered from the BulkActionBar Decommission button after ≥1 row selected.
 */
export interface BulkDecommissionDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  deviceIds: string[]
  onCompleted?: (resp: BulkDecommissionResponse) => void
}

export function BulkDecommissionDialog({
  open,
  onOpenChange,
  deviceIds,
  onCompleted,
}: BulkDecommissionDialogProps) {
  const qc = useQueryClient()
  const [reason, setReason] = useState('operator bulk decommission')
  const [submitError, setSubmitError] = useState<string | null>(null)
  const n = deviceIds.length

  const mutation = useMutation({
    mutationFn: () => bulkDecommissionDevices(deviceIds, reason),
    onSuccess: (resp) => {
      qc.invalidateQueries({ queryKey: ['devices'] })
      qc.invalidateQueries({ queryKey: ['metering-points'] })
      if (resp.failed === 0) {
        toast.success(
          `Decommissioned ${resp.succeeded} ${resp.succeeded === 1 ? 'device' : 'devices'}.`,
        )
      } else if (resp.succeeded === 0) {
        toast.error(
          `Could not decommission. ${resp.failed} ${resp.failed === 1 ? 'device' : 'devices'} failed.`,
        )
      } else {
        toast.warning(
          `Decommissioned ${resp.succeeded} of ${resp.succeeded + resp.failed}. ` +
            `${resp.failed} failed — see audit log.`,
        )
      }
      onCompleted?.(resp)
      onOpenChange(false)
      setReason('operator bulk decommission')
      setSubmitError(null)
    },
    onError: (err: unknown) => {
      const msg =
        err instanceof ApiError
          ? err.message
          : 'Could not run bulk decommission. Nothing changed.'
      setSubmitError(msg)
      toast.error('Bulk decommission failed')
    },
  })

  const title =
    n === 1 ? 'Decommission this device?' : `Decommission ${n} devices?`
  const submitLabel = mutation.isPending
    ? 'Decommissioning…'
    : n === 1
      ? 'Decommission device'
      : `Decommission ${n} devices`

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>
            We&rsquo;ll close the active binding for each device and mark them
            decommissioned. Their metering points stay active. Historical
            telemetry is preserved. You can re-add a device to any metering
            point afterward.
          </AlertDialogDescription>
        </AlertDialogHeader>

        {submitError ? (
          <Alert variant="destructive">
            <AlertDescription>{submitError}</AlertDescription>
          </Alert>
        ) : null}

        <div className="flex flex-col gap-2">
          <Label htmlFor="bulk-decommission-reason">Reason</Label>
          <Input
            id="bulk-decommission-reason"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder="operator bulk decommission"
          />
        </div>

        <AlertDialogFooter>
          <AlertDialogCancel disabled={mutation.isPending}>Cancel</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={mutation.isPending || n === 0}
            onClick={(e) => {
              // Prevent Radix from auto-closing before mutation resolves.
              e.preventDefault()
              mutation.mutate()
            }}
          >
            {submitLabel}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
