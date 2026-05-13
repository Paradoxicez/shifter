#!/usr/bin/env bash
# Shifter — bundled flavor one-shot installer (OPS-01).
#
# Usage:
#   ./install/bundled/install.sh [SHIFTER_DOMAIN]
#
# Environment overrides:
#   SHIFTER_DOMAIN     — defaults to $1 or "localhost"
#   SHIFTER_TLS_MODE   — acme | byo | internal (default: internal)
#   SHIFTER_TLS_EMAIL  — required when SHIFTER_TLS_MODE=acme (Let's Encrypt account)
#
# Steps:
#   1. Generate any missing secret files under ./secrets/
#   2. Render CADDY_TLS_BLOCK from SHIFTER_TLS_MODE (Plan 22)
#   3. Build the local shifter:0.1.0 image (multi-stage Dockerfile)
#   4. docker compose -f compose/bundled.yml up -d
#   5. Poll https://localhost/health (through Caddy) until 200 (90s deadline)
#   6. Print "Visit https://<domain>/install" to begin operator setup
#
# This script is idempotent: re-running it preserves existing secret
# files (only missing ones are generated) and re-uses already-built
# images via Docker's layer cache.

set -euo pipefail

DOMAIN="${SHIFTER_DOMAIN:-${1:-localhost}}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

echo "==> Preparing secrets in $REPO_ROOT/secrets/"
mkdir -p secrets
[ -s secrets/postgres_password.txt ]    || openssl rand -base64 24 > secrets/postgres_password.txt
[ -s secrets/session_signing_key.txt ]  || openssl rand -hex 32     > secrets/session_signing_key.txt
[ -s secrets/chirpstack_api_token.txt ] || echo "placeholder-set-after-chirpstack-boots" > secrets/chirpstack_api_token.txt
[ -s secrets/mqtt_password.txt ]        || echo ""                  > secrets/mqtt_password.txt
chmod 0600 secrets/*.txt

echo "==> Building shifter:0.1.0 image"
docker build \
  -t shifter:0.1.0 \
  --build-arg SHIFTER_VERSION=0.1.0 \
  --build-arg SHIFTER_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo dev)" \
  --build-arg SHIFTER_BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  .

# Map SHIFTER_TLS_MODE → Caddy `tls` directive (Plan 22, D-21).
# acme:     Caddy auto-issues from Let's Encrypt (requires SHIFTER_TLS_EMAIL).
# byo:      operator mounts cert.pem + key.pem; Caddy serves them.
# internal: Caddy local CA (self-signed) — LAN-only deploys + smoke tests.
case "${SHIFTER_TLS_MODE:-internal}" in
  acme)
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

echo "==> Starting bundled stack (domain=$DOMAIN, tls=${SHIFTER_TLS_MODE:-internal})"
export SHIFTER_DOMAIN="$DOMAIN"
(cd compose && docker compose -f bundled.yml up -d)

# Plan 22: poll /health THROUGH Caddy (HTTPS, -k tolerates internal CA).
# This validates the full path: Caddy → shifter:8080.
echo "==> Waiting for Shifter /health via Caddy (90s deadline)"
deadline=$((SECONDS + 90))
until curl -fsk https://localhost/health > /dev/null 2>&1; do
  if [ $SECONDS -gt $deadline ]; then
    echo "TIMEOUT — see: docker compose -f compose/bundled.yml logs"
    exit 1
  fi
  sleep 2
done

# Bootstrap ChirpStack API token so wizard step 2 can authenticate (bug #4).
./install/bundled/bootstrap-chirpstack-token.sh || true

echo "==> Shifter is up. Visit https://${DOMAIN}/install to begin setup."
