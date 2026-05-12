-- internal/db/queries/settings.sql
-- Retention configuration queries (DATA-13 / D-09 / Plan 05-11).
--
-- The retention_config table is a singleton (id=1) seeded at install time
-- by internal/install/finish.go. Queries here power the Settings → Data
-- Retention card GET + PATCH handlers.

-- name: GetRetentionConfig :one
-- Returns the singleton retention configuration row (id=1).
-- Called by both GET /api/settings/retention (read) and the PATCH handler
-- (to snapshot before-state for the audit diff).
--
-- Phase 6 Plan 06-02 schema-bridge fix: explicitly enumerate every column
-- so that adding new columns (alerts_days / audit_log_days from 0040,
-- future v2 fields) keeps the sqlc-generated row type aligned with the
-- table type. sqlc emits the canonical `RetentionConfig` struct when the
-- SELECT column set matches the table 1:1; otherwise it generates a
-- per-query row alias that breaks downstream code expecting the table type.
SELECT id, raw_days, hourly_days, daily_days, monthly_days, yearly_days,
       alerts_days, audit_log_days, updated_at
FROM retention_config
WHERE id = 1;

-- name: UpdateRetentionConfig :one
-- Partial update via COALESCE: only fields whose $N is non-NULL are changed.
-- yearly_days is NOT wrapped in COALESCE — the handler passes the resolved
-- value explicitly (including NULL to express "forever"), using a sentinel
-- flag to distinguish "omitted" from "explicitly set to null". See Plan 05-11
-- doc.go for the sentinel protocol.
--
-- $1..4 are nullable integers (sqlc maps *int32). Passing nil = COALESCE keeps
-- the existing value. $5 yearly_days is always explicit (nil = forever).
UPDATE retention_config
SET raw_days     = COALESCE(sqlc.narg('raw_days')::integer, raw_days),
    hourly_days  = COALESCE(sqlc.narg('hourly_days')::integer, hourly_days),
    daily_days   = COALESCE(sqlc.narg('daily_days')::integer, daily_days),
    monthly_days = COALESCE(sqlc.narg('monthly_days')::integer, monthly_days),
    yearly_days  = sqlc.narg('yearly_days')::integer,
    updated_at   = now()
WHERE id = 1
RETURNING id, raw_days, hourly_days, daily_days, monthly_days, yearly_days,
          alerts_days, audit_log_days, updated_at;
