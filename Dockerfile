# syntax=docker/dockerfile:1.7
#
# Shifter — multi-stage container build (OPS-01).
#
#   1. web-builder   — pnpm install + pnpm build → web/dist/
#   2. go-builder    — go build (with embedded SPA bundle) → /out/shifter
#   3. pgclient      — PostgreSQL 16 client tools from debian:bookworm-slim
#                      (debian-based so shared libraries match the distroless
#                      debian12 runtime image — RESEARCH §Decision A Note).
#   4. runtime       — distroless static-debian12:nonroot, pg_dump / pg_restore /
#                      psql copied from pgclient stage, HEALTHCHECK uses
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
#
# Plan 06-08 Dockerfile note (T-06-08-08 / OPS-02):
#   The pgclient stage installs postgresql-client-16 from debian bookworm so
#   the library ABI matches distroless/static-debian12 (both are Debian 12 /
#   bookworm based). Alpine would provide musl-linked binaries that cannot
#   run on the glibc-based distroless image. Using debian:bookworm-slim for
#   pgclient ensures the .so symlinks needed by pg_dump resolve correctly.
#   Image size impact: +~15MB for pg_dump + pg_restore + psql + libpq deps.

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

# --- 3. PostgreSQL 16 client tools -----------------------------------------
# debian:bookworm-slim is the correct base — it shares the glibc ABI with
# gcr.io/distroless/static-debian12 so the pg client binaries and their
# shared libraries work without musl/glibc conflicts.
FROM debian:bookworm-slim AS pgclient
RUN apt-get update && apt-get install -y --no-install-recommends \
    postgresql-client-16 \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# --- 4. Runtime image ------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=go-builder /out/shifter /usr/local/bin/shifter
COPY --from=go-builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# PostgreSQL 16 client binaries (Plan 06-08 / OPS-02).
# Copied from the debian:bookworm-slim pgclient stage so their glibc
# dependencies match the distroless Debian 12 base.
COPY --from=pgclient /usr/bin/pg_dump    /usr/bin/pg_dump
COPY --from=pgclient /usr/bin/pg_restore /usr/bin/pg_restore
COPY --from=pgclient /usr/bin/psql       /usr/bin/psql

# Shared libraries required by the postgresql-client-16 binaries.
# Determined via `ldd /usr/bin/pg_dump` on debian:bookworm-slim.
# Copy each needed .so with glob to capture versioned symlinks.
COPY --from=pgclient /usr/lib/x86_64-linux-gnu/libpq.so.5*        /usr/lib/x86_64-linux-gnu/
COPY --from=pgclient /usr/lib/x86_64-linux-gnu/libssl.so.3*       /usr/lib/x86_64-linux-gnu/
COPY --from=pgclient /usr/lib/x86_64-linux-gnu/libcrypto.so.3*    /usr/lib/x86_64-linux-gnu/
COPY --from=pgclient /usr/lib/x86_64-linux-gnu/libldap-2.5.so.0*  /usr/lib/x86_64-linux-gnu/
COPY --from=pgclient /usr/lib/x86_64-linux-gnu/liblber-2.5.so.0*  /usr/lib/x86_64-linux-gnu/
COPY --from=pgclient /usr/lib/x86_64-linux-gnu/libgssapi_krb5.so.2* /usr/lib/x86_64-linux-gnu/
COPY --from=pgclient /usr/lib/x86_64-linux-gnu/libkrb5.so.3*      /usr/lib/x86_64-linux-gnu/
COPY --from=pgclient /usr/lib/x86_64-linux-gnu/libk5crypto.so.3*  /usr/lib/x86_64-linux-gnu/
COPY --from=pgclient /usr/lib/x86_64-linux-gnu/libcom_err.so.2*   /usr/lib/x86_64-linux-gnu/
COPY --from=pgclient /usr/lib/x86_64-linux-gnu/libkrb5support.so.0* /usr/lib/x86_64-linux-gnu/
COPY --from=pgclient /usr/lib/x86_64-linux-gnu/libsasl2.so.2*     /usr/lib/x86_64-linux-gnu/
# libc and libgcc are already present in distroless/static-debian12;
# libpthread / libdl are folded into glibc 2.34+ and already present.

USER nonroot:nonroot
EXPOSE 8080
# PITFALL #11: `shifter healthcheck` is the localhost GET — distroless has
# no curl/wget, so the binary itself is the probe. Plan 05 implemented the
# subcommand; it exits 0 on a 200 from /health and non-zero otherwise.
HEALTHCHECK --interval=10s --timeout=5s --retries=3 \
    CMD ["/usr/local/bin/shifter", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/shifter"]
CMD ["serve"]
