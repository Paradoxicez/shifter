-- 0022_install_capabilities.up.sql
-- D-09: admin-managed dashboard adaptive-scope flag. Default 'both' lets every
-- existing install upgrade without explicit reconfig; Settings UI (Plan 07)
-- lets admin narrow later.
ALTER TABLE install_identity
    ADD COLUMN capabilities TEXT NOT NULL DEFAULT 'both'
        CHECK (capabilities IN ('water','electricity','both'));
