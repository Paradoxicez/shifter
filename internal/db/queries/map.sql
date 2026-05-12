-- Map view queries (Phase 5 MAP-01..04, plan 05-04).
--
-- These queries power GET /api/map/data in internal/map/handler.go.
--
-- D-12 (Phase 5): sites + gateways only on the map.
-- D-07 (Phase 4) online/offline rule:
--   device.last_seen_at > now() - (2 * dp.expected_interval_s * INTERVAL '1 second')
-- MAP-04 invariant: no tile URL or API key in any column returned here.
-- Column names use the actual schema: site.lat/lng, gateway.lat/lng.

-- name: ListSitesForMap :many
-- Sites with both lat AND lng populated; archived sites excluded.
-- Returns per-site rollups: MP count, online vs offline device count (per
-- Phase 4 D-07 rule using device_profile.expected_interval_s).
-- Device → MP join goes through binding (device has no metering_point_id column).
SELECT
  s.id,
  s.name,
  s.lat,
  s.lng,
  count(DISTINCT mp.id) FILTER (WHERE mp.archived_at IS NULL)                     AS mp_count,
  count(DISTINCT d.id) FILTER (
    WHERE d.decommissioned_at IS NULL
      AND b.valid_to IS NULL
      AND d.last_seen_at > now() - (2 * dp.expected_interval_s * INTERVAL '1 second')
  )                                                                                AS online_count,
  count(DISTINCT d.id) FILTER (
    WHERE d.decommissioned_at IS NULL
      AND b.valid_to IS NULL
      AND (d.last_seen_at IS NULL OR d.last_seen_at <= now() - (2 * dp.expected_interval_s * INTERVAL '1 second'))
  )                                                                                AS offline_count
FROM site s
LEFT JOIN metering_point mp ON mp.site_id = s.id AND mp.archived_at IS NULL
LEFT JOIN binding b ON b.metering_point_id = mp.id AND b.valid_to IS NULL
LEFT JOIN device d ON d.id = b.device_id AND d.decommissioned_at IS NULL
LEFT JOIN device_profile dp ON dp.id = d.device_profile_id
WHERE s.archived_at IS NULL
  AND s.lat IS NOT NULL
  AND s.lng IS NOT NULL
GROUP BY s.id, s.name, s.lat, s.lng
ORDER BY s.name;

-- name: ListGatewaysForMap :many
-- Gateways with lat/lng populated, excluding archived.
-- Online = stats_refreshed_at within the last 5 minutes.
-- (Gateway has no last_seen_at column; stats_refreshed_at is updated by the
-- cache_refresher goroutine which polls ChirpStack every ~60 s — a staleness
-- of >5 min reliably signals the gateway has gone dark.)
SELECT
  g.id,
  g.name,
  g.lat,
  g.lng,
  (g.stats_refreshed_at IS NOT NULL AND g.stats_refreshed_at > now() - INTERVAL '5 minutes') AS online
FROM gateway g
WHERE g.archived_at IS NULL
  AND g.lat IS NOT NULL
  AND g.lng IS NOT NULL
ORDER BY g.name;

-- name: TodaySiteConsumption :many
-- Per-site today (install_tz 00:00 → now) consumption split by utility_class.
-- Sources measurement_hourly (CAGG) — sums bucketed deltas for MPs belonging
-- to each site, partitioned by utility_class.
-- $1 = install_tz midnight today (computed in handler via time.LoadLocation)
SELECT
  mp.site_id,
  mp.utility_class,
  coalesce(sum(mh.cumulative_delta), 0)::bigint AS consumption
FROM measurement_hourly mh
JOIN metering_point mp ON mp.id = mh.metering_point_id
WHERE mh.bucket >= @midnight_today
  AND mh.bucket <  now()
  AND mp.archived_at IS NULL
GROUP BY mp.site_id, mp.utility_class;
