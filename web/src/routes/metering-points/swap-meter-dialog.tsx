import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Stepper } from '@/components/stepper'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { ApiError } from '@/lib/api'
import { listDevices } from '@/lib/devices'
import { getMPDetail } from '@/lib/metering-points'
import { commitSwap, type SwapRequest } from '@/lib/swap'

export interface SwapMeterDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  meteringPointId: string
  meteringPointName: string
  onSwapped?: (bindingId: string) => void
}

const STEPS = [
  { label: 'Capture R' },
  { label: 'Select new device' },
  { label: 'Review math' },
]

const STEP_HEADINGS = [
  'Capture the outgoing reading',
  'Select the new device',
  'Review the math and confirm',
]

/**
 * UI-SPEC §Swap Meter dialog (D-12 + D-13 + D-14) — 3-step Stepped dialog.
 *
 *   Step 1: Auto-fill outgoing R from latest_measurement; mandatory
 *           "I verified this matches the physical meter" checkbox; override link.
 *   Step 2: Device picker (unbound by default); new meter initial N input.
 *   Step 3: Math read-back (R = …, N = …, Proposed offset = R − N = …);
 *           Confirm swap → POST /api/metering-points/{id}/swap.
 *
 * 409 concurrent_swap → inline alert "Concurrent uplink in progress — try
 * again in a few seconds." per UI-SPEC error patterns.
 */
