-- 0042_alert_worker_state.up.sql
-- Phase 6 — Plan 06-01 (D-21 + D-22): per-worker observability row.
--
-- One row per alert worker (5 kinds). Workers UPSERT their state at the end
-- of every eval cycle so `/health/detailed` can render staleness, fires per
-- cycle, duration, and a degraded flag. The River subscriber in
-- internal/alert/degraded.go flips `degraded=true` when a job hits
-- rivertype.JobStateDiscarded (exhausted retries) so the shell banner can
-- render "Alert evaluation degraded" without alert-fatigue from each retry.
--
-- This table is intentionally NOT a hypertable — one row per worker, low
-- volume, frequently UPSERTed. Index-on-PK by worker_kind.
--
-- Migration number 0041 is RESERVED (deliberate gap) so that the security-
-- definer audit prune function gets the memorable terminal number 0043.

CREATE TABLE alert_worker_state (
    worker_kind     TEXT PRIMARY KEY CHECK (worker_kind IN (
        'threshold_instantaneous','threshold_hourly','threshold_daily',
        'offline','anomaly'
    )),
    last_run_at     TIMESTAMPTZ NOT NULL DEFAULT '1970-01-01T00:00:00Z',
    rules_evaluated INTEGER NOT NULL DEFAULT 0,
    fires_emitted   INTEGER NOT NULL DEFAULT 0,
    cleared         INTEGER NOT NULL DEFAULT 0,
    duration_ms     INTEGER NOT NULL DEFAULT 0,
    degraded        BOOLEAN NOT NULL DEFAULT FALSE,
    last_error      TEXT,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed the five worker_kind rows so the application can UPDATE rather than
-- UPSERT (one less code path; reading the row is also unconditional).
INSERT INTO alert_worker_state (worker_kind) VALUES
    ('threshold_instantaneous'),
    ('threshold_hourly'),
    ('threshold_daily'),
    ('offline'),
    ('anomaly');

COMMENT ON TABLE alert_worker_state IS 'Phase 6 D-21: one observability row per alert worker. UPSERTed at the end of every cycle; degraded flipped by the River subscriber in internal/alert/degraded.go.';
