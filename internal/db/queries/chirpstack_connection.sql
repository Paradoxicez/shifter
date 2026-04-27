-- name: GetChirpStackConnection :one
SELECT * FROM chirpstack_connection WHERE id = 1;

-- name: UpsertChirpStackConnection :one
-- Plan 15 wizard finish + Phase 5 settings page edit.
INSERT INTO chirpstack_connection (
    id, mode, grpc_url, api_token_ref,
    mqtt_url, mqtt_user, mqtt_password_ref,
    region_name, region_common_name
)
VALUES (1, $1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (id) DO UPDATE SET
    mode               = EXCLUDED.mode,
    grpc_url           = EXCLUDED.grpc_url,
    api_token_ref      = EXCLUDED.api_token_ref,
    mqtt_url           = EXCLUDED.mqtt_url,
    mqtt_user          = EXCLUDED.mqtt_user,
    mqtt_password_ref  = EXCLUDED.mqtt_password_ref,
    region_name        = EXCLUDED.region_name,
    region_common_name = EXCLUDED.region_common_name
RETURNING *;
