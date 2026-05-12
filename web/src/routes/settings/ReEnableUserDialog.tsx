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
import { useEnableUserMutation } from '@/hooks/useUsers'

interface ReEnableUserDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: string
  name: string
}

/**
 * UI-SPEC §Destructive Confirmations — D-27 re-enable. The button is
 * variant="default" (NOT destructive) per the spec — re-enabling restores
 * a user, which is a non-destructive recovery action.
 */
export function ReEnableUserDialog({
  open,
  onOpenChange,
  userId,
  name,
}: ReEnableUserDialogProps) {
  const mutation = useEnableUserMutation()
  const handleConfirm = () => {
    mutation.mutate(userId, {
      onSuccess: () => {
        toast.success('User re-enabled.')
        onOpenChange(false)
      },
      onError: () => {
        toast.error('Could not re-enable user.')
      },
    })
  }
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Re-enable {name}?</AlertDialogTitle>
          <AlertDialogDescription>
            The user will be able to sign in with their existing password.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            variant="default"
            onClick={handleConfirm}
            disabled={mutation.isPending}
          >
            Re-enable
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
