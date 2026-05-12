-- 0037_audit_vocab_phase6.up.sql
-- Phase 6 — Plan 06-01: extends audit_log CHECK vocabulary for the Phase 6
-- product surfaces (alerts, user management, audit prune, backup) and the
-- D-30 auth-event retrofit that closes Phase 2 D-21's deferred work.
-- Following the same DROP + re-ADD pattern as 0020, 0031, 0034, 0035, 0036.
--
-- New actions (27 strings):
--   D-30 operator-visible auth events:
--     auth.login_success, auth.login_failed, auth.logout,
--     auth.password_change, auth.password_reset_by_admin, auth.session_revoked
--   D-30 user mgmt:
--     user.create, user.update, user.disable, user.enable, user.role_change
--   Phase 6 alerts:
--     alert.rule_create, alert.rule_update, alert.rule_disable, alert.rule_enable,
--     alert.fired, alert.cleared, alert.acknowledged, alert.snoozed, alert.muted,
--     alert.test_fired
--   D-51 + D-35 audit + Phase 6 backup:
--     backup.start, backup.complete, backup.failed, backup.restore,
--     audit.prune, audit.export
--
-- New entity types (6 strings): user, session, alert_rule, alert, backup_run,
-- audit_log (the last one for the audit.prune meta-row per D-51).

ALTER TABLE audit_log DROP CONSTRAINT audit_log_action_valid;
ALTER TABLE audit_log ADD CONSTRAINT audit_log_action_valid CHECK (action IN (
    'create','update','archive','restore','swap','decommission',
    'profile_create','profile_update','binding_open','binding_close',
    'rollover_detected',
    'gateway.create','gateway.update','gateway.archive','gateway.restore',
    'device.bulk_import','device.reveal_secrets',
    'report.generate',
    'floor_plan.upload','floor_plan.replace_image','floor_plan.rename','floor_plan.delete',
    'placement.pin','placement.nudge','placement.remove',
    'settings.retention_change',
    -- Phase 6 (D-30 operator-visible auth events):
    'auth.login_success','auth.login_failed','auth.logout',
    'auth.password_change','auth.password_reset_by_admin','auth.session_revoked',
    -- Phase 6 user management:
    'user.create','user.update','user.disable','user.enable','user.role_change',
    -- Phase 6 alerts:
    'alert.rule_create','alert.rule_update','alert.rule_disable','alert.rule_enable',
    'alert.fired','alert.cleared','alert.acknowledged','alert.snoozed','alert.muted','alert.test_fired',
    -- Phase 6 backup/restore + audit prune (D-51) + audit export (D-35 — see Plan 06-07):
    'backup.start','backup.complete','backup.failed','backup.restore','audit.prune','audit.export'
));

ALTER TABLE audit_log DROP CONSTRAINT audit_log_entity_type_valid;
ALTER TABLE audit_log ADD CONSTRAINT audit_log_entity_type_valid CHECK (entity_type IN (
    'site','metering_point','device','device_profile','binding',
    'gateway','import_job',
    'report',
    'floor_plan',
    'placement',
    'retention_config',
    -- Phase 6 entity types (D-30 + D-51):
    'user','session','alert_rule','alert','backup_run','audit_log'
));
