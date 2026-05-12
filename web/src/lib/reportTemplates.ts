/**
 * Report Templates API client — Plan 07-11b
 *
 * Wraps the 5 CRUD endpoints from plan 11a:
 *   GET    /api/reports/templates          → list
 *   GET    /api/reports/templates/{id}     → single
 *   POST   /api/reports/templates          → create (409 on name collision)
 *   PATCH  /api/reports/templates/{id}     → update (409 on rename collision)
 *   DELETE /api/reports/templates/{id}     → hard delete
 */

import { apiFetch } from './api'

export type ReportTemplate = {
  id: string
  name: string
  description: string
  state: Record<string, unknown>
  created_at: string
  updated_at: string
}

export async function listTemplates(): Promise<ReportTemplate[]> {
  return apiFetch<ReportTemplate[]>('/api/reports/templates')
}

export async function getTemplate(id: string): Promise<ReportTemplate> {
  return apiFetch<ReportTemplate>(`/api/reports/templates/${id}`)
}

export async function createTemplate(
  name: string,
  description: string,
  state: object,
): Promise<ReportTemplate> {
  return apiFetch<ReportTemplate>('/api/reports/templates', {
    method: 'POST',
    body: JSON.stringify({ name, description, state }),
  })
}

export async function updateTemplate(
  id: string,
  name: string,
  description: string,
  state: object,
): Promise<ReportTemplate> {
  return apiFetch<ReportTemplate>(`/api/reports/templates/${id}`, {
    method: 'PATCH',
    body: JSON.stringify({ name, description, state }),
  })
}

export async function deleteTemplate(id: string): Promise<void> {
  await apiFetch(`/api/reports/templates/${id}`, { method: 'DELETE' })
}
