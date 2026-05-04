-- 0010_seed_profiles.up.sql
-- Three D-07 + D-09 vendor profiles, fully wired except for codec_js: that
-- column stays empty here. internal/profile/seed.go reads //go:embed-ed JS
-- source files at boot, pushes the codec to ChirpStack via gRPC, and
-- updates cs_profile_id + codec_js_synced_at on success (Open Question #5
-- resolution from 02-RESEARCH.md). Keeping the JS out of SQL avoids a
-- pg_dump escaping mess and keeps codec source under normal version control.
--
-- counter_modulus values:
--   Axioma Qalcosonic W1 — 32-bit cumulative volume counter → 4294967296 (2^32)
--   Acrel ADL200/ADW300  — 7-digit decimal kWh display     → 10000000  (10^7)
INSERT INTO device_profile (slug, name, vendor, family, capabilities, counter_modulus, mac_version)
VALUES
    ('axioma_w1',
     'Axioma Qalcosonic W1',
     'Axioma',
     'Qalcosonic',
     ARRAY['cumulative', 'flow_rate', 'battery', 'temperature', 'leak_detection', 'tamper_detection']::text[],
     4294967296,
     'LORAWAN_1_0_3'),
    ('acrel_adl200',
     'Acrel ADL200',
     'Acrel',
     'ADL',
     ARRAY['cumulative', 'instant_power', 'battery']::text[],
     10000000,
     'LORAWAN_1_0_3'),
    ('acrel_adw300',
     'Acrel ADW300',
     'Acrel',
     'ADW',
     ARRAY['cumulative', 'instant_power', 'battery', 'temperature', 'multi_phase', 'power_quality']::text[],
     10000000,
     'LORAWAN_1_0_3');
