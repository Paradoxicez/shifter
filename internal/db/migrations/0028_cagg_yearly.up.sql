-- 0028_cagg_yearly.up.sql
-- Yearly CAGG over measurement_monthly.
--
-- D-09 yearly retention: NEVER drops (essentially free at ~1 row/MP/year).
-- We do NOT call add_retention_policy for this CAGG.
--
-- D-11: real-time OFF (materialized_only = true) — same rationale as monthly.
--
-- Pitfall #1: WITH NO DATA avoids golang-migrate transaction-block constraint.

CREATE MATERIALIZED VIEW measurement_yearly
WITH (timescaledb.continuous, timescaledb.materialized_only = true)
AS
SELECT
  time_bucket('1 year', bucket)        AS bucket,
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
FROM measurement_monthly
GROUP BY 1, 2
WITH NO DATA;

-- start_offset=3 years so window (3y - 7d) covers >= 2 yearly buckets.
SELECT add_continuous_aggregate_policy('measurement_yearly',
  start_offset      => INTERVAL '3 years',
  end_offset        => INTERVAL '7 days',
  schedule_interval => INTERVAL '1 day');

-- NO add_retention_policy — yearly CAGG kept forever (D-09).
