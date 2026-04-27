-- 0002_users.up.sql
-- Local-account auth (PROJECT.md: "Auth — Local accounts only").
-- Roles: admin (full control) | viewer (read-only) — REQ AUTH-* and ROLE-*.
CREATE TYPE user_role AS ENUM ('admin', 'viewer');

CREATE TABLE "user" (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email                 TEXT NOT NULL,
    name                  TEXT NOT NULL,
    password_hash         TEXT NOT NULL,
    role                  user_role NOT NULL,
    must_change_password  BOOLEAN NOT NULL DEFAULT FALSE,
    disabled_at           TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Lowercase invariant — application code calls lower() before insert
    -- (Plan 09 login, Plan 15 install wizard). The CHECK guarantees no rogue
    -- code path bypasses normalization. T-03-05.
    CONSTRAINT user_email_lowercase CHECK (email = lower(email)),
    CONSTRAINT user_email_not_empty CHECK (length(email) > 0)
);

CREATE UNIQUE INDEX user_email_unique ON "user" (email);

-- Generic updated_at trigger reused by every singleton/CRUD table in Phase 1.
CREATE OR REPLACE FUNCTION touch_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END
$$ LANGUAGE plpgsql;

CREATE TRIGGER user_touch
    BEFORE UPDATE ON "user"
    FOR EACH ROW
    EXECUTE FUNCTION touch_updated_at();
