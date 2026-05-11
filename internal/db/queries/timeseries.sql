-- Time-series chart queries — D-12 bucket schedule implemented in handler.
--
-- DashboardTimeseries: cross-MP aggregate for the dashboard consumption chart
--   (GET /api/dashboard/timeseries). The handler picks bucket interval per D-12
--   schedule:
--
--   | range        | bucket_interval |
--   |--------------|-----------------|
--   | today        | 5 minutes       |
--   | 24h          | 5 minutes       |
--   | 7d           | 1 hour          |
--   | 30d          | 4 hours         |
--   | custom ≤ 30d | 1 hour          |
--   | custom > 30d | 1 day           |
--
-- MeteringPointTimeseries: per-MP detail (Plan 05/09 consume).

-- name: DashboardTimeseries :many
-- D-12: time_bucket() with handler-controlled interval. Aggregates cumulative
-- delta across all MPs of the given utility_class.
-- sqlc.arg(utility_class), sqlc.arg(start_time), sqlc.arg(end_time), sqlc.arg(bucket_interval)
SELECT
    time_bucket(sqlc.arg(bucket_interval)::interval, m.time) AS bucket,
    COALESCE(SUM(
        m.cumulative_value - LAG(m.cumulative_value) OVER (
            PARTITION BY m.metering_point_id ORDER BY m.time
        )
    ), 0)::numeric AS cumulative_delta
FROM measurement m
INNER JOIN metering_point mp ON mp.id = m.metering_point_id
WHERE mp.utility_class = sqlc.arg(utility_class)
  AND mp.archived_at IS NULL
  AND m.time >= sqlc.arg(start_time)
  AND m.time < sqlc.arg(end_time)
GROUP BY bucket
ORDER BY bucket;

-- name: MeteringPointTimeseries :many
-- For per-MP detail page (Plan 05/09 consume).
-- sqlc.arg(metering_point_id), sqlc.arg(start_time), sqlc.arg(end_time), sqlc.arg(bucket_interval)
SELECT
    time_bucket(sqlc.arg(bucket_interval)::interval, time) AS bucket,
    AVG(cumulative_value)::numeric AS cumulative_avg,
    AVG(instant_value)::numeric AS instant_avg,
    AVG(battery_pct)::numeric AS battery_avg,
    AVG(rssi)::numeric AS rssi_avg,
    AVG(snr)::numeric AS snr_avg
FROM measurement
WHERE metering_point_id = sqlc.arg(metering_point_id)
  AND time >= sqlc.arg(start_time)
  AND time < sqlc.arg(end_time)
GROUP BY bucket
ORDER BY bucket;
