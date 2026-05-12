-- 0039_alert.up.sql
-- Phase 6 — Plan 06-01 (D-08..D-12, D-19): fired alerts + lifecycle state.
--
-- One row per fired alert. payload is the D-12 canonical JSONB shape that
-- v2 webhook delivery will publish verbatim — pinning it from day 1 means
-- the deliverer ships without any schema migration.
--
-- State machine (D-08, D-09, D-10):
--   firing → acknowledged → cleared
--   firing → cleared (auto-clear on next eval when condition resolves)
--   firing → snoozed (presets 1h/8h/24h/7d, expires back to firing or cleared)
--   firing → muted   (indefinite mute; requires explicit unmute)
-- Acked alerts stay 'acknowledged'; they re-arm only after clearing — so an
-- unresolved-but-acked condition does not produce duplicate alerts.
--
-- Partial unique alert_firing_unique_idx implements idempotent fire (D-08):
-- a worker can call InsertAlert multiple times for the same (rule, target)
-- while the previous fire is still 'firing'; the second insert violates the
-- partial unique and is treated as a no-op by the application layer.

CREATE TABLE alert (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id            UUID NOT NULL REFERENCES alert_rule(id),
    rule_kind          TEXT NOT NULL,
    severity           TEXT NOT NULL CHECK (severity IN ('info','warning','critical')),
    state              TEXT NOT NULL DEFAULT 'firing' CHECK (state IN (
        'firing','acknowledged','cleared','muted','snoozed'
    )),
    payload            JSONB NOT NULL,
    target_entity_type TEXT NOT NULL,
    target_entity_id   UUID NOT NULL,
    -- D-19 test-fire flag: synthetic alerts have is_test=true, auto-clear
    -- after 60s in the application layer, badged TEST in the UI.
    is_test            BOOLEAN NOT NULL DEFAULT FALSE,
    fired_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    cleared_at         TIMESTAMPTZ,
    acked_at           TIMESTAMPTZ,
    acked_by           UUID REFERENCES "user"(id) ON DELETE SET NULL,
    ack_note           TEXT,
    snoozed_until      TIMESTAMPTZ,
    snoozed_by         UUID REFERENCES "user"(id) ON DELETE SET NULL,
    muted              BOOLEAN NOT NULL DEFAULT FALSE
);

-- Browse / drawer hot paths.
CREATE INDEX alert_firing_idx  ON alert (fired_at DESC) WHERE state = 'firing';
CREATE INDEX alert_state_idx   ON alert (state, fired_at DESC);
CREATE INDEX alert_target_idx  ON alert (target_entity_type, target_entity_id, fired_at DESC);
CREATE INDEX alert_rule_id_idx ON alert (rule_id, fired_at DESC);

-- D-08 idempotent fire: only one 'firing' row may exist per (rule, target)
-- at a time. Subsequent fire INSERTs violate this and are caught + ignored
-- by the application (no duplicate noise on the operator).
CREATE UNIQUE INDEX alert_firing_unique_idx
    ON alert (rule_id, target_entity_id)
    WHERE state = 'firing';

COMMENT ON TABLE alert IS 'Phase 6 D-08..D-12: fired alert lifecycle rows. payload JSONB is the v2-webhook-ready canonical shape from D-12.';
COMMENT ON COLUMN alert.is_test IS 'D-19: synthetic test-fire alert. UI badges as TEST; auto-cleared after 60s.';
COMMENT ON INDEX alert_firing_unique_idx IS 'D-08: idempotent fire — only one firing alert per (rule, target) at a time.';
