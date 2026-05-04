-- 0017_binding_changed_trigger.down.sql
DROP TRIGGER IF EXISTS binding_notify_after_update ON binding;
DROP TRIGGER IF EXISTS binding_notify_after_insert ON binding;
DROP FUNCTION IF EXISTS binding_notify_changed();
