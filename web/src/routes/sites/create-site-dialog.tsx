import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Map as MapIcon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { ApiError } from '@/lib/api'
import { createSite, type CreateSiteRequest, type Site } from '@/lib/sites'

export interface CreateSiteDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated?: (site: Site) => void
}

interface FormErrors {
  name?: string
  lat?: string
  lng?: string
  timezone?: string
  submit?: string
}

/**
 * UI-SPEC §Add Site dialog (D-17 + D-18):
 *   - Title: "Add site" / submit idle "Add site" / submit loading "Adding site…".
 *   - Fields: name (required), lat/lng (optional, range-validated), timezone
 *     (default Asia/Bangkok), address, description.
 *   - Lat/Lng helper: "Paste from Google Maps — example: `13.7563, 100.5018`."
 *   - "Pick on map" ghost button rendered DISABLED with tooltip
 *     "Available in v5" (D-18 forward-compat slot).
 *
 * Submit calls POST '/api/sites' (via the createSite client in
 * lib/sites.ts); on success: invalidate ['sites'] query, fire onCreated,
 * close dialog, toast success.
 */
export function CreateSiteDialog({
  open,
  onOpenChange,
  onCreated,
}: CreateSiteDialogProps) {
  const qc = useQueryClient()
  const [name, setName] = useState('')
  const [lat, setLat] = useState('')
  const [lng, setLng] = useState('')
  const [timezone, setTimezone] = useState('Asia/Bangkok')
  const [address, setAddress] = useState('')
  const [description, setDescription] = useState('')
  const [errors, setErrors] = useState<FormErrors>({})

  const reset = () => {
    setName('')
    setLat('')
    setLng('')
    setTimezone('Asia/Bangkok')
    setAddress('')
    setDescription('')
    setErrors({})
  }

  const validate = (): { valid: boolean; errs: FormErrors } => {
    const errs: FormErrors = {}
    if (!name.trim()) errs.name = 'Enter a site name.'
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
    if (!timezone.trim()) errs.timezone = 'Choose a timezone.'
    return { valid: Object.keys(errs).length === 0, errs }
  }

  const mutation = useMutation({
    mutationFn: (body: CreateSiteRequest) => createSite(body),
    onSuccess: (created) => {
      qc.invalidateQueries({ queryKey: ['sites'] })
      toast.success('Site created')
      onCreated?.(created)
      onOpenChange(false)
      reset()
    },
    onError: (err: unknown) => {
      const msg =
        err instanceof ApiError
          ? err.status === 409
            ? `A site named ${name} already exists.`
            : err.message
          : 'Could not create site.'
      setErrors((e) => ({ ...e, submit: msg }))
      toast.error('Could not create site')
    },
  })

  const onSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const { valid, errs } = validate()
    setErrors(errs)
    if (!valid) return

    const body: CreateSiteRequest = {
      name: name.trim(),
      timezone: timezone.trim(),
    }
    if (lat) body.lat = Number(lat)
    if (lng) body.lng = Number(lng)
    if (address.trim()) body.address = address.trim()
    if (description.trim()) body.description = description.trim()

    mutation.mutate(body)
  }

  const onBlurValidate = () => {
    const { errs } = validate()
    setErrors(errs)
  }

  // Submit button disabled when form has errors or busy.
  const hasErrors =
    Boolean(errors.name) ||
    Boolean(errors.lat) ||
    Boolean(errors.lng) ||
    Boolean(errors.timezone)

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(v) => {
        onOpenChange(v)
        if (!v) reset()
      }}
      title="Add site"
      footer={
        <>
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            form="create-site-form"
            type="submit"
            disabled={mutation.isPending || hasErrors}
          >
            {mutation.isPending ? 'Adding site…' : 'Add site'}
          </Button>
        </>
      }
    >
      <form
        id="create-site-form"
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
          <Label htmlFor="site-name">Site name</Label>
          <Input
            id="site-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            onBlur={onBlurValidate}
            required
          />
          {errors.name ? (
            <p className="text-sm text-destructive">{errors.name}</p>
          ) : null}
        </div>

        <div className="grid grid-cols-2 gap-4">
          <div className="flex flex-col gap-2">
            <Label htmlFor="site-lat">Latitude</Label>
            <Input
              id="site-lat"
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
            <Label htmlFor="site-lng">Longitude</Label>
            <Input
              id="site-lng"
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
          {/* D-18 forward-compat slot. Tooltip in title attr for jsdom + a11y. */}
          <Button
            type="button"
            variant="outline"
            disabled
            title="Available in v5"
            aria-label="Pick on map"
          >
            <MapIcon className="h-4 w-4 mr-2" aria-hidden="true" />
            Pick on map
          </Button>
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="site-timezone">Timezone</Label>
          <Input
            id="site-timezone"
            value={timezone}
            onChange={(e) => setTimezone(e.target.value)}
            onBlur={onBlurValidate}
            placeholder="Asia/Bangkok"
          />
          {errors.timezone ? (
            <p className="text-sm text-destructive">{errors.timezone}</p>
          ) : null}
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="site-address">Address</Label>
          <Textarea
            id="site-address"
            value={address}
            onChange={(e) => setAddress(e.target.value)}
            rows={2}
          />
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="site-description">Description</Label>
          <Textarea
            id="site-description"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={2}
          />
        </div>
      </form>
    </ResponsiveDialog>
  )
}
