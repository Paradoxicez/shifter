-- Site (D-17 + D-20) — physical/logical hierarchy anchor for metering points
-- and devices. Soft-deleted via archived_at; hard delete blocked by FKs from
-- metering_point and (transitively) binding.

-- name: CreateSite :one
-- Plan 02-08 site dialog. Caller resolves parent_id from the parent picker
-- (NULL for top-level sites). timezone is required (D-17 — every site has
-- one; UI defaults to install timezone).
INSERT INTO site (parent_id, name, site_type, lat, lng, timezone, address, description)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetSite :one
SELECT * FROM site WHERE id = $1;

-- name: ListActiveSites :many
-- Site list page (D-20). archived_at IS NULL filter hits the partial index.
SELECT * FROM site WHERE archived_at IS NULL ORDER BY name ASC;

-- name: ListArchivedSites :many
-- Archive view (D-20) — sorted most-recently-archived first so admins see
-- their last action at the top.
SELECT * FROM site WHERE archived_at IS NOT NULL ORDER BY archived_at DESC;

-- name: ListChildSites :many
-- Site detail page sub-section: direct children only (one level), so the
-- breadcrumb expands lazily rather than fetching the whole subtree.
SELECT * FROM site WHERE parent_id = $1 AND archived_at IS NULL ORDER BY name ASC;

-- name: UpdateSite :one
-- Plan 02-08 site edit dialog. parent_id intentionally NOT updatable here —
-- moving a site between parents is a separate "reparent" flow with audit
-- implications and is deferred to Phase 6.
UPDATE site SET
    name = $2, site_type = $3, lat = $4, lng = $5,
    timezone = $6, address = $7, description = $8
WHERE id = $1
RETURNING *;

-- name: ArchiveSite :one
-- D-20 soft-delete. Idempotent guard `archived_at IS NULL` — re-archiving an
-- already-archived site returns no row (caller treats as no-op).
UPDATE site SET archived_at = now()
WHERE id = $1 AND archived_at IS NULL
RETURNING *;

-- name: RestoreSite :one
-- D-20 restore from archive view. Symmetric guard.
UPDATE site SET archived_at = NULL
WHERE id = $1 AND archived_at IS NOT NULL
RETURNING *;

-- name: CountMPsOnSite :one
-- Site detail header badge "N metering points". Used by Plan 02-08 site detail
-- page; counts only active MPs (D-20 — archived MPs hide from default views).
SELECT count(*) FROM metering_point WHERE site_id = $1 AND archived_at IS NULL;
