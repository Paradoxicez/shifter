-- 0018_gateway.up.sql
-- Phase 3 D-29: gateway table for Shifter-side CRUD. ChirpStack stores
-- the canonical Gateway proto; Shifter mirrors gateway_id + operator-
-- entered metadata + the 1-min-TTL GetMetrics cache. Region is
-- Shifter-only (CS attaches region to device profiles, not gateways).
--
-- D-30 (verbatim per user decision 2026-05-11): soft-delete via archived_at
-- + ChirpStack DeleteGateway atomic. archived_snapshot JSONB preserves
-- the CS Gateway proto so RestoreGateway can re-create faithfully.
-- Partial index keeps active list-query fast. Restore = clear archive cols.
-- D-02: stats_* cache columns refreshed by single-flight async goroutine.

CREATE TABLE gateway (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    gateway_id          TEXT NOT NULL UNIQUE,
    name                TEXT NOT NULL,
    description         TEXT,
    region              TEXT NOT NULL,
    lat                 DOUBLE PRECISION,
    lng                 DOUBLE PRECISION,
    altitude            DOUBLE PRECISION,
    tags                JSONB NOT NULL DEFAULT '{}'::jsonb,
    cs_tenant_id        TEXT,
    stats_refreshed_at  TIMESTAMPTZ NULL,
    stats_rx_24h        BIGINT NULL,
    stats_tx_24h        BIGINT NULL,
    stats_tx_ok_24h     BIGINT NULL,
    stats_sparkline     JSONB NULL,
    archived_at         TIMESTAMPTZ NULL,
    archived_reason     TEXT,
    archived_snapshot   JSONB NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT gateway_eui_lower  CHECK (gateway_id = lower(gateway_id)),
    CONSTRAINT gateway_eui_hex16  CHECK (gateway_id ~ '^[0-9a-f]{16}$'),
    CONSTRAINT gateway_name_not_empty CHECK (length(name) > 0),
    CONSTRAINT gateway_lat_range CHECK (lat IS NULL OR (lat BETWEEN -90 AND 90)),
    CONSTRAINT gateway_lng_range CHECK (lng IS NULL OR (lng BETWEEN -180 AND 180))
);

-- Partial indexes over live (not-archived) rows keep the common
-- "list active gateways" and "filter by region" queries fast (D-30).
CREATE INDEX gateway_archived_idx ON gateway (archived_at) WHERE archived_at IS NULL;
CREATE INDEX gateway_region_idx   ON gateway (region)      WHERE archived_at IS NULL;

CREATE TRIGGER gateway_touch
    BEFORE UPDATE ON gateway
    FOR EACH ROW
    EXECUTE FUNCTION touch_updated_at();
