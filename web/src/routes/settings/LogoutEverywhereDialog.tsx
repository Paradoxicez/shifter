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
import { useLogoutEverywhereMutation } from '@/hooks/useUsers'

interface LogoutEverywhereDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: string
  name: string
}

export function LogoutEverywhereDialog({
  open,
  onOpenChange,
  userId,
  name,
}: LogoutEverywhereDialogProps) {
  const mutation = useLogoutEverywhereMutation()
  const handleConfirm = () => {
    mutation.mutate(userId, {
      onSuccess: () => {
        toast.success('Sessions ended.')
        onOpenChange(false)
      },
      onError: () => {
        toast.error('Could not sign user out.')
      },
    })
  }
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Sign {name} out everywhere?</AlertDialogTitle>
          <AlertDialogDescription>
            All active sessions for this user will end immediately. They can
            sign back in normally.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            onClick={handleConfirm}
            disabled={mutation.isPending}
          >
            Sign user out everywhere
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
