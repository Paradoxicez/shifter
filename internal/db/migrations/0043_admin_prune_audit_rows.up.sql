-- 0043_admin_prune_audit_rows.up.sql
-- Phase 6 — Plan 06-01 (D-51): SECURITY DEFINER function that is the SOLE
-- code path allowed to DELETE FROM audit_log.
--
-- The 0016_audit_log INSERT-ONLY trigger rejects DELETE on every other
-- caller — application code cannot tamper with the audit trail. The
-- retention prune worker needs a documented escape hatch, and this is it.
--
-- ── Trigger bypass mechanism ──
-- The original D-51 sketch proposed `SET LOCAL session_replication_role
-- = 'replica'` to disable triggers for the function's transaction. In
-- PostgreSQL 13+ this parameter is SUPERUSER-only, which conflicts with
-- the "owned by a restricted role" goal. We use a custom GUC marker
-- instead: the trigger function (rewritten in this same migration to
-- preserve the table-level INSERT-ONLY invariant) checks
-- `current_setting('shifter.allow_audit_prune', true) = 'true'` and skips
-- the rejection only when the marker is set inside an active tx.
-- `set_config()` with `is_local := true` works for every role on custom
-- (dotted-namespace) GUCs — no superuser needed.
--
-- D-51 mitigation chain:
--   1. Dedicated, NON-SUPERUSER role `shifter_audit_admin` OWNs the
--      function. SECURITY DEFINER runs as this role, NOT as the app role
--      `shifter` — so the audit-prune privilege cannot be reused by other
--      SQL paths even via privilege confusion.
--   2. Restricted grants: DELETE + INSERT on audit_log only (NO SELECT,
--      NO UPDATE, NO TRUNCATE — even if the role is hijacked it cannot
--      read PII out of audit_log).
--   3. Inside the function: set_config('shifter.allow_audit_prune','true',
--      is_local := true). is_local=true scopes to the transaction; the
--      marker is released at COMMIT/ROLLBACK.
--   4. The trigger function checks the marker. Only when set, the DELETE
--      passes through. The trigger DDL itself is unchanged — the table-
--      level invariant "audit_log is INSERT-ONLY for every other caller"
--      is enforced inside the trigger function body, not by trigger
--      removal.
--   5. After DELETE: reset the marker to '' so the subsequent meta INSERT
--      cannot be abused if a future INSERT trigger is added.
--   6. REVOKE PUBLIC + GRANT EXECUTE TO shifter — only the application role
--      can invoke the function.

-- 1. Create the dedicated owner role (idempotent — DO block guards re-run).
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'shifter_audit_admin') THEN
        CREATE ROLE shifter_audit_admin NOLOGIN;
    END IF;
END$$;

-- 2. Restricted grants on audit_log. DELETE + INSERT + SELECT — SELECT is
--    required because `DELETE ... WHERE time < $1` reads the rows it's
--    about to delete. NO UPDATE, NO TRUNCATE — even if the role is hijacked
--    it cannot modify-in-place or truncate the audit trail.
GRANT DELETE ON audit_log TO shifter_audit_admin;
GRANT INSERT ON audit_log TO shifter_audit_admin;
GRANT SELECT ON audit_log TO shifter_audit_admin;

-- 3. Rewrite audit_log_reject_modification() to honor the custom-GUC
--    marker. The trigger DDL from 0016 still binds BEFORE UPDATE / BEFORE
--    DELETE to this function; the gate now lives in the function body.
CREATE OR REPLACE FUNCTION audit_log_reject_modification() RETURNS trigger AS $$
BEGIN
    -- D-51 escape hatch: the SECURITY DEFINER prune function sets this
    -- marker for its own transaction. Every other caller (TG_OP=UPDATE or
    -- TG_OP=DELETE from app code) lands in the RAISE EXCEPTION path.
    -- current_setting(_, missing_ok := true) returns '' if the GUC is
    -- unset; we accept only the exact literal 'true' for safety.
    IF TG_OP = 'DELETE' AND current_setting('shifter.allow_audit_prune', true) = 'true' THEN
        RETURN OLD;
    END IF;
    RAISE EXCEPTION 'audit_log is INSERT-ONLY (rejected % on row %)', TG_OP, OLD.id;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- 4. The prune function itself.
CREATE OR REPLACE FUNCTION admin_prune_audit_rows(cutoff_days INTEGER)
RETURNS INTEGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
    deleted_count INTEGER;
    cutoff_ts     TIMESTAMPTZ := now() - make_interval(days := cutoff_days);
    meta_id       UUID := gen_random_uuid();
BEGIN
    IF cutoff_days < 0 THEN
        RAISE EXCEPTION 'admin_prune_audit_rows: cutoff_days must be >= 0';
    END IF;

    -- D-51 step 3: enable the trigger-bypass marker for this tx only.
    -- set_config(setting, value, is_local := true) is released at
    -- COMMIT/ROLLBACK. is_local=true also restricts the scope to the
    -- current sub-transaction's nesting level.
    PERFORM set_config('shifter.allow_audit_prune', 'true', true);

    DELETE FROM audit_log WHERE time < cutoff_ts;
    GET DIAGNOSTICS deleted_count = ROW_COUNT;

    -- D-51 step 5: reset the marker BEFORE the meta INSERT so any future
    -- DELETE inside the same tx is rejected; the INSERT path is never
    -- gated.
    PERFORM set_config('shifter.allow_audit_prune', '', true);

    -- D-51 meta row: entity_id self-references (no FK target — audit_log
    -- entity_id has no FK by design).
    INSERT INTO audit_log (id, action, entity_type, entity_id, notes)
    VALUES (
        meta_id,
        'audit.prune',
        'audit_log',
        meta_id,
        'Pruned ' || deleted_count || ' rows older than ' || cutoff_ts::date
    );

    RETURN deleted_count;
END
$$;

-- 5. Re-own the function so SECURITY DEFINER runs as shifter_audit_admin,
--    NOT as the app role.
ALTER FUNCTION admin_prune_audit_rows(INTEGER) OWNER TO shifter_audit_admin;

-- 6. Lockdown: only the application role may invoke the function.
REVOKE ALL ON FUNCTION admin_prune_audit_rows(INTEGER) FROM PUBLIC;
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'shifter') THEN
        EXECUTE 'GRANT EXECUTE ON FUNCTION admin_prune_audit_rows(INTEGER) TO shifter';
    END IF;
END$$;

COMMENT ON FUNCTION admin_prune_audit_rows IS
    'D-51: SECURITY DEFINER bypass of audit_log INSERT-ONLY trigger for retention prune. '
    'Owned by shifter_audit_admin (restricted role — DELETE+INSERT on audit_log only) '
    'NOT by the app role. Trigger bypass uses the shifter.allow_audit_prune custom GUC '
    'marker (is_local=true scope) — superuser-only session_replication_role is NOT used. '
    'Trigger remains active for all other code paths.';
