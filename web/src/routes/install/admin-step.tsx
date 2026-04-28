import { useState } from 'react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ApiError } from '@/lib/api'
import { postStep1 } from '@/lib/install'

/**
 * Wizard step 1 — admin account creation.
 *
 * Plan 15's POST /api/install/step/1 hashes the password (Argon2id) before
 * persistence; this UI surfaces 422 weak_password as inline error copy.
 * Strength helper text uses UI-SPEC verbatim copy.
 */
export function AdminStep({ onAdvance }: { onAdvance: () => void }) {
  const [email, setEmail] = useState('')
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    if (password !== confirm) {
      setError("Passwords don't match.")
      return
    }
    setBusy(true)
    try {
      await postStep1({ email, name, password })
      onAdvance()
    } catch (err) {
      if (err instanceof ApiError && err.status === 422) {
        setError('Password is too weak. Add length, mixed case, a number, and a symbol.')
      } else {
        setError('Something went wrong. Try again.')
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-4">
      <div>
        <h2 className="text-2xl font-semibold leading-8">Create the admin account</h2>
        <p className="text-sm text-muted-foreground">
          This account has full control of Shifter. You can add more users later.
        </p>
      </div>
      {error ? (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
      <div className="flex flex-col gap-2">
        <Label htmlFor="adm-email">Email address</Label>
        <Input
          id="adm-email"
          type="email"
          required
          value={email}
          onChange={(e) => setEmail(e.target.value)}
        />
      </div>
      <div className="flex flex-col gap-2">
        <Label htmlFor="adm-name">Your name</Label>
        <Input id="adm-name" required value={name} onChange={(e) => setName(e.target.value)} />
      </div>
      <div className="flex flex-col gap-2">
        <Label htmlFor="adm-pw">Password</Label>
        <Input
          id="adm-pw"
          type="password"
          required
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        <p className="text-sm text-muted-foreground">
          At least 12 characters with mixed case, a number, and a symbol.
        </p>
      </div>
      <div className="flex flex-col gap-2">
        <Label htmlFor="adm-confirm">Confirm password</Label>
        <Input
          id="adm-confirm"
          type="password"
          required
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
        />
      </div>
      <div className="flex justify-end pt-2">
        <Button type="submit" disabled={busy}>
          {busy ? 'Saving…' : 'Next'}
        </Button>
      </div>
    </form>
  )
}
