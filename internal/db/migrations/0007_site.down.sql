-- 0007_site.down.sql
DROP TRIGGER IF EXISTS site_touch ON site;
DROP INDEX IF EXISTS site_parent_id_idx;
DROP INDEX IF EXISTS site_archived_at_idx;
DROP TABLE IF EXISTS site;
