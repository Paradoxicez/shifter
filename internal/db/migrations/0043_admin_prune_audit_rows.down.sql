-- 0043_admin_prune_audit_rows.down.sql
-- Reverse order: drop the function, restore the original trigger function
-- (without the GUC marker check), strip role grants, drop the role.
-- DROP ROLE IF EXISTS is idempotent.

DROP FUNCTION IF EXISTS admin_prune_audit_rows(INTEGER);

-- Restore the original audit_log_reject_modification() body (verbatim from
-- 0016_audit_log.up.sql) so the trigger reverts to unconditional rejection.
CREATE OR REPLACE FUNCTION audit_log_reject_modification() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is INSERT-ONLY (rejected % on row %)', TG_OP, OLD.id;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- Revoke table grants before dropping the role (Postgres refuses to drop a
-- role that still owns grants).
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'shifter_audit_admin') THEN
        REVOKE ALL ON audit_log FROM shifter_audit_admin;
    END IF;
END$$;

DROP ROLE IF EXISTS shifter_audit_admin;
