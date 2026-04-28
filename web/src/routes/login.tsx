import { useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import shifterLogo from '@/assets/shifter-logo.svg'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ApiError } from '@/lib/api'
import { login } from '@/lib/auth'

/**
 * Login screen — UI-SPEC §"Login screen (no shell)" + §"Phase 1 copy table".
 *
 * Mounted under <AuthLayout /> at /login (web/src/App.tsx). Layout: centered
 * card on neutral background (the layout owns the centering); content order
 * is Shifter product logo → heading → description → form → footer copy.
 *
 * Verbatim copy strings (UI-SPEC):
 *   - Heading:     "Sign in to Shifter"
 *   - Description: "Enter your email and password to continue."
 *   - Email label: "Email address"
 *   - Password:    "Password"
 *   - Submit idle: "Sign in"
 *   - Submit busy: "Signing in…"
 *   - Footer:      "Forgot password? Contact your administrator."
 *   - Err 401:     "That email and password don't match. Try again."
 *   - Err 429:     "Too many failed attempts. Try again in 5 minutes."
 *
 * Post-success: react-router `navigate(next, { replace: true })` where `next`
 * comes from the `?next=` query param (T-23-04: react-router-dom v7's
 * navigate() treats absolute URLs as paths, blocking open-redirects to
 * external hosts). Default destination is /settings — the only protected
 * route shipped in Phase 1.
 *
 * Note: the install-identity logo never appears here; only the Shifter
 * product wordmark (UI-SPEC §"Logo & Brand Placement").
 */
export default function LoginScreen() {
  const navigate = useNavigate()
  const [search] = useSearchParams()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    setBusy(true)
    try {
      await login(email, password)
      const next = search.get('next') ?? '/settings'
      navigate(next, { replace: true })
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 429) {
          setError('Too many failed attempts. Try again in 5 minutes.')
        } else if (err.status === 401) {
          setError("That email and password don't match. Try again.")
        } else {
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
    <div className="w-full max-w-sm">
      <div className="flex flex-col items-center gap-6">
        <img src={shifterLogo} alt="Shifter" className="h-10" />
        <div className="text-center">
          <h1 className="text-2xl font-semibold leading-8">Sign in to Shifter</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Enter your email and password to continue.
          </p>
        </div>
        <form
          onSubmit={onSubmit}
          className="flex w-full flex-col gap-4 rounded-lg border bg-card p-8 shadow-sm"
        >
          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
          <div className="flex flex-col gap-2">
            <Label htmlFor="login-email">Email address</Label>
            <Input
              id="login-email"
              type="email"
              autoComplete="username"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="login-password">Password</Label>
            <Input
              id="login-password"
              type="password"
              autoComplete="current-password"
              required
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </div>
          <Button type="submit" className="w-full" disabled={busy}>
            {busy ? 'Signing in…' : 'Sign in'}
          </Button>
        </form>
        <p className="text-sm text-muted-foreground">
          Forgot password? Contact your administrator.
        </p>
      </div>
    </div>
  )
}
