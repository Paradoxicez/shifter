import { Outlet } from 'react-router-dom'
import { AccountMenu } from './account-menu'
import { AlertWorkerBanner } from './AlertWorkerBanner'
import { Sidebar } from './sidebar'
import { Topbar } from './topbar'

export interface ShellProps {
  installDisplayName: string
  installLogoUrl?: string
  userEmail: string
  userRole: 'admin' | 'viewer'
  onChangePassword: () => void
  onSignOut: () => void
}

/**
 * UI-SPEC §App shell — wraps Topbar + Sidebar around <Outlet />.
 * Single canonical shell for every authenticated route.
 *
 * Plan 06-04 adds <AlertWorkerBanner /> directly under the topbar — renders
 * a sticky warning banner when the alert engine reports degraded workers
 * via /api/health/detailed. Banner self-hides when healthy.
 */
export function ResponsiveShell(props: ShellProps) {
  return (
    <div className="flex min-h-screen flex-col">
      <Topbar
        installDisplayName={props.installDisplayName}
        installLogoUrl={props.installLogoUrl}
      >
        <AccountMenu
          userEmail={props.userEmail}
          userRole={props.userRole}
          onChangePassword={props.onChangePassword}
          onSignOut={props.onSignOut}
        />
      </Topbar>
      <AlertWorkerBanner />
      <div className="flex flex-1">
        <Sidebar />
        <main className="flex-1 px-4 py-4 md:px-8 md:py-6">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
