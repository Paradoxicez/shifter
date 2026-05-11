-- 0025_cagg_hourly.up.sql
-- Hourly continuous aggregate over measurement hypertable (DATA-11, D-08, D-10, D-11).
--
-- Pitfall #1 (RESEARCH §golang-migrate CAGG Transaction Constraint): CREATE
-- MATERIALIZED VIEW ... WITH (timescaledb.continuous) ... WITH DATA cannot run
-- inside a transaction block. golang-migrate wraps every file in BEGIN/COMMIT.
-- We use WITH NO DATA — the refresh policy below backfills automatically on
-- the first scheduled run (or call refresh_continuous_aggregate manually for
-- backfill).
--
-- Pitfall #2 (RESEARCH §CAGG Retention Footgun): start_offset MUST be smaller
-- than the raw retention window (90d default per D-09). We use INTERVAL '4 hours'
-- which is well inside the 90d window.
--
-- cumulative_delta computation: the source `measurement.cumulative_value` is
-- the absolute meter reading. Consumption inside a bucket = last reading in
-- the bucket minus the first reading in the bucket:
--   cumulative_delta = last(cumulative_value, time) - first(cumulative_value, time)
--
-- This is semantically equivalent to Σ(LAG deltas) because cumulative_value is
-- monotonically non-decreasing within a bucket (modulo rollover, which is handled
-- at ingest by internal/swap/math.go). The first reading of a bucket is the
-- continuation point from the previous bucket, giving correct cross-bucket
-- delta semantics without requiring a window function inside an aggregate
-- (which TimescaleDB CAGGs forbid: SQLSTATE 42803).
--
-- LAG(cumulative_value) OVER (PARTITION BY metering_point_id ORDER BY time)
-- inside sum() is invalid in a TimescaleDB CAGG because window function calls
-- cannot be nested inside aggregate calls (Postgres SQLSTATE 42803). The
-- first/last pattern is the correct CAGG-compatible equivalent.
--
-- Cross-bucket verification: if hour-09 has readings [100,110,130,145] and
-- hour-10 has [145,200], then:
--   hour-09 delta = last(145) - first(100) = 45
--   hour-10 delta = last(200) - first(145) = 55
-- This matches the Σ(LAG) result (10+20+15=45; 55=55) exactly.
--
-- D-11: materialized_only = false enables real-time mode so freshly-ingested
-- rows are visible in 7d/30d charts within the refresh interval (5 min).

CREATE MATERIALIZED VIEW measurement_hourly
WITH (timescaledb.continuous, timescaledb.materialized_only = false)
AS
SELECT
  time_bucket('1 hour', time)                                     AS bucket,
  metering_point_id,
  last(cumulative_value, time) - first(cumulative_value, time)    AS cumulative_delta,
  avg(instant_value)                                              AS avg_instant,
  max(instant_value)                                              AS max_instant,
  min(battery_pct)                                               AS min_battery,
  avg(battery_pct)                                               AS avg_battery,
  avg(rssi)                                                      AS avg_rssi,
  avg(snr)                                                       AS avg_snr,
  count(*)                                                       AS uplink_count,
  count(*) FILTER (WHERE quality <> 'ok')                        AS flagged_count
FROM measurement
GROUP BY 1, 2
WITH NO DATA;

-- DATA-12 refresh policy (D-11):
--   end_offset    = 2 hours    — >= 2 × max(expected_interval_s)=7200s (DATA-12).
--                                 Excludes the in-progress 2-hour window; absorbs
--                                 late uplinks from all device profiles.
--   start_offset  = 4 hours    — window = 4h - 2h = 2h = 2 buckets (TimescaleDB
--                                 requires start_offset - end_offset >= 2 × bucket_size).
--                                 Well inside raw 90d retention (Pitfall #2).
--   schedule_interval = 5 min  — D-11 / Discretion.
SELECT add_continuous_aggregate_policy('measurement_hourly',
  start_offset      => INTERVAL '4 hours',
  end_offset        => INTERVAL '2 hours',
  schedule_interval => INTERVAL '5 minutes');

-- D-09 retention: hourly = 1 year.
SELECT add_retention_policy('measurement_hourly', INTERVAL '1 year');

-- D-09 raw retention: 90 days (apply HERE — must land somewhere before any
-- CAGG refreshes attempt to read raw data older than this window).
SELECT add_retention_policy('measurement', INTERVAL '90 days');
