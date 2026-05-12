-- Phase 6 — Plan 06-05 user management: add `last_login_at` column to "user".
--
-- The Users table (Surface 5) renders a "Last login" column for every active
-- user (relative + tooltip absolute install_tz). This column is the data
-- source. Plan 06-06 (auth-event retrofit) sets `last_login_at = now()` on
-- every successful login via UpdateLastLoginAtTx; Plan 06-05 only adds the
-- column + back-fills it for legacy rows.
--
-- Back-fill policy: existing user rows have no recorded login event yet (the
-- auth-event audit retrofit lands in Plan 06-06). Use `updated_at` as a
-- reasonable lower-bound — the row last changed when the user (or an admin)
-- touched it, which is a soft-correct proxy for "active interaction" until
-- the next genuine login event overwrites it. NULL would render as "never"
-- on the Users table for the bootstrap admin, which would be misleading on
-- the first day of Phase 6.
ALTER TABLE "user" ADD COLUMN last_login_at TIMESTAMPTZ;

UPDATE "user" SET last_login_at = updated_at WHERE last_login_at IS NULL;
