-- Migration 0046: backup_run history table (Plan 06-08, OPS-02 / OPS-03 / SETT-05).
--
-- Records every backup attempt — CLI, API, or ofelia cron sidecar — with
-- outcome, tarball metadata, and the manifest snapshot.  The Settings
-- backup card (SETT-05 / Plan 06-10) queries this table for the age dot
-- and the 5-most-recent list.
--
-- Rule 3 deviation note: this plan originally claimed migration 0045, but
-- 0045 was already consumed by plan 06-02 (0045_device_gateway_link).
-- This migration uses 0046 instead.

CREATE TABLE backup_run (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Who triggered this backup.
    trigger_kind     TEXT        NOT NULL CHECK (trigger_kind IN ('cli', 'cron', 'api')),
    triggered_by     UUID        REFERENCES "user"(id) ON DELETE SET NULL,
                                 -- NULL for cron / CLI calls with no authenticated user
    -- Lifecycle status.
    status           TEXT        NOT NULL CHECK (status IN ('running', 'completed', 'failed')),
    -- Filesystem location of the tarball.
    destination_dir  TEXT        NOT NULL,
    file_name        TEXT,
    file_size_bytes  BIGINT,
    sha256           TEXT,
    -- Manifest snapshot stored inline for easy query (SETT-05 age + sha256 in list).
    manifest_json    JSONB,
    -- Context at time of backup (for cross-version restore diagnostics).
    chirpstack_mode  TEXT,
    schema_version   TEXT,
    -- Timestamps.
    started_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at      TIMESTAMPTZ,
    -- Human-readable failure reason; NULL on success.
    error_message    TEXT
);

-- Descending index drives the "last backup" + "5 most recent" queries.
CREATE INDEX backup_run_started_at_idx ON backup_run (started_at DESC);
