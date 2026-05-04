-- 0016_audit_log.up.sql
-- AUDIT-01: every state-changing action on Site, Metering Point, Device,
-- Device Profile, and Binding writes a row here. Auth-event auditing
-- (login/logout/role-change) is deferred to Phase 6 per D-21.
--
-- D-22: single audit_log table with 10 columns (id added as PK; D-22 lists 9).
-- D-23: writes are SYNCHRONOUS in the SAME txn as the domain mutation
--       (Pitfall 7 mitigation — async = compliance gap).
-- D-24: before/after capture changed fields only (diff-like). Empty-diff rows
--       are still allowed (e.g. a re-save with no semantic change).
-- D-06: audit_log is a REGULAR Postgres table, NOT a hypertable. Hundreds of
--       rows per day per single-tenant install is a regular B-tree workload;
--       Phase 6 re-evaluates if volume justifies converting.
--
-- Security: rows are INSERT-ONLY. UPDATE / DELETE rejected by trigger
-- (RESEARCH Security Domain "Audit log tampering" — T-02-04-02 mitigation).
-- Even an admin acting through the application layer cannot retroactively
-- edit the audit trail; tampering requires DBA-level direct DB access, which
-- is auditable at the OS layer.
--
-- ON DELETE SET NULL on user_id: Phase 6 will add user mgmt with disable
-- (USER-01 prefers disable over hard-delete). Defensive: if a user IS hard
-- deleted (admin emergency), audit rows survive with user_id NULL rather than
-- cascading away the audit trail.

CREATE TABLE audit_log (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    time         TIMESTAMPTZ NOT NULL DEFAULT now(),
    user_id      UUID NULL REFERENCES "user"(id) ON DELETE SET NULL,
    action       TEXT NOT NULL,
    entity_type  TEXT NOT NULL,
    entity_id    UUID NOT NULL,
    before       JSONB,
    after        JSONB,
    notes        TEXT,
    request_id   TEXT,
    CONSTRAINT audit_log_action_valid CHECK (action IN (
        'create','update','archive','restore','swap','decommission',
        'profile_create','profile_update','binding_open','binding_close',
        'rollover_detected'
    )),
    CONSTRAINT audit_log_entity_type_valid CHECK (entity_type IN (
        'site','metering_point','device','device_profile','binding'
    ))
);

-- Browse / filter hot-path indexes (Phase 6 audit viewer):
--   * Time DESC for the default "most recent first" listing.
--   * (user_id, time DESC) for "all actions by Alice".
--   * (entity_type, entity_id, time DESC) for "history of this MP".
--   * Partial on request_id for tracing a single API call across rows.
CREATE INDEX audit_log_time_idx        ON audit_log (time DESC);
CREATE INDEX audit_log_user_idx        ON audit_log (user_id, time DESC);
CREATE INDEX audit_log_entity_idx      ON audit_log (entity_type, entity_id, time DESC);
CREATE INDEX audit_log_request_id_idx  ON audit_log (request_id) WHERE request_id IS NOT NULL;

-- Audit log tampering mitigation (T-02-04-02): prevent UPDATE / DELETE.
-- The trigger fires BEFORE the operation and raises an exception that the
-- caller cannot swallow without committing the txn (which is rolled back).
-- Application code MUST only INSERT — any UPDATE / DELETE on audit_log is a
-- bug or an attack; the DB rejects both.
CREATE OR REPLACE FUNCTION audit_log_reject_modification() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is INSERT-ONLY (rejected % on row %)', TG_OP, OLD.id;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER audit_log_no_update
    BEFORE UPDATE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_reject_modification();

CREATE TRIGGER audit_log_no_delete
    BEFORE DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_reject_modification();
