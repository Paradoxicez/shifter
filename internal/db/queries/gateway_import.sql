-- gateway_import.sql
-- Phase 7 Plan 13: bulk gateway import upsert query.
--
-- UpsertGatewayForBulkImport performs an idempotent upsert on gateway_id
-- (the 16-hex EUI). The (xmax = 0) trick distinguishes INSERT vs UPDATE on
-- conflict so the service can return "created" vs "updated" vs "skipped"
-- outcome per row.
--
-- "skipped" detection: after an UPDATE, the service compares name/description/
-- lat/lng/region against the RETURNING row. If all match the input, the row
-- was effectively a no-op (same data). The query always UPDATEs on conflict
-- to keep the logic simple; the service layer determines "skipped" vs "updated".

-- name: UpsertGatewayForBulkImport :one
INSERT INTO gateway (gateway_id, name, description, region, lat, lng, cs_tenant_id, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, now(), now())
ON CONFLICT (gateway_id) DO UPDATE
SET name        = EXCLUDED.name,
    description = EXCLUDED.description,
    region      = EXCLUDED.region,
    lat         = EXCLUDED.lat,
    lng         = EXCLUDED.lng,
    updated_at  = now()
RETURNING id, gateway_id, name, description, region, lat, lng, altitude, tags, cs_tenant_id,
          stats_refreshed_at, stats_rx_24h, stats_tx_24h, stats_tx_ok_24h, stats_sparkline,
          archived_at, archived_reason, archived_snapshot, created_at, updated_at,
          (xmax = 0) AS was_inserted;
