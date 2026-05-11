-- Dashboard KPI queries (DASH-01..DASH-06, DASH-08, DASH-21, D-05..D-09, D-12).
--
-- These queries feed the three dashboard REST endpoints in internal/dashboard/:
--   GET /api/dashboard/scope     — capabilities + onboarding tuple (D-09, D-21)
--   GET /api/dashboard/snapshot  — KPI tiles + per-MP latest readings
--   GET /api/dashboard/timeseries — time-bucket chart data (D-12)
--
-- D-05 "today" boundary: date_trunc('day', now() AT TIME ZONE $tz) AT TIME ZONE $tz
-- All KPI queries accept timezone from install_identity.timezone (server-side
-- ONLY — never from request query string per T-04-04-04 threat register).
--
-- D-07 online/offline rule (verbatim from plan):
--   COUNT(*) FILTER (
--       WHERE d.last_seen_at > now() - (2 * dp.expected_interval_s * INTERVAL '1 second')
--   ) AS online_count
--
-- D-08 period delta: today [00:00→now] vs yesterday [00:00→same-time-yesterday].
-- sqlc.arg() annotations provide clean field names in the generated Params structs.

-- name: TodayConsumptionByUtility :one
-- D-05: today's consumption in install_identity.timezone.
-- sqlc.arg(timezone) / sqlc.arg(utility_class)
SELECT COALESCE(SUM(latest.cumulative_value - earliest.cumulative_value), 0)::numeric AS today_delta
FROM metering_point mp
INNER JOIN LATERAL (
    SELECT cumulative_value FROM measurement m
    WHERE m.metering_point_id = mp.id
      AND m.time >= date_trunc('day', now() AT TIME ZONE sqlc.arg(timezone)::text) AT TIME ZONE sqlc.arg(timezone)::text
    ORDER BY m.time ASC LIMIT 1
) earliest ON TRUE
INNER JOIN LATERAL (
    SELECT cumulative_value FROM measurement m
    WHERE m.metering_point_id = mp.id
      AND m.time >= date_trunc('day', now() AT TIME ZONE sqlc.arg(timezone)::text) AT TIME ZONE sqlc.arg(timezone)::text
    ORDER BY m.time DESC LIMIT 1
) latest ON TRUE
WHERE mp.utility_class = sqlc.arg(utility_class) AND mp.archived_at IS NULL;

-- name: CurrentInstantSumByUtility :one
-- D-06: latest instant_value per MP, summed across all active MPs.
-- $1 = utility_class text
SELECT COALESCE(SUM(latest.instant_value), 0)::numeric AS instant_total
FROM metering_point mp
INNER JOIN LATERAL (
    SELECT instant_value FROM measurement m
    WHERE m.metering_point_id = mp.id
    ORDER BY m.time DESC LIMIT 1
) latest ON TRUE
WHERE mp.utility_class = $1 AND mp.archived_at IS NULL;

