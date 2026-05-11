-- Bulk-import job state machine + per-row outcomes. Phase 3 D-04..D-11.
--
-- D-11 TTL 1h on preview; D-34 job_id used as audit request_id; D-35 90-day
-- retention. Per-row raw_payload JSONB preserves the original file for
-- errors.xlsx round-trip.

-- name: CreateImportJob :one
-- Plan 03-05 upload handler: creates the preview job. total_rows + expires_at
-- supplied by handler (now+1h).
INSERT INTO import_job (job_id, owner_id, file_name, file_format, total_rows, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetImportJobByID :one
SELECT * FROM import_job WHERE id = $1;

-- name: GetImportJobByJobID :one
-- External-facing lookup (paths use the publicly-shared job_id UUID).
SELECT * FROM import_job WHERE job_id = $1;

-- name: ListImportJobs :many
-- Phase 3 D-36 "/admin/imports" page. Newest first.
SELECT * FROM import_job
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountImportJobs :one
SELECT COUNT(*) FROM import_job;

-- name: UpdateImportJobToCommitted :one
-- Commit-handler terminal state. expires_at cleared so the partial-cleanup
-- sweeper (Phase 9) doesn't touch committed rows.
UPDATE import_job SET
    status = 'committed',
    committed_at = now(),
    created_count = $2,
    already_exists_count = $3,
    failed_count = $4,
    invalid_count = $5,
    valid_count = $6,
    expires_at = NULL
WHERE id = $1 AND status = 'preview'
RETURNING *;

-- name: UpdateImportJobToExpired :one
-- D-11: lazy expiry on read. Only transitions preview→expired when the
-- expires_at deadline has passed.
UPDATE import_job SET status = 'expired'
WHERE id = $1 AND status = 'preview' AND expires_at < now()
RETURNING *;

-- name: UpdateImportJobCounters :one
-- Upload handler bulk-update after dry-run validation runs. Counter values
-- supplied directly so the handler doesn't have to issue per-status COUNT(*)
-- queries against import_job_row.
UPDATE import_job SET
    total_rows = $2,
    valid_count = $3,
    invalid_count = $4,
    already_exists_count = $5
WHERE id = $1
RETURNING *;

-- name: InsertImportJobRow :one
-- Per-row outcome row from dry-run (status ∈ valid|invalid|already_exists).
-- Commit pass later UPDATEs the row to created|failed via
-- UpdateImportJobRowOutcome.
INSERT INTO import_job_row (import_job_id, row_index, raw_payload, parsed, status, reason)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListImportJobRows :many
-- Paginated job-detail page. Ordered by row_index so the UI shows file order.
SELECT * FROM import_job_row
WHERE import_job_id = $1
ORDER BY row_index ASC
LIMIT $2 OFFSET $3;

-- name: CountImportJobRows :one
SELECT COUNT(*) FROM import_job_row WHERE import_job_id = $1;

-- name: ListImportJobErrorRows :many
-- Used by errors.xlsx generator. Returns only invalid + failed rows so the
-- operator gets back the cells that need fixing plus the reason.
SELECT * FROM import_job_row
WHERE import_job_id = $1
  AND status IN ('invalid', 'failed')
ORDER BY row_index ASC;

-- name: UpdateImportJobRowOutcome :one
-- Commit pass writes the final outcome (created / failed) + the
-- created_device_id pointer so audit-replay can correlate rows to device rows.
UPDATE import_job_row SET
    status = $2,
    reason = $3,
    created_device_id = $4
WHERE id = $1
RETURNING *;
