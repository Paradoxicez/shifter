-- 0001_init.up.sql
-- Bootstrap extensions used across the entire schema.
--
-- pgcrypto      -- gen_random_uuid() for primary keys (used by 0002+)
-- timescaledb   -- Phase 2 telemetry hypertable depends on this; cheaper to load
--                  it once at the schema bottom than to special-case Phase 2.
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS timescaledb;
