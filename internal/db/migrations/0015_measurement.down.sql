-- 0015_measurement.down.sql
DROP INDEX IF EXISTS measurement_quality_flagged_idx;
DROP INDEX IF EXISTS measurement_extra_gin;
DROP INDEX IF EXISTS measurement_mp_time_idx;
-- Note: dropping a hypertable cleans up all child chunks automatically.
-- TimescaleDB attaches chunks to the parent via inheritance; DROP TABLE on the
-- root removes the chunks and the entry in timescaledb_information.hypertables.
DROP TABLE IF EXISTS measurement;
