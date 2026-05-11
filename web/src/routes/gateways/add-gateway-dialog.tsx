import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Map as MapIcon } from 'lucide-react'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
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
  createGateway,
  type CreateGatewayRequest,
  type Gateway,
  updateGateway,
  type UpdateGatewayRequest,
} from '@/lib/gateways'
import { fetchInstallState, REGIONS } from '@/lib/install'

export interface AddGatewayDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** When provided, dialog renders in Edit mode and pre-populates fields. */
  gateway?: Gateway
  onSaved?: (gw: Gateway) => void
}

interface FormErrors {
  gateway_id?: string
  name?: string
  region?: string
  lat?: string
  lng?: string
  submit?: string
}

/**
 * Strip whitespace, ':', '-' and lowercase. Mirrors the deveui-parser's
 * normalize routine. Backend tolerates the same shape but normalising in
 * the UI gives operators instant feedback.
 */
function normalizeGatewayID(raw: string): string {
  return raw.replace(/0x/gi, '').replace(/[\s:\-]/g, '').toLowerCase()
}

function parseTags(raw: string): Record<string, string> {
  // Simple `key=value,key=value` parser. Empty input → empty record.
  const out: Record<string, string> = {}
  for (const pair of raw.split(',').map((s) => s.trim()).filter(Boolean)) {
    const idx = pair.indexOf('=')
    if (idx === -1) {
      // Bare keyword → flag-style tag.
      out[pair] = ''
    } else {
      const k = pair.slice(0, idx).trim()
      const v = pair.slice(idx + 1).trim()
      if (k) out[k] = v
    }
  }
  return out
}

function formatTags(tags: Record<string, string> | null | undefined): string {
  if (!tags) return ''
  return Object.entries(tags)
    .map(([k, v]) => (v ? `${k}=${v}` : k))
    .join(', ')
}

/**
 * UI-SPEC §Add Gateway dialog (D-29 — single ResponsiveDialog, NOT stepped):
 *   - Gateway ID (mono, paste-normalized to lowercase 16-hex).
 *   - Name (required, 1–128 chars).
 *   - Description (optional Textarea).
 *   - LoRaWAN region (Select, defaults to install region per D-03).
 *   - Latitude / Longitude (numeric, range-validated).
 *   - Pick on map button rendered DISABLED with tooltip "Available in v5."
 *     per D-01 — mirrors Phase 2 Site dialog (UI-SPEC line 243).
 *   - Tags (key=value,key=value).
 *
 * Submit: useMutation(createGateway) on Add; useMutation(updateGateway) on
 * Edit. onSuccess invalidates the ['gateways'] query cache.
 */
