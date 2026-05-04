-- 0008_metering_point.down.sql
DROP TRIGGER IF EXISTS metering_point_touch ON metering_point;
DROP INDEX IF EXISTS metering_point_utility_idx;
DROP INDEX IF EXISTS metering_point_archived_at_idx;
DROP INDEX IF EXISTS metering_point_site_idx;
DROP TABLE IF EXISTS metering_point;
