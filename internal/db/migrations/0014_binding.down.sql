-- 0014_binding.down.sql
ALTER TABLE binding DROP CONSTRAINT IF EXISTS binding_no_overlap_per_device;
ALTER TABLE binding DROP CONSTRAINT IF EXISTS binding_no_overlap_per_mp;
DROP INDEX IF EXISTS binding_mp_active_idx;
DROP INDEX IF EXISTS binding_device_active_idx;
DROP TABLE IF EXISTS binding;
-- Do NOT drop EXTENSION btree_gist — it may be in use elsewhere (or by future
-- migrations); operator can drop manually if a clean uninstall is required.
