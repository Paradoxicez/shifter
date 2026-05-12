-- 0049_audit_vocab_identity.up.sql
-- Gap closure Plan 06-12 (SETT-02): adds settings.identity_update action and
-- install_identity entity type to audit_log CHECK constraints.
-- Follows the DROP+re-ADD pattern established in 0020, 0031, 0034..0037, 0048.
--
-- This migration closes the SETT-02 gap identified by the Phase 6 verification:
-- the /api/settings/identity backend was missing; the PatchIdentityHandler
-- writes a settings.identity_update audit row in the same Serializable tx.

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
    'auth.login_success','auth.login_failed','auth.logout',
    'auth.password_change','auth.password_reset_by_admin','auth.session_revoked',
    'user.create','user.update','user.disable','user.enable','user.role_change',
    'alert.rule_create','alert.rule_update','alert.rule_disable','alert.rule_enable',
    'alert.fired','alert.cleared','alert.acknowledged','alert.snoozed','alert.muted','alert.test_fired',
    'backup.start','backup.complete','backup.failed','backup.restore','audit.prune','audit.export',
    'alert.pruned',
    -- Gap closure Plan 06-12 (SETT-02):
    'settings.identity_update'
));

ALTER TABLE audit_log DROP CONSTRAINT audit_log_entity_type_valid;
ALTER TABLE audit_log ADD CONSTRAINT audit_log_entity_type_valid CHECK (entity_type IN (
    'site','metering_point','device','device_profile','binding',
    'gateway','import_job',
    'report',
    'floor_plan',
    'placement',
    'retention_config',
    'user','session','alert_rule','alert','backup_run','audit_log',
    -- Gap closure Plan 06-12 (SETT-02):
    'install_identity'
));
