-- Device — physical LoRaWAN endpoint (D-15 + D-25 + DEV-09). dev_eui is the
-- LoRaWAN-canonical 16-char lowercase hex string (CS uses lowercase across
-- v4 gRPC + MQTT topics — Plan 01-12). Schema CHECK enforces both the
-- lowercase + hex16 invariants.

-- name: CreateDevice :one
-- Plan 02-08 device dialog. cs_device_uuid filled by Plan 02-05 wrapper after
-- the ChirpStack DeviceService.Create gRPC ack. join_eui optional; AppKey is
-- DELIBERATELY NOT stored here (DEV-09 — secret lives in ChirpStack only).
INSERT INTO device (dev_eui, name, device_profile_id, cs_device_uuid, join_eui, description)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetDevice :one
SELECT * FROM device WHERE id = $1;

-- name: GetDeviceByDevEUI :one
-- Used by Plan 02-07 swap commit to find the incoming device by EUI before
-- opening the new binding. Caller MUST pass lower-cased EUI; the schema CHECK
-- guards but the unique-index hit needs the lowercase form.
SELECT * FROM device WHERE dev_eui = $1;

-- name: ListActiveDevices :many
-- Devices list page (Plan 02-08 device list). Sorted last_seen_at DESC NULLS
-- LAST so freshly-uplinking devices float to the top; LIMIT/OFFSET for the
-- paginated list. Phase 2 minimal — device counts under 100 for v1 installs.
SELECT * FROM device
WHERE decommissioned_at IS NULL
ORDER BY last_seen_at DESC NULLS LAST
LIMIT $1 OFFSET $2;

-- name: SearchDevices :many
-- Devices list page search (Phase 2 minimal: name OR dev_eui). The dev_eui
-- ILIKE branch lower()s the input so the user can paste an EUI in any case
-- and still hit the schema's lowercase-only column. T-02-06-03: this is
-- unindexed by design for Phase 2's sub-100-device installs; Phase 3 DEV-01
-- will add proper FTS or trigram index when device counts cross ~1K.
SELECT * FROM device
WHERE decommissioned_at IS NULL
  AND (name ILIKE '%' || $1 || '%' OR dev_eui ILIKE '%' || lower($1) || '%')
ORDER BY last_seen_at DESC NULLS LAST
LIMIT $2 OFFSET $3;

