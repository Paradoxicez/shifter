-- 0052_anomaly_compat_check.down.sql
-- Revert: restore the Phase 6 rule_kind CHECK (without battery_low and
-- reverse_flow_increase).

ALTER TABLE alert_rule
    DROP CONSTRAINT IF EXISTS alert_rule_rule_kind_check;

ALTER TABLE alert_rule
    ADD CONSTRAINT alert_rule_rule_kind_check CHECK (rule_kind IN (
        'threshold_instantaneous','threshold_hourly','threshold_daily',
        'offline_device','offline_gateway',
        'anomaly_p95','anomaly_iqr','anomaly_quiet_hour'
    ));
