-- 0052_anomaly_compat_check.up.sql
-- Phase 7 Plan 09b: extend alert_rule.rule_kind CHECK constraint to include
-- 'reverse_flow_increase' (D-46). In Postgres, CHECK constraints cannot be
-- altered in-place; we drop the old constraint and add a new one.
--
-- Also adds 'battery_low' which was registered in the application layer
-- (Phase 6) but never added to the DB CHECK. This migration tightens
-- both in one atomic DDL operation.

ALTER TABLE alert_rule
    DROP CONSTRAINT IF EXISTS alert_rule_rule_kind_check;

ALTER TABLE alert_rule
    ADD CONSTRAINT alert_rule_rule_kind_check CHECK (rule_kind IN (
        'threshold_instantaneous','threshold_hourly','threshold_daily',
        'offline_device','offline_gateway',
        'anomaly_p95','anomaly_iqr','anomaly_quiet_hour',
        'battery_low',
        'reverse_flow_increase'
    ));
