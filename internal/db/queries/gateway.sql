-- Gateway CRUD + stats cache + decommission. Phase 3 D-29..D-32.
--
-- D-30 verbatim (user decision 2026-05-11): soft-delete in Postgres (via
-- archived_at + archived_reason + archived_snapshot) is the PG half of the
-- atomic decommission tx; the handler issues ChirpStack DeleteGateway in the
-- same Serializable tx. archived_snapshot stores the CS Gateway proto as
-- JSON so RestoreGateway can faithfully re-create.
--
-- Phase 3 simplification: the per-gateway "X devices in last 24h" warning
-- count (D-31) is NOT served from a sqlc query here because the measurement
-- hypertable (0015_measurement.up.sql) has no `gateway_id` column yet —
-- D-31 warning copy ships without device count. Phase 4 telemetry ingest
-- adds `measurement.gateway_id` and at that point the count query lands
-- alongside this file.

-- name: CreateGateway :one
INSERT INTO gateway (gateway_id, name, description, region, lat, lng, altitude, tags, cs_tenant_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetGateway :one
SELECT * FROM gateway WHERE id = $1;

-- name: GetGatewayByGatewayID :one
-- Lowercase EUI lookup (matches Phase 2 dev_eui pattern). Caller MUST pass
-- the lowercase form — the schema CHECK enforces it on storage.
SELECT * FROM gateway WHERE gateway_id = $1;

-- name: ListGatewaysActive :many
-- D-32 default view: hides archived rows. Phase 3 ships a stable
-- created_at DESC ordering (the gateway list is small — ≤200 per page —
-- and operator workflows expect "newest first"). The handler exposes
-- sort/filter via URL params but currently maps them all to this query;
-- richer ordering arrives with the Phase 4 gateway dashboard.
SELECT * FROM gateway
WHERE archived_at IS NULL
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: ListGatewaysIncludingArchived :many
-- D-32 "Show archived" toggle. archived rows sorted by archived_at DESC
-- first, then active rows by created_at DESC.
SELECT * FROM gateway
ORDER BY archived_at DESC NULLS LAST, created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountGatewaysActive :one
SELECT COUNT(*) FROM gateway WHERE archived_at IS NULL;

-- name: UpdateGateway :one
UPDATE gateway SET
    name = $2,
    description = $3,
    region = $4,
    lat = $5, lng = $6, altitude = $7,
    tags = $8
WHERE id = $1 AND archived_at IS NULL
RETURNING *;

-- name: ArchiveGateway :one
-- D-30 verbatim (user decision 2026-05-11): soft-delete in Postgres
-- (archived_at, archived_reason, archived_snapshot) + ChirpStack
-- DeleteGateway called by the handler in the SAME transaction. Snapshot
-- preserves the CS proto (protojson) so RestoreGateway can faithfully
-- re-create.
UPDATE gateway SET archived_at = now(), archived_reason = $2, archived_snapshot = $3
WHERE id = $1 AND archived_at IS NULL
RETURNING *;

-- name: RestoreGateway :one
-- Restore reads archived_snapshot and the handler calls
-- chirpstackClient.CreateGateway with it; this UPDATE clears the archive
-- columns atomic with the CS recreate.
UPDATE gateway SET archived_at = NULL, archived_reason = NULL, archived_snapshot = NULL
WHERE id = $1 AND archived_at IS NOT NULL
RETURNING *;

-- name: UpdateGatewayStatsCache :exec
-- Called by cache_refresher.go after a successful GetMetrics fetch (D-02).
-- $6 is the chirpstack-reported last_seen_at (NULL when never_seen). Keeping
-- last_seen_at on the same cache row means a single read covers status badge
-- + 24h stats for the gateway list page (no separate query path).
UPDATE gateway SET
    stats_refreshed_at = now(),
    stats_rx_24h       = $2,
    stats_tx_24h       = $3,
    stats_tx_ok_24h    = $4,
    stats_sparkline    = $5,
    last_seen_at       = $6
WHERE id = $1;
