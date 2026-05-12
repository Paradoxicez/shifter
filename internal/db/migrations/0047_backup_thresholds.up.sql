-- 0047_backup_thresholds.up.sql
-- Phase 6 — Plan 06-10 (D-46 / SETT-05): add backup freshness-dot thresholds
-- to the retention_config singleton. The Backup Settings card reads these to
-- determine whether the freshness dot is green (fresh), yellow (warn), or red
-- (critical). Defaults: 24h warn, 168h (7d) crit.
--
-- Migration number: 0047 — plan 06-10 originally claimed 0046 but that was
-- already taken by 06-08 (backup_run table). Rule 3 deviation documented in
-- 06-10-settings-extensions-SUMMARY.md. Plan 06-11 must use 0048.

ALTER TABLE retention_config
    ADD COLUMN backup_warn_threshold_hours INTEGER NOT NULL DEFAULT 24
        CHECK (backup_warn_threshold_hours > 0 AND backup_warn_threshold_hours <= 8760),
    ADD COLUMN backup_crit_threshold_hours INTEGER NOT NULL DEFAULT 168
        CHECK (backup_crit_threshold_hours > 0 AND backup_crit_threshold_hours <= 8760),
    ADD CONSTRAINT backup_warn_lt_crit
        CHECK (backup_warn_threshold_hours < backup_crit_threshold_hours);

COMMENT ON COLUMN retention_config.backup_warn_threshold_hours
    IS 'D-46: yellow age threshold (Settings Backup card). Default 24h. Must be < backup_crit_threshold_hours.';
COMMENT ON COLUMN retention_config.backup_crit_threshold_hours
    IS 'D-46: red age threshold (Settings Backup card). Default 168h (7d). Must be > backup_warn_threshold_hours.';
