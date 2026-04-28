import { useState } from 'react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ApiError } from '@/lib/api'
import { postStep4 } from '@/lib/install'

/**
 * Wizard step 4 — install identity (display name, address, timezone, units).
 *
 * Plan 15's POST /api/install/step/4 validates timezone via time.LoadLocation
 * and units against the install_identity enum (`metric` | `imperial`). The
 * timezone defaults to the operator's system zone via Intl heuristic.
 */
export function IdentityStep({ onAdvance }: { onAdvance: () => void }) {
  const [displayName, setDisplayName] = useState('')
  const [address, setAddress] = useState('')
  const [timezone, setTimezone] = useState(
    Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC',
  )
  const [units, setUnits] = useState<'metric' | 'imperial'>('metric')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    setBusy(true)
    try {
      await postStep4({
        display_name: displayName,
        address: address || undefined,
        timezone,
        units,
      })
      onAdvance()
    } catch (err) {
      if (err instanceof ApiError && err.status === 422) {
        const body = err.body as { error?: string } | null
        if (body?.error === 'invalid_timezone') {
          setError("That timezone isn't recognized. Use an IANA name like Asia/Bangkok.")
        } else if (body?.error === 'invalid_units') {
          setError('Pick metric or imperial.')
        } else {
          setError('Please fill in every required field.')
        }
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
        <h2 className="text-2xl font-semibold leading-8">Tell us about your install</h2>
        <p className="text-sm text-muted-foreground">
          This appears in the topbar and on every report you export.
        </p>
      </div>
      {error ? (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
      <div className="flex flex-col gap-2">
        <Label htmlFor="disp">Display name</Label>
        <Input
          id="disp"
          required
          value={displayName}
          onChange={(e) => setDisplayName(e.target.value)}
        />
        <p className="text-sm text-muted-foreground">
          What your team calls this site, e.g. "Acme Water Co."
        </p>
      </div>
      <div className="flex flex-col gap-2">
        <Label htmlFor="addr">Address (optional)</Label>
        <Input id="addr" value={address} onChange={(e) => setAddress(e.target.value)} />
      </div>
      <div className="flex flex-col gap-2">
        <Label htmlFor="tz">Timezone</Label>
        <Input
          id="tz"
          required
          value={timezone}
          onChange={(e) => setTimezone(e.target.value)}
          className="font-mono"
        />
      </div>
      <fieldset className="flex flex-col gap-2">
        <legend className="text-sm font-semibold">Units</legend>
        <label className="flex items-center gap-2">
          <input
            type="radio"
            checked={units === 'metric'}
            onChange={() => setUnits('metric')}
          />{' '}
          Metric
        </label>
        <label className="flex items-center gap-2">
          <input
            type="radio"
            checked={units === 'imperial'}
            onChange={() => setUnits('imperial')}
          />{' '}
          Imperial
        </label>
      </fieldset>
      <div className="flex justify-end pt-2">
        <Button type="submit" disabled={busy}>
          {busy ? 'Saving…' : 'Next'}
        </Button>
      </div>
    </form>
  )
}
