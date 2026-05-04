-- 0009_device_profile.up.sql
-- Device profile — vendor + model definition that captures decoder logic,
-- canonical capability vocabulary, and counter rollover modulus (D-01 layer 2,
-- D-04 capabilities, D-05 counter_modulus, D-09 codec sync state).
--
-- codec_js stays empty in the seed migration (0010); internal/profile/seed.go
-- reads //go:embed-ed *.js source files at boot and pushes the codec to
-- ChirpStack via gRPC, recording cs_profile_id + codec_js_synced_at on
-- success (Open Question #5 resolution from 02-RESEARCH.md).
--
-- region NULL = inherit install region from chirpstack_connection.region_name
-- (Open Question #2). Per-profile override is rare (typically only when a
-- vendor's firmware ships fixed for a different sub-band).
--
-- capabilities is TEXT[] + a CHECK with the SQL `<@` (subset) operator so the
-- D-04 vocabulary is enforced at the DB layer (T-02-02-01) without needing an
-- ENUM type — adding a capability later is a single ALTER TABLE statement
-- changing the CHECK, no enum dance required.
CREATE TABLE device_profile (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug                  TEXT NOT NULL UNIQUE,
    name                  TEXT NOT NULL,
    vendor                TEXT NOT NULL,
    family                TEXT,
    capabilities          TEXT[] NOT NULL DEFAULT '{}',
    counter_modulus       BIGINT NOT NULL DEFAULT 4294967296,
    codec_js              TEXT NOT NULL DEFAULT '',
    cs_profile_id         UUID NULL,
    codec_js_synced_at    TIMESTAMPTZ NULL,
    region                TEXT NULL,
    mac_version           TEXT NOT NULL DEFAULT 'LORAWAN_1_0_3',
    archived_at           TIMESTAMPTZ NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT device_profile_name_not_empty            CHECK (length(name) > 0),
    CONSTRAINT device_profile_slug_lower                CHECK (slug = lower(slug)),
    CONSTRAINT device_profile_counter_modulus_positive  CHECK (counter_modulus > 0),
    CONSTRAINT device_profile_capabilities_valid CHECK (
        capabilities <@ ARRAY[
            'cumulative',
            'flow_rate',
            'instant_power',
            'battery',
            'temperature',
            'pressure',
            'leak_detection',
            'tamper_detection',
            'multi_phase',
            'power_quality'
        ]::text[]
    )
);

CREATE INDEX device_profile_slug_idx        ON device_profile (slug);
CREATE INDEX device_profile_archived_at_idx ON device_profile (archived_at)   WHERE archived_at IS NULL;
CREATE INDEX device_profile_cs_id_idx       ON device_profile (cs_profile_id) WHERE cs_profile_id IS NOT NULL;
-- GIN index supports "find profiles supporting capability X" via `capabilities @> ARRAY['multi_phase']`.
CREATE INDEX device_profile_capabilities_gin ON device_profile USING GIN (capabilities);

CREATE TRIGGER device_profile_touch
    BEFORE UPDATE ON device_profile
    FOR EACH ROW
    EXECUTE FUNCTION touch_updated_at();
