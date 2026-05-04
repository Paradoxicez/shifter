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
