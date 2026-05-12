import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'

/**
 * Plan 06-05 — typed client for /api/users/*.
 *
 * Backend route table (internal/user/handler.go):
 *   GET    /api/users[?scope=active|disabled|all]
 *   POST   /api/users                              → CreateUserResponse (initial_password ONCE)
 *   PATCH  /api/users/:id                          → User
 *   POST   /api/users/:id/disable
 *   POST   /api/users/:id/enable
 *   PATCH  /api/users/:id/role
 *   POST   /api/users/:id/reset-password           → ResetPasswordResponse (initial_password ONCE)
 *   POST   /api/users/:id/logout-everywhere
 *
 * All mutating endpoints require admin (server-side RequireAction). The
 * UI hides admin-only actions for viewers as defense-in-depth.
 */

export type UserRole = 'admin' | 'viewer'
export type UserScope = 'active' | 'disabled' | 'all'

export interface User {
  id: string
  email: string
  name: string
  role: UserRole
  must_change_password: boolean
  disabled_at: string | null
  last_login_at: string | null
  created_at: string
  updated_at: string
}

export interface ListUsersResponse {
  users: User[]
}

export interface CreateUserRequest {
  email: string
  name: string
  role: UserRole
}

export interface CreateUserResponse {
  user: User
  /** Plaintext password — shown ONCE in the AddUser dialog step-2 panel. */
  initial_password: string
}

export interface ResetPasswordResponse {
  initial_password: string
}

export function useUsersList(scope: UserScope) {
  return useQuery({
    queryKey: ['users', scope],
    queryFn: () =>
      apiFetch<ListUsersResponse>(`/api/users?scope=${encodeURIComponent(scope)}`),
  })
}

export function useCreateUserMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: CreateUserRequest) =>
      apiFetch<CreateUserResponse>('/api/users', {
        method: 'POST',
        body: JSON.stringify(req),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['users'] })
    },
  })
}

export function useUpdateUserMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, name }: { id: string; name: string }) =>
      apiFetch<User>(`/api/users/${encodeURIComponent(id)}`, {
        method: 'PATCH',
        body: JSON.stringify({ name }),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  })
}

export function useChangeRoleMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, role }: { id: string; role: UserRole }) =>
      apiFetch<{ id: string; role: UserRole }>(
        `/api/users/${encodeURIComponent(id)}/role`,
        {
          method: 'PATCH',
          body: JSON.stringify({ role }),
        },
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  })
}

export function useDisableUserMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch(`/api/users/${encodeURIComponent(id)}/disable`, { method: 'POST' }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  })
}

export function useEnableUserMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch(`/api/users/${encodeURIComponent(id)}/enable`, { method: 'POST' }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  })
}

export function useResetPasswordMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch<ResetPasswordResponse>(
        `/api/users/${encodeURIComponent(id)}/reset-password`,
        { method: 'POST' },
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  })
}

export function useLogoutEverywhereMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch(
        `/api/users/${encodeURIComponent(id)}/logout-everywhere`,
        { method: 'POST' },
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  })
}

/**
 * useLastAdminLookup returns a predicate that reports whether the given
 * userId is currently the ONLY active admin. The UI uses it to grey out
 * the Disable + Role-radio actions on the last-admin row (D-26 client-side
 * enforcement; server side is authoritative).
 */
export function useLastAdminLookup(users: User[]): (id: string) => boolean {
  const activeAdmins = users.filter(
    (u) => u.role === 'admin' && u.disabled_at === null,
  )
  return (id: string) =>
    activeAdmins.length === 1 && activeAdmins[0].id === id
}
