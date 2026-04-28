import { Navigate } from 'react-router-dom'

/**
 * Phase 1 only ships /settings under the protected shell.
 * Index redirects there. Phase 2+ replaces with the dashboard.
 */
export default function IndexRedirect() {
  return <Navigate to="/settings" replace />
}
