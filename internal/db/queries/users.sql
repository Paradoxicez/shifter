-- name: GetUserByEmail :one
-- Plan 09 (login). Email must already be lower()'d by the caller — the
-- 0002_users CHECK enforces it but we don't want to lose the index hit.
SELECT * FROM "user"
WHERE email = $1
  AND disabled_at IS NULL;

-- name: AdminExists :one
-- Plan 14 first-run gate: returns TRUE if at least one enabled admin exists.
SELECT EXISTS(
    SELECT 1 FROM "user"
    WHERE role = 'admin'
      AND disabled_at IS NULL
) AS exists;

-- name: InsertAdminUser :one
-- Plan 15 install wizard finish — creates the bootstrap admin atomically with
-- the rest of the wizard commit.
INSERT INTO "user" (email, name, password_hash, role, must_change_password)
VALUES ($1, $2, $3, 'admin', FALSE)
RETURNING id, email, name, role, created_at;

-- name: UpdateUserPassword :exec
-- Plan 11 account UI: change-password flow.
UPDATE "user"
SET password_hash = $2
WHERE id = $1;
