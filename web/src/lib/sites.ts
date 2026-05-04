import { apiFetch } from './api'

/**
 * Typed client for /api/sites/* endpoints (Plan 02-10 backend).
 *
 * Backend handler shape (internal/site/handlers.go):
 *   GET  /api/sites              → Site[]
 *   GET  /api/sites/archived     → Site[]
 *   GET  /api/sites/{id}         → Site
 *   POST /api/sites              → Site (admin only)
 *   PATCH /api/sites/{id}        → Site (admin only)
 *   POST /api/sites/{id}/archive → Site (admin only)
 *   POST /api/sites/{id}/restore → Site (admin only)
 */

export interface Site {
  id: string
  parent_id?: string | null
  name: string
  site_type?: string | null
  lat?: number | null
  lng?: number | null
  timezone: string
  address?: string | null
  description?: string | null
  archived_at?: string | null
  created_at?: string
  updated_at?: string
}

export interface CreateSiteRequest {
  name: string
  lat?: number
  lng?: number
  timezone: string
  address?: string
  description?: string
  parent_id?: string
  site_type?: string
}

export type UpdateSiteRequest = CreateSiteRequest

export const listSites = (opts: { archived?: boolean } = {}) =>
  apiFetch<Site[]>(opts.archived ? '/api/sites/archived' : '/api/sites')

export const getSite = (id: string) => apiFetch<Site>(`/api/sites/${id}`)

export const createSite = (body: CreateSiteRequest) =>
  apiFetch<Site>('/api/sites', {
    method: 'POST',
    body: JSON.stringify(body),
  })

export const updateSite = (id: string, body: UpdateSiteRequest) =>
  apiFetch<Site>(`/api/sites/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(body),
  })

export const archiveSite = (id: string) =>
  apiFetch<Site>(`/api/sites/${id}/archive`, { method: 'POST' })

export const restoreSite = (id: string) =>
  apiFetch<Site>(`/api/sites/${id}/restore`, { method: 'POST' })
