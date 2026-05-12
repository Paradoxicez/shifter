-- 0050_catalog_metadata.up.sql
-- Phase 7 D-29..D-31 + D-41..D-44: catalog tracking columns + profile-aware
-- alert metadata + Itron+KINMY device_profile seed row.
--
-- New columns (ADD COLUMN IF NOT EXISTS — safe to re-run):
--   catalog_source          TEXT NULL   — slug of the catalog entry this row came from
--   catalog_source_version  TEXT NULL   — version string at time of import/update
--   customer_edited         BOOLEAN     — TRUE once operator saves post-import edit (D-31)
--   battery_curve           TEXT        — D-44 curve identifier for SoC rendering
--   expected_uplink_interval_seconds INT — D-41 per-profile offline-threshold base
--   offline_threshold_multiplier FLOAT  — D-43 multiplier applied to the interval
--   anomaly_compatibility   TEXT        — D-42 evaluator tier for this profile
--
-- Itron+KINMY row is inserted here to keep schema + seed atomic.
-- codec_js is a placeholder; plan 07-03 RunCatalogSeedSync re-pushes the real JS.

ALTER TABLE device_profile
    ADD COLUMN IF NOT EXISTS catalog_source         TEXT     NULL,
    ADD COLUMN IF NOT EXISTS catalog_source_version TEXT     NULL,
    ADD COLUMN IF NOT EXISTS customer_edited        BOOLEAN  NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS battery_curve          TEXT     NOT NULL DEFAULT 'linear_pct',
    ADD COLUMN IF NOT EXISTS expected_uplink_interval_seconds INT NOT NULL DEFAULT 3600,
    ADD COLUMN IF NOT EXISTS offline_threshold_multiplier FLOAT NOT NULL DEFAULT 3.0,
    ADD COLUMN IF NOT EXISTS anomaly_compatibility  TEXT     NOT NULL DEFAULT 'full';

-- D-29 / D-30: backfill existing 3 seed profiles
UPDATE device_profile SET
    catalog_source         = slug,
    catalog_source_version = '1.0.0',
    customer_edited        = FALSE  -- D-31 drift check runs in seed.go post-migration
WHERE slug IN ('axioma_w1', 'acrel_adl200', 'acrel_adw300');

-- D-41 / D-43: per-profile expected_uplink_interval_seconds + offline_threshold_multiplier
UPDATE device_profile SET
    expected_uplink_interval_seconds = 3600,
    offline_threshold_multiplier     = 3.0
WHERE slug = 'axioma_w1';

UPDATE device_profile SET
    expected_uplink_interval_seconds = 900,
    offline_threshold_multiplier     = 2.0
WHERE slug IN ('acrel_adl200', 'acrel_adw300');

-- D-44 battery curves for existing seeds
UPDATE device_profile SET battery_curve = 'linear_pct'
WHERE slug IN ('axioma_w1', 'acrel_adl200', 'acrel_adw300');

-- Itron+KINMY seed row (D-48). codec_js is intentionally a placeholder here;
-- plan 07-03 RunCatalogSeedSync replaces it with the embedded JS via the
-- same path as the other profiles. customer_edited stays FALSE.
INSERT INTO device_profile (
    slug, name, vendor, family, capabilities, codec_js, counter_modulus, mac_version,
    catalog_source, catalog_source_version, customer_edited,
    battery_curve, expected_uplink_interval_seconds, offline_threshold_multiplier,
    anomaly_compatibility, region
) VALUES (
    'itron_kinmy_lora',
    'Itron Water Meter (KINMY LoRa Module)',
    'Itron',
    'KINMY LoRa Module',
    ARRAY['cumulative', 'leak_detection', 'tamper_detection', 'battery'],
    '// placeholder — replaced at boot by RunCatalogSeedSync (plan 07-03)',
    4294967296,
    'LORAWAN_1_0_3',
    'itron_kinmy_lora',
    '1.0.0',
    FALSE,
    'li_socl2_3v6',
    86400,
    1.8,
    'limited',
    NULL
)
ON CONFLICT (slug) DO NOTHING;

-- D-42 + D-44: CHECK constraints (added after INSERT so the new row passes)
ALTER TABLE device_profile
    ADD CONSTRAINT device_profile_anomaly_compat_check
    CHECK (anomaly_compatibility IN ('full', 'limited', 'unsupported'));

ALTER TABLE device_profile
    ADD CONSTRAINT device_profile_battery_curve_check
    CHECK (battery_curve IN ('linear_pct', 'li_socl2_3v6', 'li_mnox_3v0', 'alkaline_3v0', 'none'));
