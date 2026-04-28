import { useState } from 'react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { ApiError } from '@/lib/api'
import { type InstallState, postFinish } from '@/lib/install'

/**
 * Wizard step 5 — review + finish.
 *
 * Renders the four collected drafts via JSON.stringify so embedded HTML in
 * fields like display name is text, not interpreted (T-16-01 mitigation —
 * React + <pre>{...}</pre> auto-escapes). On finish success, parent navigates
 * to /login (Plan 23 owns the login screen).
 */
export function ReviewStep({
  state,
  onComplete,
}: {
  state: InstallState
  onComplete: () => void
}) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const finish = async () => {
    setBusy(true)
    setError(null)
    try {
      await postFinish()
      onComplete()
    } catch (err) {
      if (err instanceof ApiError) {
        setError(`Couldn't finish: ${err.message}`)
      } else {
        setError('Something went wrong. Try again.')
      }
    } finally {
      setBusy(false)
    }
  }

  const fmt = (v: unknown) => (v ? JSON.stringify(v, null, 2) : '(empty)')

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h2 className="text-2xl font-semibold leading-8">Review and finish</h2>
        <p className="text-sm text-muted-foreground">
          Confirm everything looks right. You can change any of this in Settings later.
        </p>
      </div>
      {error ? (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
      <pre className="bg-muted rounded-md p-4 font-mono text-xs overflow-auto max-h-96">
        {`Admin:        ${fmt(state.Step1Admin)}
ChirpStack:   ${fmt(state.Step2ChirpStack)}
Region:       ${fmt(state.Step3Region)}
Identity:     ${fmt(state.Step4Identity)}`}
      </pre>
      <div className="flex justify-end pt-2">
        <Button onClick={finish} disabled={busy}>
          {busy ? 'Finishing setup…' : 'Finish setup'}
        </Button>
      </div>
    </div>
  )
}
