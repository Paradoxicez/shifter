-- 0006_chirpstack_connection.up.sql
-- ChirpStack integration target. Singleton (id=1).
--
-- mode = 'bundled' | 'external' — corresponds to docker-compose.bundled.yml vs
-- docker-compose.external.yml in Plans 20/21. The token (and optional MQTT
-- password) is stored by REF — *_ref columns hold a path under /run/secrets/.
--
-- region_name (e.g. "as923_2") is the ChirpStack `name` (URL-safe identifier);
-- region_common_name (e.g. "AS923_2") is ChirpStack's `common_name`. Both are
-- needed because ChirpStack APIs accept the `name` while the UI displays the
-- `common_name`.
CREATE TYPE chirpstack_mode AS ENUM ('bundled', 'external');

CREATE TABLE chirpstack_connection (
    id                  INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    mode                chirpstack_mode NOT NULL,
    grpc_url            TEXT NOT NULL,
    api_token_ref       TEXT NOT NULL,
    mqtt_url            TEXT NOT NULL,
    mqtt_user           TEXT,
    mqtt_password_ref   TEXT,
    region_name         TEXT NOT NULL,
    region_common_name  TEXT NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER chirpstack_connection_touch
    BEFORE UPDATE ON chirpstack_connection
    FOR EACH ROW
    EXECUTE FUNCTION touch_updated_at();
