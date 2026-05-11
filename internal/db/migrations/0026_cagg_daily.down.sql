-- 0026_cagg_daily.down.sql
-- Idempotent reverse of 0026.
SELECT remove_retention_policy('measurement_daily', if_exists => true);
SELECT remove_continuous_aggregate_policy('measurement_daily', if_exists => true);
DROP MATERIALIZED VIEW IF EXISTS measurement_daily;
