-- 0028_cagg_yearly.down.sql
-- Idempotent reverse of 0028.
SELECT remove_continuous_aggregate_policy('measurement_yearly', if_exists => true);
DROP MATERIALIZED VIEW IF EXISTS measurement_yearly;
