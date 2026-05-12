-- Alerts — Phase 6 Plan 06-02 threshold + offline evaluator queries.
--
-- D-12 payload shape is built in Go (internal/alert/payload.go) — these
-- queries are the data sources the workers feed into BuildPayload.
--
-- Schema note (06-02 Rule 3 deviation): the Phase 2 schema names columns
-- differently than the 06-RESEARCH SQL did — these queries use the actual
-- column names (`device.name` not `device.label`, `device.last_seen_at`
-- not `last_uplink_at`, `device.device_profile_id` not `profile_id`,
-- `device.decommissioned_at` not `disabled_at`). Migration 0045 added
-- `device.gateway_id` + `gateway.last_seen_at` to satisfy ALERT-03.

-- name: ListOfflineDevicesWithGatewayStatus :many
-- D-15 + D-14: returns device-offline candidates AND a gateway-offline flag
-- per-row. Worker uses the flag to suppress device alerts behind downed
-- gateways and to emit a single gateway-offline alert.
--
-- The single LEFT JOIN keeps this an O(active-devices) query — without it
-- the worker would N+1-fetch gateway last_seen_at per row. The boolean
-- expression for gateway_offline mirrors the per-profile offline threshold
-- (D-41/D-43 Phase 7: expected_uplink_interval_seconds × offline_threshold_multiplier).
SELECT
    d.id                                              AS device_id,
    d.name                                            AS device_label,
    d.last_seen_at                                    AS last_uplink_at,
    dp.expected_uplink_interval_seconds               AS expected_interval_s,
    dp.offline_threshold_multiplier                   AS offline_threshold_multiplier,
    g.id                                              AS gateway_id,
    g.name                                            AS gateway_label,
    g.last_seen_at                                    AS gateway_last_seen_at,
    (
        g.id IS NOT NULL
        AND g.last_seen_at IS NOT NULL
        AND now() - g.last_seen_at > make_interval(secs => dp.expected_uplink_interval_seconds::float8 * dp.offline_threshold_multiplier)
    )                                                 AS gateway_offline
FROM device d
JOIN device_profile dp ON d.device_profile_id = dp.id
LEFT JOIN gateway g    ON d.gateway_id = g.id
WHERE d.decommissioned_at IS NULL
  AND d.last_seen_at IS NOT NULL
  AND now() - d.last_seen_at > make_interval(secs => dp.expected_uplink_interval_seconds::float8 * dp.offline_threshold_multiplier);

-- name: ListHysteresisClearOffline :many
-- D-15 hysteresis: devices currently 'firing' an offline alert whose
-- last_uplink is back inside 1× expected_uplink_interval_seconds — those clear.
--
-- Clear threshold = 1× interval (device uplinked within its normal cadence).
-- Fire threshold = profile.offline_threshold_multiplier × interval (≥ 1.8×).
-- This guarantees the clear threshold is ALWAYS strictly less than the fire
-- threshold regardless of the per-profile multiplier, preventing both
-- immediate auto-clear (the Phase 7 Itron bug) and hysteresis flapping.
SELECT
    a.id        AS alert_id,
    d.id        AS device_id,
    a.rule_id   AS rule_id
FROM alert a
JOIN device d           ON a.target_entity_id = d.id
JOIN device_profile dp  ON d.device_profile_id = dp.id
WHERE a.state = 'firing'
  AND a.rule_kind = 'offline_device'
  AND d.last_seen_at IS NOT NULL
  AND now() - d.last_seen_at < make_interval(secs => dp.expected_uplink_interval_seconds::float8);

-- name: GetLatestMeasurementForMP :one
-- ThresholdInstantaneousWorker pulls the most-recent uplink and compares
-- instant_value against the rule's bound(s). The (metering_point_id,
-- time DESC) hot-path index on `measurement` (0015) makes this an
-- index scan + LIMIT 1.
SELECT
    time,
    metering_point_id,
    instant_value,
    cumulative_value
FROM measurement
WHERE metering_point_id = $1
ORDER BY time DESC
LIMIT 1;

-- name: GetLatestHourlyForMP :one
-- ThresholdHourlyWorker reads the most-recent 1-hour bucket from the
-- measurement_hourly continuous aggregate (0025). avg_instant is what the
-- rule's bound is compared against (a meter "averaged above threshold"
-- semantic) — sum_consumption is reported in the payload but is not the
-- breach metric.
SELECT
    bucket,
    metering_point_id,
    avg_instant,
    cumulative_delta AS sum_consumption
FROM measurement_hourly
WHERE metering_point_id = $1
ORDER BY bucket DESC
LIMIT 1;

-- name: GetLatestDailyForMP :one
-- ThresholdDailyWorker reads the most-recent 1-day bucket from the
-- measurement_daily continuous aggregate (0026). cumulative_delta is the
-- day's total consumption — the breach metric for "daily total > N".
SELECT
    bucket,
    metering_point_id,
    avg_instant,
    cumulative_delta AS sum_consumption
FROM measurement_daily
WHERE metering_point_id = $1
ORDER BY bucket DESC
LIMIT 1;

