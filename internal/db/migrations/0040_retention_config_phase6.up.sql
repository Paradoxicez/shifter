-- 0040_retention_config_phase6.up.sql
-- Phase 6 — Plan 06-01 (D-13 + D-38): extend retention_config with alerts +
-- audit_log windows. Both columns are NOT NULL with sane defaults:
--   * alerts_days     default 365  (1 year) — D-13
--   * audit_log_days  default 1825 (5 years) — D-38
-- CHECK bounds prevent operator from setting cutoff_days=0 (would wipe the
-- entire table) or runaway values that exceed storage planning.

ALTER TABLE retention_config
    ADD COLUMN alerts_days    INTEGER NOT NULL DEFAULT 365
        CHECK (alerts_days BETWEEN 30 AND 3650),
    ADD COLUMN audit_log_days INTEGER NOT NULL DEFAULT 1825
        CHECK (audit_log_days BETWEEN 90 AND 18250);

COMMENT ON COLUMN retention_config.alerts_days
    IS 'D-13: alerts table retention. Default 365d (1 year). Settings UI exposes the slider in Plan 06-10.';
COMMENT ON COLUMN retention_config.audit_log_days
    IS 'D-38: audit_log retention. Default 1825d (5 years). Pruned via admin_prune_audit_rows() — see 0043.';
