import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  CheckCircle2,
  Copy,
  Eye,
  EyeOff,
  HelpCircle,
  ShieldCheck,
  ShieldOff,
} from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Stepper } from '@/components/stepper'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { ApiError } from '@/lib/api'
import {
  addDevice,
  type AddDeviceRequest,
  type AddDeviceResponse,
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
  /** When set, step 2 (Bind) pre-selects this MP and is read-only. */
  initialMPId?: string
  onCreated?: (device: Device) => void
}

type ActivationMode = 'OTAA' | 'ABP'

const STEPS = [
  { label: 'Identity' },
  { label: 'Bind' },
  { label: 'Activation' },
  { label: 'Keys' },
  { label: 'Review' },
]

const STEP_HEADINGS = [
  'Paste the DevEUI from the meter sticker',
  'Bind to a metering point',
  'Choose activation mode',
  'Enter activation keys',
  'Review and add',
]

const HEX_16 = /^[0-9a-f]{16}$/
const HEX_32 = /^[0-9a-f]{32}$/
const HEX_8 = /^[0-9a-f]{8}$/

/**
 * Plan 03-07 / UI-SPEC §Add Device dialog (5 steps — supersedes Phase 2 4-step).
 *
 *   Step 1 (Identity): DevEUIParser + name + description.
 *   Step 2 (Bind)    : Metering point combobox (optional binding).
 *   Step 3 (Activation): OTAA (recommended) / ABP (not recommended) radio
 *                        + Device Profile select. D-19.
 *   Step 4 (Keys)    : Mode-specific fields.
 *                        OTAA → AppKey (masked) + Join EUI (AppEUI for v1.0).
 *                        ABP  → DevAddr + NwkSKey + AppSKey + FCntUp + FCntDown.
 *   Step 5 (Review)  : Read-only summary + preflight + Add device.
 *
 * After successful submit: the dialog body re-renders as the **D-21 success
 * state** — a Card listing the keys + a "Copy keys" button. Closing fires
 * onOpenChange(false) and clears local state. The mutation is configured
 * with `gcTime: 0` so TanStack Query never retains the response (T-3-72).
 *
 * UX-03: no "tenant" / "application" wording surfaces — operator-facing copy
 * uses Shifter vocabulary only.
 */
