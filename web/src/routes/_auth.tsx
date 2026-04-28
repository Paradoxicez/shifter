import { Outlet } from 'react-router-dom'

/**
 * Public auth-flow layout — wraps /login (Plan 23) and /install (Plan 16).
 * Centered card on neutral background per UI-SPEC §Login screen.
 */
export default function AuthLayout() {
  return (
    <div className="flex min-h-screen items-center justify-center px-4 py-8">
      <Outlet />
    </div>
  )
}
