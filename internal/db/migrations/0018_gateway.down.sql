-- 0018_gateway.down.sql
DROP TRIGGER IF EXISTS gateway_touch ON gateway;
DROP INDEX IF EXISTS gateway_region_idx;
DROP INDEX IF EXISTS gateway_archived_idx;
DROP TABLE IF EXISTS gateway CASCADE;
