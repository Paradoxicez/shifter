-- 0002_users.down.sql
DROP TRIGGER IF EXISTS user_touch ON "user";
DROP TABLE IF EXISTS "user";
DROP FUNCTION IF EXISTS touch_updated_at();
DROP TYPE IF EXISTS user_role;
