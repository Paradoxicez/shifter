-- 0009_device_profile.down.sql
DROP TRIGGER IF EXISTS device_profile_touch ON device_profile;
DROP INDEX IF EXISTS device_profile_capabilities_gin;
DROP INDEX IF EXISTS device_profile_cs_id_idx;
DROP INDEX IF EXISTS device_profile_archived_at_idx;
DROP INDEX IF EXISTS device_profile_slug_idx;
DROP TABLE IF EXISTS device_profile;
