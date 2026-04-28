# syntax=docker/dockerfile:1.7
#
# Shifter — multi-stage container build (OPS-01).
#
#   1. web-builder   — pnpm install + pnpm build → web/dist/
#   2. go-builder    — go build (with embedded SPA bundle) → /out/shifter
#   3. runtime       — distroless static-debian12:nonroot, HEALTHCHECK uses
#                      `shifter healthcheck` (PITFALL #11 — distroless has
#                      no curl/wget, the binary itself probes /health).
#
# Build args injected by the Justfile / install script:
#   SHIFTER_VERSION    — semver tag (default: 0.1.0)
#   SHIFTER_COMMIT     — short git SHA (default: unknown)
#   SHIFTER_BUILD_TIME — RFC3339 timestamp (default: unknown)
#
# All ldflags variables are package-level vars in internal/version (Plan 04);
# `-X` overrides them at link time so `shifter version` reports the correct
# build provenance.

# --- 1. Frontend build -----------------------------------------------------
FROM node:22-alpine AS web-builder
WORKDIR /web
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

# --- 2. Go build -----------------------------------------------------------
FROM golang:1.24-alpine AS go-builder
RUN apk add --no-cache git ca-certificates
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Replace the placeholder web/dist/ with the real Vite output so `go:embed
# all:dist` (web/embed.go) bakes the SPA bundle into the binary.
RUN rm -rf ./web/dist
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

# --- 3. Runtime image ------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=go-builder /out/shifter /usr/local/bin/shifter
COPY --from=go-builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
USER nonroot:nonroot
EXPOSE 8080
# PITFALL #11: `shifter healthcheck` is the localhost GET — distroless has
# no curl/wget, so the binary itself is the probe. Plan 05 implemented the
# subcommand; it exits 0 on a 200 from /health and non-zero otherwise.
HEALTHCHECK --interval=10s --timeout=5s --retries=3 \
    CMD ["/usr/local/bin/shifter", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/shifter"]
CMD ["serve"]
