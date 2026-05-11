-- uplinks.sql — DETL-02 cursor-paginated uplink log for the per-MP uplinks tab.
--
-- ListUplinksByMP uses (metering_point_id, time DESC) index for the no-filter
-- case and the partial index measurement_quality_flagged_idx for flagged-only
-- queries. Cursor is on time DESC so result pages are stable even as new rows
-- insert (unlike OFFSET which shifts under concurrent ingest).
--
-- QualitySummaryByMP computes the D-19 "X of last 100 uplinks flagged" badge:
-- the inner sub-select limits to 100 rows, the outer groups by quality value.

-- name: ListUplinksByMP :many
-- DETL-02: cursor-paginated, time DESC, optional quality filter.
-- Index path: measurement_mp_time_idx (metering_point_id, time DESC).
-- $1 = mp_id, $2 = limit (<=500), $3 = before (nullable timestamptz cursor),
-- $4 = quality filter (TEXT[]; NULL or empty = no filter).
SELECT
    time, cumulative_value, instant_value,
    battery_pct, rssi, snr, fcnt, quality,
    raw_payload, decoded_object
FROM measurement
WHERE metering_point_id = $1
  AND time < COALESCE($3::timestamptz, 'infinity'::timestamptz)
  AND (
      $4::text[] IS NULL
      OR cardinality($4::text[]) = 0
      OR quality = ANY($4::text[])
  )
ORDER BY time DESC
LIMIT $2;

-- name: QualitySummaryByMP :many
-- D-19: window of last 100 uplinks, grouped by quality.
-- Returns one row per quality value present in the window.
SELECT quality, COUNT(*)::bigint AS count
FROM (
    SELECT quality FROM measurement
    WHERE metering_point_id = $1
    ORDER BY time DESC
    LIMIT 100
) recent
GROUP BY quality;
