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

-- name: ListProfilesWithCatalogMetadata :many
-- Plan 07-02 — Vendor Catalog Settings tab reads this to render the table.
SELECT
    id, slug, name, vendor, family, capabilities,
    codec_js_synced_at,
    catalog_source, catalog_source_version, customer_edited,
    battery_curve, expected_uplink_interval_seconds,
    offline_threshold_multiplier, anomaly_compatibility,
    counter_modulus, mac_version, region,
    created_at, updated_at
FROM device_profile
ORDER BY vendor ASC, family ASC;

-- name: GetProfileCatalogMetadata :one
SELECT
    id, slug, name, vendor, family, capabilities,
    codec_js,
    catalog_source, catalog_source_version, customer_edited,
    battery_curve, expected_uplink_interval_seconds,
    offline_threshold_multiplier, anomaly_compatibility
FROM device_profile
WHERE id = $1;

-- name: SetProfileCatalogSource :exec
-- Plan 07-04 invokes this after a catalog Import or Update succeeds.
UPDATE device_profile
SET catalog_source         = $2,
    catalog_source_version = $3,
    customer_edited        = $4,
    updated_at             = now()
WHERE id = $1;

-- name: MarkProfileCustomerEdited :exec
-- Plan 07-04 invokes this when an operator saves an edit to a catalog-sourced
-- profile so future catalog Updates show the "you edited this" flag.
UPDATE device_profile
SET customer_edited = TRUE,
    updated_at      = now()
WHERE id = $1;

-- name: ApplyCatalogUpdate :exec
-- Plan 07-04 catalog Update flow: writes the merged fields and clears
-- codec_js_synced_at to NULL so the Phase 2 seed routine re-pushes to
-- ChirpStack (D-36).
UPDATE device_profile
SET catalog_source_version           = $2,
    codec_js                         = $3,
    capabilities                     = $4,
    battery_curve                    = $5,
    expected_uplink_interval_seconds = $6,
    offline_threshold_multiplier     = $7,
    anomaly_compatibility            = $8,
    counter_modulus                  = $9,
    mac_version                      = $10,
    region                           = $11,
    codec_js_synced_at               = NULL,
    updated_at                       = now()
WHERE id = $1;

-- name: CreateDeviceProfileFromCatalog :one
-- Plan 07-04 ImportFromCatalogHandler calls this when an operator imports a
-- catalog entry. Inserts a NEW device_profile row populated from the catalog
-- entry; customer_edited starts FALSE; codec_js_synced_at is left NULL so the
-- Phase 2 seed routine pushes the codec to ChirpStack on next pass.
-- Param order matches plan 07-04 Task 2 db.CreateDeviceProfileFromCatalogParams struct.
INSERT INTO device_profile (
    name,
    codec_js,
    capabilities,
    catalog_source,
    catalog_source_version,
    battery_curve,
    expected_uplink_interval_seconds,
    offline_threshold_multiplier,
    anomaly_compatibility,
    customer_edited,
    slug,
    vendor,
    family,
    counter_modulus,
    mac_version,
    region,
    codec_js_synced_at,
    created_at,
    updated_at
) VALUES (
    $1,           -- name
    $2,           -- codec_js
    $3,           -- capabilities (text[])
    $4,           -- catalog_source (slug)
    $5,           -- catalog_source_version
    $6,           -- battery_curve
    $7,           -- expected_uplink_interval_seconds
    $8,           -- offline_threshold_multiplier
    $9,           -- anomaly_compatibility
    FALSE,        -- customer_edited starts FALSE on fresh import
    $10,          -- slug
    $11,          -- vendor
    $12,          -- family
    $13,          -- counter_modulus
    $14,          -- mac_version
    $15,          -- region (nullable TEXT)
    NULL,         -- codec_js_synced_at; Phase 2 seed pushes to ChirpStack
    now(),
    now()
)
RETURNING id, updated_at;

-- name: GetProfileForCodecTest :one
-- Plan 07-07: codec test handler loads codec_js from the profile row.
-- Returns only the columns needed so the handler avoids a full DeviceProfile scan.
SELECT id, slug, codec_js
FROM device_profile WHERE id = $1;

-- name: ListCatalogProfilesForDriftCheck :many
-- Plan 07-03 RunCatalogDriftCheck: list profiles that were imported from the
-- catalog (catalog_source IS NOT NULL) and have not been customer-edited, so
-- the drift check can compare codec_js hashes to the embedded catalog source.
SELECT id, slug, codec_js, catalog_source, customer_edited
FROM device_profile
WHERE catalog_source IS NOT NULL AND customer_edited = FALSE;

-- name: OverwriteProfileCodec :exec
-- Plan 07-03 RunCatalogDriftCheck uses this to replace the Itron+KINMY
-- migration placeholder with the real embedded codec source. Clears
-- codec_js_synced_at so the Phase 2 seed routine re-pushes to ChirpStack.
UPDATE device_profile
SET codec_js           = $2,
    codec_js_synced_at = NULL,
    updated_at         = now()
WHERE id = $1;
