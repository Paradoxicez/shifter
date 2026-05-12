/**
 * Plan 06-04 — React Query hooks for the alert center surface.
 *
 * Hooks:
 *   useAlertsList(params)          GET /api/alerts (with filter chips)
 *   useAlertsRecent()              GET /api/alerts/recent (bell + drawer poll, 30s)
 *   useAlertRulesList(showDisabled) GET /api/alerts/rules
 *   useAnomalyRoster()             GET /api/anomaly-roster
 *   useMPAnomalyState(mpId)        GET /api/metering-points/{id}/anomaly-state
 *   useAckMutation()               POST /api/alerts/{id}/ack
 *   useSnoozeMutation()            POST /api/alerts/{id}/snooze
 *   useCreateRuleMutation()        POST /api/alerts/rules
 *   useToggleMPAnomalyRuleMutation() PATCH /api/metering-points/{id}/anomaly-rules/{kind}
 *   useTestFireMutation()          POST /api/alerts/rules/{id}/test-fire
 */

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type { AlertsParams } from '@/lib/alertParams'

// ─── Wire shapes ───────────────────────────────────────────────────────────

export interface AlertDTO {
  id: string
  rule_id: string
  rule_kind: string
  severity: 'critical' | 'warning' | 'info'
  state: 'firing' | 'acknowledged' | 'snoozed' | 'muted' | 'cleared'
  payload: unknown
  target_entity_type: string
  target_entity_id: string
  is_test: boolean
  fired_at: string
  cleared_at?: string
  acked_at?: string
  acked_by?: string
  ack_note?: string
  snoozed_until?: string
  snoozed_by?: string
  muted: boolean
}

export interface UnreadCounts {
  critical: number
  warning: number
  info: number
}

export interface AlertListResponse {
  rows: AlertDTO[]
  next_cursor?: string
  unread_counts: UnreadCounts
}

export interface AlertRecentResponse {
  rows: AlertDTO[]
  unread_counts: UnreadCounts
}

export interface RuleDTO {
  id: string
  rule_kind: string
  scope_kind: string
  scope_id?: string
  high_bound?: number
  low_bound?: number
  comparison?: string
  unit?: string
  flow_threshold?: number
  days_of_week?: number
  severity: string
  name?: string
  notes?: string
  cooldown_seconds: number
  disabled_at?: string
}

export interface AnomalyRosterRow {
  metering_point_id: string
  metering_point_label: string
  site_label: string
  days_until_eligible: number
}

export interface MPAnomalyState {
  eligible: boolean
  days_until_eligible: number
  rules: Array<{
    rule_kind: string
    rule_id?: string
    severity?: string
    enabled: boolean
    flow_threshold?: number
    days_of_week?: number
  }>
}

// ─── Queries ───────────────────────────────────────────────────────────────

export function useAlertsList(params: AlertsParams) {
  const qs = buildQuery(params)
  return useQuery({
    queryKey: ['alerts', 'list', params] as const,
    queryFn: () => apiFetch<AlertListResponse>(`/api/alerts${qs}`),
    staleTime: 10_000,
  })
}

export function useAlertsRecent() {
  return useQuery({
    queryKey: ['alerts', 'recent'] as const,
    queryFn: () => apiFetch<AlertRecentResponse>('/api/alerts/recent'),
    refetchInterval: 30_000,
    refetchOnWindowFocus: true,
  })
}

export function useAlertRulesList(showDisabled: boolean) {
  return useQuery({
    queryKey: ['alert-rules', showDisabled] as const,
    queryFn: () =>
      apiFetch<{ rules: RuleDTO[] }>(
        `/api/alerts/rules${showDisabled ? '?show_disabled=1' : ''}`,
      ),
  })
}

export function useAnomalyRoster() {
  return useQuery({
    queryKey: ['anomaly-roster'] as const,
    queryFn: () => apiFetch<{ rows: AnomalyRosterRow[] }>('/api/anomaly-roster'),
  })
}

export function useMPAnomalyState(mpId: string | undefined) {
  return useQuery({
    queryKey: ['mp-anomaly-state', mpId] as const,
    queryFn: () =>
      apiFetch<MPAnomalyState>(`/api/metering-points/${mpId}/anomaly-state`),
    enabled: Boolean(mpId),
  })
}

// ─── Mutations ─────────────────────────────────────────────────────────────

export function useAckMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: { id: string; note?: string }) => {
      return apiFetch(`/api/alerts/${input.id}/ack`, {
        method: 'POST',
        body: JSON.stringify({ note: input.note ?? '' }),
      })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['alerts'] })
    },
  })
}

export function useSnoozeMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: { id: string; duration: string }) => {
      return apiFetch(`/api/alerts/${input.id}/snooze`, {
        method: 'POST',
        body: JSON.stringify({ duration: input.duration }),
      })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['alerts'] })
    },
  })
}

export function useCreateRuleMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: Record<string, unknown>) => {
      return apiFetch<RuleDTO>('/api/alerts/rules', {
        method: 'POST',
        body: JSON.stringify(input),
      })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['alert-rules'] })
    },
  })
}

export function useToggleMPAnomalyRuleMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: { mpId: string; kind: string; enabled: boolean }) => {
      return apiFetch(`/api/metering-points/${input.mpId}/anomaly-rules/${input.kind}`, {
        method: 'PATCH',
        body: JSON.stringify({ enabled: input.enabled }),
      })
    },
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: ['mp-anomaly-state', vars.mpId] })
      qc.invalidateQueries({ queryKey: ['anomaly-roster'] })
    },
  })
}

export function useTestFireMutation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: { ruleId: string }) => {
      return apiFetch<AlertDTO>(`/api/alerts/rules/${input.ruleId}/test-fire`, {
        method: 'POST',
      })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['alerts'] })
    },
  })
}

// ─── Helpers ───────────────────────────────────────────────────────────────

function buildQuery(params: AlertsParams): string {
  const parts: string[] = []
  if (params.severity && params.severity !== 'all') parts.push(`severity=${params.severity}`)
  if (params.status && params.status !== 'all') parts.push(`status=${params.status}`)
  if (params.category && params.category !== 'all') parts.push(`category=${params.category}`)
  if (params.target_type && params.target_type !== 'all')
    parts.push(`target_type=${params.target_type}`)
  if (params.from) parts.push(`from=${encodeURIComponent(params.from)}`)
  if (params.to) parts.push(`to=${encodeURIComponent(params.to)}`)
  return parts.length ? `?${parts.join('&')}` : ''
}
