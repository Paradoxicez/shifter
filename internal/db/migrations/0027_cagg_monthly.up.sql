-- 0027_cagg_monthly.up.sql
-- Monthly CAGG over measurement_daily.
--
-- D-11: real-time mode OFF (materialized_only = true) — monthly reports are
-- rarely-queried; real-time would scan-on-read combining materialized data with
-- all daily rows since last refresh, which is wasteful (T-05-02-02 DoS risk).
--
-- Pitfall #1: WITH NO DATA avoids golang-migrate transaction-block constraint.

CREATE MATERIALIZED VIEW measurement_monthly
WITH (timescaledb.continuous, timescaledb.materialized_only = true)
AS
SELECT
  time_bucket('1 month', bucket)       AS bucket,
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
FROM measurement_daily
GROUP BY 1, 2
WITH NO DATA;

-- start_offset=3 months so window (3m - 1d) covers >= 2 monthly buckets.
SELECT add_continuous_aggregate_policy('measurement_monthly',
  start_offset      => INTERVAL '3 months',
  end_offset        => INTERVAL '1 day',
  schedule_interval => INTERVAL '6 hours');

SELECT add_retention_policy('measurement_monthly', INTERVAL '20 years');
