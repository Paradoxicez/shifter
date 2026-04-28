default:
    @just --list

# Install required dev tools
bootstrap:
    go install github.com/air-verse/air@latest
    go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
    go install go.uber.org/mock/mockgen@latest
    cd web && pnpm install --frozen-lockfile

# Run Go binary (Air) + Vite SPA dev server in parallel
dev:
    #!/usr/bin/env bash
    set -euo pipefail
    (air) & (cd web && pnpm dev) & wait

# Build production binary + SPA bundle
build:
    cd web && pnpm build
    touch web/dist/.gitkeep
    go build -o bin/shifter ./cmd/shifter

# Run all tests
test:
    go test ./... -race -count=1
    cd web && pnpm test --run

# Quick test (unit only, skip -count=1 cache)
test-quick:
    go test ./internal/... -short -race
    cd web && pnpm test --run

# Lint everything
lint:
    golangci-lint run ./...
    cd web && pnpm lint

# Apply migrations via the binary
migrate *args:
    go run ./cmd/shifter migrate {{args}}

# Build the local shifter:0.1.0 image (used by both compose flavors).
_compose-build-image:
    docker build -t shifter:0.1.0 \
      --build-arg SHIFTER_VERSION=0.1.0 \
      --build-arg SHIFTER_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo dev)" \
      --build-arg SHIFTER_BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
      .

# Pre-populate dummy secrets so smoke tests can run without operator setup.
_compose-prep-secrets:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p secrets
    [ -s secrets/postgres_password.txt ]    || openssl rand -base64 24 > secrets/postgres_password.txt
    [ -s secrets/session_signing_key.txt ]  || openssl rand -hex 32     > secrets/session_signing_key.txt
    [ -s secrets/chirpstack_api_token.txt ] || echo placeholder         > secrets/chirpstack_api_token.txt
    [ -s secrets/mqtt_password.txt ]        || echo ""                  > secrets/mqtt_password.txt
    chmod 0600 secrets/*.txt

# Smoke-test bundled compose flavor (OPS-01): bring up stack, poll /health, tear down.
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

# Smoke-test external compose flavor (Wave 0 stub — Plan 21 implements).
compose-smoke-external:
    @echo "TODO: implemented in Plan 21"
    @exit 1
