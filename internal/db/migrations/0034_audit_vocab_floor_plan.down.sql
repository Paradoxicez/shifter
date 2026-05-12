-- 0034_audit_vocab_floor_plan.down.sql
-- Restores audit_log CHECK vocabulary to the Phase 5 report-only set (0031).
ALTER TABLE audit_log DROP CONSTRAINT IF EXISTS audit_log_action_valid;
ALTER TABLE audit_log ADD CONSTRAINT audit_log_action_valid CHECK (action IN (
    'create','update','archive','restore','swap','decommission',
    'profile_create','profile_update','binding_open','binding_close',
    'rollover_detected',
    'gateway.create','gateway.update','gateway.archive','gateway.restore',
    'device.bulk_import','device.reveal_secrets',
    'report.generate'
));

ALTER TABLE audit_log DROP CONSTRAINT IF EXISTS audit_log_entity_type_valid;
ALTER TABLE audit_log ADD CONSTRAINT audit_log_entity_type_valid CHECK (entity_type IN (
    'site','metering_point','device','device_profile','binding',
    'gateway','import_job',
    'report'
));
