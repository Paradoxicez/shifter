-- Metering Point (D-19 + D-20) — the canonical "thing being measured" that
-- persists across physical meter swaps (DATA-01 invariant). MP is the join
-- key for telemetry; binding history captures which device fed it when.

-- name: CreateMP :one
-- Plan 02-08 MP create dialog. (site_id, name) UNIQUE constraint surfaces
-- as a 23505 error to the caller — handler maps to a friendly toast.
INSERT INTO metering_point (site_id, name, utility_class, location_description)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetMP :one
SELECT * FROM metering_point WHERE id = $1;

-- name: ListActiveMPs :many
-- Global MP list (rare — usually scoped by site). archived_at filter hits the
-- partial index.
SELECT * FROM metering_point WHERE archived_at IS NULL ORDER BY name ASC;

-- name: ListMPsBySite :many
-- Site detail MP-list section. Active-only by default; site_idx covers the
-- filter, archived_at NULL is the common case.
SELECT * FROM metering_point
WHERE site_id = $1 AND archived_at IS NULL
ORDER BY name ASC;

-- name: ListArchivedMPs :many
SELECT * FROM metering_point WHERE archived_at IS NOT NULL ORDER BY archived_at DESC;

-- name: UpdateMP :one
-- Plan 02-08 MP edit. utility_class IS editable here (operators sometimes
-- mis-classify; the CHECK constraint still bounds the values to water |
-- electricity).
UPDATE metering_point SET
    name = $2, utility_class = $3, location_description = $4
WHERE id = $1
RETURNING *;

-- name: ArchiveMP :one
-- D-20 soft-delete. Idempotent guard.
UPDATE metering_point SET archived_at = now()
WHERE id = $1 AND archived_at IS NULL
RETURNING *;

-- name: RestoreMP :one
UPDATE metering_point SET archived_at = NULL
WHERE id = $1 AND archived_at IS NOT NULL
RETURNING *;

-- name: GetMPWithActiveBinding :one
-- MP detail page (Plan 02-08): shows "currently bound device + reading offset".
-- LEFT JOINs return NULL for binding/device/profile columns when no active
-- binding exists (a fresh MP that hasn't been wired up yet, or a swap-in-
-- progress where the previous binding closed and the next hasn't opened).
SELECT mp.id, mp.site_id, mp.name, mp.utility_class, mp.location_description,
       mp.archived_at, mp.created_at, mp.updated_at,
       b.id  AS binding_id,
       b.device_id,
       b.valid_from AS binding_valid_from,
       b.reading_offset,
       d.dev_eui,
       d.name AS device_name,
       dp.id AS device_profile_id,
       dp.name AS device_profile_name,
       dp.capabilities,
       dp.counter_modulus
FROM metering_point mp
LEFT JOIN binding         b  ON b.metering_point_id = mp.id AND b.valid_to IS NULL
LEFT JOIN device          d  ON d.id = b.device_id
LEFT JOIN device_profile  dp ON dp.id = d.device_profile_id
WHERE mp.id = $1;
