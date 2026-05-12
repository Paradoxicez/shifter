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
-- expression for gateway_offline mirrors D-15's 3× threshold (i.e. a
-- gateway is "offline" when it has been silent for 3 × the device's
-- expected interval — same yardstick the device uses).
SELECT
    d.id                                              AS device_id,
    d.name                                            AS device_label,
    d.last_seen_at                                    AS last_uplink_at,
    dp.expected_interval_s                            AS expected_interval_s,
    g.id                                              AS gateway_id,
    g.name                                            AS gateway_label,
    g.last_seen_at                                    AS gateway_last_seen_at,
    (
        g.id IS NOT NULL
        AND g.last_seen_at IS NOT NULL
        AND now() - g.last_seen_at > make_interval(secs => 3 * dp.expected_interval_s)
    )                                                 AS gateway_offline
FROM device d
JOIN device_profile dp ON d.device_profile_id = dp.id
LEFT JOIN gateway g    ON d.gateway_id = g.id
WHERE d.decommissioned_at IS NULL
  AND d.last_seen_at IS NOT NULL
  AND now() - d.last_seen_at > make_interval(secs => 3 * dp.expected_interval_s);

-- name: ListHysteresisClearOffline :many
-- D-15 hysteresis: devices currently 'firing' an offline alert whose
-- last_uplink is back inside 2× expected_interval — those clear.
--
-- Compared to D-15's STRICTER fire threshold (3×), the clear threshold is
-- LOOSER (<2×) so a device that just barely recovered does not immediately
-- re-fire on the next cycle if its uplinks bounce back into the 2×–3× band.
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
  AND now() - d.last_seen_at < make_interval(secs => 2 * dp.expected_interval_s);

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