-- name: ListMeteringPointsBySite :many
-- expandScope() helper for site-scoped rules — returns MP ids + labels
-- ordered for deterministic iteration in tests.
SELECT
    id,
    name AS label
FROM metering_point
WHERE site_id = $1
  AND archived_at IS NULL
ORDER BY id;

-- name: ListAllActiveMeteringPoints :many
-- expandScope() helper for global-scoped rules.
SELECT
    id,
    name AS label
FROM metering_point
WHERE archived_at IS NULL
ORDER BY id;

-- name: GetMeteringPointLabel :one
-- expandScope() helper for metering_point-scoped rules: load the MP label
-- so the payload's target.label field is populated for the alert center.
SELECT
    id,
    name AS label
FROM metering_point
WHERE id = $1;

-- name: GetInstallDisplayName :one
-- One-shot read at the top of every eval cycle; the workers cache the
-- result locally for the cycle's per-fire payload assembly.
SELECT display_name
FROM install_identity
WHERE id = 1;

-- ============================================================================
-- Phase 6 Plan 06-03 — anomaly evaluators (D-16 cold-start gate + D-17
-- statistical rules). Queries below feed AnomalyWorker + the warmup-roster
-- surface used by Plan 06-04 (MP detail card) and Plan 06-10 (Settings →
-- Alerts warmup section).
-- ============================================================================

-- name: GetMPAnomalyCompatibility :one
-- Phase 7 D-42: returns the anomaly_compatibility + expected_uplink_interval_seconds
-- for the device profile currently bound to this metering point (via the active
-- binding — valid_to IS NULL). Used by IsMPEligibleForAnomaly and
-- CreateRuleHandler (I-2 server-side guard).
SELECT dp.anomaly_compatibility, dp.expected_uplink_interval_seconds
FROM metering_point mp
JOIN binding b    ON b.metering_point_id = mp.id AND b.valid_to IS NULL
JOIN device d     ON d.id = b.device_id
JOIN device_profile dp ON dp.id = d.device_profile_id
WHERE mp.id = $1;

-- name: IsMPEligibleForAnomaly :one
-- D-16: True iff the metering point has at least one measurement ≥ 21 days
-- old. The 21-day constant is hardcoded in v1; Phase 7 may promote it to a
-- retention_config column. Kept for backward compatibility with existing callers
-- (e.g. warmup roster UI path which always uses 21d).
SELECT EXISTS(
    SELECT 1 FROM measurement
    WHERE metering_point_id = $1
      AND time < now() - INTERVAL '21 days'
) AS eligible;

-- name: IsMPEligibleForAnomalyDays :one
-- Phase 7 D-42: parameterized version of IsMPEligibleForAnomaly — accepts
-- warmup_days as $2 so IsMPEligibleForAnomaly() can use 21d for 'full'
-- profiles and 60d for 'limited' profiles.
SELECT EXISTS(
    SELECT 1 FROM measurement
    WHERE metering_point_id = sqlc.arg(metering_point_id)::UUID
      AND time < now() - make_interval(days => sqlc.arg(warmup_days)::INT)
) AS eligible;

-- name: ListAnomalyWarmupRoster :many
-- D-16: returns days_until_eligible per active (non-archived) MP. 0 means
-- eligible; >0 means still warming up. Schema note: metering_point uses
-- archived_at for soft-delete (not disabled_at). site label column is `name`.
SELECT
    mp.id                                                  AS metering_point_id,
    mp.name                                                AS metering_point_label,
    COALESCE(s.name, '')                                   AS site_label,
    CASE
        WHEN MIN(m.time) IS NULL THEN 21
        ELSE GREATEST(0, 21 - EXTRACT(DAY FROM (now() - MIN(m.time)))::INT)
    END                                                    AS days_until_eligible
FROM metering_point mp
LEFT JOIN measurement m ON mp.id = m.metering_point_id
LEFT JOIN site s        ON mp.site_id = s.id
WHERE mp.archived_at IS NULL
GROUP BY mp.id, mp.name, s.name
ORDER BY days_until_eligible ASC, mp.name ASC;

-- name: GetMPAnomalyRules :many
-- Returns the three anomaly rules (p95, iqr, quiet_hour) that target this MP
-- (scope_kind='metering_point' AND scope_id=$1) or are global (scope_kind='global').
-- Used by the MP detail Anomaly Detection card (Plan 06-04) to render per-rule
-- toggles.
SELECT id,
       rule_kind,
       severity,
       (disabled_at IS NULL)               AS enabled,
       quiet_window_start,
       quiet_window_end,
       flow_threshold,
       days_of_week
FROM alert_rule
WHERE rule_kind IN ('anomaly_p95','anomaly_iqr','anomaly_quiet_hour')
  AND (
      (scope_kind = 'metering_point' AND scope_id = $1)
      OR scope_kind = 'global'
  );

