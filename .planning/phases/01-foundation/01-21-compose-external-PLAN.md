---
phase: 01-foundation
plan: 21
type: execute
wave: 14
depends_on: [20]
files_modified:
  - compose/external.yml
  - install/external/install.sh
  - install/external/.env.example
  - Justfile
autonomous: true
requirements:
  - OPS-01
must_haves:
  truths:
    - "compose/external.yml contains only Postgres+TimescaleDB, Caddy, and Shifter — NO ChirpStack/Mosquitto/Redis/gateway-bridge"
    - "External-mode shifter service reads SHIFTER_CHIRPSTACK_GRPC_URL and SHIFTER_MQTT_URL from operator's environment (not bundled services)"
    - "Same shifter:0.1.0 image as bundled (OPS-01: 'using the same backend image')"
    - "All image tags pinned; same secrets block as bundled (D-06, OPS-07)"
    - "just compose-smoke-external brings up the 3-service stack with a stub external CS+MQTT URL pointing back at the bundled flavor's services on the same host (smoke-only)"
    - "install/external/.env.example documents required external URLs"
  artifacts:
    - path: "compose/external.yml"
      provides: "External flavor: Postgres + Caddy + Shifter only; CS/MQTT URLs from env"
      contains: "SHIFTER_CHIRPSTACK_GRPC_URL"
    - path: "install/external/install.sh"
      provides: "External-mode installer: requires .env.example to be copied + edited first"
      contains: "docker compose"
    - path: "install/external/.env.example"
      provides: "Documented operator-required env vars"
      contains: "SHIFTER_CHIRPSTACK_GRPC_URL"
  key_links:
    - from: "compose/external.yml shifter service"
      to: "operator's external ChirpStack + MQTT broker"
      via: "SHIFTER_* env vars (not Compose secrets)"
      pattern: "SHIFTER_CHIRPSTACK_GRPC_URL"
---

<objective>
Implement OPS-01 external flavor: Postgres+TimescaleDB + Caddy + Shifter only, with ChirpStack gRPC URL + MQTT URL provided via env vars (`SHIFTER_CHIRPSTACK_GRPC_URL`, `SHIFTER_MQTT_URL`). Reuses the SAME `shifter:0.1.0` image as bundled (OPS-01 requirement: same backend image).

Purpose: OPS-01 brownfield path for customers who already operate ChirpStack + an MQTT broker. The wizard step 2 captures the external URLs/credentials at install time.

