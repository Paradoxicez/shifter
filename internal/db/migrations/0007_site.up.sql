-- 0007_site.up.sql
-- Site is the physical (or logical) location entity that anchors metering
-- points and devices on the floor-plan / map (D-17 + D-18 + D-20).
--
-- parent_id supports nested hierarchies (campus → building → floor) per D-17.
-- Self-referential FK is RESTRICTED on delete so a parent cannot be removed
-- while children still reference it; archive the children first or reparent
-- them. Soft-delete via archived_at preserves history of where a meter once
-- lived even after the room is decommissioned (D-20).
--
-- lat/lng are nullable because not every site has a useful map pin (e.g. a
-- nested logical "Floor 3" has the building's coords). When provided, range
-- CHECKs reject geographic nonsense at insert time (T-02-02-04).
CREATE TABLE site (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_id     UUID NULL REFERENCES site(id) ON DELETE RESTRICT,
    name          TEXT NOT NULL,
    site_type     TEXT NULL,
    lat           DOUBLE PRECISION,
    lng           DOUBLE PRECISION,
    timezone      TEXT NOT NULL,
    address       TEXT,
    description   TEXT,
    archived_at   TIMESTAMPTZ NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT site_lat_range       CHECK (lat IS NULL OR (lat BETWEEN -90 AND 90)),
    CONSTRAINT site_lng_range       CHECK (lng IS NULL OR (lng BETWEEN -180 AND 180)),
    CONSTRAINT site_name_not_empty  CHECK (length(name) > 0)
);

-- Partial index over live rows only — archived sites should not slow down the
-- common "list active sites" query (D-20).
CREATE INDEX site_archived_at_idx ON site (archived_at) WHERE archived_at IS NULL;
CREATE INDEX site_parent_id_idx   ON site (parent_id)   WHERE parent_id IS NOT NULL;

CREATE TRIGGER site_touch
    BEFORE UPDATE ON site
    FOR EACH ROW
    EXECUTE FUNCTION touch_updated_at();
