-- name: GetInstallIdentity :one
SELECT * FROM install_identity WHERE id = 1;

-- name: UpsertInstallIdentity :one
-- Plan 15 wizard finish + Phase 5 settings page edit.
INSERT INTO install_identity (id, display_name, logo_path, address, timezone, units)
VALUES (1, $1, $2, $3, $4, $5)
ON CONFLICT (id) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    logo_path    = EXCLUDED.logo_path,
    address      = EXCLUDED.address,
    timezone     = EXCLUDED.timezone,
    units        = EXCLUDED.units
RETURNING *;
