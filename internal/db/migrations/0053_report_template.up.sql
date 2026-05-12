-- 0053_report_template.up.sql
-- Phase 7 Plan 11a: saved report templates (UX-POWER Surface 6 + V2-VEND-03).
--
-- report_template stores user-saved report configurations (the JSON state of the
-- ReportConfigPanel). Operators can save a set of report parameters as a named
-- template and recall it from a dropdown — eliminating repetitive filter selection
-- for daily/monthly/yearly recurring reporting workflows.
--
-- Design notes:
--   - state JSONB stores the opaque frontend config object. The backend stores and
--     returns it as-is; the generate endpoint validates the shape. Stored as a
--     parameterized JSONB value — never interpolated into SQL (T-07-11a-01).
--   - name UNIQUE enforces no collisions at the DB layer (T-07-11a-04 last-write-wins
--     is acceptable; UNIQUE constraint means races return 23505 → 409 at handler).
--   - created_by is NULLable so system-imported templates (future) can exist without
--     a user row. On DELETE of the user, rows remain (no FK ON DELETE CASCADE).
--   - lower(name) functional index enables O(log N) ORDER BY lower(name) ASC.

CREATE TABLE IF NOT EXISTS report_template (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL UNIQUE,
    description TEXT        NOT NULL DEFAULT '',
    state       JSONB       NOT NULL,
    created_by  UUID        NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS report_template_name_lower_idx
    ON report_template (lower(name));
