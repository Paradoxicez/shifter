-- reports.sql
-- sqlc-annotated queries for the Phase 5 report system (REPT-01..07).
-- All data queries hit CAGGs (measurement_hourly/daily/monthly/yearly),
-- NEVER raw measurement (90d+ raw queries are expensive per Phase 4 D-12).
--
-- Range → CAGG mapping (must_haves truth #6):
--   range=daily   → measurement_hourly (bucketed to days in query)
--   range=monthly → measurement_daily  (bucketed to months in query)
--   range=yearly  → measurement_monthly (bucketed to years in query)

---------- REPORT METADATA CRUD ----------

-- name: CreateReport :one
INSERT INTO report (id, user_id, scope, site_id, metering_point_id, group_by, range_kind, range_start, range_end, artifact_dir, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetReport :one
SELECT * FROM report WHERE id = $1;

-- name: UpdateReportPDFStatus :exec
UPDATE report SET pdf_status = $2, pdf_path = $3 WHERE id = $1;

-- name: ListExpiredReports :many
SELECT id, artifact_dir FROM report
WHERE expires_at < now() AND pdf_status <> 'expired';

-- name: MarkReportExpired :exec
UPDATE report SET pdf_status = 'expired' WHERE id = $1;

---------- DATA QUERIES ----------

-- name: ReportDailyByMP :many
-- Daily report — sources measurement_hourly, buckets to days.
-- Returns one row per (day, metering_point_id).
SELECT
  time_bucket('1 day', bucket) AS period,
  metering_point_id,
  sum(cumulative_delta)        AS consumption,
  sum(uplink_count)            AS uplinks,
  sum(flagged_count)           AS flagged
FROM measurement_hourly
WHERE metering_point_id = $1
  AND bucket >= $2 AND bucket < $3
GROUP BY 1, 2
ORDER BY 1;

-- name: ReportDailyBySite :many
SELECT
  time_bucket('1 day', mh.bucket) AS period,
  mp.site_id,
  s.name                          AS site_name,
  sum(mh.cumulative_delta)        AS consumption
FROM measurement_hourly mh
JOIN metering_point mp ON mp.id = mh.metering_point_id
JOIN site s ON s.id = mp.site_id
WHERE mh.bucket >= $1 AND mh.bucket < $2
  AND ($3::UUID IS NULL OR mp.site_id = $3)
GROUP BY 1, 2, 3
ORDER BY 1, 3;

-- name: ReportDailyByCategory :many
-- Group by metering_point.utility_class (D-04: utility class = water | electricity).
SELECT
  time_bucket('1 day', mh.bucket) AS period,
  mp.utility_class,
  sum(mh.cumulative_delta)        AS consumption
FROM measurement_hourly mh
JOIN metering_point mp ON mp.id = mh.metering_point_id
WHERE mh.bucket >= $1 AND mh.bucket < $2
GROUP BY 1, 2
ORDER BY 1, 2;

-- name: ReportMonthlyByMP :many
SELECT
  time_bucket('1 month', bucket) AS period,
  metering_point_id,
  sum(cumulative_delta) AS consumption,
  sum(uplink_count) AS uplinks,
  sum(flagged_count) AS flagged
FROM measurement_daily
WHERE metering_point_id = $1
  AND bucket >= $2 AND bucket < $3
GROUP BY 1, 2
ORDER BY 1;

-- name: ReportMonthlyBySite :many
SELECT
  time_bucket('1 month', md.bucket) AS period,
  mp.site_id,
  s.name             AS site_name,
  sum(md.cumulative_delta) AS consumption
FROM measurement_daily md
JOIN metering_point mp ON mp.id = md.metering_point_id
JOIN site s ON s.id = mp.site_id
WHERE md.bucket >= $1 AND md.bucket < $2
  AND ($3::UUID IS NULL OR mp.site_id = $3)
GROUP BY 1, 2, 3
ORDER BY 1, 3;

-- name: ReportMonthlyByCategory :many
SELECT
  time_bucket('1 month', md.bucket) AS period,
  mp.utility_class,
  sum(md.cumulative_delta) AS consumption
FROM measurement_daily md
JOIN metering_point mp ON mp.id = md.metering_point_id
WHERE md.bucket >= $1 AND md.bucket < $2
GROUP BY 1, 2
ORDER BY 1, 2;

-- name: ReportYearlyByMP :many
SELECT
  time_bucket('1 year', bucket) AS period,
  metering_point_id,
  sum(cumulative_delta) AS consumption
FROM measurement_monthly
WHERE metering_point_id = $1
  AND bucket >= $2 AND bucket < $3
GROUP BY 1, 2
ORDER BY 1;

-- name: ReportYearlyBySite :many
SELECT
  time_bucket('1 year', my.bucket) AS period,
  mp.site_id,
  s.name             AS site_name,
  sum(my.cumulative_delta) AS consumption
FROM measurement_monthly my
JOIN metering_point mp ON mp.id = my.metering_point_id
JOIN site s ON s.id = mp.site_id
WHERE my.bucket >= $1 AND my.bucket < $2
  AND ($3::UUID IS NULL OR mp.site_id = $3)
GROUP BY 1, 2, 3
ORDER BY 1, 3;

-- name: ReportYearlyByCategory :many
SELECT
  time_bucket('1 year', my.bucket) AS period,
  mp.utility_class,
  sum(my.cumulative_delta) AS consumption
FROM measurement_monthly my
JOIN metering_point mp ON mp.id = my.metering_point_id
WHERE my.bucket >= $1 AND my.bucket < $2
GROUP BY 1, 2
ORDER BY 1, 2;

-- name: ListMetersInScope :many
-- Returns metering points in scope (all / site / single) for the meter_rows table.
SELECT
  mp.id,
  mp.name,
  mp.utility_class,
  mp.site_id,
  s.name AS site_name
FROM metering_point mp
JOIN site s ON s.id = mp.site_id
WHERE
  ($1::TEXT = 'all') OR
  ($1::TEXT = 'site' AND mp.site_id = $2) OR
  ($1::TEXT = 'meter' AND mp.id = $3)
ORDER BY s.name, mp.name;