-- name: P95BaselineForMPAndHour :one
-- D-17 Rule 1: trailing-30-day P95 of instant_value for this MP at this
-- hour-of-day. instant_value is NUMERIC in the schema; cast to DOUBLE
-- PRECISION so the percentile aggregate's result lands as float64 in Go.
-- percentile_cont returns NULL when no rows match; sqlc emits *float64.
SELECT percentile_cont(0.95) WITHIN GROUP (ORDER BY instant_value::DOUBLE PRECISION)::DOUBLE PRECISION AS p95
FROM measurement
WHERE metering_point_id = sqlc.arg(metering_point_id)::UUID
  AND time >= now() - INTERVAL '30 days'
  AND time < now()
  AND EXTRACT(HOUR FROM time)::INT = sqlc.arg(hour_of_day)::INT;

-- name: IQRBaselineForMP :one
-- D-17 Rule 2: Q1, Q3 over trailing 30 days. Worker computes
--   iqr = q3 - q1; lower = q1 - 1.5*iqr; upper = q3 + 1.5*iqr.
-- and fires when latest instant_value > upper OR < lower.
SELECT
    percentile_cont(0.25) WITHIN GROUP (ORDER BY instant_value::DOUBLE PRECISION) AS q1,
    percentile_cont(0.75) WITHIN GROUP (ORDER BY instant_value::DOUBLE PRECISION) AS q3
FROM measurement
WHERE metering_point_id = $1
  AND time >= now() - INTERVAL '30 days'
  AND time < now();

-- name: LatestInstantValueForMPAnomaly :one
-- Anomaly evaluators need just the latest instant_value (numeric → float in
-- Go via numericToFloat). Distinct from GetLatestMeasurementForMP because the
-- column projection is narrower.
SELECT instant_value, time
FROM measurement
WHERE metering_point_id = $1
ORDER BY time DESC
LIMIT 1;

-- ============================================================================
-- Phase 6 Plan 06-04 — alert center HTTP handlers (list / detail / bell
-- counts / drawer recent).
-- ============================================================================

-- name: CountAlertsUnreadBySeverity :one
-- Powers the bell badge — returns one row with critical/warning/info counts
-- of unread (firing|snoozed) non-muted alerts. The ListHandler returns this
-- alongside the page rows to save a second round-trip from the UI.
SELECT
    count(*) FILTER (WHERE severity = 'critical' AND state IN ('firing','snoozed')) AS critical_count,
    count(*) FILTER (WHERE severity = 'warning'  AND state IN ('firing','snoozed')) AS warning_count,
    count(*) FILTER (WHERE severity = 'info'     AND state IN ('firing','snoozed')) AS info_count
FROM alert
WHERE muted = FALSE;

-- name: ListRecentAlertsForDrawer :many
-- Top 10 currently-active alerts for the slide-over drawer (UI-SPEC §Surface 1).
-- Sort: critical > warning > info, then most-recent first within each tier.
SELECT a.id, a.rule_id, a.rule_kind, a.severity, a.state, a.payload, a.fired_at,
       a.target_entity_type, a.target_entity_id, a.is_test
FROM alert a
WHERE a.state IN ('firing','acknowledged','snoozed')
  AND a.muted = FALSE
ORDER BY
    CASE a.severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END ASC,
    a.fired_at DESC
LIMIT 10;

-- name: NonZeroFlowDuringQuietWindow :one
-- D-17 Rule 3 + Pitfall 9 cross-midnight OR-form.
-- Parameters (named via sqlc.arg so the generated Params struct fields read
-- semantically rather than as Column2/Column3/...):
--   metering_point_id  — the MP whose window is checked
--   flow_threshold     — instant_value > flow_threshold counts as "non-zero"
--   quiet_window_start — TIME of day the quiet window begins (install-local)
--   quiet_window_end   — TIME of day the quiet window ends   (install-local)
--   install_tz         — IANA name of the install timezone (e.g. 'Asia/Bangkok')
-- The (time AT TIME ZONE install_tz)::TIME cast reads the timestamp in
-- install-local time so "22:00–06:00" is operator-local, not UTC.
SELECT instant_value, time
FROM measurement
WHERE metering_point_id = sqlc.arg(metering_point_id)::UUID
  AND time >= now() - INTERVAL '24 hours'
  AND instant_value::DOUBLE PRECISION > sqlc.arg(flow_threshold)::DOUBLE PRECISION
  AND (
      -- Same-day window (start < end, e.g. 08:00→18:00)
      (sqlc.arg(quiet_window_start)::TIME < sqlc.arg(quiet_window_end)::TIME
       AND (time AT TIME ZONE sqlc.arg(install_tz)::TEXT)::TIME
           BETWEEN sqlc.arg(quiet_window_start)::TIME AND sqlc.arg(quiet_window_end)::TIME)
      OR
      -- Cross-midnight window (start >= end, e.g. 22:00→06:00)
      (sqlc.arg(quiet_window_start)::TIME >= sqlc.arg(quiet_window_end)::TIME
       AND ((time AT TIME ZONE sqlc.arg(install_tz)::TEXT)::TIME >= sqlc.arg(quiet_window_start)::TIME
            OR (time AT TIME ZONE sqlc.arg(install_tz)::TEXT)::TIME <= sqlc.arg(quiet_window_end)::TIME))
  )
ORDER BY time DESC
LIMIT 1;
