import { useState } from 'react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { ApiError, changePassword } from '@/lib/auth'

export interface ChangePasswordDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: () => void
}

/**
 * Change-password dialog wired to POST /api/account/password (Plan 09).
 *
 * UI-SPEC §"Phase 1 copy table" copy strings (verbatim):
 *   - Title: "Change password"
 *   - Submit idle: "Change password"
 *   - Submit loading: "Changing password…"
 *   - Strength hint: "At least 12 characters with mixed case, a number, and a symbol."
 *   - 401 error: "Current password incorrect."
 *   - 422 error: "Password is too weak. Add length, mixed case, a number, and a symbol."
 *   - Success toast (raised by the parent): "Password changed"
 *
 * UI-SPEC §"Dialog Conventions" anatomy: Cancel on the LEFT, primary on the
 * RIGHT — the footer JSX preserves that order.
 */
export function ChangePasswordDialog({
  open,
  onOpenChange,
  onSuccess,
}: ChangePasswordDialogProps) {
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const reset = () => {
    setCurrent('')
    setNext('')
    setConfirm('')
    setError(null)
    setBusy(false)
  }

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    if (next !== confirm) {
      setError("New passwords don't match.")
      return
    }
    setBusy(true)
    try {
      await changePassword(current, next)
      onSuccess()
      onOpenChange(false)
      reset()
    } catch (err) {
      if (err instanceof ApiError) {
        switch (err.status) {
          case 401:
            setError('Current password incorrect.')
            break
          case 422:
            setError('Password is too weak. Add length, mixed case, a number, and a symbol.')
            break
          default:
            setError('Something went wrong. Try again.')
        }
      } else {
        setError('Something went wrong. Try again.')
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(v) => {
        onOpenChange(v)
        if (!v) reset()
      }}
      title="Change password"
      footer={
        <>
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button form="change-pw-form" type="submit" disabled={busy}>
            {busy ? 'Changing password…' : 'Change password'}
          </Button>
        </>
      }
    >
      <form id="change-pw-form" onSubmit={onSubmit} className="flex flex-col gap-4">
        {error ? (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}
        <div className="flex flex-col gap-2">
          <Label htmlFor="cp-current">Current password</Label>
          <Input
            id="cp-current"
            type="password"
            autoComplete="current-password"
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
            required
          />
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="cp-new">New password</Label>
          <Input
            id="cp-new"
            type="password"
            autoComplete="new-password"
            value={next}
            onChange={(e) => setNext(e.target.value)}
            required
          />
          <p className="text-sm text-muted-foreground">
            At least 12 characters with mixed case, a number, and a symbol.
          </p>
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="cp-confirm">Confirm new password</Label>
          <Input
            id="cp-confirm"
            type="password"
            autoComplete="new-password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            required
          />
        </div>
      </form>
    </ResponsiveDialog>
  )
}
