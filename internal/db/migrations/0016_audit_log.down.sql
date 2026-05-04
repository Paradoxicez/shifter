-- 0016_audit_log.down.sql
DROP TRIGGER IF EXISTS audit_log_no_delete ON audit_log;
DROP TRIGGER IF EXISTS audit_log_no_update ON audit_log;
DROP FUNCTION IF EXISTS audit_log_reject_modification();
DROP INDEX IF EXISTS audit_log_request_id_idx;
DROP INDEX IF EXISTS audit_log_entity_idx;
DROP INDEX IF EXISTS audit_log_user_idx;
DROP INDEX IF EXISTS audit_log_time_idx;
DROP TABLE IF EXISTS audit_log;
