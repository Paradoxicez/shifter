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
import { archiveGateway, type Gateway } from '@/lib/gateways'

export interface DecommissionGatewayDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  gateway: Gateway
  onDecommissioned?: (gw: Gateway) => void
}

/**
 * UI-SPEC §Decommission Gateway destructive dialog (D-30 + D-31):
 *   - Title: "Decommission this gateway?"
 *   - Body: archive copy + warning banner about devices routing via others.
 *   - Footer: Cancel (outline) + Decommission gateway (destructive).
 *   - NO type-to-confirm (D-10 button-only — soft-delete is reversible).
 *
 * On confirm: POST /api/gateways/:id/archive { reason } → invalidate
 * ['gateways'] cache → toast → close.
 */
export function DecommissionGatewayDialog({
  open,
  onOpenChange,
  gateway,
  onDecommissioned,
}: DecommissionGatewayDialogProps) {
  const qc = useQueryClient()
  const [reason, setReason] = useState('operator decommission')
  const [submitError, setSubmitError] = useState<string | null>(null)

  const mutation = useMutation({
    mutationFn: () => archiveGateway(gateway.id, reason.trim() || 'operator decommission'),
    onSuccess: (archived) => {
      qc.invalidateQueries({ queryKey: ['gateways'] })
      qc.invalidateQueries({ queryKey: ['gateway', gateway.id] })
      toast.success(`Gateway ${gateway.name} decommissioned.`)
      onDecommissioned?.(archived)
      onOpenChange(false)
      setReason('operator decommission')
      setSubmitError(null)
    },
    onError: (err: unknown) => {
      const msg =
        err instanceof ApiError
          ? `ChirpStack rejected the delete: ${err.message}. Nothing changed.`
          : 'Could not decommission gateway.'
      setSubmitError(msg)
      toast.error('Could not decommission gateway')
    },
  })

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Decommission this gateway?</AlertDialogTitle>
          <AlertDialogDescription>
            We&rsquo;ll archive {gateway.name} and remove it from ChirpStack. Historical
            telemetry from devices behind it stays preserved. You can restore later.
          </AlertDialogDescription>
        </AlertDialogHeader>

        {submitError ? (
          <Alert variant="destructive">
            <AlertDescription>{submitError}</AlertDescription>
          </Alert>
        ) : null}

        {/* D-31 warning banner: devices currently bound to this gateway will route
            via other gateways. Rendered unconditionally for v1 — backend doesn't
            yet surface a per-gateway uplink count. */}
        <Alert>
          <AlertDescription>
            Devices currently bound to this gateway will route via other gateways.
          </AlertDescription>
        </Alert>

        <div className="flex flex-col gap-2">
          <Label htmlFor="decommission-reason">Reason</Label>
          <Input
            id="decommission-reason"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder="operator decommission"
          />
        </div>

        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={mutation.isPending}
            onClick={(e) => {
              // Prevent the Radix Action from closing the dialog before the
              // mutation finishes; we close on success ourselves.
              e.preventDefault()
              mutation.mutate()
            }}
          >
            {mutation.isPending ? 'Decommissioning gateway…' : 'Decommission gateway'}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
