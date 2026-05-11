-- 0026_cagg_daily.up.sql
-- Daily CAGG over measurement_hourly (CAGG-over-CAGG, D-08).
--
-- Pitfall #7: time_bucket on a fixed-width bucket (1h) -> fixed-width (1d) is
-- valid. Do NOT skip the daily level and go directly hourly -> monthly (1mo is
-- variable-width).
--
-- Pitfall #1: WITH NO DATA avoids golang-migrate transaction-block constraint.
-- The daily CAGG sources from measurement_hourly (not raw), so its start_offset
-- can exceed the 90d raw retention without triggering Pitfall #2.
--
-- D-11: materialized_only = false enables real-time mode for 30d charts.

CREATE MATERIALIZED VIEW measurement_daily
WITH (timescaledb.continuous, timescaledb.materialized_only = false)
AS
SELECT
  time_bucket('1 day', bucket)         AS bucket,
  metering_point_id,
  sum(cumulative_delta)                AS cumulative_delta,
  avg(avg_instant)                     AS avg_instant,
  max(max_instant)                     AS max_instant,
  min(min_battery)                     AS min_battery,
  avg(avg_battery)                     AS avg_battery,
  avg(avg_rssi)                        AS avg_rssi,
  avg(avg_snr)                         AS avg_snr,
  sum(uplink_count)                    AS uplink_count,
  sum(flagged_count)                   AS flagged_count
FROM measurement_hourly
GROUP BY 1, 2
WITH NO DATA;

-- start_offset=3 days, end_offset=2 hours:
--   window (3d - 2h ≈ 2d 22h) covers >= 2 daily buckets.
--   end_offset=2h >= 2 × max(expected_interval_s)=7200s (DATA-12).
SELECT add_continuous_aggregate_policy('measurement_daily',
  start_offset      => INTERVAL '3 days',
  end_offset        => INTERVAL '2 hours',
  schedule_interval => INTERVAL '30 minutes');

SELECT add_retention_policy('measurement_daily', INTERVAL '5 years');
