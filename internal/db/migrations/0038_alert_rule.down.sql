-- 0038_alert_rule.down.sql
DROP TRIGGER IF EXISTS alert_rule_set_updated_at ON alert_rule;
DROP TABLE IF EXISTS alert_rule;
