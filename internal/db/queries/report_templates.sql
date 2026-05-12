-- report_templates.sql — Phase 7 Plan 11a: Saved Report Templates (UX-POWER Surface 6).
--
-- All 5 queries target the report_template table added by migration 0053.
-- RBAC enforcement is at the HTTP handler layer; these queries have no auth logic.

-- name: ListReportTemplates :many
-- Returns all report templates ordered case-insensitively by name ascending.
-- The lower(name) functional index (0053) makes this sort O(log N).
SELECT id, name, description, state, created_by, created_at, updated_at
FROM report_template
ORDER BY lower(name) ASC;

-- name: GetReportTemplate :one
-- Fetches a single report template by UUID primary key.
-- Returns pgx.ErrNoRows when the template does not exist.
SELECT id, name, description, state, created_by, created_at, updated_at
FROM report_template
WHERE id = $1;

-- name: CreateReportTemplate :one
-- Inserts a new report template and returns the full row.
-- The UNIQUE constraint on name means a duplicate raises pgconn error 23505 (unique_violation).
-- Caller maps 23505 → HTTP 409.
INSERT INTO report_template (name, description, state, created_by)
VALUES ($1, $2, $3, $4)
RETURNING id, name, description, state, created_by, created_at, updated_at;

-- name: UpdateReportTemplate :exec
-- Updates name, description, and state for an existing template.
-- Caller checks rows-affected = 0 → 404.
-- Duplicate name on rename raises pgconn 23505 → caller maps to HTTP 409.
UPDATE report_template
SET name        = $2,
    description = $3,
    state       = $4,
    updated_at  = now()
WHERE id = $1;

-- name: DeleteReportTemplate :exec
-- Hard-deletes a report template by primary key.
-- Caller checks rows-affected = 0 → 404.
DELETE FROM report_template
WHERE id = $1;
