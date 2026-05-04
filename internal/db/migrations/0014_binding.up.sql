-- 0014_binding.up.sql
-- DATA-02: each device-to-metering-point binding has [valid_from, valid_to)
-- window + reading_offset. D-14 sets valid_to = swap.confirm_time on swap;
-- D-25 invalidates resolver via NOTIFY binding_changed (trigger added in
-- Plan 02-07 alongside the resolver listener so NOTIFY+LISTEN ship together).
--
-- Open Q #1 resolution: half-open interval [valid_from, valid_to) — a
-- gateway_rx_time exactly equal to valid_to is attributed to the NEW binding.
-- This matches Postgres tstzrange '[)' bound semantics + makes "swap at exact
-- second" deterministic.
--
-- last_raw_value is stored on binding (not device or measurement) so the
-- rollover detector in Plan 02-09 can SELECT one row to fetch the per-binding
-- previous raw counter without a JOIN to a 1B-row hypertable. This column is
-- updated on every successful uplink ingest.
--
-- Pitfall 10 mitigation: btree_gist enables combining UUID equality with
-- tstzrange overlap in a single EXCLUDE constraint (the GIST default operator
-- class doesn't support uuid =).

CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE binding (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    metering_point_id  UUID NOT NULL REFERENCES metering_point(id) ON DELETE RESTRICT,
    device_id          UUID NOT NULL REFERENCES device(id) ON DELETE RESTRICT,
    valid_from         TIMESTAMPTZ NOT NULL,
    valid_to           TIMESTAMPTZ NULL,
    reading_offset     NUMERIC NOT NULL DEFAULT 0,
    last_raw_value     NUMERIC NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT binding_window_valid CHECK (valid_to IS NULL OR valid_to > valid_from)
);

-- Resolver hot-path indexes:
--   binding_device_active_idx — dev_eui→MP lookup (Plan 02-07 resolver SELECT)
--   binding_mp_active_idx     — "history at this MP" listing (Phase 5 reports)
CREATE INDEX binding_device_active_idx ON binding (device_id, valid_from DESC);
CREATE INDEX binding_mp_active_idx     ON binding (metering_point_id, valid_from DESC);

-- T-02-03-02 + Pitfall 10 mitigation + RESEARCH Pattern 3:
-- At most ONE active binding per metering_point at any instant.
-- Race-safe by Postgres serializability — concurrent swap commits will block
-- on the EXCLUDE index, the loser receives 23P01 and must retry.
ALTER TABLE binding ADD CONSTRAINT binding_no_overlap_per_mp
    EXCLUDE USING gist (
        metering_point_id WITH =,
        tstzrange(valid_from, COALESCE(valid_to, 'infinity'::timestamptz), '[)') WITH &&
    );

-- Rule 2 critical-correctness companion (orthogonal to per-MP exclusion):
-- At most ONE active binding per device at any instant. Without this, the
-- resolver dev_eui→MP query could return >1 row when a device is mistakenly
-- bound to two MPs simultaneously — silent telemetry corruption, hard to
-- detect after the fact. Per-device exclusion makes the invariant a hard
-- DB-layer rule rather than relying on application discipline.
ALTER TABLE binding ADD CONSTRAINT binding_no_overlap_per_device
    EXCLUDE USING gist (
        device_id WITH =,
        tstzrange(valid_from, COALESCE(valid_to, 'infinity'::timestamptz), '[)') WITH &&
    );
