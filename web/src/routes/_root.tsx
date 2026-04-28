import { ResponsiveShell } from '@/components/shell/responsive-shell'

/**
 * Protected app shell. Plan 11 (account-ui) replaces this stub with a
 * real session-aware loader that pulls the current user from
 * /api/auth/whoami; today the displayed values are placeholders so the
 * router compiles and the shell renders.
 */
export default function RootLayout() {
  return (
    <ResponsiveShell
      installDisplayName="Shifter"
      userEmail="placeholder@local"
      userRole="admin"
      onChangePassword={() => {
        /* Plan 11 wires the change-password dialog */
      }}
      onSignOut={() => {
        window.location.assign('/login')
      }}
    />
  )
}