export function SwapMeterDialog({
  open,
  onOpenChange,
  meteringPointId,
  meteringPointName,
  onSwapped,
}: SwapMeterDialogProps) {
  const qc = useQueryClient()
  const [step, setStep] = useState(0)

  const detailQuery = useQuery({
    queryKey: ['mp-detail', meteringPointId],
    queryFn: () => getMPDetail(meteringPointId),
    enabled: open,
  })

  const devicesQuery = useQuery({
    queryKey: ['devices'],
    queryFn: () => listDevices(),
    enabled: open && step >= 1,
  })

  // Form state.
  const [outgoingReading, setOutgoingReading] = useState('')
  const [overrideOpen, setOverrideOpen] = useState(false)
  const [verified, setVerified] = useState(false)
  const [incomingDeviceId, setIncomingDeviceId] = useState('')
  const [initialN, setInitialN] = useState('0')
  const [operatorOverride, setOperatorOverride] = useState('')
  const [operatorNotes, setOperatorNotes] = useState('')
  const [submitError, setSubmitError] = useState<string | null>(null)

  // Auto-fill R from latest_measurement on first load.
  const detail = detailQuery.data
  const cumulative = detail?.latest_measurement?.cumulative_value
  useEffect(() => {
    if (cumulative && !outgoingReading) {
      setOutgoingReading(cumulative)
    }
    // We intentionally don't depend on outgoingReading — once the user types
    // a manual override, we don't want to re-overwrite from the query.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cumulative])

  const reset = () => {
    setStep(0)
    setOutgoingReading('')
    setOverrideOpen(false)
    setVerified(false)
    setIncomingDeviceId('')
    setInitialN('0')
    setOperatorOverride('')
    setOperatorNotes('')
    setSubmitError(null)
  }

  const lateUplink = (() => {
    if (!detail?.latest_measurement?.time) return false
    const t = new Date(detail.latest_measurement.time).getTime()
    return Date.now() - t > 60 * 60 * 1000
  })()

  const minutesAgo = (() => {
    if (!detail?.latest_measurement?.time) return null
    const t = new Date(detail.latest_measurement.time).getTime()
    return Math.max(0, Math.round((Date.now() - t) / 60000))
  })()

  const unboundDevices = (devicesQuery.data ?? []).filter((d) => !d.decommissioned_at)

  const proposedOffset = (() => {
    const r = Number.parseFloat(outgoingReading || '0')
    const n = Number.parseFloat(initialN || '0')
    if (Number.isNaN(r) || Number.isNaN(n)) return ''
    return String(r - n)
  })()

  const mutation = useMutation({
    mutationFn: (body: SwapRequest) => commitSwap(meteringPointId, body),
    onSuccess: (resp) => {
      qc.invalidateQueries({ queryKey: ['mp-detail', meteringPointId] })
      qc.invalidateQueries({ queryKey: ['metering-points'] })
      toast.success(`Meter swapped on ${meteringPointName}.`)
      onSwapped?.(resp.binding_id)
      onOpenChange(false)
      reset()
    },
    onError: (err: unknown) => {
      if (err instanceof ApiError && err.status === 409) {
        setSubmitError(
          'Concurrent uplink in progress — try again in a few seconds.',
        )
      } else if (err instanceof ApiError) {
        setSubmitError(err.message)
      } else {
        setSubmitError('Could not commit swap.')
      }
      toast.error('Could not commit swap')
    },
  })

  const onSubmit = () => {
    setSubmitError(null)
    const body: SwapRequest = {
      incoming_device_id: incomingDeviceId,
      outgoing_reading_r: outgoingReading || '0',
      incoming_initial_n: initialN || '0',
    }
    if (operatorOverride.trim()) body.operator_override = operatorOverride.trim()
    if (operatorNotes.trim()) body.operator_notes = operatorNotes.trim()
    mutation.mutate(body)
  }

  const step1Ready = verified && Boolean(outgoingReading)
  const step2Ready = Boolean(incomingDeviceId)

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(v) => {
        onOpenChange(v)
        if (!v) reset()
      }}
      title={`Swap meter on ${meteringPointName}`}
      size="lg"
      footer={
        <div className="flex w-full items-center justify-between">
          <Button
            type="button"
            variant="ghost"
            onClick={() => setStep((s) => Math.max(0, s - 1))}
            disabled={step === 0 || mutation.isPending}
          >
            Back
          </Button>
          <span className="text-sm text-muted-foreground">
            Step {step + 1} of {STEPS.length}
          </span>
          {step < STEPS.length - 1 ? (
            <Button
              type="button"
              onClick={() => setStep((s) => s + 1)}
              disabled={(step === 0 && !step1Ready) || (step === 1 && !step2Ready)}
            >
              Next
            </Button>
          ) : (
            <Button type="button" onClick={onSubmit} disabled={mutation.isPending}>
              {mutation.isPending ? 'Swapping meter…' : 'Confirm swap'}
            </Button>
          )}
        </div>
      }
    >
      <div className="flex flex-col gap-6">
        <Stepper steps={STEPS} currentIndex={step} />

        {submitError ? (
          <Alert variant="destructive">
            <AlertDescription>{submitError}</AlertDescription>
          </Alert>
        ) : null}

        {/* Step 1 — Capture outgoing reading */}
        {step === 0 ? (
          <div className="flex flex-col gap-4">
            <h2 className="text-lg font-semibold leading-7">{STEP_HEADINGS[0]}</h2>
            <p className="text-sm text-muted-foreground">
              We'll use this reading to keep the cumulative chart continuous after the swap.
            </p>

            {detail?.latest_measurement ? (
              lateUplink ? (
                <Alert variant="default">
                  <AlertDescription>
                    Last uplink was {minutesAgo} min ago. Verify the meter still reflects this
                    reading.
                  </AlertDescription>
                </Alert>
              ) : (
                <Card className="bg-secondary">
                  <CardContent className="py-4 text-sm">
                    <p>
                      Outgoing reading:{' '}
                      <span className="font-mono font-semibold">{outgoingReading || cumulative}</span>{' '}
                      — uplink {minutesAgo ?? '?'} min ago
                    </p>
                  </CardContent>
                </Card>
              )
            ) : (
              <Alert variant="default">
                <AlertDescription>
                  No uplink yet — enter the outgoing reading manually below.
                </AlertDescription>
              </Alert>
            )}

            <div className="flex items-start gap-2">
              <Checkbox
                id="verify-reading"
                checked={verified}
                onCheckedChange={(v) => setVerified(Boolean(v))}
              />
              <Label htmlFor="verify-reading" className="text-sm cursor-pointer leading-5">
                I verified this matches the physical meter.
              </Label>
            </div>

            <div>
              <Button
                type="button"
                variant="link"
                className="h-auto p-0"
                onClick={() => setOverrideOpen((v) => !v)}
              >
                Reading doesn't match? Enter manually
              </Button>
            </div>

            {overrideOpen ? (
              <div className="flex flex-col gap-2">
                <Label htmlFor="manual-reading">Manual outgoing reading</Label>
                <Input
                  id="manual-reading"
                  className="font-mono"
                  value={outgoingReading}
                  onChange={(e) => setOutgoingReading(e.target.value)}
                />
                <p className="text-xs text-muted-foreground">
                  Using a manual override adds a warning chip on the audit-log row.
                </p>
              </div>
            ) : null}
          </div>
        ) : null}

        {/* Step 2 — Select new device */}
        {step === 1 ? (
          <div className="flex flex-col gap-4">
            <h2 className="text-lg font-semibold leading-7">{STEP_HEADINGS[1]}</h2>
            <p className="text-sm text-muted-foreground">
              Pick a device that's already added but not yet bound, or add a new one.
            </p>

            <div className="flex flex-col gap-2">
              <Label htmlFor="new-device">New device</Label>
              <Select value={incomingDeviceId} onValueChange={setIncomingDeviceId}>
                <SelectTrigger id="new-device" className="w-full" aria-label="New device">
                  <SelectValue placeholder="Select a device" />
                </SelectTrigger>
                <SelectContent>
                  {unboundDevices.map((d) => (
                    <SelectItem key={d.id} value={d.id}>
                      {d.name} — {d.dev_eui}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="flex flex-col gap-2">
              <Label htmlFor="initial-n">New meter initial reading (N)</Label>
              <Input
                id="initial-n"
                className="font-mono"
                value={initialN}
                onChange={(e) => setInitialN(e.target.value)}
                placeholder="0"
              />
              <p className="text-xs text-muted-foreground">
                If the new meter shows a non-zero starting value, enter it here.
              </p>
            </div>
          </div>
        ) : null}

        {/* Step 3 — Review math */}
        {step === 2 ? (
          <div className="flex flex-col gap-4">
            <h2 className="text-lg font-semibold leading-7">{STEP_HEADINGS[2]}</h2>
            <p className="text-sm text-muted-foreground">
              Operator-readable math so you can sanity-check before commit.
            </p>

            <Card>
              <CardContent className="flex flex-col gap-3 py-4">
                <div className="flex items-center justify-between">
                  <span className="text-sm font-semibold">Outgoing reading R</span>
                  <span className="text-lg font-mono">{outgoingReading || '—'}</span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-sm font-semibold">New meter initial N</span>
                  <span className="text-lg font-mono">{initialN || '0'}</span>
                </div>
                <div className="flex items-center justify-between border-t pt-3">
                  <span className="text-sm font-semibold">Proposed offset = R − N</span>
                  <span className="text-lg font-mono">{proposedOffset || '—'}</span>
                </div>
              </CardContent>
            </Card>

            <p className="text-sm">
              After swap, the cumulative chart will continue from{' '}
              <span className="font-mono">{outgoingReading || '—'}</span> — no spike.
            </p>

            <div className="flex flex-col gap-2">
              <Label htmlFor="operator-override">Override proposed offset (optional)</Label>
              <Input
                id="operator-override"
                className="font-mono"
                value={operatorOverride}
                onChange={(e) => setOperatorOverride(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                Use only if you have a specific reason — the audit log will record the
                override.
              </p>
            </div>

            <div className="flex flex-col gap-2">
              <Label htmlFor="operator-notes">Notes (optional)</Label>
              <Input
                id="operator-notes"
                value={operatorNotes}
                onChange={(e) => setOperatorNotes(e.target.value)}
              />
            </div>
          </div>
        ) : null}
      </div>
    </ResponsiveDialog>
  )
}
