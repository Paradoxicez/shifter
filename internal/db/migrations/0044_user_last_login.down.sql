-- Phase 6 — Plan 06-05: revert user.last_login_at column.
ALTER TABLE "user" DROP COLUMN last_login_at;
