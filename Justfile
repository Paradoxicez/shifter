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

# Smoke-test bundled compose (Wave 0 stub — Plan 20 implements)
compose-smoke-bundled:
    @echo "TODO: implemented in Plan 20"
    @exit 1

compose-smoke-external:
    @echo "TODO: implemented in Plan 21"
    @exit 1
