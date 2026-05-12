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
import { useDisableUserMutation } from '@/hooks/useUsers'

interface DisableUserDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: string
  name: string
}

export function DisableUserDialog({
  open,
  onOpenChange,
  userId,
  name,
}: DisableUserDialogProps) {
  const mutation = useDisableUserMutation()
  const handleConfirm = () => {
    mutation.mutate(userId, {
      onSuccess: () => {
        toast.success('User disabled. Their sessions have been ended.')
        onOpenChange(false)
      },
      onError: () => {
        toast.error('Could not disable user.')
      },
    })
  }
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Disable {name}?</AlertDialogTitle>
          <AlertDialogDescription>
            The user will be signed out and unable to sign in. You can re-enable
            them later.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            onClick={handleConfirm}
            disabled={mutation.isPending}
          >
            Disable user
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
