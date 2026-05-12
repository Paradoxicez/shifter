import { useEffect, useState } from 'react'
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
  useChangeRoleMutation,
  useUpdateUserMutation,
  type User,
  type UserRole,
} from '@/hooks/useUsers'
import { RoleChangeDialog } from './RoleChangeDialog'

const schema = z.object({
  name: z.string().min(1, 'Name is required').max(120),
  role: z.enum(['admin', 'viewer']),
})

type FormValues = z.infer<typeof schema>

interface EditUserDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  user: User
  /** True if this row is the current logged-in user (D-26 self-edit grey-out). */
  isSelf: boolean
  /** True if this row is the only active admin (D-26 last-admin grey-out). */
  isLastAdmin: boolean
}

export function EditUserDialog({
  open,
  onOpenChange,
  user,
  isSelf,
  isLastAdmin,
}: EditUserDialogProps) {
  const updateMutation = useUpdateUserMutation()
  const changeRoleMutation = useChangeRoleMutation()
  const [pendingRoleChange, setPendingRoleChange] = useState<UserRole | null>(null)

  const {
    register,
    handleSubmit,
    setValue,
    watch,
    reset,
    formState: { errors },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: user.name, role: user.role },
  })

  // Reset form when user changes (e.g. dialog reopened on another row).
  useEffect(() => {
    reset({ name: user.name, role: user.role })
  }, [user.id, user.name, user.role, reset])

  const role = watch('role')
  const roleDisabled = isSelf || isLastAdmin
  const roleHelper = isSelf
    ? "You can't change your own role."
    : isLastAdmin
      ? 'At least one admin must remain.'
      : ''

  const onSubmit = async (values: FormValues) => {
    const nameChanged = values.name !== user.name
    const roleChanged = values.role !== user.role
    if (!nameChanged && !roleChanged) {
      onOpenChange(false)
      return
    }
    if (nameChanged) {
      try {
        await updateMutation.mutateAsync({ id: user.id, name: values.name })
        toast.success('User updated.')
      } catch {
        toast.error('Could not update user.')
        return
      }
    }
    if (roleChanged) {
      setPendingRoleChange(values.role)
      return
    }
    onOpenChange(false)
  }

  const handleConfirmRoleChange = async () => {
    if (!pendingRoleChange) return
    try {
      await changeRoleMutation.mutateAsync({ id: user.id, role: pendingRoleChange })
      toast.success(
        'Role changed. The user has been signed out and must sign back in.',
      )
      setPendingRoleChange(null)
      onOpenChange(false)
    } catch {
      toast.error('Could not change role.')
      setPendingRoleChange(null)
    }
  }

  return (
    <>
      <ResponsiveDialog
        open={open}
        onOpenChange={onOpenChange}
        title="Edit user"
        description={user.email}
        footer={
          <>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" form="edit-user-form">
              Save
            </Button>
          </>
        }
      >
        <form
          id="edit-user-form"
          onSubmit={handleSubmit(onSubmit)}
          className="flex flex-col gap-4"
        >
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="edit-name">Name</Label>
            <Input id="edit-name" {...register('name')} />
            {errors.name ? (
              <span className="text-sm text-destructive">{errors.name.message}</span>
            ) : null}
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>Role</Label>
            <RadioGroup
              value={role}
              onValueChange={(v) => setValue('role', v as UserRole)}
              disabled={roleDisabled}
              className="flex gap-4"
            >
              <label className="flex items-center gap-2 text-sm">
                <RadioGroupItem value="viewer" disabled={roleDisabled} /> Viewer
              </label>
              <label className="flex items-center gap-2 text-sm">
                <RadioGroupItem value="admin" disabled={roleDisabled} /> Admin
              </label>
            </RadioGroup>
            {roleHelper ? (
              <span className="text-xs text-muted-foreground">{roleHelper}</span>
            ) : null}
          </div>
        </form>
      </ResponsiveDialog>
      {pendingRoleChange ? (
        <RoleChangeDialog
          open={true}
          onOpenChange={(next) => {
            if (!next) setPendingRoleChange(null)
          }}
          name={user.name}
          onConfirm={handleConfirmRoleChange}
        />
      ) : null}
    </>
  )
}