-- name: ListDevicesBySite :many
-- Site detail page section: devices currently bound to MPs on this site.
-- The JOIN through binding (active only) → metering_point handles the
-- "device → site" association which is otherwise indirect (devices have no
-- direct site_id; that's by design — D-15 + DATA-01).
SELECT d.* FROM device d
JOIN binding b ON b.device_id = d.id AND b.valid_to IS NULL
JOIN metering_point mp ON mp.id = b.metering_point_id
WHERE mp.site_id = $1 AND d.decommissioned_at IS NULL
ORDER BY d.last_seen_at DESC NULLS LAST;

-- name: UpdateDevice :one
-- Plan 02-08 device edit dialog. dev_eui + device_profile_id are
-- intentionally NOT updatable here — changing either is a "swap" workflow
-- (close the binding, decommission/recreate device) per D-15.
UPDATE device SET
    name = $2, join_eui = $3, description = $4
WHERE id = $1
RETURNING *;

-- name: UpdateDeviceLastSeen :exec
-- Called by ingest pipeline (Plan 02-09) on every successful uplink. The
-- per-device write rate is bounded (one row per uplink ≤ once per minute
-- for the highest-frequency Phase 2 vendor); no need to batch.
UPDATE device SET last_seen_at = $2 WHERE id = $1;

-- name: SetDeviceCSUUID :exec
-- Called by Plan 02-05 atomic-create-rollback path after ChirpStack confirms
-- the device — fills the cs_device_uuid column so subsequent gRPC calls can
-- address by UUID rather than dev_eui.
UPDATE device SET cs_device_uuid = $2 WHERE id = $1;

-- name: DecommissionDevice :one
-- D-15 decommission: marks the device retired. The active binding closure
-- is a separate transaction (Plan 02-07 swap.commit calls CloseBinding)
-- because the binding may already be closed when the operator decommissions
-- (or vice versa); the application layer composes them.
UPDATE device SET decommissioned_at = now()
WHERE id = $1 AND decommissioned_at IS NULL
RETURNING *;

-- name: ListDevicesFiltered :many
-- Phase 3 D-12..D-18: server-side filter/sort/page for the Devices list.
-- Site join via active binding (no current_site_id denormalisation per
-- RESEARCH Open Q #2). The device schema's soft-delete column is
-- decommissioned_at (see migration 0012), not archived_at — the plan-level
-- "archived_at" wording refers to "live" devices and maps to
-- `decommissioned_at IS NULL` here.
-- Filter args:
--   $1 = site_ids UUID[] — empty = no filter
--   $2 = status_filter TEXT — 'active'|'inactive'|'never_joined'|'' (empty = no filter)
--   $3 = last_seen_cutoff TIMESTAMPTZ NULL — devices with last_seen_at >= cutoff (or any if NULL)
--   $4 = text_q TEXT — '' = no filter; matches name ILIKE %q% OR dev_eui ILIKE %q%
--   $5 = sort_col TEXT — 'name'|'dev_eui'|'site'|'last_seen'|'created_at'
--   $6 = sort_desc BOOL
--   $7 = limit_n INT
--   $8 = offset_n INT
WITH active_bindings AS (
    SELECT b.device_id, mp.site_id, s.name AS site_name
    FROM binding b
    JOIN metering_point mp ON mp.id = b.metering_point_id
    JOIN site s ON s.id = mp.site_id
    WHERE b.valid_to IS NULL
)
SELECT
    d.id,
    d.dev_eui,
    d.name,
    d.device_profile_id,
    d.cs_device_uuid,
    d.join_eui,
    d.description,
    d.last_seen_at,
    d.decommissioned_at,
    d.created_at,
    d.updated_at,
    ab.site_id   AS current_site_id,
    ab.site_name AS current_site_name,
    dp.name AS device_profile_name,
    dp.expected_interval_s
FROM device d
LEFT JOIN active_bindings ab ON ab.device_id = d.id
LEFT JOIN device_profile dp ON dp.id = d.device_profile_id
WHERE d.decommissioned_at IS NULL
  AND (
        cardinality($1::uuid[]) = 0
        OR ab.site_id = ANY($1::uuid[])
      )
  AND (
        $2::text = ''
        OR ($2 = 'active'        AND d.last_seen_at IS NOT NULL AND d.last_seen_at >  now() - interval '24 hours')
        OR ($2 = 'inactive'      AND d.last_seen_at IS NOT NULL AND d.last_seen_at <= now() - interval '24 hours')
        OR ($2 = 'never_joined'  AND d.last_seen_at IS NULL)
      )
  AND ($3::timestamptz IS NULL OR d.last_seen_at >= $3)
  AND (
        $4::text = ''
        OR d.name    ILIKE '%' || $4 || '%'
        OR d.dev_eui ILIKE '%' || $4 || '%'
      )
ORDER BY
    CASE WHEN $5::text = 'name'        AND $6::bool = false THEN d.name             END ASC,
    CASE WHEN $5::text = 'name'        AND $6::bool = true  THEN d.name             END DESC,
    CASE WHEN $5::text = 'dev_eui'     AND $6::bool = false THEN d.dev_eui          END ASC,
    CASE WHEN $5::text = 'dev_eui'     AND $6::bool = true  THEN d.dev_eui          END DESC,
    CASE WHEN $5::text = 'site'        AND $6::bool = false THEN ab.site_name       END ASC NULLS LAST,
    CASE WHEN $5::text = 'site'        AND $6::bool = true  THEN ab.site_name       END DESC NULLS LAST,
    CASE WHEN $5::text = 'last_seen'   AND $6::bool = false THEN d.last_seen_at     END ASC NULLS LAST,
    CASE WHEN $5::text = 'last_seen'   AND $6::bool = true  THEN d.last_seen_at     END DESC NULLS LAST,
    CASE WHEN $5::text = 'created_at'  AND $6::bool = false THEN d.created_at       END ASC,
    CASE WHEN $5::text = 'created_at'  AND $6::bool = true  THEN d.created_at       END DESC,
    d.last_seen_at DESC NULLS LAST,
    d.id ASC
LIMIT $7 OFFSET $8;

-- name: CountDevicesFiltered :one
-- Same WHERE clause as ListDevicesFiltered for accurate total_count.
WITH active_bindings AS (
    SELECT b.device_id, mp.site_id
    FROM binding b
    JOIN metering_point mp ON mp.id = b.metering_point_id
    WHERE b.valid_to IS NULL
)
SELECT COUNT(*)
FROM device d
LEFT JOIN active_bindings ab ON ab.device_id = d.id
WHERE d.decommissioned_at IS NULL
  AND (cardinality($1::uuid[]) = 0 OR ab.site_id = ANY($1::uuid[]))
  AND (
        $2::text = ''
        OR ($2 = 'active'        AND d.last_seen_at IS NOT NULL AND d.last_seen_at >  now() - interval '24 hours')
        OR ($2 = 'inactive'      AND d.last_seen_at IS NOT NULL AND d.last_seen_at <= now() - interval '24 hours')
        OR ($2 = 'never_joined'  AND d.last_seen_at IS NULL)
      )
  AND ($3::timestamptz IS NULL OR d.last_seen_at >= $3)
  AND (
        $4::text = ''
        OR d.name    ILIKE '%' || $4 || '%'
        OR d.dev_eui ILIKE '%' || $4 || '%'
      );
