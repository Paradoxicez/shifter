-- 0054_audit_vocab_report_template.up.sql
-- Phase 7 Plan 11a: add report_template action vocab + entity type to audit_log
-- CHECK constraints. Follows the DROP+re-ADD pattern established in migrations
-- 0020, 0031, 0034..0037, 0048, 0049, 0051.
--
-- Adds 3 new action literals for Saved Report Templates (UX-POWER Surface 6):
--   report_template.created  — POST /api/reports/templates creates a template
--   report_template.updated  — PATCH /api/reports/templates/{id} renames/edits
--   report_template.deleted  — DELETE /api/reports/templates/{id} removes a template
--
-- Adds 1 new entity type:
--   report_template           — entity_type for the above audit rows

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
    'settings.identity_update',
    'catalog.profile.imported',
    'catalog.profile.updated',
    'catalog.profile.codec_resynced',
    -- Phase 7 Plan 11a: report template lifecycle.
    'report_template.created',
    'report_template.updated',
    'report_template.deleted'
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
    'install_identity',
    -- Phase 7 Plan 11a: saved report template.
    'report_template'
));
