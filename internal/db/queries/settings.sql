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
       alerts_days, audit_log_days,
       backup_warn_threshold_hours, backup_crit_threshold_hours,
       updated_at
FROM retention_config
WHERE id = 1;

-- name: UpdateRetentionConfig :one
-- Partial update via COALESCE: only fields whose $N is non-NULL are changed.
-- yearly_days is NOT wrapped in COALESCE — the handler passes the resolved
-- value explicitly (including NULL to express "forever"), using a sentinel
-- flag to distinguish "omitted" from "explicitly set to null". See Plan 05-11
-- doc.go for the sentinel protocol.
--
-- Phase 6 Plan 06-10: alerts_days and audit_log_days added. Both use COALESCE
-- (omitting them from PATCH preserves the existing value). The Phase 5
-- TimescaleDB policy reconciliation does NOT apply to alerts/audit_log — those
-- tables are NOT hypertables; their retention is enforced by the alerts-prune
-- worker (Plan 06-11 task 1) and the AuditPruneWorker (Plan 06-01). The
-- config row is the sole source of truth; the workers read it on each run.
UPDATE retention_config
SET raw_days      = COALESCE(sqlc.narg('raw_days')::integer, raw_days),
    hourly_days   = COALESCE(sqlc.narg('hourly_days')::integer, hourly_days),
    daily_days    = COALESCE(sqlc.narg('daily_days')::integer, daily_days),
    monthly_days  = COALESCE(sqlc.narg('monthly_days')::integer, monthly_days),
    yearly_days   = sqlc.narg('yearly_days')::integer,
    alerts_days   = COALESCE(sqlc.narg('alerts_days')::integer, alerts_days),
    audit_log_days = COALESCE(sqlc.narg('audit_log_days')::integer, audit_log_days),
    updated_at    = now()
WHERE id = 1
RETURNING id, raw_days, hourly_days, daily_days, monthly_days, yearly_days,
          alerts_days, audit_log_days, updated_at;
