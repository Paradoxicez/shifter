-- 0021_measurement_inserted_trigger.up.sql
-- D-02: compact 7-field NOTIFY payload (metering_point_id, time, cumulative_value,
-- instant_value, quality, battery_pct, rssi) ≤200B. NEVER include the bulky
-- columns (8KB pg_notify cap; silent truncation risk).
-- D-03: NOTIFY is transactional — listeners only see the event after COMMIT, so
-- the UI never reflects unpersisted data (DASH-03 success criterion).
-- TimescaleDB auto-propagates parent-table triggers to chunks (issue 2304).

CREATE OR REPLACE FUNCTION measurement_notify() RETURNS trigger AS $$
DECLARE
    payload TEXT;
BEGIN
    payload := json_build_object(
        'metering_point_id', NEW.metering_point_id,
        'time',              to_char(NEW.time AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
        'cumulative_value',  NEW.cumulative_value,
        'instant_value',     NEW.instant_value,
        'quality',           NEW.quality,
        'battery_pct',       NEW.battery_pct,
        'rssi',              NEW.rssi
    )::text;
    PERFORM pg_notify('measurement_inserted', payload);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER measurement_inserted_notify
    AFTER INSERT ON measurement
    FOR EACH ROW
    EXECUTE FUNCTION measurement_notify();
