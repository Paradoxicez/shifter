-- floor_plan.sql — Phase 5 SITE-02/03 (D-16 schema + D-18 image storage).
-- Serves upload (CreateFloorPlan), listing (ListFloorPlansBySite), detail
-- (GetFloorPlan), image replace (UpdateFloorPlanImage), rename (UpdateFloorPlanLabel),
-- delete (DeleteFloorPlan), and pin-count (CountPinsOnFloorPlan).

-- name: CreateFloorPlan :one
-- Upload handler inserts immediately after the image bytes are written to
-- disk (before tx commit). sort_order defaults to the next slot; UNIQUE
-- (site_id, sort_order) rejects duplicates with a 23505 error.
INSERT INTO floor_plan (id, site_id, label, sort_order, image_path, image_w, image_h)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListFloorPlansBySite :many
-- Site detail — Floor plan tab. Ordered by sort_order then uploaded_at so
-- ties resolve deterministically (upload order breaks ties within the same
-- sort_order — though UNIQUE prevents true dupes within a site).
SELECT id, site_id, label, sort_order, image_path, image_w, image_h, uploaded_at, updated_at
FROM floor_plan
WHERE site_id = $1
ORDER BY sort_order, uploaded_at;

-- name: GetFloorPlan :one
SELECT id, site_id, label, sort_order, image_path, image_w, image_h, uploaded_at, updated_at
FROM floor_plan
WHERE id = $1;

-- name: UpdateFloorPlanImage :one
-- PATCH /api/floor-plans/:id — replaces the image file while keeping the
-- label/sort_order and ALL device_floor_plan_placement rows intact (D-24).
-- Handler writes the new file, calls UpdateFloorPlanImage, then unlinks the
-- old file on commit (on rollback the new file is unlinked).
UPDATE floor_plan
SET image_path = $2, image_w = $3, image_h = $4, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateFloorPlanLabel :one
-- PATCH /api/floor-plans/:id/label — renames + optionally reorders a plan.
-- Conflict on UNIQUE (site_id, sort_order) if another plan already holds the
-- new sort_order → handler returns 409.
UPDATE floor_plan
SET label = $2, sort_order = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteFloorPlan :exec
-- DELETE /api/floor-plans/:id. Cascades to device_floor_plan_placement (0033).
-- Handler writes audit row inside the same tx before committing.
DELETE FROM floor_plan WHERE id = $1;

-- name: CountPinsOnFloorPlan :one
-- Used by the PATCH replace-image handler to build the D-24 confirmation
-- dialog copy: "Existing {N} pins will be kept at the same fractional
-- positions on the new image."
SELECT count(*) FROM device_floor_plan_placement WHERE floor_plan_id = $1;

-- name: UpsertPlacement :one
-- INSERT or UPDATE — device_id is PK (a device pins to one plan at a time).
-- ON CONFLICT on device_id moves the pin to the new plan (SITE-04 UPSERT).
INSERT INTO device_floor_plan_placement (device_id, floor_plan_id, x_frac, y_frac)
VALUES ($1, $2, $3, $4)
ON CONFLICT (device_id) DO UPDATE
SET floor_plan_id = EXCLUDED.floor_plan_id,
    x_frac        = EXCLUDED.x_frac,
    y_frac        = EXCLUDED.y_frac
RETURNING *;

-- name: UpdatePlacement :one
-- Drag-to-nudge: only x_frac / y_frac change; floor_plan_id stays.
UPDATE device_floor_plan_placement
SET x_frac = $2, y_frac = $3
WHERE device_id = $1
RETURNING *;

-- name: DeletePlacementByDevice :exec
-- Called by both right-click remove AND device decommission (D-25).
DELETE FROM device_floor_plan_placement WHERE device_id = $1;

-- name: ListPlacementsByPlan :many
-- Returns placements joined with the device + device_profile data needed for
-- client-side D-22 health computation (state colors) without a follow-up call.
-- battery_pct and rssi come from the latest measurement row for the active
-- metering point (measurement has no device_id per DATA-01 invariant).
SELECT
  p.device_id,
  p.floor_plan_id,
  p.x_frac,
  p.y_frac,
  p.created_at,
  d.name           AS device_name,
  d.last_seen_at,
  mp.id            AS metering_point_id,
  mp.utility_class,
  dp.expected_interval_s,
  latest.battery_pct,
  latest.rssi
FROM device_floor_plan_placement p
JOIN device d        ON d.id = p.device_id
LEFT JOIN binding b  ON b.device_id = d.id AND b.valid_to IS NULL
LEFT JOIN metering_point mp ON mp.id = b.metering_point_id
JOIN device_profile dp ON dp.id = d.device_profile_id
LEFT JOIN LATERAL (
  SELECT battery_pct, rssi
  FROM measurement
  WHERE metering_point_id = mp.id
  ORDER BY time DESC
  LIMIT 1
) latest ON mp.id IS NOT NULL
WHERE p.floor_plan_id = $1
  AND d.decommissioned_at IS NULL;

-- name: GetPlacementByDevice :one
SELECT * FROM device_floor_plan_placement WHERE device_id = $1;

-- name: GetDeviceSiteID :one
-- Helper for the same-site integrity check: returns the device's site via
-- its active binding's metering_point.
SELECT mp.site_id
FROM device d
LEFT JOIN binding b ON b.device_id = d.id AND b.valid_to IS NULL
LEFT JOIN metering_point mp ON mp.id = b.metering_point_id
WHERE d.id = $1
  AND d.decommissioned_at IS NULL;
