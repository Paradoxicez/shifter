-- 0004_install_state.up.sql
-- Reentrant install wizard scratch space (D-10, D-11).
--
-- Singleton (id always 1) — there is exactly one wizard run per install.
-- Per-step JSONB drafts are validated client-side (zod) and re-validated by
-- Plan 15's handlers before persistence. Secrets are stored by reference
-- (path under /run/secrets) — never the raw value.
--
-- After "Finish setup" (Plan 15), the row is DELETED. The two-state contract is:
--   no admin user → wizard runs (gate in Plan 14 middleware)
--   admin user exists → app runs; install_state has no row.
CREATE TABLE install_state (
    id                INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    started_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at      TIMESTAMPTZ,
    current_step      SMALLINT NOT NULL DEFAULT 1 CHECK (current_step BETWEEN 1 AND 5),
    step1_admin       JSONB,  -- { email, name, password_hash } (already Argon2id'd at submit time)
    step2_chirpstack  JSONB,  -- { mode, grpc_url, api_token_ref, mqtt_url, mqtt_user, mqtt_pass_ref }
    step3_region      JSONB,  -- { common_name: "AS923", sub_band: "AS923_2" }
    step4_identity    JSONB,  -- { display_name, logo_path, address, timezone, units }
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER install_state_touch
    BEFORE UPDATE ON install_state
    FOR EACH ROW
    EXECUTE FUNCTION touch_updated_at();