export function AddDeviceDialog({
  open,
  onOpenChange,
  initialMPId,
  onCreated,
}: AddDeviceDialogProps) {
  const qc = useQueryClient()
  const [step, setStep] = useState(0)

  // Step 1 — Identity.
  const [devEUI, setDevEUI] = useState('')
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')

  // Step 2 — Bind.
  const [mpId, setMpId] = useState(initialMPId ?? '')
  const [initialReading, setInitialReading] = useState('0')

  // Step 3 — Activation.
  const [activationMode, setActivationMode] = useState<ActivationMode>('OTAA')
  const [profileId, setProfileId] = useState('')

  // Step 4 — Mode-specific keys.
  // OTAA:
  const [appKey, setAppKey] = useState('')
  const [joinEUI, setJoinEUI] = useState('0000000000000000')
  const [showAppKey, setShowAppKey] = useState(false)
  // ABP:
  const [devAddr, setDevAddr] = useState('')
  const [nwkSKey, setNwkSKey] = useState('')
  const [appSKey, setAppSKey] = useState('')
  const [showNwkSKey, setShowNwkSKey] = useState(false)
  const [showAppSKey, setShowAppSKey] = useState(false)
  const [fCntUp, setFCntUp] = useState('0')
  const [fCntDown, setFCntDown] = useState('0')

  // Submit-side error + success state (D-21).
  const [submitError, setSubmitError] = useState<string | null>(null)
  const [successKeys, setSuccessKeys] = useState<AddDeviceResponse | null>(null)

  const profilesQuery = useQuery({
    queryKey: ['device-profiles'],
    queryFn: () => listProfiles(),
    enabled: open && step >= 2,
  })

  const mpsQuery = useQuery({
    queryKey: ['metering-points'],
    queryFn: () => listMPs(),
    enabled: open && step >= 1,
  })

  // Preflight only runs when the review step is active.
  const preflightQuery = useQuery({
    queryKey: ['devices-preflight'],
    queryFn: () => preflight(),
    enabled: open && step === 4 && !successKeys,
    retry: false,
  })

  const reset = () => {
    setStep(0)
    setDevEUI('')
    setName('')
    setDescription('')
    setMpId(initialMPId ?? '')
    setInitialReading('0')
    setActivationMode('OTAA')
    setProfileId('')
    setAppKey('')
    setJoinEUI('0000000000000000')
    setShowAppKey(false)
    setDevAddr('')
    setNwkSKey('')
    setAppSKey('')
    setShowNwkSKey(false)
    setShowAppSKey(false)
    setFCntUp('0')
    setFCntDown('0')
    setSubmitError(null)
    setSuccessKeys(null)
  }

  const profiles: Profile[] = profilesQuery.data ?? []
  const mps: MeteringPoint[] = mpsQuery.data ?? []
  const selectedProfile = profiles.find((p) => p.id === profileId)
  const selectedMP = mps.find((m) => m.id === mpId)

  const preflightFailed =
    preflightQuery.data &&
    (preflightQuery.data.grpc === 'err' || preflightQuery.data.mqtt === 'err')

  // D-21 + T-3-72: gcTime:0 so the mutation result is never retained in cache.
  const mutation = useMutation({
    gcTime: 0,
    mutationFn: (body: AddDeviceRequest) => addDevice(body),
    onSuccess: (created) => {
      qc.invalidateQueries({ queryKey: ['devices'] })
      qc.invalidateQueries({ queryKey: ['metering-points'] })
      setSuccessKeys(created)
      const mpLabel = selectedMP ? ` and bound to ${selectedMP.name}` : ''
      toast.success(`Device ${created.name} added${mpLabel}.`)
      onCreated?.(created as Device)
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

  const buildBody = (): AddDeviceRequest | null => {
    const base = {
      dev_eui: devEUI,
      name: name.trim(),
      device_profile_id: profileId,
      ...(description.trim() ? { description: description.trim() } : {}),
      ...(mpId ? { metering_point_id: mpId, initial_reading: initialReading || '0' } : {}),
    }
    if (activationMode === 'OTAA') {
      if (!appKey) return null
      return {
        ...base,
        activation_mode: 'OTAA',
        app_key: appKey.toLowerCase(),
        join_eui: (joinEUI || '0000000000000000').toLowerCase(),
      }
    }
    if (!devAddr || !nwkSKey || !appSKey) return null
    return {
      ...base,
      activation_mode: 'ABP',
      dev_addr: devAddr.toLowerCase(),
      nwk_s_key: nwkSKey.toLowerCase(),
      app_s_key: appSKey.toLowerCase(),
      fcnt_up: Number(fCntUp) || 0,
      fcnt_down: Number(fCntDown) || 0,
    }
  }

  const onSubmit = () => {
    setSubmitError(null)
    const body = buildBody()
    if (!body || !devEUI || !profileId || !name.trim()) {
      setSubmitError('Missing required field. Step back to fix.')
      return
    }
    mutation.mutate(body)
  }

  // Per-step "next" gate.
  const step1Ready = devEUI.length === 16 && name.trim().length > 0
  const step2Ready = !mpId || /^-?\d+(\.\d+)?$/.test(initialReading)
  const step3Ready = Boolean(profileId) && (activationMode === 'OTAA' || activationMode === 'ABP')
  const step4Ready =
    activationMode === 'OTAA'
      ? HEX_32.test(appKey.toLowerCase()) && HEX_16.test(joinEUI.toLowerCase())
      : HEX_8.test(devAddr.toLowerCase()) &&
        HEX_32.test(nwkSKey.toLowerCase()) &&
        HEX_32.test(appSKey.toLowerCase())

  const handleCloseRequested = (v: boolean) => {
    onOpenChange(v)
    if (!v) reset()
  }

  // ---------- D-21 success state -------------------------------------------
  if (successKeys) {
    return (
      <ResponsiveDialog
        open={open}
        onOpenChange={handleCloseRequested}
        title="Device added"
        size="lg"
        footer={
          <div className="flex w-full items-center justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                // "Add another device" — keep dialog open, clear state.
                reset()
              }}
            >
              Add another device
            </Button>
            <Button type="button" onClick={() => handleCloseRequested(false)}>
              Done
            </Button>
          </div>
        }
      >
        <div className="flex flex-col gap-6">
          <div className="flex flex-col items-center gap-2 text-center">
            <CheckCircle2 className="h-10 w-10 text-success" aria-hidden="true" />
            <h2 className="text-2xl font-semibold">Device {successKeys.name} added</h2>
            <p className="text-sm text-muted-foreground">
              Save these keys somewhere safe. After you click Done, you can re-fetch
              them from the device detail page (admin only).
            </p>
          </div>

          <KeysPanel response={successKeys} />
        </div>
      </ResponsiveDialog>
    )
  }

  // ---------- Stepper body --------------------------------------------------
  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={handleCloseRequested}
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
                (step === 2 && !step3Ready) ||
                (step === 3 && !step4Ready)
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

        {/* Step 1 — Identity */}
        {step === 0 ? (
          <div className="flex flex-col gap-4">
            <h2 className="text-lg font-semibold leading-7">{STEP_HEADINGS[0]}</h2>
            <p className="text-sm text-muted-foreground">
              We'll show both endianness interpretations so you can pick the right one.
            </p>
            <DevEUIParser
              onPick={(picked) => {
                setDevEUI(picked)
              }}
            />
            {devEUI ? (
              <p className="text-sm text-muted-foreground">
                Picked: <span className="font-mono">{devEUI}</span>
              </p>
            ) : null}

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

        {/* Step 2 — Bind */}
        {step === 1 ? (
          <div className="flex flex-col gap-4">
            <h2 className="text-lg font-semibold leading-7">{STEP_HEADINGS[1]}</h2>
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

        {/* Step 3 — Activation mode + Profile */}
        {step === 2 ? (
          <div className="flex flex-col gap-4">
            <h2 className="text-lg font-semibold leading-7">{STEP_HEADINGS[2]}</h2>

            <RadioGroup
              value={activationMode}
              onValueChange={(v) => setActivationMode(v as ActivationMode)}
              aria-label="Activation mode"
            >
              <Card className="px-4 py-3">
                <Label htmlFor="mode-otaa" className="flex items-start gap-3 cursor-pointer">
                  <RadioGroupItem id="mode-otaa" value="OTAA" className="mt-1" />
                  <ShieldCheck className="h-5 w-5 text-success mt-0.5" aria-hidden="true" />
                  <span className="flex flex-col gap-1">
                    <span className="text-sm font-semibold">OTAA (recommended)</span>
                    <span className="text-xs text-muted-foreground">
                      Device joins by negotiating a session key. Standard, more secure.
                    </span>
                  </span>
                </Label>
              </Card>
              <Card className="px-4 py-3">
                <Label htmlFor="mode-abp" className="flex items-start gap-3 cursor-pointer">
                  <RadioGroupItem id="mode-abp" value="ABP" className="mt-1" />
                  <ShieldOff className="h-5 w-5 text-muted-foreground mt-0.5" aria-hidden="true" />
                  <span className="flex flex-col gap-1">
                    <span className="text-sm font-semibold flex items-center gap-2">
                      ABP
                      <Badge variant="secondary">not recommended</Badge>
                    </span>
                    <span className="text-xs text-muted-foreground">
                      Device pre-baked with session keys. Only for legacy devices or
                      special cases.
                    </span>
                  </span>
                </Label>
              </Card>
            </RadioGroup>

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
          </div>
        ) : null}

        {/* Step 4 — Keys (mode-specific) */}
        {step === 3 ? (
          <div className="flex flex-col gap-4">
            <h2 className="text-lg font-semibold leading-7">{STEP_HEADINGS[3]}</h2>

            {activationMode === 'OTAA' ? (
              <>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="app-key" className="text-sm font-semibold">
                    AppKey
                  </Label>
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
                  <Label htmlFor="join-eui" className="text-sm font-semibold flex items-center gap-2">
                    <span>Join EUI (AppEUI for v1.0)</span>
                    <TooltipProvider>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <span
                            tabIndex={0}
                            aria-label="Why two names?"
                            className="inline-flex"
                          >
                            <HelpCircle className="h-3 w-3 text-muted-foreground" />
                          </span>
                        </TooltipTrigger>
                        <TooltipContent>
                          AppEUI in LoRaWAN 1.0.x became Join EUI in 1.1. ChirpStack
                          accepts both; Shifter writes Join EUI internally.
                        </TooltipContent>
                      </Tooltip>
                    </TooltipProvider>
                  </Label>
                  <Input
                    id="join-eui"
                    className="font-mono"
                    value={joinEUI}
                    onChange={(e) => setJoinEUI(e.target.value)}
                    placeholder="16 hex chars"
                  />
                </div>
              </>
            ) : (
              <>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="dev-addr" className="text-sm font-semibold">
                    Dev Addr
                  </Label>
                  <Input
                    id="dev-addr"
                    className="font-mono"
                    value={devAddr}
                    onChange={(e) => setDevAddr(e.target.value)}
                    placeholder="8 hex chars"
                  />
                </div>

                <div className="flex flex-col gap-2">
                  <Label htmlFor="nwk-s-key" className="text-sm font-semibold">
                    Network Session Key
                  </Label>
                  <div className="flex items-center gap-2">
                    <Input
                      id="nwk-s-key"
                      type={showNwkSKey ? 'text' : 'password'}
                      className="font-mono"
                      value={nwkSKey}
                      onChange={(e) => setNwkSKey(e.target.value)}
                      placeholder="32 hex chars"
                    />
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => setShowNwkSKey((v) => !v)}
                      aria-label={showNwkSKey ? 'Hide NwkSKey' : 'Show NwkSKey'}
                    >
                      {showNwkSKey ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                    </Button>
                  </div>
                </div>

                <div className="flex flex-col gap-2">
                  <Label htmlFor="app-s-key" className="text-sm font-semibold">
                    AppSKey
                  </Label>
                  <div className="flex items-center gap-2">
                    <Input
                      id="app-s-key"
                      type={showAppSKey ? 'text' : 'password'}
                      className="font-mono"
                      value={appSKey}
                      onChange={(e) => setAppSKey(e.target.value)}
                      placeholder="32 hex chars"
                    />
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => setShowAppSKey((v) => !v)}
                      aria-label={showAppSKey ? 'Hide AppSKey' : 'Show AppSKey'}
                    >
                      {showAppSKey ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                    </Button>
                  </div>
                </div>

                <div className="grid grid-cols-2 gap-4">
                  <div className="flex flex-col gap-2">
                    <Label htmlFor="fcnt-up" className="text-sm font-semibold">
                      FCnt Up
                    </Label>
                    <Input
                      id="fcnt-up"
                      type="number"
                      min={0}
                      value={fCntUp}
                      onChange={(e) => setFCntUp(e.target.value)}
                    />
                    <p className="text-xs text-muted-foreground">
                      Set to 0 unless migrating from an existing install.
                    </p>
                  </div>
                  <div className="flex flex-col gap-2">
                    <Label htmlFor="fcnt-down" className="text-sm font-semibold">
                      FCnt Down
                    </Label>
                    <Input
                      id="fcnt-down"
                      type="number"
                      min={0}
                      value={fCntDown}
                      onChange={(e) => setFCntDown(e.target.value)}
                    />
                  </div>
                </div>
              </>
            )}
          </div>
        ) : null}

        {/* Step 5 — Review */}
        {step === 4 ? (
          <div className="flex flex-col gap-4">
            <h2 className="text-lg font-semibold leading-7">{STEP_HEADINGS[4]}</h2>
            <p className="text-sm text-muted-foreground">
              We'll create everything in ChirpStack and Shifter in one step. If anything
              fails, nothing is saved.
            </p>

            <Card>
              <CardContent className="flex flex-col gap-2 py-4 text-sm">
                <ReviewRow label="DevEUI" value={<span className="font-mono">{devEUI}</span>} />
                <ReviewRow label="Device name" value={name || '—'} />
                <ReviewRow label="Profile" value={selectedProfile?.name ?? '—'} />
                <ReviewRow label="Activation" value={activationMode} />
                {activationMode === 'OTAA' ? (
                  <>
                    <ReviewRow
                      label="AppKey"
                      value={
                        <span className="font-mono">
                          {appKey ? `••••••••••••••••••••••••••••${appKey.slice(-4)}` : '—'}
                        </span>
                      }
                    />
                    <ReviewRow
                      label="Join EUI"
                      value={<span className="font-mono">{joinEUI}</span>}
                    />
                  </>
                ) : (
                  <>
                    <ReviewRow
                      label="Dev Addr"
                      value={<span className="font-mono">{devAddr}</span>}
                    />
                    <ReviewRow
                      label="NwkSKey"
                      value={
                        <span className="font-mono">
                          {nwkSKey ? `••••••••••••••••••••••••••••${nwkSKey.slice(-4)}` : '—'}
                        </span>
                      }
                    />
                    <ReviewRow
                      label="AppSKey"
                      value={
                        <span className="font-mono">
                          {appSKey ? `••••••••••••••••••••••••••••${appSKey.slice(-4)}` : '—'}
                        </span>
                      }
                    />
                    <ReviewRow
                      label="FCnt Up"
                      value={<span className="font-mono">{fCntUp || '0'}</span>}
                    />
                    <ReviewRow
                      label="FCnt Down"
                      value={<span className="font-mono">{fCntDown || '0'}</span>}
                    />
                  </>
                )}
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

/**
 * D-21 success-state KeysPanel — renders all keys returned by POST /api/devices
 * plus a Copy keys button that writes a JSON blob to the clipboard.
 *
 * The keys are not persisted in TanStack Query (gcTime:0 on the mutation) nor
 * in localStorage; they live only in the parent's local React state until the
 * dialog closes (reset() clears).
 */
function KeysPanel({ response }: { response: AddDeviceResponse }) {
  const rows: { label: string; value: string }[] =
    response.activation_mode === 'OTAA'
      ? [
          { label: 'Activation', value: 'OTAA' },
          { label: 'Join EUI', value: response.join_eui },
          { label: 'AppKey', value: response.app_key },
          { label: 'NwkKey', value: response.nwk_key },
        ]
      : [
          { label: 'Activation', value: 'ABP' },
          { label: 'Dev Addr', value: response.dev_addr },
          { label: 'NwkSKey', value: response.nwk_s_key },
          { label: 'AppSKey', value: response.app_s_key },
          { label: 'FCnt Up', value: String(response.f_cnt_up) },
          { label: 'FCnt Down', value: String(response.f_cnt_down) },
        ]

  const onCopy = async () => {
    const payload = JSON.stringify(
      Object.fromEntries(rows.map((r) => [r.label, r.value])),
      null,
      2,
    )
    try {
      await navigator.clipboard.writeText(payload)
      toast.success('Key copied')
    } catch {
      toast.error('Could not copy to clipboard')
    }
  }

  return (
    <Card>
      <CardContent className="flex flex-col gap-4 p-6">
        {rows.map((row) => (
          <div key={row.label} className="flex items-center justify-between gap-2">
            <span className="text-sm font-semibold">{row.label}</span>
            <span className="text-sm font-mono select-all break-all text-right">
              {row.value}
            </span>
          </div>
        ))}
        <div className="flex justify-end pt-2">
          <Button type="button" onClick={onCopy}>
            <Copy className="mr-2 h-4 w-4" aria-hidden="true" />
            Copy keys
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
