import { apiFetch } from './api'

/**
 * Typed client for /api/gateways/* endpoints (Plan 03-04 backend).
 *
 * Backend handler shape (internal/gateway/handlers.go):
 *   GET   /api/gateways                  → ListGatewaysResponse
 *     ?limit=&offset=&order_by=&order_desc=&include_archived=
 *   GET   /api/gateways/:id              → Gateway
 *   POST  /api/gateways                  → Gateway (admin)
 *   PATCH /api/gateways/:id              → Gateway (admin)
 *   POST  /api/gateways/:id/archive      → Gateway (admin) — body {reason}
 *   POST  /api/gateways/:id/restore      → Gateway (admin)
 *
 * Response shape — Phase 3 Plan 03-04 contract. `stats_*` fields are cached
 * (D-02, 1 min TTL); `last_seen_at` + `state` are merged live from
 * ChirpStack list response.
 */

export interface GatewaySparkline {
  bucket_start: string
  bucket_width_s: number
  rx: number[]
  tx: number[]
}

export interface Gateway {
  id: string
  gateway_id: string
  name: string
  description: string | null
  region: string
  lat: number | null
  lng: number | null
  altitude: number | null
  tags: Record<string, string>
  archived_at: string | null
  archived_reason: string | null
  created_at: string
  updated_at: string
  // Cached stats (D-02):
  stats_refreshed_at: string | null
  stats_rx_24h: number | null
  stats_tx_24h: number | null
  stats_tx_ok_24h: number | null
  stats_sparkline: GatewaySparkline | null
  // Live (from CS list response merged):
  last_seen_at: string | null
  state: 'NEVER_SEEN' | 'ONLINE' | 'OFFLINE' | null
}

export interface ListGatewaysResponse {
  total: number
  items: Gateway[]
}

export interface ListGatewaysOptions {
  limit?: number
  offset?: number
  includeArchived?: boolean
  orderBy?: string
  orderDesc?: boolean
}

export async function listGateways(
  opts: ListGatewaysOptions = {},
): Promise<ListGatewaysResponse> {
  const sp = new URLSearchParams()
  if (opts.limit !== undefined) sp.set('limit', String(opts.limit))
  if (opts.offset !== undefined) sp.set('offset', String(opts.offset))
  if (opts.includeArchived) sp.set('include_archived', 'true')
  if (opts.orderBy) sp.set('order_by', opts.orderBy)
  if (opts.orderDesc) sp.set('order_desc', 'true')
  const qs = sp.toString()
  return apiFetch<ListGatewaysResponse>(
    qs ? `/api/gateways?${qs}` : '/api/gateways',
  )
}

export async function getGateway(id: string): Promise<Gateway> {
  return apiFetch<Gateway>(`/api/gateways/${encodeURIComponent(id)}`)
}

export interface CreateGatewayRequest {
  gateway_id: string
  name: string
  description?: string
  region?: string
  lat?: number | null
  lng?: number | null
  altitude?: number | null
  tags?: Record<string, string>
}

export async function createGateway(req: CreateGatewayRequest): Promise<Gateway> {
  return apiFetch<Gateway>('/api/gateways', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

export type UpdateGatewayRequest = Partial<Omit<CreateGatewayRequest, 'gateway_id'>>

export async function updateGateway(
  id: string,
  req: UpdateGatewayRequest,
): Promise<Gateway> {
  return apiFetch<Gateway>(`/api/gateways/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body: JSON.stringify(req),
  })
}

export async function archiveGateway(id: string, reason: string): Promise<Gateway> {
  return apiFetch<Gateway>(
    `/api/gateways/${encodeURIComponent(id)}/archive`,
    {
      method: 'POST',
      body: JSON.stringify({ reason }),
    },
  )
}

export async function restoreGateway(id: string): Promise<Gateway> {
  return apiFetch<Gateway>(
    `/api/gateways/${encodeURIComponent(id)}/restore`,
    { method: 'POST' },
  )
}
