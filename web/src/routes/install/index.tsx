import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import shifterLogo from '@/assets/shifter-logo.svg'
import { Stepper } from '@/components/stepper'
import { type InstallState, fetchInstallState } from '@/lib/install'
import { AdminStep } from './admin-step'
import { ChirpStackStep } from './chirpstack-step'
import { IdentityStep } from './identity-step'
import { RegionStep } from './region-step'
import { ReviewStep } from './review-step'

const STEPS = [
  { label: 'Admin' },
  { label: 'ChirpStack' },
  { label: 'Region' },
  { label: 'Identity' },
  { label: 'Review' },
]

/**
 * Install wizard shell — full-screen 5-step flow at /install.
 *
 * UI-SPEC §"Install wizard (full-screen stepped dialog)" — this is the
 * canonical exception to UX-01's modal-first interaction model. The shell
 * loads /api/install/state on mount; on 410 (already completed) it redirects
 * to /login. Steps render based on `state.CurrentStep` (1..5); each step
 * calls `onAdvance` which re-fetches state to reveal the next step.
 *
 * On finish success, ReviewStep invokes `onComplete` which navigates to /login
 * (Plan 23 ships the login screen).
 */
export default function InstallWizard() {
  const navigate = useNavigate()
  const [state, setState] = useState<InstallState | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    fetchInstallState()
      .then((s) => {
        if (s === null) navigate('/login', { replace: true })
        else setState(s)
      })
      .finally(() => setLoading(false))
  }, [navigate])

  if (loading || !state) return null

  const reload = async () => {
    const s = await fetchInstallState()
    if (s === null) navigate('/login', { replace: true })
    else setState(s)
  }

  const idx = state.CurrentStep - 1

  return (
    <div className="mx-auto flex max-w-3xl flex-col gap-6 px-4 py-8 md:py-12">
      <div className="flex items-center gap-3">
        <img src={shifterLogo} alt="Shifter" className="h-6" />
        <div>
          <h1 className="text-2xl font-semibold leading-8">Set up Shifter</h1>
          <p className="text-sm text-muted-foreground">
            Five quick steps and you're ready to monitor.
          </p>
        </div>
      </div>
      <div className="rounded-lg border bg-card p-6 md:p-8">
        <Stepper steps={STEPS} currentIndex={idx} />
        <div className="mt-8">
          {state.CurrentStep === 1 && <AdminStep onAdvance={reload} />}
          {state.CurrentStep === 2 && <ChirpStackStep onAdvance={reload} />}
          {state.CurrentStep === 3 && <RegionStep onAdvance={reload} />}
          {state.CurrentStep === 4 && <IdentityStep onAdvance={reload} />}
          {state.CurrentStep === 5 && (
            <ReviewStep
              state={state}
              onComplete={() => navigate('/login', { replace: true })}
            />
          )}
        </div>
      </div>
    </div>
  )
}
