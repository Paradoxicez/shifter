-- 0047_backup_thresholds.down.sql
ALTER TABLE retention_config
    DROP CONSTRAINT IF EXISTS backup_warn_lt_crit,
    DROP COLUMN IF EXISTS backup_warn_threshold_hours,
    DROP COLUMN IF EXISTS backup_crit_threshold_hours;
