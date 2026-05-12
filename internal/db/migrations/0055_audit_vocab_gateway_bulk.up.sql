-- 0055_audit_vocab_gateway_bulk.up.sql
-- Phase 7 Plan 13: add gateway.bulk_imported to audit_log action CHECK constraint.
-- Follows the DROP+re-ADD pattern established in migrations 0020, 0031, 0034..0037,
-- 0048, 0049, 0051, 0054.
--
-- Adds 1 new action literal:
--   gateway.bulk_imported  — POST /api/gateways/bulk-import/commit writes one
--                            audit row per successfully imported gateway

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
    'report_template.created',
    'report_template.updated',
    'report_template.deleted',
    -- Phase 7 Plan 13: bulk gateway import per-row audit.
    'gateway.bulk_imported'
));
