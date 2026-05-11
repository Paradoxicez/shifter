-- 0025_cagg_hourly.down.sql
-- Idempotent reverse of 0025.

SELECT remove_retention_policy('measurement', if_exists => true);
SELECT remove_retention_policy('measurement_hourly', if_exists => true);
SELECT remove_continuous_aggregate_policy('measurement_hourly', if_exists => true);

DROP MATERIALIZED VIEW IF EXISTS measurement_hourly;
