-- 0010_seed_profiles.down.sql
-- Idempotent — re-running is safe; missing rows simply delete zero rows.
DELETE FROM device_profile WHERE slug IN ('axioma_w1', 'acrel_adl200', 'acrel_adw300');
