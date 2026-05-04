-- 0017_binding_changed_trigger.up.sql
-- D-25 (eventless cache invalidation) + Pitfall 12 mitigation:
-- Whenever a binding row is created or its valid_to flips from NULL → ts
-- (binding closed at swap), emit NOTIFY binding_changed with the affected
-- device's dev_eui (lowercase) as payload. The resolver process LISTENs on
-- this channel through a dedicated pgx connection and drops the cache entry
-- for that dev_eui — the next uplink for that EUI re-loads from the DB.
--
-- Why both INSERT and UPDATE-of-valid_to (not all UPDATE):
--   * INSERT — a new binding has appeared (e.g. brand-new MP, or the second
--     half of a swap). The resolver must re-load even if it already had a
--     stale entry for this dev_eui.
--   * UPDATE OF valid_to — the binding has been closed (first half of a
--     swap). The resolver's cached "still active" view of this dev_eui is
--     now wrong; force a re-load. Other UPDATEs (e.g. UpdateBindingLastRaw,
--     AdvanceReadingOffset) DO NOT need to invalidate — those modify state
--     the resolver doesn't cache (last_raw_value / reading_offset live on
--     the cached row, but the rollover/ingest path holds its own per-binding
--     state and does not rely on resolver cache freshness for those columns).
--
-- Payload semantics: we always look up dev_eui from the device table by
-- NEW.device_id. dev_eui is stored lowercase by schema (CHECK in 0012);
-- the listener still applies strings.ToLower defensively.
--
-- Trigger does NOT fire for DELETE — bindings are never deleted (D-19);
-- soft-close via UPDATE OF valid_to is the only "removal" path. If a future
-- migration ever needs to hard-delete a binding row, that's an operational
-- DBA action and the resolver's reconnect-on-startup will pick up the new
-- state on next process restart.

CREATE OR REPLACE FUNCTION binding_notify_changed() RETURNS trigger AS $$
DECLARE
    deveui_text TEXT;
BEGIN
    IF TG_OP = 'INSERT' THEN
        SELECT dev_eui INTO deveui_text FROM device WHERE id = NEW.device_id;
        IF deveui_text IS NOT NULL THEN
            PERFORM pg_notify('binding_changed', deveui_text);
        END IF;
    ELSIF TG_OP = 'UPDATE' THEN
        -- Only fire when valid_to flips from NULL → ts (binding closed at swap).
        IF OLD.valid_to IS NULL AND NEW.valid_to IS NOT NULL THEN
            SELECT dev_eui INTO deveui_text FROM device WHERE id = NEW.device_id;
            IF deveui_text IS NOT NULL THEN
                PERFORM pg_notify('binding_changed', deveui_text);
            END IF;
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER binding_notify_after_insert
    AFTER INSERT ON binding
    FOR EACH ROW EXECUTE FUNCTION binding_notify_changed();

CREATE TRIGGER binding_notify_after_update
    AFTER UPDATE OF valid_to ON binding
    FOR EACH ROW EXECUTE FUNCTION binding_notify_changed();
