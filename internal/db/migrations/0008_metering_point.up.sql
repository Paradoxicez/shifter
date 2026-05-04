-- 0008_metering_point.up.sql
-- Metering point — the canonical "thing being measured" (a water riser,
-- an electrical sub-panel, etc). Persists across physical meter swaps;
-- D-19 + D-20.
--
-- expected_interval_s and unit are deliberately NOT here — those are inherited
-- from the bound device profile (D-01 layer 2) so a meter swap to a different
-- profile auto-updates them without touching the metering_point row.
--
-- utility_class is enforced at the DB layer (T-02-02-03) — only 'water' or
-- 'electricity' for v1; new utilities require both a migration and a UI
-- capability flag (D-19, PROJECT.md capabilities config).
--
-- (site_id, name) UNIQUE prevents two same-named MPs in one site, which would
-- be impossible to disambiguate in dialogs and reports.
CREATE TABLE metering_point (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id               UUID NOT NULL REFERENCES site(id) ON DELETE RESTRICT,
    name                  TEXT NOT NULL,
    utility_class         TEXT NOT NULL,
    location_description  TEXT,
    archived_at           TIMESTAMPTZ NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT metering_point_name_not_empty       CHECK (length(name) > 0),
    CONSTRAINT metering_point_utility_class_valid  CHECK (utility_class IN ('water', 'electricity')),
    CONSTRAINT metering_point_unique_name_per_site UNIQUE (site_id, name)
);

CREATE INDEX metering_point_site_idx        ON metering_point (site_id);
CREATE INDEX metering_point_archived_at_idx ON metering_point (archived_at) WHERE archived_at IS NULL;
CREATE INDEX metering_point_utility_idx     ON metering_point (utility_class) WHERE archived_at IS NULL;

CREATE TRIGGER metering_point_touch
    BEFORE UPDATE ON metering_point
    FOR EACH ROW
    EXECUTE FUNCTION touch_updated_at();
