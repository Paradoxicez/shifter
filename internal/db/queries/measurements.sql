-- Measurement (DATA-01 + DATA-03 + DATA-07 + DATA-08 + D-02 + D-26).
-- TimescaleDB hypertable. The ONLY way Shifter writes telemetry is via
-- AppendMeasurement; raw `pool.Exec INSERT INTO measurement ...` in handlers
-- is a code-review reject (CLAUDE.md: sqlc + pgx, never raw SQL in handlers).
--
-- DATA-01 invariant: keyed by metering_point_id only. NO device_id /
-- dev_eui column on this table — the active binding at write time implies
-- the device, captured forward-compat in binding_id (NULLABLE) so future
-- Phase 4/5 historical-binding JOINs don't need time-range gymnastics.

-- name: AppendMeasurement :exec
-- Plan 02-09 ingest pipeline: the single canonical write path. 20 column
-- positional placeholders match the migration column order:
--   $1  time             — server-side ingest time (DATA-03 + Pitfall 4)
--   $2  metering_point_id (DATA-01 invariant — NEVER device_id)
--   $3..$5  raw_value, cumulative_value, instant_value (D-02 Layer-1)
--   $6..$8  battery_pct, rssi, snr (D-02 diagnostics)
--   $9..$10 temperature_c, pressure_kpa (D-02 environmentals)
--   $11..$12 leak_detected, tamper_detected (D-02 booleans)
--   $13 extra (JSONB; vendor-specific fields per D-08 hybrid wide+JSONB)
--   $14 raw_payload (BYTEA; DATA-07 — never lose raw bytes)
--   $15 decoded_object (JSONB; DATA-07 — never lose decoded JSON)
--   $16 quality (D-26 — 'ok' | 'decode_fail' | 'missing_canonical' |
--               'out_of_range' | 'duplicate_fcnt')
--   $17..$19 fcnt, gateway_rx_time, device_time (diagnostic only — server
--               time is authoritative)
--   $20 binding_id (forward-compat for Phase 4/5; NULL when no binding
--               covers the timestamp — quality='missing_canonical' per Q#3)
INSERT INTO measurement (
    time, metering_point_id,
    raw_value, cumulative_value, instant_value,
    battery_pct, rssi, snr,
    temperature_c, pressure_kpa,
    leak_detected, tamper_detected,
    extra, raw_payload, decoded_object,
    quality, fcnt, gateway_rx_time, device_time, binding_id
)
VALUES (
    $1,  $2,
    $3,  $4,  $5,
    $6,  $7,  $8,
    $9,  $10,
    $11, $12,
    $13, $14, $15,
    $16, $17, $18, $19, $20
);

-- name: GetLatestMeasurement :one
-- Plan 02-08 MP detail page header card "Last reading at <time>". The
-- (metering_point_id, time DESC) hot-path index makes this an index scan +
-- LIMIT 1 — fast even on a 1B-row hypertable (chunk pruning by MP).
SELECT * FROM measurement
WHERE metering_point_id = $1
ORDER BY time DESC
LIMIT 1;

-- name: ListRecentMeasurements :many
-- Plan 02-08 MP detail "recent uplinks" tab + Phase 4 DETL-01 chart preload.
-- Bounded by both time floor ($2) AND row count ($3) so a misconfigured UI
-- can't accidentally page through years of telemetry.
SELECT * FROM measurement
WHERE metering_point_id = $1 AND time >= $2
ORDER BY time DESC
LIMIT $3;

-- name: CountFlaggedRecent :one
-- D-26 quality-flag categorization: powers the MP detail "X uplinks flagged"
-- badge (Plan 02-08) and Phase 4 dashboard tile. count(*) FILTER is the
-- canonical Postgres pattern for parallel category counts in one scan; far
-- cheaper than 5 separate aggregate queries. The partial index on
-- `quality <> 'ok'` (0015) keeps this fast for the flagged-only branches.
SELECT
    count(*) AS total,
    count(*) FILTER (WHERE quality = 'decode_fail')        AS decode_fail,
    count(*) FILTER (WHERE quality = 'missing_canonical')  AS missing_canonical,
    count(*) FILTER (WHERE quality = 'out_of_range')       AS out_of_range,
    count(*) FILTER (WHERE quality = 'duplicate_fcnt')     AS duplicate_fcnt
FROM measurement
WHERE metering_point_id = $1
  AND quality <> 'ok'
  AND time >= $2;

-- name: CountMeasurementsByMP :one
-- Plan 02-13 testharness W3 sync barrier: scenarios poll this every 100ms
-- (10s ceiling) after publishing an uplink to the MQTT broker so the next
-- step (e.g. CommitSwap) doesn't race with the not-yet-persisted measurement
-- row. Also useful for any caller that needs an MP's row count.
SELECT count(*) FROM measurement WHERE metering_point_id = $1;
