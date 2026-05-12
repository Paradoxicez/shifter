/**
 * InstallIdentityCard — displays install identity fields with admin edit (SETT-01 / SETT-02).
 *
 * GET  /api/settings/identity  — admin + viewer (read-only display)
 * PATCH /api/settings/identity — admin only (via EditIdentityDialog)
 *
 * Plan 06-12 gap closure: backend now exists; card is mounted in settings.tsx.
 * Admin sees an Edit button that opens a dialog for display_name, address,
 * timezone, and units. Changes propagate to future report branding (D-47).
 */

import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { apiFetch } from '@/lib/api'
import { useCurrentUser } from '@/lib/use-current-user'
import { ResponsiveDialog } from '@/components/responsive-dialog'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface InstallIdentity {
  install_id: string
  site_name: string
  display_name: string
  address?: string
  timezone: string
  units: 'metric' | 'imperial'
  version: string
}

interface IdentityPatch {
  display_name?: string
  address?: string
  timezone?: string
  units?: 'metric' | 'imperial'
}

// ---------------------------------------------------------------------------
// Form schema
// ---------------------------------------------------------------------------

const identitySchema = z.object({
  display_name: z.string().min(1, 'Required').max(200, 'Max 200 characters'),
  address: z.string().optional(),
  timezone: z.string().min(1, 'Required').max(64, 'Max 64 characters'),
  units: z.enum(['metric', 'imperial']),
})

type IdentityFormValues = z.infer<typeof identitySchema>

// ---------------------------------------------------------------------------
// Data fetching
// ---------------------------------------------------------------------------

function fetchInstallIdentity(): Promise<InstallIdentity> {
  return apiFetch<InstallIdentity>('/api/settings/identity')
}

function patchIdentity(patch: IdentityPatch): Promise<InstallIdentity> {
  return apiFetch<InstallIdentity>('/api/settings/identity', {
    method: 'PATCH',
    body: JSON.stringify(patch),
  })
}

// ---------------------------------------------------------------------------
// EditIdentityDialog
// ---------------------------------------------------------------------------

interface EditIdentityDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  current: InstallIdentity
  onSaved: () => void
}

function EditIdentityDialog({ open, onOpenChange, current, onSaved }: EditIdentityDialogProps) {
  const {
    register,
    handleSubmit,
    setValue,
    watch,
    formState: { errors, isDirty },
    reset,
  } = useForm<IdentityFormValues>({
    resolver: zodResolver(identitySchema),
    defaultValues: {
      display_name: current.display_name,
      address: current.address ?? '',
      timezone: current.timezone,
      units: current.units,
    },
  })

  const units = watch('units')

  const mutation = useMutation({
    mutationFn: (values: IdentityFormValues) => {
      const patch: IdentityPatch = {
        display_name: values.display_name,
        address: values.address || undefined,
        timezone: values.timezone,
        units: values.units,
      }
      return patchIdentity(patch)
    },
    onSuccess: () => {
      toast.success('Identity updated')
      onSaved()
    },
    onError: () => toast.error('Failed to update identity'),
  })

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      reset()
    }
    onOpenChange(nextOpen)
  }

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={handleOpenChange}
      title="Edit install identity"
      description="Changes apply to future reports (D-47)."
      footer={
        <>
          <Button variant="outline" onClick={() => handleOpenChange(false)}>
            Cancel
          </Button>
          <Button
            onClick={handleSubmit((values) => mutation.mutate(values))}
            disabled={!isDirty || mutation.isPending}
          >
            {mutation.isPending ? 'Saving…' : 'Save'}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <div className="flex flex-col gap-1">
          <Label htmlFor="identity-display-name">Site name</Label>
          <Input
            id="identity-display-name"
            {...register('display_name')}
            placeholder="Acme Water"
          />
          {errors.display_name && (
            <p className="text-xs text-destructive">{errors.display_name.message}</p>
          )}
        </div>

        <div className="flex flex-col gap-1">
          <Label htmlFor="identity-address">Address</Label>
          <Input
            id="identity-address"
            {...register('address')}
            placeholder="123 Main St, City, Country (optional)"
          />
          {errors.address && (
            <p className="text-xs text-destructive">{errors.address.message}</p>
          )}
        </div>

        <div className="flex flex-col gap-1">
          <Label htmlFor="identity-timezone">Timezone</Label>
          <Input
            id="identity-timezone"
            {...register('timezone')}
            placeholder="UTC"
          />
          {errors.timezone && (
            <p className="text-xs text-destructive">{errors.timezone.message}</p>
          )}
        </div>

        <div className="flex flex-col gap-1">
          <Label htmlFor="identity-units">Units</Label>
          <Select
            value={units}
            onValueChange={(val) =>
              setValue('units', val as 'metric' | 'imperial', { shouldDirty: true })
            }
          >
            <SelectTrigger id="identity-units">
              <SelectValue placeholder="Select units" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="metric">Metric</SelectItem>
              <SelectItem value="imperial">Imperial</SelectItem>
            </SelectContent>
          </Select>
          {errors.units && (
            <p className="text-xs text-destructive">{errors.units.message}</p>
          )}
        </div>
      </div>
    </ResponsiveDialog>
  )
}

// ---------------------------------------------------------------------------
// InstallIdentityCard
// ---------------------------------------------------------------------------

export function InstallIdentityCard() {
  const user = useCurrentUser()
  const isAdmin = user?.role === 'admin'
  const qc = useQueryClient()
  const [editOpen, setEditOpen] = useState(false)

  const { data, isLoading } = useQuery({
    queryKey: ['settings', 'identity'],
    queryFn: fetchInstallIdentity,
    // Identity rarely changes — long stale time.
    staleTime: 5 * 60 * 1000,
  })

  return (
    <>
      <Card data-testid="install-identity-card">
        <CardHeader>
          <CardTitle>Install identity</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-2">
          {isLoading || !data ? (
            <Skeleton className="h-16 w-full" />
          ) : (
            <>
              <div className="flex justify-between">
                <span className="text-sm font-medium">Install ID</span>
                <span className="font-mono text-sm" data-testid="install-id">{data.install_id}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-sm font-medium">Site name</span>
                <span className="text-sm" data-testid="site-name">{data.site_name}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-sm font-medium">Version</span>
                <span className="font-mono text-sm" data-testid="version">{data.version}</span>
              </div>

              {/* D-47 propagation note + Edit button — admin only */}
              {isAdmin && (
                <>
                  <p className="mt-2 text-xs text-muted-foreground" data-testid="d47-propagation-note">
                    The install ID is embedded in backup archives. When restoring on a new host, ensure
                    the target install has a matching identity before importing the backup.
                    Changes to identity fields apply to future reports.
                  </p>
                  <div className="mt-2 flex justify-end">
                    <Button
                      variant="outline"
                      size="sm"
                      data-testid="edit-identity-button"
                      onClick={() => setEditOpen(true)}
                    >
                      Edit identity
                    </Button>
                  </div>
                </>
              )}
            </>
          )}
        </CardContent>
      </Card>

      {data && isAdmin && (
        <EditIdentityDialog
          open={editOpen}
          onOpenChange={setEditOpen}
          current={data}
          onSaved={() => {
            qc.invalidateQueries({ queryKey: ['settings', 'identity'] })
            setEditOpen(false)
          }}
        />
      )}
    </>
  )
}
