#!/usr/bin/env bash
# Shifter — bundled flavor one-shot installer (OPS-01).
#
# Usage:
#   ./install/bundled/install.sh [SHIFTER_DOMAIN]
#
# Steps:
#   1. Generate any missing secret files under ./secrets/
#   2. Build the local shifter:0.1.0 image (multi-stage Dockerfile)
#   3. docker compose -f compose/bundled.yml up -d
#   4. Poll http://localhost:8080/health until 200 (90s deadline)
#   5. Print "Visit https://<domain>/install" to begin operator setup
#
# This script is idempotent: re-running it preserves existing secret
# files (only missing ones are generated) and re-uses already-built
# images via Docker's layer cache.

set -euo pipefail

DOMAIN="${1:-localhost}"
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

echo "==> Starting bundled stack (domain=$DOMAIN)"
export SHIFTER_DOMAIN="$DOMAIN"
export CADDY_TLS_BLOCK="${CADDY_TLS_BLOCK:-tls internal}"
(cd compose && docker compose -f bundled.yml up -d)

echo "==> Waiting for Shifter /health (90s deadline)"
deadline=$((SECONDS + 90))
until curl -fs http://localhost:8080/health > /dev/null 2>&1; do
  if [ $SECONDS -gt $deadline ]; then
    echo "TIMEOUT — see: docker compose -f compose/bundled.yml logs"
    exit 1
  fi
  sleep 2
done

echo "==> Shifter is up. Visit https://${DOMAIN}/install to begin setup."
