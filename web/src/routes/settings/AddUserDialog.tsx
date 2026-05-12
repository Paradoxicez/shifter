import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { zodResolver } from '@hookform/resolvers/zod'
import { toast } from 'sonner'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import {
  useCreateUserMutation,
  type CreateUserResponse,
  type UserRole,
} from '@/hooks/useUsers'
import { ShareCredentialsPanel } from './ShareCredentialsPanel'

const schema = z.object({
  name: z.string().min(1, 'Name is required').max(120),
  email: z.string().email('Invalid email'),
  role: z.enum(['admin', 'viewer']),
})

type FormValues = z.infer<typeof schema>

interface AddUserDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

/**
 * Plan 06-05 / D-23 — 2-step Add User dialog.
 *
 * Step 1: name + email + role form. On submit calls useCreateUserMutation.
 * Step 2: ShareCredentialsPanel renders the response.initial_password
 *         exactly once. Clicking "I've shared this" closes the dialog
 *         and clears the users React Query cache.
 */
export function AddUserDialog({ open, onOpenChange }: AddUserDialogProps) {
  const [credentials, setCredentials] = useState<CreateUserResponse | null>(null)
  const mutation = useCreateUserMutation()

  const {
    register,
    handleSubmit,
    setValue,
    watch,
    reset,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: '', email: '', role: 'viewer' },
  })

  const role = watch('role')

  const onSubmit = (values: FormValues) => {
    mutation.mutate(values, {
      onSuccess: (resp) => {
        setCredentials(resp)
        toast.success(
          'User created. Share the credentials before closing this window.',
        )
      },
      onError: (err: unknown) => {
        const status = (err as { status?: number } | undefined)?.status
        if (status === 409) {
          toast.error('That email is already in use.')
        } else {
          toast.error('Could not create user. Please try again.')
        }
      },
    })
  }

  const handleClose = (next: boolean) => {
    if (!next) {
      reset()
      setCredentials(null)
    }
    onOpenChange(next)
  }

  if (credentials) {
    return (
      <ResponsiveDialog
        open={open}
        onOpenChange={handleClose}
        title="User created"
      >
        <ShareCredentialsPanel
          email={credentials.user.email}
          password={credentials.initial_password}
          onConfirm={() => handleClose(false)}
        />
      </ResponsiveDialog>
    )
  }

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={handleClose}
      title="Add user"
      description="Create an admin or viewer account. The user will be forced to change their password on first sign-in."
      footer={
        <>
          <Button type="button" variant="outline" onClick={() => handleClose(false)}>
            Cancel
          </Button>
          <Button
            type="submit"
            form="add-user-form"
            disabled={isSubmitting || mutation.isPending}
          >
            {mutation.isPending ? 'Creating…' : 'Create user'}
          </Button>
        </>
      }
    >
      <form
        id="add-user-form"
        onSubmit={handleSubmit(onSubmit)}
        className="flex flex-col gap-4"
      >
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="name">Name</Label>
          <Input id="name" autoComplete="off" {...register('name')} />
          {errors.name ? (
            <span className="text-sm text-destructive">{errors.name.message}</span>
          ) : null}
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="email">Email</Label>
          <Input id="email" type="email" autoComplete="off" {...register('email')} />
          {errors.email ? (
            <span className="text-sm text-destructive">{errors.email.message}</span>
          ) : null}
        </div>
        <div className="flex flex-col gap-1.5">
          <Label>Role</Label>
          <RadioGroup
            value={role}
            onValueChange={(v) => setValue('role', v as UserRole)}
            className="flex gap-4"
          >
            <label className="flex items-center gap-2 text-sm">
              <RadioGroupItem value="viewer" /> Viewer
            </label>
            <label className="flex items-center gap-2 text-sm">
              <RadioGroupItem value="admin" /> Admin
            </label>
          </RadioGroup>
        </div>
      </form>
    </ResponsiveDialog>
  )
}
