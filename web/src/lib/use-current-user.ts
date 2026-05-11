import { useRouteLoaderData } from 'react-router-dom'
import type { SessionUser } from './auth'

/**
 * Plan 03-09 — typed accessor for the SessionUser loaded by `_root.tsx` /
 * `rootLoader`. Child routes call this to gate admin-only UI surfaces
 * (e.g. bulk-action bar, /admin/imports nav). Defense-in-depth — server-side
 * RequireAction is authoritative; this just hides what the user can't use.
 *
 * Depends on the root route registered in App.tsx with `id: 'root'`.
 *
 * Returns `null` when called outside the root layout (e.g. /login) so callers
 * can safely render without crashing.
 */
export function useCurrentUser(): SessionUser | null {
  const data = useRouteLoaderData('root') as { user: SessionUser } | undefined
  return data?.user ?? null
}
