-- 0011_chirpstack_connection_cs_ids.up.sql
-- D-28 idempotent bootstrap — pin the CS UUIDs of the auto-created (or
-- reused) ChirpStack tenant + application so subsequent serve boots are
-- no-ops. Both columns are NULL on the existing Phase 1 install row; first
-- serve boot fills them via internal/chirpstack/bootstrap.go's
-- EnsureTenantAndApplication() (Plan 02-05).
--
-- Why TEXT, not UUID: ChirpStack v4 returns IDs as strings via gRPC; storing
-- them as TEXT avoids client-side UUID parsing on every boot. Validated as
-- 36-char strings at the application layer when read back.
ALTER TABLE chirpstack_connection
    ADD COLUMN cs_tenant_id      TEXT NULL,
    ADD COLUMN cs_application_id TEXT NULL;
