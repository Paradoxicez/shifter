-- 0050_catalog_metadata.down.sql
-- Reverses migration 0050: drops CHECK constraints, removes Itron+KINMY seed
-- row, and drops all 7 Phase 7 catalog-metadata columns.

ALTER TABLE device_profile DROP CONSTRAINT IF EXISTS device_profile_battery_curve_check;
ALTER TABLE device_profile DROP CONSTRAINT IF EXISTS device_profile_anomaly_compat_check;

DELETE FROM device_profile WHERE slug = 'itron_kinmy_lora';

ALTER TABLE device_profile
    DROP COLUMN IF EXISTS anomaly_compatibility,
    DROP COLUMN IF EXISTS offline_threshold_multiplier,
    DROP COLUMN IF EXISTS expected_uplink_interval_seconds,
    DROP COLUMN IF EXISTS battery_curve,
    DROP COLUMN IF EXISTS customer_edited,
    DROP COLUMN IF EXISTS catalog_source_version,
    DROP COLUMN IF EXISTS catalog_source;
