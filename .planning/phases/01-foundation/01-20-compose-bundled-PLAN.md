---
phase: 01-foundation
plan: 20
type: execute
wave: 13
depends_on: [04, 18]
files_modified:
  - compose/bundled.yml
  - compose/mosquitto.conf
  - Dockerfile
  - secrets/.gitkeep
  - secrets/README.md
  - Justfile
  - install/bundled/install.sh
  - .dockerignore
autonomous: true
requirements:
  - OPS-01
must_haves:
  truths:
    - "compose/bundled.yml has top-level 'secrets:' block referencing files in ./secrets/"
    - "All image tags pinned (no :latest) — postgres+timescale, mosquitto, chirpstack, chirpstack-gateway-bridge, chirpstack-rest-api, redis, caddy, shifter"
    - "logging driver json-file with size+file caps configured on every service (OPS-05 forward-look)"
    - "shifter service has HEALTHCHECK CMD ['shifter', 'healthcheck']"
    - "Mosquitto and ChirpStack ports are NOT exposed externally (only inside compose network)"
    - "just compose-smoke-bundled brings up the stack, waits for /health, tears down — exits 0"
  artifacts:
    - path: "compose/bundled.yml"
      provides: "Bundled flavor: postgres+timescale, mosquitto, chirpstack, gateway-bridge, rest-api, redis, caddy, shifter"
      contains: "secrets:"
    - path: "Dockerfile"
      provides: "Multi-stage build: pnpm build + go build → scratch+ca-certs image"
      contains: "FROM"
    - path: "compose/mosquitto.conf"
      provides: "Mosquitto config: anonymous internal listener; no external port"
      contains: "allow_anonymous"
    - path: "install/bundled/install.sh"
      provides: "One-shot install script that copies secrets templates + runs compose up"
      contains: "docker compose"
  key_links:
    - from: "compose/bundled.yml shifter service"
      to: "secrets/postgres_password, chirpstack_api_token, session_signing_key, mqtt_password"
      via: "Compose secrets file mounts at /run/secrets/*"
      pattern: "/run/secrets"
---

<objective>
Implement OPS-01 bundled flavor: a Docker Compose file that brings up Postgres+TimescaleDB, Mosquitto, ChirpStack v4 + gateway-bridge + rest-api, Redis (ChirpStack dep), Caddy (Plan 22 supplies the Caddyfile), and the Shifter binary — all using Compose secrets (D-06), pinned image tags (OPS-07), json-file logging caps (OPS-05 forward-look). Add the Dockerfile for the Shifter image and the `install.sh` one-shot bootstrapper.

Purpose: OPS-01 (bundled compose flavor for greenfield customers per CONTEXT.md). Plan 21 mirrors the structure for external mode; Plan 22 contributes the Caddyfile.

Output: `just compose-smoke-bundled` (Justfile recipe placeholder from Plan 01 implemented here) brings up all services, polls `/health` until 200, then tears down. Exits 0 on success.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/01-foundation/01-RESEARCH.md
@.planning/phases/01-foundation/01-CONTEXT.md
@.planning/research/PITFALLS.md
@01-04-config-secrets-PLAN.md
@01-18-router-health-PLAN.md

