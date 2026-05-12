-- 0032_floor_plan.up.sql
-- Site floor plans (Phase 5 SITE-02 + SITE-03 + D-16).
--
-- One row per uploaded image. Vertical (multi-floor) layouts modelled as
-- multiple rows ordered by sort_order with admin-typed labels (e.g. B1, GF,
-- 1F, 2F). No layout_type enum — horizontal vs vertical inferred from row
-- count. Site is FK source with ON DELETE CASCADE so site deletion sweeps
-- the plans (and via 0033's cascade, the placements).
--
-- image_path stores a RELATIVE path under the floor_plans volume root
-- (/var/lib/shifter/floor-plans). The static-serve handler in plan 05-07
-- composes the absolute path from a server-side constant + this column.
--
-- image_w + image_h capped at 8192 (D-19 / Pitfall §upload DoS). Validated
-- both server-side (plan 05-05 Task 2) AND via DB CHECK to defense-in-depth
-- catch any bypass of the handler.
--
-- NOTE: Migration originally numbered 0031 in the plan; renumbered to 0032
-- because plan 05-03 already created 0031_audit_vocab_phase5.

CREATE TABLE floor_plan (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  site_id     UUID NOT NULL REFERENCES site(id) ON DELETE CASCADE,
  label       TEXT NOT NULL CHECK (length(trim(label)) > 0 AND length(label) <= 64),
  sort_order  INTEGER NOT NULL DEFAULT 0,
  image_path  TEXT NOT NULL,
  image_w     INTEGER NOT NULL CHECK (image_w > 0 AND image_w <= 8192),
  image_h     INTEGER NOT NULL CHECK (image_h > 0 AND image_h <= 8192),
  uploaded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (site_id, sort_order)
);

CREATE INDEX floor_plan_site_sort_idx ON floor_plan (site_id, sort_order);

COMMENT ON TABLE floor_plan IS 'Floor plan images per site (SITE-02/03). Multiple rows = multi-floor layout (D-16).';
COMMENT ON COLUMN floor_plan.image_path IS 'Relative path under /var/lib/shifter/floor-plans (D-18). Static serve via plan 05-07.';
