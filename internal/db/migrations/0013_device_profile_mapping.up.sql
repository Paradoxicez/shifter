-- 0013_device_profile_mapping.up.sql
-- Mapping rows from decoded JSON to canonical measurement columns (D-08, DATA-09).
--
-- json_pointer: RFC 6901 path string (e.g. "/cumulative_l", "/phase_a/voltage").
--   Empty string addresses the whole decoded document — allowed (rare, but
--   useful for codecs that emit a single scalar at the root).
--
-- target: either a canonical measurement column name (e.g. "cumulative_value",
--   "instant_value", "battery_pct") OR an "extra.<key>" freeform path that
--   lands in measurement.extra JSONB at <key>. The Plan 02-08 profile editor
--   surfaces a typeahead populated from the canonical-column list + lets
--   operators free-type "extra.*" entries (D-08).
--
-- scale: multiplied with the decoded numeric value before persistence
--   (e.g. 0.001 to convert mV → V; 1000 to convert kWh → Wh). NUMERIC keeps
--   precision exact for the per-vendor scale factors that frequently appear.
--
-- data_type: numeric|int|bool|text — drives the type coercion in normalize.go
--   (Plan 02-09). `numeric` covers fractional canonical fields
--   (cumulative_value, flow_rate); `int` for SMALLINT cols (battery_pct,
--   rssi); `bool` for leak_detected/tamper_detected; `text` for extra.*
--   string fields. (Discretion item 7.)
--
-- position drives deterministic mapping pass order in the normalize engine —
--   stable order matters when a later mapping references a value materialized
--   by an earlier one (e.g. compute fan-out fields from a base reading).
--
-- FK ON DELETE CASCADE: profile delete cascades mappings (a profile with no
--   mappings is meaningless). device_profile uses soft-delete archived_at,
--   so this only fires on hard delete which is admin-emergency-only.
--
-- UNIQUE (device_profile_id, target) prevents two mappings claiming the same
--   canonical column on one profile (T-02-03-03 mitigation) — the editor UI
--   would otherwise allow a confusing "last write wins" footgun.
CREATE TABLE device_profile_mapping (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_profile_id  UUID NOT NULL REFERENCES device_profile(id) ON DELETE CASCADE,
    json_pointer       TEXT NOT NULL,
    target             TEXT NOT NULL,
    scale              NUMERIC NOT NULL DEFAULT 1,
    data_type          TEXT NOT NULL,
    position           INT NOT NULL DEFAULT 0,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT device_profile_mapping_data_type_valid
        CHECK (data_type IN ('numeric', 'int', 'bool', 'text')),
    CONSTRAINT device_profile_mapping_pointer_starts_with_slash
        CHECK (json_pointer = '' OR json_pointer LIKE '/%'),
    CONSTRAINT device_profile_mapping_target_not_empty
        CHECK (length(target) > 0),
    CONSTRAINT device_profile_mapping_unique_target_per_profile
        UNIQUE (device_profile_id, target)
);

CREATE INDEX device_profile_mapping_profile_idx
    ON device_profile_mapping (device_profile_id, position);

CREATE TRIGGER device_profile_mapping_touch
    BEFORE UPDATE ON device_profile_mapping
    FOR EACH ROW
    EXECUTE FUNCTION touch_updated_at();
