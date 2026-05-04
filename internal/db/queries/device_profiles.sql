-- Device Profile (D-01 layer 2 + D-04 capabilities + D-05 counter_modulus +
-- D-09 codec sync state). Seeded by 0010 with three vendor profiles
-- (axioma_w1, acrel_adl200, acrel_adw300); the bootstrap routine in
-- internal/profile/seed.go (Plan 02-08) reads //go:embed-ed *.js codec
-- bodies, pushes them to ChirpStack, and back-fills cs_profile_id +
-- codec_js_synced_at via MarkProfileSyncedToChirpStack.

-- name: CreateDeviceProfile :one
-- Plan 02-08 profile editor "create new profile" path (rare in practice;
-- seeded profiles cover Phase 2 — operator-authored profiles unlock in
-- Phase 6). codec_js may be empty at creation; the seed routine fills it.
INSERT INTO device_profile (
    slug, name, vendor, family, capabilities, counter_modulus,
    codec_js, region, mac_version
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetDeviceProfile :one
SELECT * FROM device_profile WHERE id = $1;

-- name: GetDeviceProfileBySlug :one
-- Plan 02-08 seed routine + Plan 02-08 profile editor URL routing
-- (`/profiles/axioma_w1`).
SELECT * FROM device_profile WHERE slug = $1;

-- name: ListActiveDeviceProfiles :many
-- Profile list page + device-create dialog dropdown. Sorted vendor-then-name
-- so users see Acrel/Axioma grouped.
SELECT * FROM device_profile
WHERE archived_at IS NULL
ORDER BY vendor ASC, name ASC;

-- name: UpdateDeviceProfile :one
-- Plan 02-08 profile editor save path. capabilities + counter_modulus + codec
-- are all editable; mappings are stored in device_profile_mapping (separate
-- queries — see device_profile_mappings.sql). cs_profile_id is intentionally
-- NOT updatable here; only MarkProfileSyncedToChirpStack writes that column.
UPDATE device_profile SET
    name = $2, vendor = $3, family = $4,
    capabilities = $5, counter_modulus = $6,
    codec_js = $7, region = $8, mac_version = $9
WHERE id = $1
RETURNING *;

-- name: ArchiveDeviceProfile :one
-- D-20 soft-delete for profiles. Note: device.device_profile_id has
-- ON DELETE RESTRICT, so the row stays referenceable even when archived
-- (existing devices keep working; archive only hides from the create-device
-- dropdown).
UPDATE device_profile SET archived_at = now()
WHERE id = $1 AND archived_at IS NULL
RETURNING *;

-- name: MarkProfileSyncedToChirpStack :exec
-- Plan 02-08 seed routine — called after a successful CS DeviceProfileService
-- Create/Update gRPC call. Records the CS-side UUID + sync timestamp so the
-- next boot's ListUnsyncedProfiles query no longer returns this row.
UPDATE device_profile
SET cs_profile_id = $2, codec_js_synced_at = now()
WHERE id = $1;

-- name: ListUnsyncedProfiles :many
-- Plan 02-08 boot-time seed routine reads this list to decide which profiles
-- need a CS push. A profile is "unsynced" if it has no cs_profile_id (never
-- pushed) OR the codec_js was modified after the last push (codec_js_synced_at
-- is NULL after a SetProfileCodecJS write — the seed routine sets it back
-- to now() once CS confirms the new codec).
SELECT * FROM device_profile
WHERE archived_at IS NULL
  AND (cs_profile_id IS NULL OR codec_js_synced_at IS NULL);

-- name: SetProfileCodecJS :exec
-- Plan 02-08 seed routine — writes the //go:embed-ed codec body into the row
-- and clears codec_js_synced_at so the next pass re-pushes to ChirpStack.
UPDATE device_profile
SET codec_js = $2, codec_js_synced_at = NULL
WHERE id = $1;
