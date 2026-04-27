-- 0005_install_identity.up.sql
-- Per-install branding/timezone/units. Singleton (id=1).
-- Populated at wizard finish; later edited via the Settings UI (Phase 5).
CREATE TYPE units_system AS ENUM ('metric', 'imperial');

CREATE TABLE install_identity (
    id            INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    display_name  TEXT NOT NULL,
    logo_path     TEXT,
    address       TEXT,
    timezone      TEXT NOT NULL DEFAULT 'UTC',
    units         units_system NOT NULL DEFAULT 'metric',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER install_identity_touch
    BEFORE UPDATE ON install_identity
    FOR EACH ROW
    EXECUTE FUNCTION touch_updated_at();
