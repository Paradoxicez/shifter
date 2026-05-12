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
