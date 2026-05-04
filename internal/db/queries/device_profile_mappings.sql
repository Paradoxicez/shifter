-- Device Profile Mapping (D-08 + DATA-09) — rows from decoded JSON to
-- canonical measurement columns. The Plan 02-08 profile editor saves the
-- whole mapping list at once: DeleteMappingsByProfile then a sequence of
-- CreateMapping calls inside a single transaction (replace-all semantics
-- so the editor never leaves a half-saved mapping set).

-- name: CreateMapping :one
-- Plan 02-08 profile editor save (per-row insert). UNIQUE (device_profile_id,
-- target) surfaces as 23505 if the editor lets two rows claim the same
-- canonical column — handler maps to a friendly error.
INSERT INTO device_profile_mapping (
    device_profile_id, json_pointer, target, scale, data_type, position
)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetMapping :one
SELECT * FROM device_profile_mapping WHERE id = $1;

-- name: ListMappingsByProfile :many
-- Plan 02-09 normalize engine: fetches the mapping table once at boot and
-- caches per-profile (cache invalidates on profile save). Position ASC drives
-- deterministic mapping pass order — required when a later mapping references
-- a value materialized by an earlier one.
SELECT * FROM device_profile_mapping
WHERE device_profile_id = $1
ORDER BY position ASC, target ASC;

-- name: DeleteMappingsByProfile :exec
-- Plan 02-08 profile editor save: wipes existing mappings before bulk
-- re-insert. Wrapped in a tx with the subsequent CreateMapping calls so
-- the editor never observes a half-saved state.
DELETE FROM device_profile_mapping WHERE device_profile_id = $1;

-- name: CountMappingsByProfile :one
-- Profile list page badge "N mappings" — quick count without fetching rows.
SELECT count(*) FROM device_profile_mapping WHERE device_profile_id = $1;

-- name: DeleteMapping :exec
-- Single-row delete — used by Phase 6 admin actions; Phase 2's editor uses
-- the bulk DeleteMappingsByProfile path.
DELETE FROM device_profile_mapping WHERE id = $1;