export function AddGatewayDialog({
  open,
  onOpenChange,
  gateway,
  onSaved,
}: AddGatewayDialogProps) {
  const qc = useQueryClient()
  const isEdit = Boolean(gateway)

  const [gatewayID, setGatewayID] = useState(gateway?.gateway_id ?? '')
  const [name, setName] = useState(gateway?.name ?? '')
  const [description, setDescription] = useState(gateway?.description ?? '')
  const [region, setRegion] = useState(gateway?.region ?? '')
  const [lat, setLat] = useState(gateway?.lat != null ? String(gateway.lat) : '')
  const [lng, setLng] = useState(gateway?.lng != null ? String(gateway.lng) : '')
  const [tagsRaw, setTagsRaw] = useState(formatTags(gateway?.tags))
  const [errors, setErrors] = useState<FormErrors>({})

  // Install state powers the region default (D-03). The install endpoint
  // returns 410 once the install is complete (null state); in that case the
  // backend value lives in /api/install/state's step-3 payload. For the
  // dialog default we fall back to AS923-2 (Thai operator base) if the call
  // returns null or fails.
  const installQuery = useQuery({
    queryKey: ['install-state'],
    queryFn: () => fetchInstallState(),
    staleTime: 60_000,
    enabled: open && !isEdit,
  })

  useEffect(() => {
    if (!open) return
    if (isEdit) {
      setRegion(gateway?.region ?? '')
      return
    }
    if (region) return
    // Read step3 region from install state if present.
    const step3 = (installQuery.data?.Step3Region ?? null) as
      | { name?: string }
      | string
      | null
    let pick: string | null = null
    if (step3 && typeof step3 === 'object' && 'name' in step3 && step3.name) {
      pick = step3.name
    } else if (typeof step3 === 'string' && step3) {
      pick = step3
    }
    setRegion(pick ?? REGIONS.find((r) => r.default_for_country === 'TH')?.name ?? 'as923_2')
  }, [open, isEdit, gateway, installQuery.data, region])

  const reset = () => {
    setGatewayID('')
    setName('')
    setDescription('')
    setRegion('')
    setLat('')
    setLng('')
    setTagsRaw('')
    setErrors({})
  }

  const validate = (): { valid: boolean; errs: FormErrors } => {
    const errs: FormErrors = {}
    const normalized = normalizeGatewayID(gatewayID)
    if (!isEdit) {
      if (!normalized) {
        errs.gateway_id = 'Enter a 16-character hex gateway ID.'
      } else if (!/^[0-9a-f]{16}$/.test(normalized)) {
        errs.gateway_id = 'Enter a 16-character hex gateway ID.'
      }
    }
    if (!name.trim()) errs.name = 'Enter a gateway name.'
    if (lat) {
      const n = Number(lat)
      if (!Number.isFinite(n) || n < -90 || n > 90) {
        errs.lat = 'Latitude must be between −90 and 90.'
      }
    }
    if (lng) {
      const n = Number(lng)
      if (!Number.isFinite(n) || n < -180 || n > 180) {
        errs.lng = 'Longitude must be between −180 and 180.'
      }
    }
    if (!region) errs.region = 'Choose a LoRaWAN region.'
    return { valid: Object.keys(errs).length === 0, errs }
  }

  const onBlurValidate = () => {
    setGatewayID((v) => {
      const n = normalizeGatewayID(v)
      return n
    })
    const { errs } = validate()
    setErrors(errs)
  }

  const createMutation = useMutation({
    mutationFn: (body: CreateGatewayRequest) => createGateway(body),
    onSuccess: (created) => {
      qc.invalidateQueries({ queryKey: ['gateways'] })
      toast.success(`Gateway ${created.name} added.`)
      onSaved?.(created)
      onOpenChange(false)
      reset()
    },
    onError: (err: unknown) => {
      const msg =
        err instanceof ApiError
          ? err.status === 409
            ? `A gateway with that ID already exists.`
            : `ChirpStack rejected the gateway: ${err.message}.`
          : 'Could not create gateway.'
      setErrors((e) => ({ ...e, submit: msg }))
      toast.error('Could not create gateway')
    },
  })

  const updateMutation = useMutation({
    mutationFn: (body: UpdateGatewayRequest) =>
      updateGateway(gateway?.id ?? '', body),
    onSuccess: (saved) => {
      qc.invalidateQueries({ queryKey: ['gateways'] })
      qc.invalidateQueries({ queryKey: ['gateway', gateway?.id] })
      toast.success(`Gateway ${saved.name} updated.`)
      onSaved?.(saved)
      onOpenChange(false)
    },
    onError: (err: unknown) => {
      const msg =
        err instanceof ApiError
          ? `ChirpStack rejected the change: ${err.message}.`
          : 'Could not update gateway.'
      setErrors((e) => ({ ...e, submit: msg }))
      toast.error('Could not update gateway')
    },
  })

  const onSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const { valid, errs } = validate()
    setErrors(errs)
    if (!valid) return

    const tags = parseTags(tagsRaw)
    const common = {
      name: name.trim(),
      description: description.trim() || undefined,
      region: region || undefined,
      lat: lat ? Number(lat) : null,
      lng: lng ? Number(lng) : null,
      tags: Object.keys(tags).length > 0 ? tags : undefined,
    }

    if (isEdit) {
      updateMutation.mutate(common as UpdateGatewayRequest)
    } else {
      createMutation.mutate({
        gateway_id: normalizeGatewayID(gatewayID),
        ...common,
      } as CreateGatewayRequest)
    }
  }

  const mutation = isEdit ? updateMutation : createMutation
  const submitIdle = isEdit ? 'Save changes' : 'Add gateway'
  const submitLoading = isEdit ? 'Saving…' : 'Adding gateway…'
  const hasErrors =
    Boolean(errors.gateway_id) ||
    Boolean(errors.name) ||
    Boolean(errors.lat) ||
    Boolean(errors.lng) ||
    Boolean(errors.region)

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(v) => {
        onOpenChange(v)
        if (!v && !isEdit) reset()
      }}
      title={isEdit ? 'Edit gateway' : 'Add gateway'}
      description={isEdit ? 'Changes apply immediately to ChirpStack.' : undefined}
      footer={
        <>
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            form="gateway-form"
            type="submit"
            disabled={mutation.isPending || hasErrors}
          >
            {mutation.isPending ? submitLoading : submitIdle}
          </Button>
        </>
      }
    >
      <form
        id="gateway-form"
        onSubmit={onSubmit}
        className="flex flex-col gap-4"
        noValidate
      >
        {errors.submit ? (
          <Alert variant="destructive">
            <AlertDescription>{errors.submit}</AlertDescription>
          </Alert>
        ) : null}

        <div className="flex flex-col gap-2">
          <Label htmlFor="gateway-id">Gateway ID</Label>
          <Input
            id="gateway-id"
            className="font-mono"
            placeholder="0102030405060708"
            value={gatewayID}
            onChange={(e) => setGatewayID(e.target.value)}
            onBlur={onBlurValidate}
            disabled={isEdit}
          />
          <p className="text-sm text-muted-foreground">
            Paste from the device sticker. We&rsquo;ll strip separators and lowercase it.
          </p>
          {errors.gateway_id ? (
            <p className="text-sm text-destructive">{errors.gateway_id}</p>
          ) : null}
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="gateway-name">Name</Label>
          <Input
            id="gateway-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            onBlur={onBlurValidate}
            required
          />
          {errors.name ? (
            <p className="text-sm text-destructive">{errors.name}</p>
          ) : null}
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="gateway-description">Description</Label>
          <Textarea
            id="gateway-description"
            value={description ?? ''}
            onChange={(e) => setDescription(e.target.value)}
            rows={2}
          />
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="gateway-region">LoRaWAN region</Label>
          <Select value={region} onValueChange={setRegion}>
            <SelectTrigger id="gateway-region">
              <SelectValue placeholder="Choose a region" />
            </SelectTrigger>
            <SelectContent>
              {REGIONS.map((r) => (
                <SelectItem key={r.name} value={r.name}>
                  {r.display}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="text-sm text-muted-foreground">
            Defaults to the install region. Override only for multi-region installs.
          </p>
          {errors.region ? (
            <p className="text-sm text-destructive">{errors.region}</p>
          ) : null}
        </div>

        <div className="grid grid-cols-2 gap-4">
          <div className="flex flex-col gap-2">
            <Label htmlFor="gateway-lat">Latitude</Label>
            <Input
              id="gateway-lat"
              type="text"
              inputMode="decimal"
              value={lat}
              onChange={(e) => setLat(e.target.value)}
              onBlur={onBlurValidate}
              placeholder="13.7563"
            />
            {errors.lat ? (
              <p className="text-sm text-destructive">{errors.lat}</p>
            ) : null}
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="gateway-lng">Longitude</Label>
            <Input
              id="gateway-lng"
              type="text"
              inputMode="decimal"
              value={lng}
              onChange={(e) => setLng(e.target.value)}
              onBlur={onBlurValidate}
              placeholder="100.5018"
            />
            {errors.lng ? (
              <p className="text-sm text-destructive">{errors.lng}</p>
            ) : null}
          </div>
        </div>

        <div className="flex items-start gap-3">
          <p className="text-sm text-muted-foreground flex-1">
            Paste from Google Maps — example:{' '}
            <span className="font-mono">13.7563, 100.5018</span>.
          </p>
          {/* D-01 forward-compat slot — Phase 5 ships the map picker. Tooltip in
              title attr for jsdom + a11y. */}
          <Button
            type="button"
            variant="outline"
            disabled
            title="Available in v5."
            aria-label="Pick on map"
          >
            <MapIcon className="h-4 w-4 mr-2" aria-hidden="true" />
            Pick on map
          </Button>
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="gateway-tags">Tags</Label>
          <Input
            id="gateway-tags"
            value={tagsRaw}
            onChange={(e) => setTagsRaw(e.target.value)}
            placeholder="rooftop=building-a, vendor=mikrotik"
          />
          <p className="text-sm text-muted-foreground">
            Optional. Comma-separated keywords like &ldquo;rooftop, building-a&rdquo;.
          </p>
        </div>
      </form>
    </ResponsiveDialog>
  )
}
