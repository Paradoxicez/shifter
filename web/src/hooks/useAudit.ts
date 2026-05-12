/**
 * Plan 06-07 — React Query hooks for /audit browse + export.
 *
 * D-37: No live-tail / no SSE — manual Refresh button + refetchOnWindowFocus.
 */

import { useInfiniteQuery, useMutation, useQuery } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type { AuditFilters } from '@/lib/auditParams'

export interface AuditRow {
  id: string
  time: string
  action: string
  entity_type: string
  entity_id: string
  user_email: string
  user_id: string | null
  request_id: string | null
  notes: string | null
  before_json: string | null
  after_json: string | null
}

export interface AuditListPage {
  rows: AuditRow[]
  next_cursor: string | null
  total: number
}

export interface AuditCountResponse {
  count: number
}

export interface AuditDistinctsResponse {
  actions: string[]
  entity_types: string[]
}

function filtersToQuery(params: Partial<AuditFilters>): string {
  const sp = new URLSearchParams()
  if (params.from) sp.set('from', params.from)
  if (params.to) sp.set('to', params.to)
  if (params.user_id) sp.set('user_id', params.user_id)
  if (params.entity_type?.length) {
    for (const et of params.entity_type) sp.append('entity_type', et)
  }
  if (params.action?.length) {
    for (const a of params.action) sp.append('action', a)
  }
  if (params.request_id) sp.set('request_id', params.request_id)
  if (params.cursor) sp.set('cursor', params.cursor)
  return sp.toString() ? `?${sp.toString()}` : ''
}

export function useAuditList(params: AuditFilters) {
  return useInfiniteQuery({
    queryKey: ['audit', params],
    queryFn: ({ pageParam }) =>
      apiFetch<AuditListPage>(`/api/audit${filtersToQuery({ ...params, cursor: pageParam as string | undefined })}`),
    getNextPageParam: (lastPage: AuditListPage) => lastPage.next_cursor ?? undefined,
    initialPageParam: undefined as string | undefined,
    refetchOnWindowFocus: false, // D-37: no live tail
  })
}

export function useAuditCount(params: Partial<AuditFilters>) {
  return useQuery({
    queryKey: ['audit-count', params],
    queryFn: () =>
      apiFetch<AuditCountResponse>(`/api/audit/count${filtersToQuery(params)}`),
    refetchOnWindowFocus: false,
  })
}

export function useAuditDistincts() {
  return useQuery({
    queryKey: ['audit-distincts'],
    queryFn: () => apiFetch<AuditDistinctsResponse>('/api/audit/distincts'),
    staleTime: 5 * 60 * 1000, // 5 min — vocab changes rarely
    refetchOnWindowFocus: false,
  })
}

export function useExportAuditAsync() {
  return useMutation({
    mutationFn: (params: Partial<AuditFilters>) =>
      apiFetch<{ job_id: string; status: string }>(`/api/audit/export-async${filtersToQuery(params)}`, {
        method: 'POST',
      }),
  })
}
