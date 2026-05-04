import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Eye, EyeOff } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Stepper } from '@/components/stepper'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { ApiError } from '@/lib/api'
import {
  addDevice,
  type AddDeviceRequest,
  type Device,
  preflight,
  type PreflightResult,
} from '@/lib/devices'
import { listMPs, type MeteringPoint } from '@/lib/metering-points'
import { listProfiles, type Profile } from '@/lib/profiles'
import { DevEUIParser } from './deveui-parser'

export interface AddDeviceDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** When set, step 3 pre-selects this MP and is read-only. */
  initialMPId?: string
  onCreated?: (device: Device) => void
}

const STEPS = [
  { label: 'Paste DevEUI' },
  { label: 'Pick profile' },
  { label: 'Bind to MP' },
  { label: 'Review' },
]

const STEP_HEADINGS = [
  'Paste the DevEUI from the meter sticker',
  'Choose a device profile',
  'Bind to a metering point',
  'Review and add',
]

/**
 * UI-SPEC §Add Device dialog (D-10 + D-11 + D-16) — 4-step Stepped dialog.
 *
 *   Step 1: DevEUIParser — paste DevEUI, pick MSB/LSB, emit lowercase 16-hex.
 *   Step 2: Profile <Select>, AppKey (masked), JoinEUI, device name.
 *   Step 3: Metering Point <Select> (or pre-bound when initialMPId set).
 *   Step 4: Read-only summary + ChirpStack preflight (POST /api/devices/preflight)
 *           + Add device button. preflight grpc/mqtt err → alert + disabled submit.
 *
 * Submit calls POST /api/devices (CHIRP-04 atomic). Failure → inline alert.
 * Success → toast "Device {name} added and bound to {MP name}." + onCreated.
 */
