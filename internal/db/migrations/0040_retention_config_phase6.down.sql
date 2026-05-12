-- 0040_retention_config_phase6.down.sql
ALTER TABLE retention_config
    DROP COLUMN IF EXISTS alerts_days,
    DROP COLUMN IF EXISTS audit_log_days;
