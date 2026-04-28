/**
 * Typed wrappers around the auth/account API endpoints.
 *
 * All network IO goes through `apiFetch`, which already attaches the
 * `X-Requested-With: shifter` CSRF header (Plan 06) and intercepts 401
 * responses to redirect the SPA to /login.
 *
 * - fetchSessionUser  → GET  /api/account/me      (Plan 11 backend addendum)
 * - login             → POST /api/auth/login      (Plan 09)
 * - logout            → POST /api/auth/logout     (Plan 09)
 * - changePassword    → POST /api/account/password (Plan 09 / AUTH-05)
 */

import { ApiError, apiFetch } from './api'

export interface SessionUser {
  id: string
  email: string
  role: 'admin' | 'viewer'
  must_change_password: boolean
}

export async function fetchSessionUser(): Promise<SessionUser> {
  const body = await apiFetch<{ user: SessionUser }>('/api/account/me')
  return body.user
}

export async function login(email: string, password: string): Promise<SessionUser> {
  const body = await apiFetch<{ user: SessionUser }>('/api/auth/login', {
    method: 'POST',
    body: JSON.stringify({ email, password }),
  })
  return body.user
}

export async function logout(): Promise<void> {
  await apiFetch('/api/auth/logout', { method: 'POST' })
}

export async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
  await apiFetch('/api/account/password', {
    method: 'POST',
    body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
  })
}

export { ApiError }
