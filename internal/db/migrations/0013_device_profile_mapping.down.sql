-- 0013_device_profile_mapping.down.sql
DROP TRIGGER IF EXISTS device_profile_mapping_touch ON device_profile_mapping;
DROP INDEX IF EXISTS device_profile_mapping_profile_idx;
DROP TABLE IF EXISTS device_profile_mapping;
