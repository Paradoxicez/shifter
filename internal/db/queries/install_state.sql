-- name: GetOrCreateInstallState :one
-- Plan 15 reentrant wizard: GET /api/install/state returns the singleton row,
-- creating it on first call.
INSERT INTO install_state (id) VALUES (1)
ON CONFLICT (id) DO UPDATE SET id = 1
RETURNING *;

-- name: UpdateStep1 :exec
UPDATE install_state
SET step1_admin = $1,
    current_step = GREATEST(current_step, 2)
WHERE id = 1;

-- name: UpdateStep2 :exec
UPDATE install_state
SET step2_chirpstack = $1,
    current_step = GREATEST(current_step, 3)
WHERE id = 1;

-- name: UpdateStep3 :exec
UPDATE install_state
SET step3_region = $1,
    current_step = GREATEST(current_step, 4)
WHERE id = 1;

-- name: UpdateStep4 :exec
UPDATE install_state
SET step4_identity = $1,
    current_step = GREATEST(current_step, 5)
WHERE id = 1;

-- name: DeleteInstallState :exec
-- Called by FinishSetup after all four target tables are populated.
DELETE FROM install_state WHERE id = 1;