export function AddDeviceDialog({
  open,
  onOpenChange,
  initialMPId,
  onCreated,
}: AddDeviceDialogProps) {
  const qc = useQueryClient()
  const [step, setStep] = useState(0)

  // Step 1.
  const [devEUI, setDevEUI] = useState('')
  // Step 2.
  const [profileId, setProfileId] = useState('')
  const [appKey, setAppKey] = useState('')
  const [joinEUI, setJoinEUI] = useState('0000000000000000')
  const [showAppKey, setShowAppKey] = useState(false)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  // Step 3.
  const [mpId, setMpId] = useState(initialMPId ?? '')
  const [initialReading, setInitialReading] = useState('0')
  // Submit-side error.
  const [submitError, setSubmitError] = useState<string | null>(null)

  const profilesQuery = useQuery({
    queryKey: ['device-profiles'],
    queryFn: () => listProfiles(),
    enabled: open && step >= 1,
  })

  const mpsQuery = useQuery({
    queryKey: ['metering-points'],
    queryFn: () => listMPs(),
    enabled: open && step >= 2,
  })

  // Preflight only runs when step 4 is active.
  const preflightQuery = useQuery({
    queryKey: ['devices-preflight'],
    queryFn: () => preflight(),
    enabled: open && step === 3,
    retry: false,
  })

  const reset = () => {
    setStep(0)
    setDevEUI('')
    setProfileId('')
    setAppKey('')
    setJoinEUI('0000000000000000')
    setShowAppKey(false)
    setName('')
    setDescription('')
    setMpId(initialMPId ?? '')
    setInitialReading('0')
    setSubmitError(null)
  }

  const profiles: Profile[] = profilesQuery.data ?? []
  const mps: MeteringPoint[] = mpsQuery.data ?? []
  const selectedProfile = profiles.find((p) => p.id === profileId)
  const selectedMP = mps.find((m) => m.id === mpId)

  const preflightFailed =
    preflightQuery.data &&
    (preflightQuery.data.grpc === 'err' || preflightQuery.data.mqtt === 'err')

  const mutation = useMutation({
    mutationFn: (body: AddDeviceRequest) => addDevice(body),
    onSuccess: (created) => {
      qc.invalidateQueries({ queryKey: ['devices'] })
      qc.invalidateQueries({ queryKey: ['metering-points'] })
      const mpLabel = selectedMP ? ` and bound to ${selectedMP.name}` : ''
      toast.success(`Device ${created.name} added${mpLabel}.`)
      onCreated?.(created)
      onOpenChange(false)
      reset()
    },
    onError: (err: unknown) => {
      let msg = 'Could not add device.'
      if (err instanceof ApiError) {
        if (err.status === 502) {
          msg = `ChirpStack rejected the device: ${err.message}. Nothing was saved.`
        } else if (err.status === 409) {
          msg = `This DevEUI is already added.`
        } else {
          msg = err.message
        }
      }
      setSubmitError(msg)
      toast.error('Could not add device')
    },
  })

  const onSubmit = () => {
    setSubmitError(null)
    if (!devEUI || !profileId || !appKey || !name.trim()) {
      setSubmitError('Missing required field. Step back to fix.')
      return
    }
    const body: AddDeviceRequest = {
      dev_eui: devEUI,
      name: name.trim(),
      device_profile_id: profileId,
      app_key: appKey.toLowerCase(),
      join_eui: joinEUI.toLowerCase(),
    }
    if (description.trim()) body.description = description.trim()
    if (mpId) {
      body.metering_point_id = mpId
      body.initial_reading = initialReading || '0'
    }
    mutation.mutate(body)
  }

  // Per-step "next" gate.
  const step1Ready = devEUI.length === 16
  const step2Ready =
    Boolean(profileId) && /^[0-9a-fA-F]{32}$/.test(appKey) && name.trim().length > 0
  const step3Ready = !mpId || /^-?\d+(\.\d+)?$/.test(initialReading)

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(v) => {
        onOpenChange(v)
        if (!v) reset()
      }}
      title="Add device"
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
              disabled={
                (step === 0 && !step1Ready) ||
                (step === 1 && !step2Ready) ||
                (step === 2 && !step3Ready)
              }
            >
              Next
            </Button>
          ) : (
            <Button
              type="button"
              onClick={onSubmit}
              disabled={mutation.isPending || Boolean(preflightFailed)}
            >
              {mutation.isPending ? 'Adding device…' : 'Add device'}
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

        {/* Step 1 — Paste DevEUI */}
        {step === 0 ? (
          <div className="flex flex-col gap-4">
            <h2 className="text-lg font-semibold leading-7">{STEP_HEADINGS[0]}</h2>
            <p className="text-sm text-muted-foreground">
              We'll show both endianness interpretations so you can pick the right one.
            </p>
            <DevEUIParser
              onPick={(picked) => {
                setDevEUI(picked)
                setStep(1)
              }}
            />
            {devEUI ? (
              <p className="text-sm text-muted-foreground">
                Picked: <span className="font-mono">{devEUI}</span>
              </p>
            ) : null}
          </div>
        ) : null}

        {/* Step 2 — Pick profile */}
        {step === 1 ? (
          <div className="flex flex-col gap-4">
            <h2 className="text-lg font-semibold leading-7">{STEP_HEADINGS[1]}</h2>
            <p className="text-sm text-muted-foreground">
              The profile sets the codec, expected interval, and which canonical fields
              are populated.
            </p>

            <div className="flex flex-col gap-2">
              <Label htmlFor="device-profile">Device profile</Label>
              <Select value={profileId} onValueChange={setProfileId}>
                <SelectTrigger id="device-profile" className="w-full" aria-label="Device profile">
                  <SelectValue placeholder="Select a profile" />
                </SelectTrigger>
                <SelectContent>
                  {profiles
                    .filter((p) => !p.archived_at)
                    .map((p) => (
                      <SelectItem key={p.id} value={p.id}>
                        {p.name}
                      </SelectItem>
                    ))}
                </SelectContent>
              </Select>
            </div>

            <div className="flex flex-col gap-2">
              <Label htmlFor="device-name">Device name</Label>
              <Input
                id="device-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. Building A apt 305 water meter"
              />
            </div>

            <div className="flex flex-col gap-2">
              <Label htmlFor="app-key">AppKey</Label>
              <div className="flex items-center gap-2">
                <Input
                  id="app-key"
                  type={showAppKey ? 'text' : 'password'}
                  className="font-mono"
                  value={appKey}
                  onChange={(e) => setAppKey(e.target.value)}
                  placeholder="32 hex chars"
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => setShowAppKey((v) => !v)}
                  aria-label={showAppKey ? 'Hide AppKey' : 'Show AppKey'}
                >
                  {showAppKey ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </Button>
              </div>
            </div>

            <div className="flex flex-col gap-2">
              <Label htmlFor="join-eui">JoinEUI</Label>
              <Input
                id="join-eui"
                className="font-mono"
                value={joinEUI}
                onChange={(e) => setJoinEUI(e.target.value)}
                placeholder="16 hex chars"
              />
            </div>

            <p className="text-xs text-muted-foreground">
              Activation: OTAA — ABP support arrives in Phase 3.
            </p>

            <div className="flex flex-col gap-2">
              <Label htmlFor="device-description">Description (optional)</Label>
              <Textarea
                id="device-description"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                rows={2}
              />
            </div>
          </div>
        ) : null}

        {/* Step 3 — Bind to MP */}
        {step === 2 ? (
          <div className="flex flex-col gap-4">
            <h2 className="text-lg font-semibold leading-7">{STEP_HEADINGS[2]}</h2>
            <p className="text-sm text-muted-foreground">
              Telemetry from this device will be keyed on the metering point you choose.
              You can swap the device later without losing history.
            </p>

            <div className="flex flex-col gap-2">
              <Label htmlFor="mp-select">Metering point</Label>
              <Select value={mpId} onValueChange={setMpId}>
                <SelectTrigger id="mp-select" className="w-full" aria-label="Metering point">
                  <SelectValue placeholder="Select a metering point" />
                </SelectTrigger>
                <SelectContent>
                  {mps
                    .filter((m) => !m.archived_at)
                    .map((m) => (
                      <SelectItem key={m.id} value={m.id}>
                        {m.name}
                      </SelectItem>
                    ))}
                </SelectContent>
              </Select>
            </div>

            {mpId ? (
              <div className="flex flex-col gap-2">
                <Label htmlFor="initial-reading">Initial reading (offset basis)</Label>
                <Input
                  id="initial-reading"
                  className="font-mono"
                  value={initialReading}
                  onChange={(e) => setInitialReading(e.target.value)}
                  placeholder="0"
                />
                <p className="text-xs text-muted-foreground">
                  The new meter's starting display value. Default 0.
                </p>
              </div>
            ) : null}
          </div>
        ) : null}

        {/* Step 4 — Review */}
        {step === 3 ? (
          <div className="flex flex-col gap-4">
            <h2 className="text-lg font-semibold leading-7">{STEP_HEADINGS[3]}</h2>
            <p className="text-sm text-muted-foreground">
              We'll create everything in ChirpStack and Shifter in one step. If anything
              fails, nothing is saved.
            </p>

            <Card>
              <CardContent className="flex flex-col gap-2 py-4 text-sm">
                <ReviewRow label="DevEUI" value={<span className="font-mono">{devEUI}</span>} />
                <ReviewRow label="Profile" value={selectedProfile?.name ?? '—'} />
                <ReviewRow
                  label="AppKey"
                  value={
                    <span className="font-mono">
                      {appKey ? `••••••••••••••••••••••••••••${appKey.slice(-4)}` : '—'}
                    </span>
                  }
                />
                <ReviewRow label="JoinEUI" value={<span className="font-mono">{joinEUI}</span>} />
                <ReviewRow label="Device name" value={name || '—'} />
                <ReviewRow label="Metering point" value={selectedMP?.name ?? '— (unbound)'} />
                {selectedMP ? (
                  <ReviewRow
                    label="Initial reading"
                    value={<span className="font-mono">{initialReading}</span>}
                  />
                ) : null}
              </CardContent>
            </Card>

            <Card>
              <CardContent className="flex flex-col gap-2 py-4">
                <p className="text-sm font-semibold">ChirpStack pre-flight</p>
                <PreflightStatus result={preflightQuery.data} />
              </CardContent>
            </Card>

            {preflightFailed ? (
              <Alert variant="destructive">
                <AlertDescription>
                  ChirpStack is unreachable. Open Settings → Test connection.
                </AlertDescription>
              </Alert>
            ) : null}
          </div>
        ) : null}
      </div>
    </ResponsiveDialog>
  )
}

function ReviewRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between">
      <span className="text-muted-foreground">{label}</span>
      <span>{value}</span>
    </div>
  )
}

function PreflightStatus({ result }: { result: PreflightResult | undefined }) {
  if (!result) {
    return <p className="text-sm text-muted-foreground">Running pre-flight…</p>
  }
  return (
    <div className="flex flex-col gap-1 text-sm">
      <div className="flex items-center justify-between">
        <span>gRPC</span>
        <span
          className={
            result.grpc === 'ok'
              ? 'text-success'
              : result.grpc === 'err'
                ? 'text-destructive'
                : 'text-muted-foreground'
          }
        >
          {result.grpc === 'ok'
            ? 'Reachable'
            : result.grpc === 'err'
              ? 'Unreachable'
              : 'Skipped'}
        </span>
      </div>
      <div className="flex items-center justify-between">
        <span>MQTT</span>
        <span
          className={
            result.mqtt === 'ok'
              ? 'text-success'
              : result.mqtt === 'err'
                ? 'text-destructive'
                : 'text-muted-foreground'
          }
        >
          {result.mqtt === 'ok'
            ? 'Reachable'
            : result.mqtt === 'err'
              ? 'Unreachable'
              : 'Skipped'}
        </span>
      </div>
    </div>
  )
}
