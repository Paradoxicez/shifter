#!/usr/bin/env bash
# bootstrap-chirpstack-token.sh — generate a ChirpStack global API key and
# write it to secrets/chirpstack_api_token.txt so wizard step 2 can authenticate.
# Called by install.sh after the health poll. Non-fatal on failure (|| true).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

echo "==> Bootstrapping ChirpStack API token..."

# Wait up to 30s for the gRPC API to be serving (chirpstack logs "api server
# listening" but there is no healthcheck on the chirpstack service itself).
deadline=$((SECONDS + 30))
until docker exec compose-chirpstack-1 \
      chirpstack -c /tmp/chirpstack --help > /dev/null 2>&1; do
  if [ $SECONDS -gt $deadline ]; then
    echo "WARNING: ChirpStack not ready after 30s — skipping token bootstrap." \
         "Re-run ./install/bundled/bootstrap-chirpstack-token.sh manually." >&2
    exit 0
  fi
  sleep 2
done

# create-api-key prints a single line: "token: <jwt>"
OUTPUT=$(docker exec compose-chirpstack-1 \
  chirpstack -c /tmp/chirpstack create-api-key --name shifter-bootstrap 2>&1) || true

TOKEN=$(echo "$OUTPUT" | grep -oP '(?<=token: )eyJ[A-Za-z0-9._-]+' || true)

if [ -z "$TOKEN" ]; then
  echo "WARNING: Could not extract JWT from create-api-key output." \
       "Output was: $OUTPUT" >&2
  echo "Re-run ./install/bundled/bootstrap-chirpstack-token.sh manually." >&2
  exit 0
fi

printf '%s' "$TOKEN" > secrets/chirpstack_api_token.txt
chmod 0600 secrets/chirpstack_api_token.txt

# Restart shifter so it picks up the new secret on next start.
docker restart compose-shifter-1 > /dev/null 2>&1 || true

echo "==> ChirpStack API token bootstrapped — wizard step 2 ready"
