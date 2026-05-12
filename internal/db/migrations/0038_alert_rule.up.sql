-- 0038_alert_rule.up.sql
-- Phase 6 — Plan 06-01 (D-01..D-07 + D-17): persistent alert rule table.
--
-- One row per rule. Rules are soft-deleted via disabled_at (D-04) — fired-alert
-- history survives disable + re-enable. Cooldown (D-05) lives on the row;
-- workers enforce it in-engine via `last_fired_at + cooldown_seconds < now()`
-- before evaluating a rule against its target.
--
-- Severity defaults to 'critical' (D-07); operator can override per rule.
-- quiet_window_start/end + flow_threshold + days_of_week support the
-- anomaly_quiet_hour rule kind (D-17 — water leak signature).
--
-- CHECK (scope_kind = 'global' OR scope_id IS NOT NULL) enforces that only
-- the global-scope rule kinds (offline_gateway across all gateways, e.g.)
-- can run without a target UUID.

CREATE TABLE alert_rule (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_kind        TEXT NOT NULL CHECK (rule_kind IN (
        'threshold_instantaneous','threshold_hourly','threshold_daily',
        'offline_device','offline_gateway',
        'anomaly_p95','anomaly_iqr','anomaly_quiet_hour'
    )),
    scope_kind       TEXT NOT NULL CHECK (scope_kind IN (
        'metering_point','site','device','gateway','global'
    )),
    scope_id         UUID,
    high_bound       DOUBLE PRECISION,
    low_bound        DOUBLE PRECISION,
    comparison       TEXT CHECK (comparison IS NULL OR comparison IN ('gt','gte','lt','lte','eq')),
    unit             TEXT,
    -- D-17 anomaly_quiet_hour extension columns:
    quiet_window_start TIME,
    quiet_window_end   TIME,
    flow_threshold     DOUBLE PRECISION DEFAULT 0.0,
    days_of_week       INTEGER,   -- bitmask Mon=1,Tue=2,...,Sun=64; NULL=all days
    -- D-07 severity + D-05 cooldown + D-06 name/notes:
    severity         TEXT NOT NULL DEFAULT 'critical' CHECK (severity IN ('info','warning','critical')),
    name             TEXT,
    notes            TEXT,
    cooldown_seconds INTEGER NOT NULL DEFAULT 900 CHECK (cooldown_seconds >= 0),
    last_fired_at    TIMESTAMPTZ,
    -- D-04 soft-delete:
    disabled_at      TIMESTAMPTZ,
    created_by       UUID REFERENCES "user"(id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- scope_kind='global' is the only kind allowed to have NULL scope_id:
    CONSTRAINT alert_rule_scope_id_required CHECK (scope_kind = 'global' OR scope_id IS NOT NULL)
);

-- Hot paths:
--   * Worker eval cycle: SELECT WHERE rule_kind=$1 AND disabled_at IS NULL.
--   * UI rule library: WHERE scope_kind=$1 AND scope_id=$2 AND disabled_at IS NULL.
CREATE INDEX alert_rule_active_kind_idx ON alert_rule (rule_kind) WHERE disabled_at IS NULL;
CREATE INDEX alert_rule_scope_idx       ON alert_rule (scope_kind, scope_id) WHERE disabled_at IS NULL;

-- touch_updated_at() was defined in 0002_users and reused project-wide.
CREATE TRIGGER alert_rule_set_updated_at
BEFORE UPDATE ON alert_rule
FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

COMMENT ON TABLE alert_rule IS 'Phase 6 D-01..D-07 + D-17: persistent alert rule definitions. Soft-deletable via disabled_at; cooldown enforced in-engine via last_fired_at.';
COMMENT ON COLUMN alert_rule.cooldown_seconds IS 'D-05: per-rule cooldown window. Default 900s (15 min). Workers MUST gate on `last_fired_at + cooldown_seconds < now()` before raising a new fire.';
COMMENT ON COLUMN alert_rule.disabled_at IS 'D-04: soft delete. Disabled rules do not fire; existing fired-alert history stays queryable.';
