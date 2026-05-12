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
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { useResetPasswordMutation } from '@/hooks/useUsers'
import { ShareCredentialsPanel } from './ShareCredentialsPanel'

interface ResetPasswordDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: string
  email: string
  name: string
}

/**
 * Plan 06-05 / D-23 — Reset password is a two-stage flow:
 *   Stage 1: AlertDialog destructive confirm "Reset password for {name}?"
 *   Stage 2: ResponsiveDialog with ShareCredentialsPanel (same shape as
 *            AddUser step 2). Plaintext shown once.
 */
export function ResetPasswordDialog({
  open,
  onOpenChange,
  userId,
  email,
  name,
}: ResetPasswordDialogProps) {
  const [newPassword, setNewPassword] = useState<string | null>(null)
  const mutation = useResetPasswordMutation()

  const handleConfirm = () => {
    mutation.mutate(userId, {
      onSuccess: (resp) => {
        setNewPassword(resp.initial_password)
        toast.success(
          'Password reset. Share the new credentials with the user.',
        )
      },
      onError: () => {
        toast.error('Could not reset password.')
      },
    })
  }

  const close = () => {
    setNewPassword(null)
    onOpenChange(false)
  }

  if (newPassword) {
    return (
      <ResponsiveDialog
        open={open}
        onOpenChange={(next) => {
          if (!next) close()
        }}
        title="Password reset"
      >
        <ShareCredentialsPanel email={email} password={newPassword} onConfirm={close} />
      </ResponsiveDialog>
    )
  }

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Reset password for {name}?</AlertDialogTitle>
          <AlertDialogDescription>
            A new random password will be generated. The user will be signed
            out and forced to change it on next sign-in.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            onClick={handleConfirm}
            disabled={mutation.isPending}
          >
            Reset password
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
