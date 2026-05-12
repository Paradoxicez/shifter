-- Phase 6 — Plan 06-02 schema bridge for ALERT-02 + ALERT-03 (D-14, D-15).
--
-- The offline evaluator and gateway-down suppression queries (per
-- 06-RESEARCH §Decision D) reference two columns that the Phase 2/3 schema
-- did not yet ship:
--
--   1. device.gateway_id — the device's last-known forwarding gateway.
--      Required by D-14 ("does D's last-known gateway have last_seen_at
--      older than the ALERT-02 threshold? if yes, suppress device alert").
--      NULLABLE (a device with no recorded gateway simply doesn't qualify
--      for gateway-down suppression — the LEFT JOIN in the evaluator query
--      produces gateway_offline=FALSE for that row, and the device-offline
--      alert fires normally). ON DELETE SET NULL because archiving a
--      gateway must not block decommissioning unrelated devices.
--
--   2. gateway.last_seen_at — last time the gateway was observed up.
--      Required by D-14 suppression eval. NULLABLE for freshly-created
--      gateways that have never been heard from (suppression also degrades
--      gracefully to FALSE in that case).
--
-- Wiring note: the ingest pipeline does NOT yet populate either column.
-- Phase 6 Plan 06-02 lands the schema + the evaluator; an orthogonal future
-- plan wires the ingest persist step to UPDATE device.gateway_id from the
-- uplink's rx_info[0].gateway_id and gateway.last_seen_at from the same
-- timestamp. Until that wiring lands, both columns stay NULL in production
-- and ALERT-03 effectively no-ops (which is the documented graceful
-- degradation — operator sees device-offline alerts as before, no false
-- gateway-down suppression). Tests seed the columns directly to exercise
-- the suppression code path.
--
-- Index strategy: device.gateway_id has a partial index over not-decommissioned
-- rows (the only rows the offline evaluator scans). gateway.last_seen_at
-- gets an index sorted DESC NULLS LAST so the per-row LEFT JOIN remains
-- index-driven even on installs with thousands of gateways.

ALTER TABLE device
    ADD COLUMN gateway_id UUID NULL REFERENCES gateway(id) ON DELETE SET NULL;

ALTER TABLE gateway
    ADD COLUMN last_seen_at TIMESTAMPTZ NULL;

CREATE INDEX device_gateway_active_idx ON device (gateway_id) WHERE decommissioned_at IS NULL;
CREATE INDEX gateway_last_seen_idx     ON gateway (last_seen_at DESC NULLS LAST);
