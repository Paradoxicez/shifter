-- 0023_device_profile_expected_interval.up.sql
-- D-07: online/offline KPI rule is `device.last_seen_at > now() - 2 *
-- device_profile.expected_interval_s`. Per-profile so a 60-min water meter
-- and a 5-min electricity meter get appropriately different thresholds.
-- Default 3600s (1 hour) matches typical water-meter cadence; electricity
-- profiles are updated below (seed backfill) to 300s.
ALTER TABLE device_profile
    ADD COLUMN expected_interval_s INTEGER NOT NULL DEFAULT 3600
        CHECK (expected_interval_s > 0);

-- Backfill seeded profiles (0010_seed_profiles.up.sql) with realistic per-vendor
-- cadences. Axioma Qalcosonic W1 (water) keeps the 3600s default. Both Acrel
-- electricity profiles (ADL200 + ADW300) ship with a 5-minute uplink cadence.
-- Idempotent UPDATE: missing slugs simply update zero rows on a fresh install
-- with no seeded profiles (e.g. test harness rolls back 0010 first).
UPDATE device_profile
   SET expected_interval_s = 300
 WHERE slug IN ('acrel_adl200', 'acrel_adw300');
