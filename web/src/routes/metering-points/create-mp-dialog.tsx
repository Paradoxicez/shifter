import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
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
import { ApiError } from '@/lib/api'
import { createMP, type CreateMPRequest, type MeteringPoint } from '@/lib/metering-points'
import { listSites } from '@/lib/sites'

export interface CreateMeteringPointDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** When set, locks the site picker to this site (e.g. opened from /sites/:id). */
  siteId?: string
  onCreated?: (mp: MeteringPoint) => void
}

interface FormErrors {
  name?: string
  site_id?: string
  utility_class?: string
  submit?: string
}

/**
 * UI-SPEC §Add MP dialog (D-19):
 *   - Title "Add metering point" / submit idle "Add metering point" /
 *     loading "Adding metering point…".
 *   - Fields: name (required), site (Select listing non-archived sites; locked
 *     when `siteId` prop is set), utility_class (RadioGroup water/electricity),
 *     location_description (Textarea, optional).
 *
 * Submit calls POST '/api/metering-points'. On 409 (duplicate name on site),
 * inline alert per UI-SPEC error patterns.
 */
export function CreateMeteringPointDialog({
  open,
  onOpenChange,
  siteId,
  onCreated,
}: CreateMeteringPointDialogProps) {
  const qc = useQueryClient()
  const [name, setName] = useState('')
  const [selectedSite, setSelectedSite] = useState<string>(siteId ?? '')
  const [utilityClass, setUtilityClass] = useState<'water' | 'electricity'>('water')
  const [locationDescription, setLocationDescription] = useState('')
  const [errors, setErrors] = useState<FormErrors>({})

  const sitesQuery = useQuery({
    queryKey: ['sites', 'all'],
    queryFn: () => listSites(),
    enabled: open && !siteId,
  })

  // Sync selected site when prop changes.
  useEffect(() => {
    if (siteId) setSelectedSite(siteId)
  }, [siteId])

  const reset = () => {
    setName('')
    setSelectedSite(siteId ?? '')
    setUtilityClass('water')
    setLocationDescription('')
    setErrors({})
  }

  const validate = (): { valid: boolean; errs: FormErrors } => {
    const errs: FormErrors = {}
    if (!name.trim()) errs.name = 'Enter a metering point name.'
    if (!selectedSite) errs.site_id = 'Choose a site.'
    return { valid: Object.keys(errs).length === 0, errs }
  }

  const mutation = useMutation({
    mutationFn: (body: CreateMPRequest) => createMP(body),
    onSuccess: (created) => {
      qc.invalidateQueries({ queryKey: ['metering-points'] })
      qc.invalidateQueries({ queryKey: ['site', selectedSite] })
      toast.success('Metering point created')
      onCreated?.(created)
      onOpenChange(false)
      reset()
    },
    onError: (err: unknown) => {
      const msg =
        err instanceof ApiError
          ? err.status === 409
            ? `A metering point named ${name} already exists on this site.`
            : err.message
          : 'Could not create metering point.'
      setErrors((e) => ({ ...e, submit: msg }))
      toast.error('Could not create metering point')
    },
  })

  const onSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const { valid, errs } = validate()
    setErrors(errs)
    if (!valid) return
    const body: CreateMPRequest = {
      site_id: selectedSite,
      name: name.trim(),
      utility_class: utilityClass,
    }
    if (locationDescription.trim()) body.location_description = locationDescription.trim()
    mutation.mutate(body)
  }

  const sites = sitesQuery.data ?? []
  const hasErrors = Boolean(errors.name) || Boolean(errors.site_id)

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(v) => {
        onOpenChange(v)
        if (!v) reset()
      }}
      title="Add metering point"
      footer={
        <>
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            form="create-mp-form"
            type="submit"
            disabled={mutation.isPending || hasErrors}
          >
            {mutation.isPending ? 'Adding metering point…' : 'Add metering point'}
          </Button>
        </>
      }
    >
      <form
        id="create-mp-form"
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
          <Label htmlFor="mp-name">Metering point name</Label>
          <Input
            id="mp-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
          />
          {errors.name ? (
            <p className="text-sm text-destructive">{errors.name}</p>
          ) : null}
        </div>

        {!siteId ? (
          <div className="flex flex-col gap-2">
            <Label htmlFor="mp-site">Site</Label>
            <Select value={selectedSite} onValueChange={setSelectedSite}>
              <SelectTrigger id="mp-site" className="w-full">
                <SelectValue placeholder="Select a site" />
              </SelectTrigger>
              <SelectContent>
                {sites
                  .filter((s) => !s.archived_at)
                  .map((s) => (
                    <SelectItem key={s.id} value={s.id}>
                      {s.name}
                    </SelectItem>
                  ))}
              </SelectContent>
            </Select>
            {errors.site_id ? (
              <p className="text-sm text-destructive">{errors.site_id}</p>
            ) : null}
          </div>
        ) : null}

        <div className="flex flex-col gap-2">
          <Label>What does this metering point measure?</Label>
          <RadioGroup
            value={utilityClass}
            onValueChange={(v) => setUtilityClass(v as 'water' | 'electricity')}
            className="grid grid-cols-2 gap-2"
          >
            <Label
              htmlFor="utility-water"
              className="flex items-center gap-2 rounded-md border px-3 py-2 cursor-pointer"
            >
              <RadioGroupItem id="utility-water" value="water" />
              Water
            </Label>
            <Label
              htmlFor="utility-electricity"
              className="flex items-center gap-2 rounded-md border px-3 py-2 cursor-pointer"
            >
              <RadioGroupItem id="utility-electricity" value="electricity" />
              Electricity
            </Label>
          </RadioGroup>
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="mp-location">Location description</Label>
          <Textarea
            id="mp-location"
            value={locationDescription}
            onChange={(e) => setLocationDescription(e.target.value)}
            rows={2}
            placeholder="e.g. Building A, Floor 3, Apartment 305"
          />
        </div>
      </form>
    </ResponsiveDialog>
  )
}
