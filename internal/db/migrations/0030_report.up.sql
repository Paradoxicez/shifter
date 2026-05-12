-- 0030_report.up.sql
-- Report metadata records (Phase 5 REPT-01..07). Ephemeral artifacts (D-07):
-- expires_at is 24h after created_at; River PeriodicJob in plan 05-06 purges
-- expired rows + their artifact_dir contents.

CREATE TABLE report (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id           UUID NOT NULL REFERENCES "user"(id),
  scope             TEXT NOT NULL CHECK (scope IN ('all','site','meter')),
  site_id           UUID REFERENCES site(id),
  metering_point_id UUID REFERENCES metering_point(id),
  group_by          TEXT CHECK (group_by IN ('site','category','none')),
  range_kind        TEXT NOT NULL CHECK (range_kind IN ('daily','monthly','yearly','custom')),
  range_start       TIMESTAMPTZ NOT NULL,
  range_end         TIMESTAMPTZ NOT NULL CHECK (range_end > range_start),
  artifact_dir      TEXT NOT NULL,
  pdf_status        TEXT NOT NULL DEFAULT 'pending'
                    CHECK (pdf_status IN ('pending','running','ready','failed','expired')),
  pdf_path          TEXT,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at        TIMESTAMPTZ NOT NULL,
  CONSTRAINT report_scope_keys CHECK (
    (scope = 'site' AND site_id IS NOT NULL AND metering_point_id IS NULL) OR
    (scope = 'meter' AND metering_point_id IS NOT NULL AND site_id IS NULL) OR
    (scope = 'all' AND site_id IS NULL AND metering_point_id IS NULL)
  )
);

CREATE INDEX report_user_created_idx ON report (user_id, created_at DESC);
CREATE INDEX report_expires_idx     ON report (expires_at) WHERE pdf_status <> 'expired';
