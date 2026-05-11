-- 0027_cagg_monthly.down.sql
-- Idempotent reverse of 0027.
SELECT remove_retention_policy('measurement_monthly', if_exists => true);
SELECT remove_continuous_aggregate_policy('measurement_monthly', if_exists => true);
DROP MATERIALIZED VIEW IF EXISTS measurement_monthly;
