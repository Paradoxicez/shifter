-- 0048_audit_vocab_alert_prune.up.sql
-- Phase 6 — Plan 06-11: adds 'alert.pruned' to the audit_log action vocabulary.
--
-- The alerts retention prune worker (AlertsPruneWorker) writes an audit row
-- per cycle with action='alert.pruned'. This is semantically distinct from
-- 'alert.cleared' (condition resolved) — a retention-aged-out alert is pruned
-- by the system, not cleared by an evaluator or operator.
--
-- Migration number: 0048 — plan 06-11 originally claimed 0047 but that was
-- already taken by 06-10 (backup_thresholds). Rule 3 deviation documented in
-- 06-11-ops-hardening-doctor-runbook-SUMMARY.md.
--
-- Pattern: DROP + re-ADD CHECK constraint (mirrors 0020, 0031, 0034, 0035,
-- 0036, 0037). Carries every action from 0037 plus 'alert.pruned'.

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
    -- Phase 6 backup/restore + audit prune (D-51) + audit export (D-35):
    'backup.start','backup.complete','backup.failed','backup.restore','audit.prune','audit.export',
    -- Phase 6 Plan 06-11: alerts retention prune (distinct from alert.cleared):
    'alert.pruned'
));
