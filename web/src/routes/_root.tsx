import { useState } from 'react'
import { redirect, useLoaderData, useRevalidator } from 'react-router-dom'
import { toast } from 'sonner'
import { ResponsiveShell } from '@/components/shell/responsive-shell'
import { fetchSessionUser, logout, type SessionUser } from '@/lib/auth'
import { fetchInstallState, INSTALL_DONE_KEY } from '@/lib/install'
import { ChangePasswordDialog } from './change-password-dialog'

/**
 * Protected app shell.
 *
 * `rootLoader` first checks /api/install/state (Plan 16): if the install
 * wizard is still in progress (200 OK with non-null state), bounce to
 * /install. Only when install is complete (410 Gone → null) does it move on
 * to the session check (Plan 11): GET /api/account/me before any protected
 * route renders. On 401 (or any error), the loader throws
 * `redirect('/login?next=...')` so the SPA bounces the user to the login
 * screen and brings them back to the intended path post-auth. apiFetch's own
 * 401 handler already does window.location.assign('/login?...'); the
 * loader-thrown redirect is the react-router-native way to surface the same
 * outcome inside SSR/loader land (and prevents the protected layout from
 * rendering with stale data).
 */
export async function rootLoader({ request }: { request: Request }) {
  // Install-state pre-check: install incomplete → bounce to wizard.
  // Network failures fall through to the session check (which will also fail
  // and redirect to /login) — never block the user on a flaky probe.
  try {
    const installState = await fetchInstallState()
    if (installState !== null) throw redirect('/install')
  } catch (err) {
    if (err instanceof Response) throw err // re-throw the redirect
    // Otherwise: probe failed for a non-410 reason (network / 5xx). Continue
    // to session check; FirstRunGate (Plan 14) will catch a missing-admin
    // condition on the /api/account/me call.
  }

  try {
    const user = await fetchSessionUser()
    return { user }
  } catch {
    const url = new URL(request.url)
    throw redirect(`/login?next=${encodeURIComponent(url.pathname)}`)
  }
}

export default function RootLayout() {
  const { user } = useLoaderData() as { user: SessionUser }
  const revalidator = useRevalidator()
  const [changePwOpen, setChangePwOpen] = useState(false)

  const handleSignOut = async () => {
    try {
      await logout()
    } finally {
      sessionStorage.removeItem(INSTALL_DONE_KEY)
      window.location.assign('/login')
    }
  }

  return (
    <>
      <ResponsiveShell
        installDisplayName="Shifter"
        userEmail={user.email}
        userRole={user.role}
        onChangePassword={() => setChangePwOpen(true)}
        onSignOut={handleSignOut}
      />
      <ChangePasswordDialog
        open={changePwOpen}
        onOpenChange={setChangePwOpen}
        onSuccess={() => {
          // UI-SPEC §"Phase 1 copy table": success toast string "Password changed".
          toast.success('Password changed')
          revalidator.revalidate()
        }}
      />
    </>
  )
}
