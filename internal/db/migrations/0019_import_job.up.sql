-- 0019_import_job.up.sql
-- Phase 3 D-04..D-11, D-33..D-36: bulk-import job + per-row outcomes.
-- D-11 TTL 1h on preview; D-34 job_id used as audit request_id; D-35
-- 90-day retention. Per-row raw_payload JSONB preserves the original
-- file for errors.xlsx round-trip.

CREATE TYPE import_job_status     AS ENUM ('preview', 'committed', 'expired', 'failed');
CREATE TYPE import_job_row_status AS ENUM ('valid', 'invalid', 'already_exists', 'created', 'failed');

CREATE TABLE import_job (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id                  UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    owner_id                UUID NOT NULL REFERENCES "user"(id) ON DELETE SET NULL,
    file_name               TEXT NOT NULL,
    file_format             TEXT NOT NULL CHECK (file_format IN ('xlsx','csv')),
    total_rows              INTEGER NOT NULL DEFAULT 0,
    status                  import_job_status NOT NULL DEFAULT 'preview',
    valid_count             INTEGER NOT NULL DEFAULT 0,
    invalid_count           INTEGER NOT NULL DEFAULT 0,
    already_exists_count    INTEGER NOT NULL DEFAULT 0,
    created_count           INTEGER NOT NULL DEFAULT 0,
    failed_count            INTEGER NOT NULL DEFAULT 0,
    expires_at              TIMESTAMPTZ NULL,
    committed_at            TIMESTAMPTZ NULL,
    failed_reason           TEXT,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Hot-path indexes:
--   * owner_id + created_at DESC for "my recent imports" listing.
--   * Partial on preview-status for the expiry sweeper (Phase 9 cron).
CREATE INDEX import_job_owner_idx  ON import_job (owner_id, created_at DESC);
CREATE INDEX import_job_status_idx ON import_job (status, expires_at) WHERE status = 'preview';

CREATE TRIGGER import_job_touch
    BEFORE UPDATE ON import_job
    FOR EACH ROW
    EXECUTE FUNCTION touch_updated_at();

CREATE TABLE import_job_row (
    id                 BIGSERIAL PRIMARY KEY,
    import_job_id      UUID NOT NULL REFERENCES import_job(id) ON DELETE CASCADE,
    row_index          INTEGER NOT NULL,
    raw_payload        JSONB NOT NULL,
    parsed             JSONB,
    status             import_job_row_status NOT NULL DEFAULT 'valid',
    reason             TEXT,
    created_device_id  UUID NULL REFERENCES device(id),
    UNIQUE (import_job_id, row_index)
);

CREATE INDEX import_job_row_status_idx ON import_job_row (import_job_id, status);
