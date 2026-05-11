-- 0020_audit_log_vocabulary.down.sql
-- Reverts audit_log CHECK vocabulary back to the Phase 2 set (mirrors 0016).
-- WARNING: If any audit_log rows already use Phase 3 vocab (e.g.
-- 'gateway.create' or entity_type 'gateway'/'import_job'), the ADD CONSTRAINT
-- will fail with 23514. This is intentional for production safety — a dev
-- reset path can TRUNCATE audit_log first, but the down migration MUST NOT
-- silently delete audit rows.

ALTER TABLE audit_log DROP CONSTRAINT audit_log_action_valid;
ALTER TABLE audit_log ADD CONSTRAINT audit_log_action_valid CHECK (action IN (
    'create','update','archive','restore','swap','decommission',
    'profile_create','profile_update','binding_open','binding_close',
    'rollover_detected'
));

ALTER TABLE audit_log DROP CONSTRAINT audit_log_entity_type_valid;
ALTER TABLE audit_log ADD CONSTRAINT audit_log_entity_type_valid CHECK (entity_type IN (
    'site','metering_point','device','device_profile','binding'
));
