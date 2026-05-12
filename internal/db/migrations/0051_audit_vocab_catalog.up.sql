-- 0051_audit_vocab_catalog.up.sql
-- Phase 7 Plan 04: add catalog profile action vocab to audit_log CHECK constraint.
-- Follows the DROP+re-ADD pattern established in 0020, 0031, 0034..0037, 0048, 0049.
--
-- Adds 3 new action literals for the Vendor Catalog HTTP API (V2-VEND-01):
--   catalog.profile.imported   — POST /api/catalog/import creates a device_profile
--   catalog.profile.updated    — POST /api/catalog/{id}/update applies per-field diff
--   catalog.profile.codec_resynced — reserved for future explicit codec re-sync tooling
--
-- Entity type device_profile already exists in the CHECK constraint; no change needed.

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
    -- Phase 7 Plan 07-04: catalog profile mutations.
    'catalog.profile.imported',
    'catalog.profile.updated',
    'catalog.profile.codec_resynced'
));
