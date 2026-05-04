-- 0015_measurement.up.sql
-- Telemetry hypertable (DATA-01, DATA-02, DATA-03, DATA-07, DATA-08).
-- D-02 frozen canonical column set; future additions are migrations.
-- D-03: `time` is server-side ingest time, NOT gateway_rx_time (Pitfall 4).
--       gateway_rx_time + device_time persisted as diagnostic columns only.
-- D-06: chunk_time_interval = 1 day (TigerData IoT recommendation for sub-minute
--       interval ingest). audit_log is a regular table, NOT a hypertable — see
--       0016_audit_log.up.sql for that decision.
-- D-08: hybrid wide+JSONB schema — Layer-1 typed columns + extra JSONB. The
--       canonical columns are queried in dashboards/reports; vendor-specific
--       extras land in `extra` JSONB and surface in the per-meter "advanced"
--       view via jsonb_each (DETL-01 in Phase 4).
-- D-26: quality flag — never silent-drop. All uplinks persist with one of the
--       five quality values; ingest pipeline (Plan 02-09) sets the value.
--
-- DATA-01 INVARIANT: NO device_id or dev_eui column on this table — telemetry
-- is keyed by metering_point_id only. The active binding at write time implies
-- the device; later swaps don't break historical queries because the MP is
-- stable across swaps and the binding history is the source-of-truth for
-- "which device produced this row at this time."
--
-- Pitfall 1 (PITFALLS §1) is the highest-cost mistake in the entire schema:
-- once telemetry rows exist with device_id as a column instead of MP, the only
-- way to fix it is a destructive migration. This file's column ordering puts
-- (time, metering_point_id) first to make the invariant visible at a glance.
--
-- FK constraints intentionally absent: TimescaleDB hypertable child chunks
-- don't propagate FKs cleanly across all versions. Application layer (resolver
-- in Plan 02-07) only writes valid metering_point_id by construction; the
-- btree_gist EXCLUDE on `binding` already enforces the consistency we need.

CREATE TABLE measurement (
    time              TIMESTAMPTZ NOT NULL,
    metering_point_id UUID NOT NULL,
    raw_value         NUMERIC,
    cumulative_value  NUMERIC,
    instant_value     NUMERIC,
    battery_pct       SMALLINT,
    rssi              SMALLINT,
    snr               REAL,
    temperature_c     REAL,
    pressure_kpa      REAL,
    leak_detected     BOOLEAN,
    tamper_detected   BOOLEAN,
    extra             JSONB NOT NULL DEFAULT '{}'::jsonb,
    raw_payload       BYTEA NOT NULL,
    decoded_object    JSONB NOT NULL,
    quality           TEXT NOT NULL DEFAULT 'ok',
    fcnt              INTEGER,
    gateway_rx_time   TIMESTAMPTZ,
    device_time       TIMESTAMPTZ,
    binding_id        UUID,
    CONSTRAINT measurement_quality_valid CHECK (
        quality IN ('ok','decode_fail','missing_canonical','out_of_range','duplicate_fcnt')
    )
);

-- Hypertable conversion. Function form per docs.tigerdata.com/api/latest/hypertable/create_hypertable.
-- chunk_time_interval=1 day matches TigerData IoT recommendation (CONTEXT D-06).
SELECT create_hypertable('measurement', 'time', chunk_time_interval => INTERVAL '1 day');

-- Hot-path index: per-MP latest reading + history queries.
-- (metering_point_id, time DESC) — the resolver's "newest reading per MP" query
-- and Phase 4's per-meter timeline both walk this index.
CREATE INDEX measurement_mp_time_idx ON measurement (metering_point_id, time DESC);

-- Vendor-specific extra-field GIN for advanced view (DETL-01 in Phase 4).
-- jsonb_path_ops is the right opclass when the access pattern is `extra @> '{...}'`
-- (containment), not generic JSON path queries. Smaller index than the default.
CREATE INDEX measurement_extra_gin ON measurement USING GIN (extra jsonb_path_ops);

-- Quality-flag filtered partial index for the "X uplinks flagged" badge (D-26).
-- Most rows are quality='ok'; a partial index keeps the flagged-only listing fast
-- without bloating the index for the common case.
CREATE INDEX measurement_quality_flagged_idx ON measurement (metering_point_id, time DESC)
    WHERE quality <> 'ok';

-- LISTEN/NOTIFY trigger for `measurement` is RESERVED for Phase 4 SSE — Phase 2
-- does NOT add this trigger (emitting NOTIFY on every uplink with no subscribers
-- is wasted work). Phase 2 only adds binding_changed (Plan 02-07).
