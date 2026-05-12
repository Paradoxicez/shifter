-- compare.sql
-- sqlc-annotated queries for the Plan 07-12 Compare View (Surface 5).
-- Both queries hit measurement_daily CAGG for day-level bucketing.
-- Range param semantics: from inclusive, to exclusive (half-open interval).

-- name: CompareSiteDaily :many
-- Returns daily consumption totals for a given site (summing all metering points).
SELECT
  md.bucket::date          AS bucket,
  sum(md.cumulative_delta) AS value
FROM measurement_daily md
JOIN metering_point mp ON mp.id = md.metering_point_id
WHERE mp.site_id = $1
  AND md.bucket >= $2
  AND md.bucket <  $3
GROUP BY md.bucket::date
ORDER BY md.bucket::date;

-- name: CompareMeteringPointDaily :many
-- Returns daily consumption totals for a single metering point.
SELECT
  bucket::date             AS bucket,
  cumulative_delta         AS value
FROM measurement_daily
WHERE metering_point_id = $1
  AND bucket >= $2
  AND bucket <  $3
ORDER BY bucket::date;