<interfaces>
RESEARCH §Pattern 12 (lines 925-994) — verbatim Compose secrets layout.
RESEARCH §"Sources" lines 1872-1873 — pinned ChirpStack docker tags: `chirpstack/chirpstack:4`, `chirpstack/chirpstack-gateway-bridge:4`, `chirpstack/chirpstack-rest-api:4`, `postgres:14-alpine` (we use timescale/timescaledb instead — D-04 of CONTEXT plus Phase 2's hypertable need).
PITFALLS §15 — pinned tags + Compose secrets are mandatory.

Image tags (pin specific versions, not floating tags):
- `timescale/timescaledb:2.26.0-pg16`
- `eclipse-mosquitto:2.0.20`
- `chirpstack/chirpstack:4.10`
- `chirpstack/chirpstack-gateway-bridge:4.0`
- `chirpstack/chirpstack-rest-api:4.10`
- `redis:7-alpine` (Phase 1 stays consistent with ChirpStack's Redis dep — separate from Shifter deps)
- `caddy:2.8`
- `shifter:0.1.0` (built from Dockerfile in this plan)

Shifter Docker image: multi-stage:
1. `node:22-alpine` — `pnpm install && pnpm build`
2. `golang:1.24-alpine` — `go build -trimpath -ldflags "-X github.com/shifter-io/shifter/internal/version.Version=v0.1.0" -o /out/shifter ./cmd/shifter`
3. `gcr.io/distroless/static-debian12:nonroot` — copy binary; HEALTHCHECK runs `shifter healthcheck` (D-15 + PITFALL #11).
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: Dockerfile + Mosquitto config + secrets dir + bundled compose + just recipes</name>
  <files>Dockerfile, .dockerignore, compose/bundled.yml, compose/mosquitto.conf, secrets/.gitkeep, secrets/README.md, Justfile, install/bundled/install.sh</files>
  <read_first>
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pattern 12: Compose secrets idiom" (lines 925-994)
    - .planning/phases/01-foundation/01-CONTEXT.md (D-06, D-15, D-20)
    - .planning/research/PITFALLS.md §15 (pinned tags + Compose secrets)
    - 01-01-repo-scaffold-PLAN.md (existing Justfile recipe placeholders)
  </read_first>
  <action>
1. Create `Dockerfile` (multi-stage, distroless final image):
   ```dockerfile
   # syntax=docker/dockerfile:1.7

   # 1. Frontend build
   FROM node:22-alpine AS web-builder
   WORKDIR /web
   RUN corepack enable
   COPY web/package.json web/pnpm-lock.yaml ./
   RUN --mount=type=cache,target=/root/.local/share/pnpm/store pnpm install --frozen-lockfile
   COPY web/ ./
   RUN pnpm build

   # 2. Go build
   FROM golang:1.24-alpine AS go-builder
   RUN apk add --no-cache git ca-certificates
   WORKDIR /src
   COPY go.mod go.sum ./
   RUN go mod download
   COPY . .
   COPY --from=web-builder /web/dist ./web/dist
   ARG SHIFTER_VERSION=0.1.0
   ARG SHIFTER_COMMIT=unknown
   ARG SHIFTER_BUILD_TIME=unknown
   RUN go build \
       -trimpath \
       -ldflags "-s -w \
         -X github.com/shifter-io/shifter/internal/version.Version=${SHIFTER_VERSION} \
         -X github.com/shifter-io/shifter/internal/version.Commit=${SHIFTER_COMMIT} \
         -X github.com/shifter-io/shifter/internal/version.BuildTime=${SHIFTER_BUILD_TIME}" \
       -o /out/shifter ./cmd/shifter

   # 3. Runtime image
   FROM gcr.io/distroless/static-debian12:nonroot
   COPY --from=go-builder /out/shifter /usr/local/bin/shifter
   COPY --from=go-builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
   USER nonroot:nonroot
   EXPOSE 8080
   # PITFALL #11: shifter healthcheck does the localhost GET — distroless has no curl.
   HEALTHCHECK --interval=10s --timeout=5s --retries=3 \
       CMD ["/usr/local/bin/shifter", "healthcheck"]
   ENTRYPOINT ["/usr/local/bin/shifter"]
   CMD ["serve"]
   ```

2. Update `.dockerignore` to ensure clean build context. Append to existing file:
   ```
   # Already excluded: .git, .gitignore, /bin, /web/node_modules, /web/dist, /tmp, /secrets, *.md, .planning
   # Add:
   /coverage
   *.test
   ```

3. Create `compose/mosquitto.conf`:
   ```
   # Bundled-mode Mosquitto: anonymous, listening on internal Docker network only.
   # Per CONTEXT.md note: ports for Mosquitto are NOT exposed externally.
   # External-mode customers run their own broker; we don't bundle Mosquitto then.
   persistence true
   persistence_location /mosquitto/data/
   log_dest stdout
   listener 1883 0.0.0.0
   allow_anonymous true
   ```

4. Create `secrets/.gitkeep` (empty file). Create `secrets/README.md`:
   ```markdown
   # Secrets

   This directory holds file-mounted secrets per D-06 (no .env for secrets).
   The install kit pre-populates these files. Do not commit real secrets.

   Required files:

   - `postgres_password.txt`     — Postgres + TimescaleDB password
   - `chirpstack_api_token.txt`  — ChirpStack v4 API token (set after first ChirpStack boot)
   - `session_signing_key.txt`   — 32+ byte random — `openssl rand -hex 32`
   - `mqtt_password.txt`         — Optional; only needed if Mosquitto is configured with auth

   Generate the session signing key:

       openssl rand -hex 32 > secrets/session_signing_key.txt

   Generate the Postgres password:

       openssl rand -base64 24 > secrets/postgres_password.txt

   Permissions:

       chmod 0600 secrets/*.txt
   ```

5. Create `compose/bundled.yml`:
   ```yaml
   # Shifter — bundled compose flavor (OPS-01).
   # Brings up Postgres+TimescaleDB, Mosquitto, ChirpStack v4 (+ gateway-bridge + rest-api + Redis), Caddy, and Shifter.
   #
   # All image tags pinned (OPS-07).
   # All secrets via top-level `secrets:` block (D-06).
   # All services use json-file driver with size + file caps (OPS-05 forward-look).

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

     mosquitto:
       image: eclipse-mosquitto:2.0.20
       restart: unless-stopped
       command: ["mosquitto", "-c", "/mosquitto/config/mosquitto.conf"]
       volumes:
         - ./mosquitto.conf:/mosquitto/config/mosquitto.conf:ro
         - mosquitto_data:/mosquitto/data
       logging: *json-logging
       networks: [shifter]

     redis:
       image: redis:7-alpine
       restart: unless-stopped
       volumes:
         - redis_data:/data
       logging: *json-logging
       networks: [shifter]

     chirpstack:
       image: chirpstack/chirpstack:4.10
       restart: unless-stopped
       depends_on:
         postgres:  { condition: service_healthy }
         mosquitto: { condition: service_started }
         redis:     { condition: service_started }
       secrets: [postgres_password]
       environment:
         POSTGRESQL__DSN_FILE: /run/secrets/postgres_password
       volumes:
         - chirpstack_config:/etc/chirpstack
       logging: *json-logging
       networks: [shifter]

     chirpstack-gateway-bridge:
       image: chirpstack/chirpstack-gateway-bridge:4.0
       restart: unless-stopped
       depends_on: [mosquitto]
       ports:
         - "1700:1700/udp"
       logging: *json-logging
       networks: [shifter]

     chirpstack-rest-api:
       image: chirpstack/chirpstack-rest-api:4.10
       restart: unless-stopped
       depends_on: [chirpstack]
       command: ["--server", "chirpstack:8080", "--bind", "0.0.0.0:8090", "--insecure"]
       logging: *json-logging
       networks: [shifter]

     shifter:
       image: shifter:0.1.0
       restart: unless-stopped
       depends_on:
         postgres:  { condition: service_healthy }
         mosquitto: { condition: service_started }
         chirpstack: { condition: service_started }
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
         SHIFTER_CHIRPSTACK_GRPC_URL: chirpstack:8080
         SHIFTER_CHIRPSTACK_INSECURE: "true"
         SHIFTER_CHIRPSTACK_API_TOKEN_FILE: /run/secrets/chirpstack_api_token
         SHIFTER_MQTT_URL: tcp://mosquitto:1883
         SHIFTER_MQTT_PASSWORD_FILE: /run/secrets/mqtt_password
         SHIFTER_SESSION_KEY_FILE: /run/secrets/session_signing_key
         SHIFTER_TLS_MODE: internal
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
     mosquitto_data:
     redis_data:
     chirpstack_config:
     shifter_logos:
     caddy_data:
     caddy_config:

   networks:
     shifter:
       driver: bridge
   ```

6. Replace the bundled-smoke recipe in `Justfile` (the Plan 01 stub):
   ```just
   # Smoke test the bundled compose flavor.
   compose-smoke-bundled:
       #!/usr/bin/env bash
       set -euo pipefail
       just _compose-prep-secrets
       just _compose-build-image
       (cd compose && docker compose -f bundled.yml up -d)
       echo "Waiting for shifter /health ..."
       deadline=$((SECONDS + 90))
       until curl -fs http://localhost:8080/health > /dev/null 2>&1; do
         if [ $SECONDS -gt $deadline ]; then
           echo "TIMEOUT waiting for /health"
           (cd compose && docker compose -f bundled.yml logs shifter)
           (cd compose && docker compose -f bundled.yml down -v)
           exit 1
         fi
         sleep 2
       done
       echo "PASS bundled smoke"
       (cd compose && docker compose -f bundled.yml down -v)

   # Build the local shifter image used by both compose flavors
   _compose-build-image:
       docker build -t shifter:0.1.0 \
         --build-arg SHIFTER_VERSION=0.1.0 \
         --build-arg SHIFTER_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo dev) \
         --build-arg SHIFTER_BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
         .

   # Pre-populate dummy secrets so the smoke test can run without operator setup
   _compose-prep-secrets:
       #!/usr/bin/env bash
       set -euo pipefail
       mkdir -p secrets
       [ -s secrets/postgres_password.txt ]    || openssl rand -base64 24 > secrets/postgres_password.txt
       [ -s secrets/session_signing_key.txt ]  || openssl rand -hex 32     > secrets/session_signing_key.txt
       [ -s secrets/chirpstack_api_token.txt ] || echo placeholder         > secrets/chirpstack_api_token.txt
       [ -s secrets/mqtt_password.txt ]        || echo ""                  > secrets/mqtt_password.txt
       chmod 0600 secrets/*.txt
   ```

7. Create `install/bundled/install.sh`:
   ```bash
   #!/usr/bin/env bash
   # Shifter — bundled flavor one-shot installer (OPS-01).
   # Usage:
   #   ./install/bundled/install.sh [SHIFTER_DOMAIN]
   #
   # Steps:
   #   1. Generate secrets if missing
   #   2. Pull/Build images
   #   3. docker compose up -d
   #   4. Wait for /health
   #   5. Print "Visit https://<domain>"
   set -euo pipefail

   DOMAIN="${1:-localhost}"
   REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
   cd "$REPO_ROOT"

   echo "==> Preparing secrets"
   mkdir -p secrets
   [ -s secrets/postgres_password.txt ]    || openssl rand -base64 24 > secrets/postgres_password.txt
   [ -s secrets/session_signing_key.txt ]  || openssl rand -hex 32     > secrets/session_signing_key.txt
   [ -s secrets/chirpstack_api_token.txt ] || echo "placeholder-set-after-chirpstack-boots" > secrets/chirpstack_api_token.txt
   [ -s secrets/mqtt_password.txt ]        || echo ""                  > secrets/mqtt_password.txt
   chmod 0600 secrets/*.txt

   echo "==> Building shifter image"
   docker build -t shifter:0.1.0 \
     --build-arg SHIFTER_VERSION=0.1.0 \
     --build-arg SHIFTER_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo dev)" \
     --build-arg SHIFTER_BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
     .

   echo "==> Starting bundled stack (domain=$DOMAIN)"
   export SHIFTER_DOMAIN="$DOMAIN"
   export CADDY_TLS_BLOCK="${CADDY_TLS_BLOCK:-tls internal}"
   (cd compose && docker compose -f bundled.yml up -d)

   echo "==> Waiting for /health (90s deadline)"
   deadline=$((SECONDS + 90))
   until curl -fs http://localhost:8080/health > /dev/null 2>&1; do
     if [ $SECONDS -gt $deadline ]; then
       echo "TIMEOUT — see: docker compose -f compose/bundled.yml logs"
       exit 1
     fi
     sleep 2
   done

   echo "==> Shifter is up: visit https://${DOMAIN}/install to begin setup"
   ```
   Make it executable: `chmod +x install/bundled/install.sh`.
  </action>
  <verify>
    <automated>just _compose-prep-secrets && just _compose-build-image && just compose-smoke-bundled</automated>
  </verify>
  <acceptance_criteria>
    - File `Dockerfile` is a multi-stage build (3 FROM lines: web-builder, go-builder, distroless runtime)
    - Dockerfile final stage is `gcr.io/distroless/static-debian12:nonroot` (PITFALL #11)
    - Dockerfile sets `HEALTHCHECK CMD ["/usr/local/bin/shifter", "healthcheck"]` (D-15)
    - Dockerfile uses `-trimpath` and `-ldflags "-s -w -X .../version.Version=..."` for reproducible build
    - File `compose/bundled.yml` has top-level `secrets:` block with 4 entries (postgres_password, chirpstack_api_token, session_signing_key, mqtt_password) (D-06)
    - File pinned image tags: `timescale/timescaledb:2.26.0-pg16`, `eclipse-mosquitto:2.0.20`, `chirpstack/chirpstack:4.10`, `redis:7-alpine`, `caddy:2.8`, `shifter:0.1.0` — NO `:latest` anywhere (OPS-07)
    - File has 8 services: postgres, mosquitto, redis, chirpstack, chirpstack-gateway-bridge, chirpstack-rest-api, shifter, caddy
    - shifter service has 4 env vars ending in `_FILE` matching the secrets block (D-06)
    - Mosquitto service has NO `ports:` mapping (NOT exposed externally per CONTEXT.md `<specifics>`)
    - Every service has `logging:` with `max-size: "10m"` and `max-file: "3"` (OPS-05 forward-look)
    - File `Justfile` recipe `compose-smoke-bundled` exists and curl-polls `/health` until 200
    - File `install/bundled/install.sh` exists and is executable (`stat -c '%a' install/bundled/install.sh` returns a number containing `7`)
    - Command `docker build .` exits 0 (image builds successfully)
    - Command `just compose-smoke-bundled` exits 0 (full stack smoke passes)
  </acceptance_criteria>
  <done>
    Bundled compose flavor working end-to-end. Plan 21 builds external flavor; Plan 22 contributes Caddyfile.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| docker host → container fs | Compose secrets land at `/run/secrets/<name>` (mode 0400 by default) |
| container network → outside | Caddy is the only externally-exposed service (80/443) |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-20-01 | Information Disclosure | secrets in `.env` files | mitigate | D-06 enforces file-mounted secrets via top-level `secrets:` block. ASVS V8. |
| T-20-02 | Tampering | floating image tags drift across customers | mitigate | OPS-07: every tag is pinned. PITFALLS §15. ASVS V10. |
| T-20-03 | Tampering | Mosquitto exposed externally | mitigate | No `ports:` block on mosquitto service; only internal network access. CONTEXT.md `<specifics>`. ASVS V14. |
| T-20-04 | Information Disclosure | container logs grow unbounded | mitigate | json-file driver with `max-size: 10m` + `max-file: 3` on every service. OPS-05 forward-look. |
| T-20-05 | Spoofing | container runs as root by default | mitigate | Distroless `:nonroot` user; final stage `USER nonroot:nonroot`. ASVS V14. |
| T-20-06 | Tampering | shell access into Shifter container | accept | Distroless has no shell; `docker exec` produces an error. Required mitigation for prod. |
| T-20-07 | Spoofing (ChirpStack v3) | operator points at v3 server | mitigate | Plan 18 `serve` refuses to start; INST-05. |
</threat_model>

<verification>
- `compose/bundled.yml` mounts secrets via `secrets:` (no `.env` for secrets)
- All image tags pinned (no `:latest`)
- Mosquitto + ChirpStack ports not exposed externally
- shifter service has json-file logging caps
- HEALTHCHECK uses `shifter healthcheck` (D-15, PITFALL #11)
- `just compose-smoke-bundled` brings up stack, polls /health, tears down — exits 0
</verification>

<success_criteria>
- OPS-01 bundled flavor complete
- D-06 enforced (Compose secrets only)
- OPS-07 enforced (pinned tags)
- D-15 enforced (HEALTHCHECK uses binary, not curl)
- D-20 enforced (Caddy in compose)
- json-file logging caps in place (OPS-05 forward-look)
- One-shot `install.sh` for greenfield customers
</success_criteria>

<output>
After completion, create `.planning/phases/01-foundation/01-20-SUMMARY.md` documenting:
- Service list + image tags
- Secret file conventions
- HEALTHCHECK contract
- install.sh entry point
</output>
