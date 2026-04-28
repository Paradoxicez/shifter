#!/usr/bin/env bash
# Shifter — external flavor installer (OPS-01).
#
# For customers who already operate ChirpStack v4 + an MQTT broker.
#
# Prerequisites (operator does these BEFORE running this script):
#
#   1. Copy install/external/.env.example to install/external/.env and fill
#      in real values (ChirpStack gRPC URL, MQTT URL, domain, TLS mode).
#
#   2. Provide secrets in ./secrets/ (paths relative to repo root):
#        secrets/chirpstack_api_token.txt   ← REQUIRED — paste the API token
#                                             issued by your ChirpStack admin
#        secrets/mqtt_password.txt          ← optional (empty for anonymous)
#      `chmod 0600 secrets/*.txt` after creating them.
#
# Steps this script performs:
#
#   1. Source .env, validate REQUIRED vars (exits 2 if missing).
#   2. Generate any missing non-operator secrets (postgres_password,
#      session_signing_key) — chirpstack_api_token MUST be operator-set.
#   3. Render the Caddy TLS block from SHIFTER_TLS_MODE.
#   4. docker build -t shifter:0.1.0 (same image as bundled — OPS-01).
#   5. docker compose -f compose/external.yml up -d.
#   6. Poll http://localhost:8080/health until 200 (90s deadline).
#   7. Print the post-install URL.
#
# This script is idempotent: re-running preserves any existing secret
# files (only missing ones are generated) and re-uses already-built
# images via Docker's layer cache.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

ENV_FILE="install/external/.env"
if [ ! -s "$ENV_FILE" ]; then
  echo "ERROR: $ENV_FILE missing or empty."
  echo "       cp install/external/.env.example install/external/.env"
  echo "       then edit it with your real ChirpStack + MQTT URLs."
  exit 2
fi

# Source the env file (set -a exports every assignment).
set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
set +a

# Validate REQUIRED vars. Compose's `:?required` would catch this too,
# but a pre-flight check produces a friendlier error.
if [ -z "${SHIFTER_CHIRPSTACK_GRPC_URL:-}" ] || [ -z "${SHIFTER_MQTT_URL:-}" ]; then
  echo "ERROR: SHIFTER_CHIRPSTACK_GRPC_URL and SHIFTER_MQTT_URL must be set in $ENV_FILE"
  echo "       (see install/external/.env.example for examples)"
  exit 2
fi

echo "==> Preparing secrets in $REPO_ROOT/secrets/"
mkdir -p secrets
[ -s secrets/postgres_password.txt ]    || openssl rand -base64 24 > secrets/postgres_password.txt
[ -s secrets/session_signing_key.txt ]  || openssl rand -hex 32     > secrets/session_signing_key.txt

# ChirpStack API token MUST be operator-supplied — this is the brownfield
# install path; the token comes from the customer's existing ChirpStack
# admin UI. We refuse to fabricate one.
if [ ! -s secrets/chirpstack_api_token.txt ]; then
  echo "ERROR: secrets/chirpstack_api_token.txt missing or empty."
  echo "       Issue an API token in your ChirpStack admin UI and paste"
  echo "       it into that file (chmod 0600 secrets/chirpstack_api_token.txt)."
  exit 2
fi

# MQTT password is optional (anonymous brokers exist) — empty file is fine.
[ -s secrets/mqtt_password.txt ] || echo "" > secrets/mqtt_password.txt

chmod 0600 secrets/*.txt

# Map SHIFTER_TLS_MODE → Caddy directive. install.sh exports the rendered
# block so compose/external.yml's caddy service picks it up via env interpolation.
case "${SHIFTER_TLS_MODE:-internal}" in
  acme)
    # Caddy auto-issues; the Caddyfile must reference SHIFTER_TLS_EMAIL.
    export CADDY_TLS_BLOCK=""
    ;;
  byo)
    export CADDY_TLS_BLOCK="tls /etc/caddy/cert.pem /etc/caddy/key.pem"
    ;;
  internal)
    export CADDY_TLS_BLOCK="tls internal"
    ;;
  *)
    echo "ERROR: SHIFTER_TLS_MODE must be one of: acme | byo | internal (got: ${SHIFTER_TLS_MODE:-<unset>})"
    exit 2
    ;;
esac

echo "==> Building shifter:0.1.0 image"
docker build \
  -t shifter:0.1.0 \
  --build-arg SHIFTER_VERSION=0.1.0 \
  --build-arg SHIFTER_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo dev)" \
  --build-arg SHIFTER_BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  .

echo "==> Starting external stack (domain=${SHIFTER_DOMAIN:-localhost}, mode=${SHIFTER_TLS_MODE:-internal})"
echo "    chirpstack=${SHIFTER_CHIRPSTACK_GRPC_URL}  mqtt=${SHIFTER_MQTT_URL}"
(cd compose && docker compose -f external.yml up -d)

echo "==> Waiting for Shifter /health (90s deadline)"
deadline=$((SECONDS + 90))
until curl -fs http://localhost:8080/health > /dev/null 2>&1; do
  if [ $SECONDS -gt $deadline ]; then
    echo "TIMEOUT — see: docker compose -f compose/external.yml logs"
    exit 1
  fi
  sleep 2
done

echo "==> Shifter is up. Visit https://${SHIFTER_DOMAIN:-localhost}/install to begin setup."
