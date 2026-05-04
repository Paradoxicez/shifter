-- 0012_device.down.sql
DROP TRIGGER IF EXISTS device_touch ON device;
DROP INDEX IF EXISTS device_last_seen_idx;
DROP INDEX IF EXISTS device_decommissioned_idx;
DROP INDEX IF EXISTS device_profile_idx;
DROP TABLE IF EXISTS device;
