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

interface RoleChangeDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  name: string
  onConfirm: () => void
}

/**
 * UI-SPEC §Destructive Confirmations — D-25 role change confirmation.
 * Verbatim copy.
 */
export function RoleChangeDialog({
  open,
  onOpenChange,
  name,
  onConfirm,
}: RoleChangeDialogProps) {
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Change role for {name}?</AlertDialogTitle>
          <AlertDialogDescription>
            Changing the role will sign {name} out everywhere. They&apos;ll see
            the new role the next time they sign in.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={onConfirm}>
            Change role
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
