-- mp_detail.sql — DETL-01 composite query for the per-MP detail page.
--
-- GetMeteringPointDetail: returns MP + site name + active binding + latest
-- reading in one round-trip. All binding/measurement joins are LEFT so D-22
-- (no binding, no uplinks) returns the MP row cleanly — callers set
-- active_binding=null and latest_reading=null from the null-valued columns.
--
-- MeteringPointOnlineStatus: D-07 online/offline rule. Returns NULL when no
-- device is bound (D-22 empty case); TRUE when last_seen_at is within
-- 2 * expected_interval_s of now(); FALSE otherwise.

-- name: GetMeteringPointDetail :one
-- Composite query: MP + site name + active binding + latest reading.
-- All bindings/measurements are LEFT JOINs so D-22 (no binding / no uplinks) returns the MP row.
-- $1 = mp_id
SELECT
    mp.id, mp.name, mp.site_id, mp.utility_class, mp.location_description,
    s.name AS site_name,
    -- active binding fields (LEFT JOIN)
    b.id AS binding_id,
    b.device_id,
    b.valid_from,
    d.dev_eui,
    dp.name AS device_profile_name,
    -- latest reading via LATERAL
    latest.time AS latest_time,
    latest.cumulative_value,
    latest.instant_value,
    latest.quality,
    latest.battery_pct,
    latest.rssi,
    latest.snr,
    latest.fcnt,
    latest.decoded_object,
    latest.extra,
    latest.raw_payload
FROM metering_point mp
LEFT JOIN site s ON s.id = mp.site_id
LEFT JOIN binding b ON b.metering_point_id = mp.id AND b.valid_to IS NULL
LEFT JOIN device d ON d.id = b.device_id
LEFT JOIN device_profile dp ON dp.id = d.device_profile_id
LEFT JOIN LATERAL (
    SELECT time, cumulative_value, instant_value, quality, battery_pct, rssi, snr, fcnt, decoded_object, extra, raw_payload
    FROM measurement m
    WHERE m.metering_point_id = mp.id
    ORDER BY m.time DESC LIMIT 1
) latest ON TRUE
WHERE mp.id = $1 AND mp.archived_at IS NULL;

-- name: MeteringPointOnlineStatus :one
-- D-07: online = bound device's last_seen_at within 2 * expected_interval_s.
-- Returns device_bound=false when no binding (D-22 empty case).
-- Handler emits online=null when device_bound=false, online=true|false otherwise.
-- Two non-null booleans avoids sqlc CASE/NULL inference issues with nullable scans.
SELECT
    (b.device_id IS NOT NULL) AS device_bound,
    COALESCE(
        d.last_seen_at > now() - (2 * dp.expected_interval_s * INTERVAL '1 second'),
        FALSE
    ) AS is_online
FROM metering_point mp
LEFT JOIN binding b ON b.metering_point_id = mp.id AND b.valid_to IS NULL
LEFT JOIN device d ON d.id = b.device_id
LEFT JOIN device_profile dp ON dp.id = d.device_profile_id
WHERE mp.id = $1;
