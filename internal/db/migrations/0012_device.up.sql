-- 0012_device.up.sql
-- Device — physical LoRaWAN endpoint. dev_eui is the LoRaWAN-canonical 16-char
-- lowercase hex string (CS uses lowercase across v4 gRPC + MQTT topics).
--
-- D-15: decommission = closes active binding + sets decommissioned_at; the
-- device row stays for historical reference (no hard-delete). The active
-- binding closing is enforced by the application layer (Plan 02-07 swap
-- commit), this column is only the operator-visible "is this device retired?"
-- flag — partial-index here keeps "active devices" list queries fast.
--
-- cs_device_uuid persists the ChirpStack DeviceService.Create return so
-- subsequent Get/Update/Delete calls can address by id rather than dev_eui.
-- NULL during the brief window between Shifter row insert and CS gRPC ack.
--
-- last_seen_at is updated by the ingest pipeline on every uplink (Plan 02-09).
--
-- DEV-09: AppKey is intentionally NOT stored in Shifter — it lives in
-- ChirpStack only via DeviceService.CreateKeys. join_eui is OK to store
-- (LoRaWAN routing identifier, not a secret).
CREATE TABLE device (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dev_eui            TEXT NOT NULL UNIQUE,
    name               TEXT NOT NULL,
    device_profile_id  UUID NOT NULL REFERENCES device_profile(id) ON DELETE RESTRICT,
    cs_device_uuid     TEXT NULL,
    join_eui           TEXT NULL,
    description        TEXT,
    last_seen_at       TIMESTAMPTZ NULL,
    decommissioned_at  TIMESTAMPTZ NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT device_dev_eui_lower      CHECK (dev_eui = lower(dev_eui)),
    CONSTRAINT device_dev_eui_hex16      CHECK (dev_eui ~ '^[0-9a-f]{16}$'),
    CONSTRAINT device_join_eui_hex16     CHECK (join_eui IS NULL OR join_eui ~ '^[0-9a-f]{16}$'),
    CONSTRAINT device_name_not_empty     CHECK (length(name) > 0)
);

CREATE INDEX device_profile_idx        ON device (device_profile_id);
-- Partial index over live (not-yet-decommissioned) devices keeps the common
-- "active devices" listing fast; matches the soft-delete pattern from
-- site/metering_point/device_profile.
CREATE INDEX device_decommissioned_idx ON device (decommissioned_at) WHERE decommissioned_at IS NULL;
CREATE INDEX device_last_seen_idx      ON device (last_seen_at DESC NULLS LAST);

CREATE TRIGGER device_touch
    BEFORE UPDATE ON device
    FOR EACH ROW
    EXECUTE FUNCTION touch_updated_at();