Output: `just compose-smoke-external` runs successfully against a bundled-stack-as-external (smoke uses the bundled compose's chirpstack/mosquitto on host:port for the smoke run).
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/01-foundation/01-RESEARCH.md
@.planning/phases/01-foundation/01-CONTEXT.md
@01-20-compose-bundled-PLAN.md

<interfaces>
RESEARCH §"External-mode Compose" (lines 996-1006) — verbatim shape.
CONTEXT.md "Stack Patterns by Variant > docker-compose.external.yml" — env vars for `CHIRPSTACK_GRPC_URL`, `MQTT_URL`, optional API token via `_FILE`.

External flavor ONLY ships:
- `postgres` — same TimescaleDB image
- `caddy` — same Caddy image (Plan 22 Caddyfile)
- `shifter` — same image, configured to point at external CS+MQTT URLs

Operator provides via `.env` (next to compose/external.yml) — NOTE: `.env` is OK for non-secret env vars. Secrets still use Compose secrets:
- `SHIFTER_DOMAIN` (for Caddy)
- `SHIFTER_CHIRPSTACK_GRPC_URL` (e.g., `chirpstack.acme.local:8080`)
- `SHIFTER_MQTT_URL` (e.g., `tcp://mqtt.acme.local:1883`)
- `SHIFTER_MQTT_USER` (optional, plain env)
- ChirpStack API token + MQTT password remain Compose secrets per D-06
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: External compose + install.sh + smoke harness</name>
  <files>compose/external.yml, install/external/install.sh, install/external/.env.example, Justfile</files>
  <read_first>
    - .planning/phases/01-foundation/01-RESEARCH.md §"External-mode Compose" (lines 996-1006)
    - .planning/phases/01-foundation/01-CONTEXT.md (CONTEXT D-06 secrets)
    - 01-20-compose-bundled-PLAN.md (image, secrets layout, healthcheck recipe)
  </read_first>
  <action>
1. Create `compose/external.yml`:
   ```yaml
   # Shifter — external compose flavor (OPS-01).
   # Brings up Postgres+TimescaleDB, Caddy, and Shifter only.
   # Customer's existing ChirpStack v4 + MQTT broker accessed via env vars.
   #
   # Same shifter:0.1.0 image as bundled. Same Compose secrets layout for sensitive values.

   secrets:
     postgres_password:
       file: ../secrets/postgres_password.txt
     chirpstack_api_token:
       file: ../secrets/chirpstack_api_token.txt
     session_signing_key:
       file: ../secrets/session_signing_key.txt
     mqtt_password:
       file: ../secrets/mqtt_password.txt

   x-logging: &json-logging
     driver: json-file
     options:
       max-size: "10m"
       max-file: "3"

   services:
     postgres:
       image: timescale/timescaledb:2.26.0-pg16
       restart: unless-stopped
       secrets: [postgres_password]
       environment:
         POSTGRES_USER: shifter
         POSTGRES_DB: shifter
         POSTGRES_PASSWORD_FILE: /run/secrets/postgres_password
       volumes:
         - postgres_data:/var/lib/postgresql/data
       healthcheck:
         test: ["CMD-SHELL", "pg_isready -U shifter -d shifter"]
         interval: 10s
         timeout: 5s
         retries: 5
       logging: *json-logging
       networks: [shifter]

     shifter:
       image: shifter:0.1.0
       restart: unless-stopped
       depends_on:
         postgres: { condition: service_healthy }
       secrets:
         - postgres_password
         - chirpstack_api_token
         - session_signing_key
         - mqtt_password
       environment:
         SHIFTER_ENV: production
         SHIFTER_HTTP_PORT: "8080"
         SHIFTER_LOG_LEVEL: info
         SHIFTER_DB_HOST: postgres
         SHIFTER_DB_PORT: "5432"
         SHIFTER_DB_USER: shifter
         SHIFTER_DB_DATABASE: shifter
         SHIFTER_DB_PASSWORD_FILE: /run/secrets/postgres_password
         SHIFTER_DB_SSL_MODE: disable
         # External ChirpStack + MQTT — operator-provided, REQUIRED.
         SHIFTER_CHIRPSTACK_GRPC_URL: ${SHIFTER_CHIRPSTACK_GRPC_URL:?required}
         SHIFTER_CHIRPSTACK_INSECURE: ${SHIFTER_CHIRPSTACK_INSECURE:-false}
         SHIFTER_CHIRPSTACK_API_TOKEN_FILE: /run/secrets/chirpstack_api_token
         SHIFTER_MQTT_URL: ${SHIFTER_MQTT_URL:?required}
         SHIFTER_MQTT_USER: ${SHIFTER_MQTT_USER:-}
         SHIFTER_MQTT_PASSWORD_FILE: /run/secrets/mqtt_password
         SHIFTER_SESSION_KEY_FILE: /run/secrets/session_signing_key
         SHIFTER_TLS_MODE: ${SHIFTER_TLS_MODE:-internal}
         SHIFTER_TLS_DOMAIN: ${SHIFTER_DOMAIN:-localhost}
         SHIFTER_LOGO_STORAGE_DIR: /var/lib/shifter/logos
       volumes:
         - shifter_logos:/var/lib/shifter/logos
       logging: *json-logging
       networks: [shifter]

     caddy:
       image: caddy:2.8
       restart: unless-stopped
       depends_on: [shifter]
       ports:
         - "80:80"
         - "443:443"
       environment:
         SHIFTER_DOMAIN: ${SHIFTER_DOMAIN:-localhost}
         SHIFTER_TLS_EMAIL: ${SHIFTER_TLS_EMAIL:-}
         CADDY_TLS_BLOCK: ${CADDY_TLS_BLOCK:-tls internal}
         CADDY_GLOBAL_TLS_BLOCK: ${CADDY_GLOBAL_TLS_BLOCK:-}
       volumes:
         - ../Caddyfile:/etc/caddy/Caddyfile:ro
         - caddy_data:/data
         - caddy_config:/config
       logging: *json-logging
       networks: [shifter]

   volumes:
     postgres_data:
     shifter_logos:
     caddy_data:
     caddy_config:

   networks:
     shifter:
       driver: bridge
   ```

2. Create `install/external/.env.example`:
   ```bash
   # Shifter — external mode env (copy to install/external/.env and edit).

   # The hostname operators visit. Caddy uses this for ACME or BYO TLS.
   SHIFTER_DOMAIN=shifter.example.com

   # ChirpStack gRPC endpoint — REQUIRED (host:port).
   SHIFTER_CHIRPSTACK_GRPC_URL=chirpstack.example.local:8080
   # Use TLS to the gRPC endpoint? false in trusted networks; true otherwise.
   SHIFTER_CHIRPSTACK_INSECURE=false

   # MQTT broker URL — REQUIRED (tcp:// or ssl://).
   SHIFTER_MQTT_URL=tcp://mqtt.example.local:1883

   # MQTT username (leave empty for anonymous brokers).
   SHIFTER_MQTT_USER=

   # TLS mode: acme | byo | internal
   #   acme    — Caddy obtains a certificate from Let's Encrypt for SHIFTER_DOMAIN
   #   byo     — operator mounts cert.pem + key.pem into the Caddy container
   #   internal— Caddy issues a self-signed cert; one-time browser warning
   SHIFTER_TLS_MODE=acme
   SHIFTER_TLS_EMAIL=ops@example.com

   # Caddy TLS-block templating (driven by SHIFTER_TLS_MODE — install.sh sets this for you).
   # CADDY_TLS_BLOCK=tls internal
   # CADDY_TLS_BLOCK=tls /etc/caddy/cert.pem /etc/caddy/key.pem
   ```

3. Create `install/external/install.sh`:
   ```bash
   #!/usr/bin/env bash
   # Shifter — external flavor installer (OPS-01).
   #
   # Prerequisite:
   #   1. Copy install/external/.env.example to install/external/.env and fill in real values.
   #   2. Provide secrets in ../secrets/ (chirpstack_api_token.txt, mqtt_password.txt, etc.).
   #
   # This script does NOT prompt; it expects .env to be populated.
   set -euo pipefail

   REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
   cd "$REPO_ROOT"

   ENV_FILE="install/external/.env"
   if [ ! -s "$ENV_FILE" ]; then
     echo "ERROR: $ENV_FILE missing. Copy install/external/.env.example and fill in real values."
     exit 2
   fi
   set -a; source "$ENV_FILE"; set +a

   if [ -z "${SHIFTER_CHIRPSTACK_GRPC_URL:-}" ] || [ -z "${SHIFTER_MQTT_URL:-}" ]; then
     echo "ERROR: SHIFTER_CHIRPSTACK_GRPC_URL and SHIFTER_MQTT_URL must be set in $ENV_FILE"
     exit 2
   fi

   echo "==> Preparing secrets"
   mkdir -p secrets
   [ -s secrets/postgres_password.txt ]    || openssl rand -base64 24 > secrets/postgres_password.txt
   [ -s secrets/session_signing_key.txt ]  || openssl rand -hex 32     > secrets/session_signing_key.txt
   [ -s secrets/chirpstack_api_token.txt ] || { echo "ERROR: secrets/chirpstack_api_token.txt missing — paste your ChirpStack API token there (chmod 0600)"; exit 2; }
   [ -s secrets/mqtt_password.txt ]        || echo ""                  > secrets/mqtt_password.txt
   chmod 0600 secrets/*.txt

   case "${SHIFTER_TLS_MODE:-internal}" in
     acme)
       export CADDY_TLS_BLOCK=""    # Caddy auto-issues
       ;;
     byo)
       export CADDY_TLS_BLOCK="tls /etc/caddy/cert.pem /etc/caddy/key.pem"
       ;;
     internal)
       export CADDY_TLS_BLOCK="tls internal"
       ;;
     *)
       echo "ERROR: SHIFTER_TLS_MODE invalid (acme | byo | internal)"; exit 2
       ;;
   esac

   echo "==> Building shifter image"
   docker build -t shifter:0.1.0 \
     --build-arg SHIFTER_VERSION=0.1.0 \
     --build-arg SHIFTER_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo dev)" \
     --build-arg SHIFTER_BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
     .

   echo "==> Starting external stack (domain=${SHIFTER_DOMAIN}, mode=${SHIFTER_TLS_MODE})"
   (cd compose && docker compose -f external.yml up -d)

   echo "==> Waiting for /health (90s deadline)"
   deadline=$((SECONDS + 90))
   until curl -fs http://localhost:8080/health > /dev/null 2>&1; do
     if [ $SECONDS -gt $deadline ]; then
       echo "TIMEOUT — see: docker compose -f compose/external.yml logs"
       exit 1
     fi
     sleep 2
   done

   echo "==> Shifter is up: visit https://${SHIFTER_DOMAIN}/install to begin setup"
   ```
   `chmod +x install/external/install.sh`.

4. Replace the `compose-smoke-external` Justfile recipe (the Plan 01 stub):
   ```just
   # Smoke test the external compose flavor.
   # Strategy: run the bundled stack first to provide a real ChirpStack + MQTT,
   # then bring up the external flavor pointing at those host-mapped ports.
   compose-smoke-external:
       #!/usr/bin/env bash
       set -euo pipefail
       just _compose-prep-secrets
       just _compose-build-image

       # Bring up bundled flavor only the chirpstack + mosquitto + their deps,
       # exposing them on host ports so external flavor can dial them as if
       # they were customer-managed services.
       echo "==> Starting bundled CS+MQTT (smoke proxy)"
       (cd compose && docker compose -f bundled.yml up -d postgres mosquitto redis chirpstack chirpstack-gateway-bridge chirpstack-rest-api)
       trap 'cd "${PWD}" && cd compose && docker compose -f bundled.yml down -v && docker compose -f external.yml down -v || true' EXIT

       # Wait for ChirpStack
       deadline=$((SECONDS + 90))
       while ! docker compose -f compose/bundled.yml exec -T chirpstack /bin/sh -c "exit 0" >/dev/null 2>&1; do
         if [ $SECONDS -gt $deadline ]; then echo "ChirpStack not up"; exit 1; fi; sleep 2
       done

       # Bring up external flavor pointing AT the bundled CS network
       export SHIFTER_DOMAIN=localhost
       export SHIFTER_TLS_MODE=internal
       export CADDY_TLS_BLOCK="tls internal"
       export SHIFTER_CHIRPSTACK_GRPC_URL=chirpstack:8080
       export SHIFTER_CHIRPSTACK_INSECURE=true
       export SHIFTER_MQTT_URL=tcp://mosquitto:1883
       export SHIFTER_MQTT_USER=""
       (cd compose && docker compose -f external.yml --project-name shifter-external up -d)

       echo "==> Waiting for external shifter /health"
       deadline=$((SECONDS + 90))
       until curl -fs http://localhost:8080/health > /dev/null 2>&1; do
         if [ $SECONDS -gt $deadline ]; then
           echo "TIMEOUT"
           docker compose -f compose/external.yml --project-name shifter-external logs shifter
           exit 1
         fi
         sleep 2
       done
       echo "PASS external smoke"

       (cd compose && docker compose -f external.yml --project-name shifter-external down -v)
       (cd compose && docker compose -f bundled.yml down -v)
       trap - EXIT
   ```

   *Note*: External smoke requires both compose files share the docker network; if `external.yml` defines its own network, the smoke must explicitly attach to bundled's. For Phase 1 simplicity the recipe above lets external talk to bundled's `chirpstack:8080` by reusing the same `shifter` network name (Compose project naming convention picks one). If networking isolation prevents that, adjust to map host ports for chirpstack/mosquitto in bundled.yml override and use `host.docker.internal` from external. Keep the recipe straightforward and document any edge case in 01-21-SUMMARY.md.
  </action>
  <verify>
    <automated>just _compose-build-image && just compose-smoke-external</automated>
  </verify>
  <acceptance_criteria>
    - File `compose/external.yml` has exactly 3 services: `postgres`, `shifter`, `caddy` (NO `mosquitto`, `chirpstack`, `chirpstack-gateway-bridge`, `chirpstack-rest-api`, `redis`)
    - File pinned image tags match Plan 20 (timescale/timescaledb:2.26.0-pg16, caddy:2.8, shifter:0.1.0)
    - `shifter` service uses `image: shifter:0.1.0` (SAME image as bundled — OPS-01 requirement)
    - `shifter` service env contains `SHIFTER_CHIRPSTACK_GRPC_URL: ${SHIFTER_CHIRPSTACK_GRPC_URL:?required}` (REQUIRED env var with `:?` syntax)
    - `shifter` service env contains `SHIFTER_MQTT_URL: ${SHIFTER_MQTT_URL:?required}`
    - File `install/external/.env.example` documents `SHIFTER_DOMAIN`, `SHIFTER_CHIRPSTACK_GRPC_URL`, `SHIFTER_MQTT_URL`, `SHIFTER_MQTT_USER`, `SHIFTER_TLS_MODE`
    - File `install/external/install.sh` exists and is executable
    - File `install/external/install.sh` validates `SHIFTER_CHIRPSTACK_GRPC_URL` and `SHIFTER_MQTT_URL` are set; exits 2 otherwise
    - `Justfile` recipe `compose-smoke-external` polls `/health` until 200
    - Command `just compose-smoke-external` exits 0
  </acceptance_criteria>
  <done>
    External compose flavor working. Plan 22 supplies the Caddyfile that both compose files mount.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| operator's environment → Shifter compose | env file (.env) for non-secret URLs; Compose secrets for sensitive values |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-21-01 | Information Disclosure | secrets in .env | mitigate | .env contains URLs only; tokens/passwords go via Compose secrets (D-06). ASVS V8. |
| T-21-02 | Tampering | external CS uses TLS but operator skips verification | accept | `SHIFTER_CHIRPSTACK_INSECURE` is documented; default false; operator-explicit override. ASVS V9. |
| T-21-03 | Spoofing | external CS is impersonated | mitigate | TLS + API token; v3 detection at boot (Plan 18). ASVS V9. |
| T-21-04 | Tampering | floating tags in external compose | mitigate | OPS-07: same pinned tags as bundled. ASVS V10. |
| T-21-05 | Information Disclosure | env var SHIFTER_CHIRPSTACK_GRPC_URL leaked in `docker inspect` output | accept | URLs are not secrets; tokens are file-mounted. ASVS V8. |
</threat_model>

<verification>
- `compose/external.yml` has exactly 3 services
- `shifter:0.1.0` image reused (no separate build path)
- `SHIFTER_CHIRPSTACK_GRPC_URL` and `SHIFTER_MQTT_URL` are required (`:?required` syntax)
- All image tags pinned
- `install/external/install.sh` validates env before launch
- `just compose-smoke-external` exits 0
</verification>

<success_criteria>
- OPS-01 external flavor complete
- Same backend image (`shifter:0.1.0`) as bundled — explicit OPS-01 requirement
- Operator-supplied URLs validated at compose-up
- D-06 secrets discipline preserved
- Documented `.env` for non-secret env
</success_criteria>

<output>
After completion, create `.planning/phases/01-foundation/01-21-SUMMARY.md` documenting:
- 3-service shape
- env vs secrets split
- install.sh prerequisites
- Smoke harness shape (bundled-as-external trick)
</output>
