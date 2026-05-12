-- 0033_device_floor_plan_placement.up.sql
-- Device placement on a floor plan (Phase 5 SITE-04 + D-20 + D-25).
--
-- device_id is the PRIMARY KEY — a device sits on AT MOST one floor plan at
-- any time. Fractional coordinates (x_frac, y_frac) are the load-bearing
-- design choice: resolution-independent, survive image replacement (D-24)
-- and Retina/mobile DPR.
--
-- ON DELETE CASCADE on both FKs:
--   - device deleted (hard) → placement deleted (cascade)
--   - floor_plan deleted → placements on it deleted (cascade)
--
-- BUT decommission is a SOFT delete (sets decommissioned_at, no DELETE FROM
-- device). Plan 05-07 handles the soft-delete case by issuing a DELETE on
-- the placement row inside the decommission handler's tx (D-25 invariant).
--
-- NOTE: Migration originally numbered 0032 in the plan; renumbered to 0033
-- because plan 05-03 already created 0031_audit_vocab_phase5, pushing
-- floor_plan to 0032 and this table to 0033.

CREATE TABLE device_floor_plan_placement (
  device_id     UUID PRIMARY KEY REFERENCES device(id) ON DELETE CASCADE,
  floor_plan_id UUID NOT NULL REFERENCES floor_plan(id) ON DELETE CASCADE,
  x_frac        REAL NOT NULL CHECK (x_frac >= 0 AND x_frac <= 1),
  y_frac        REAL NOT NULL CHECK (y_frac >= 0 AND y_frac <= 1),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX device_floor_plan_placement_plan_idx ON device_floor_plan_placement (floor_plan_id);