-- name: PeriodDeltaByUtility :one
-- D-08: today [00:00→now] vs yesterday [00:00→same-time-yesterday].
-- Returns both windows so the handler can compute abs + pct change.
-- If yesterday window has no data the handler returns period_delta_abs = null.
-- sqlc.arg(timezone) / sqlc.arg(utility_class)
WITH bounds AS (
    SELECT
        date_trunc('day', now() AT TIME ZONE sqlc.arg(timezone)::text) AT TIME ZONE sqlc.arg(timezone)::text AS today_start,
        now() AS now_ts
),
today_slice AS (
    SELECT COALESCE(SUM(latest.cumulative_value - earliest.cumulative_value), 0)::numeric AS d
    FROM bounds, metering_point mp
    INNER JOIN LATERAL (
        SELECT cumulative_value FROM measurement m
        WHERE m.metering_point_id = mp.id AND m.time >= bounds.today_start
        ORDER BY m.time ASC LIMIT 1
    ) earliest ON TRUE
    INNER JOIN LATERAL (
        SELECT cumulative_value FROM measurement m
        WHERE m.metering_point_id = mp.id AND m.time <= bounds.now_ts
        ORDER BY m.time DESC LIMIT 1
    ) latest ON TRUE
    WHERE mp.utility_class = sqlc.arg(utility_class) AND mp.archived_at IS NULL
),
yesterday_slice AS (
    SELECT COALESCE(SUM(latest.cumulative_value - earliest.cumulative_value), 0)::numeric AS d
    FROM bounds, metering_point mp
    INNER JOIN LATERAL (
        SELECT cumulative_value FROM measurement m
        WHERE m.metering_point_id = mp.id
          AND m.time >= bounds.today_start - INTERVAL '1 day'
        ORDER BY m.time ASC LIMIT 1
    ) earliest ON TRUE
    INNER JOIN LATERAL (
        SELECT cumulative_value FROM measurement m
        WHERE m.metering_point_id = mp.id
          AND m.time <= bounds.today_start - INTERVAL '1 day' + (bounds.now_ts - bounds.today_start)
        ORDER BY m.time DESC LIMIT 1
    ) latest ON TRUE
    WHERE mp.utility_class = sqlc.arg(utility_class) AND mp.archived_at IS NULL
)
SELECT today_slice.d AS today_d, yesterday_slice.d AS yesterday_d
FROM today_slice, yesterday_slice;

-- name: DeviceOnlineCount :one
-- D-07: per-profile expected_interval_s; online = last_seen_at within 2x interval.
-- Joins device → device_profile → binding → metering_point to filter by utility.
-- $1 = utility_class text
SELECT
    COUNT(*) FILTER (
        WHERE d.last_seen_at > now() - (2 * dp.expected_interval_s * INTERVAL '1 second')
    )::bigint AS online_count,
    COUNT(*)::bigint AS total_count
FROM device d
INNER JOIN device_profile dp ON dp.id = d.device_profile_id
LEFT JOIN binding b ON b.device_id = d.id AND b.valid_to IS NULL
LEFT JOIN metering_point mp ON mp.id = b.metering_point_id
WHERE d.decommissioned_at IS NULL
  AND (mp.utility_class = $1 OR (mp.utility_class IS NULL AND $1 = ''));

-- name: DashboardLatestReadings :many
-- Per-MP latest reading for the snapshot response.
-- Filtered to utility_classes in the active capabilities set.
-- $1 = utility_classes (text[]; e.g. {'water'}, {'electricity'}, {'water','electricity'})
SELECT
    mp.id AS metering_point_id,
    mp.utility_class,
    latest.time,
    latest.cumulative_value,
    latest.instant_value,
    latest.quality,
    latest.battery_pct,
    latest.rssi
FROM metering_point mp
LEFT JOIN LATERAL (
    SELECT time, cumulative_value, instant_value, quality, battery_pct, rssi
    FROM measurement m
    WHERE m.metering_point_id = mp.id
    ORDER BY m.time DESC LIMIT 1
) latest ON TRUE
WHERE mp.archived_at IS NULL
  AND mp.utility_class = ANY($1::text[]);

-- name: OnboardingCounts :one
-- D-21: progressive empty-state counts used by GET /api/dashboard/scope.
-- Cheap aggregates across three small tables; never touches the measurement hypertable.
SELECT
    (SELECT COUNT(*)::bigint FROM gateway WHERE archived_at IS NULL) AS gateway_count,
    (SELECT COUNT(*)::bigint FROM device WHERE decommissioned_at IS NULL) AS device_count,
    (SELECT COUNT(*)::bigint FROM measurement) AS uplink_count;

-- name: GetCapabilities :one
-- D-09: install_identity.capabilities (singleton row).
-- Returns the capability string: 'water', 'electricity', or 'both'.
SELECT capabilities FROM install_identity WHERE id = 1;
