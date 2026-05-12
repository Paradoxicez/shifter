-- Phase 6 — Plan 06-02 schema bridge rollback. Drops the indexes first
-- (order matters: index drop reads the column it references), then the
-- columns. ON DELETE SET NULL on the FK is implicit-dropped when the
-- column drops.
DROP INDEX IF EXISTS device_gateway_active_idx;
DROP INDEX IF EXISTS gateway_last_seen_idx;
ALTER TABLE device  DROP COLUMN IF EXISTS gateway_id;
ALTER TABLE gateway DROP COLUMN IF EXISTS last_seen_at;
