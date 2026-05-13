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
#   Quick-fix 260513-c4v: pgclient stage installs pg-16 from PGDG apt repo
#   (bookworm main only ships pg-15) and runtime COPYs use multiarch glob
#   /usr/lib/*-linux-gnu/ so the image builds on both amd64 and arm64.

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
FROM golang:1.26-alpine AS go-builder
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
# Bookworm main only ships postgresql-client-15. Add PGDG apt repo for pg-16.
# https://wiki.postgresql.org/wiki/Apt
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl gnupg \
    && install -d /etc/apt/keyrings \
    && curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc \
       | gpg --dearmor -o /etc/apt/keyrings/postgresql.gpg \
    && echo "deb [signed-by=/etc/apt/keyrings/postgresql.gpg] https://apt.postgresql.org/pub/repos/apt bookworm-pgdg main" \
       > /etc/apt/sources.list.d/pgdg.list \
    && apt-get update && apt-get install -y --no-install-recommends \
      postgresql-client-16 \
    && rm -rf /var/lib/apt/lists/*

# --- 4. Runtime image ------------------------------------------------------
# distroless/cc-debian12 (not static) is required because the pg client
# binaries (pg_dump, pg_restore, psql) are dynamically linked against glibc.
# distroless/static-debian12 has no dynamic linker; cc includes libc + ld.
# The shifter Go binary is statically linked (CGO_ENABLED=0) so it works on
# both; only the pg binaries drive this base-image choice.
FROM gcr.io/distroless/cc-debian12:nonroot
COPY --from=go-builder /out/shifter /usr/local/bin/shifter
COPY --from=go-builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# PostgreSQL 16 client binaries (Plan 06-08 / OPS-02).
# Copied from the debian:bookworm-slim pgclient stage so their glibc
# dependencies match the distroless Debian 12 base.
# NOTE: /usr/bin/pg_dump on Debian is a Perl wrapper symlink (pg_wrapper).
# The actual ELF binary is at /usr/lib/postgresql/16/bin/pg_dump.
COPY --from=pgclient /usr/lib/postgresql/16/bin/pg_dump    /usr/bin/pg_dump
COPY --from=pgclient /usr/lib/postgresql/16/bin/pg_restore /usr/bin/pg_restore
COPY --from=pgclient /usr/lib/postgresql/16/bin/psql       /usr/bin/psql

# Shared libraries required by postgresql-client-16.
# pg-16 from PGDG has a deep transitive dep chain (libpq → libgnutls →
# libp11-kit, libidn2, libunistring, libnettle, libhogweed, libgmp, libffi,
# libtasn1, libkeyutils; psql additionally needs libreadline → libtinfo).
# Copying the entire multiarch lib dir is simpler and future-proof: any new
# dep pulled in by a pg-16 minor upgrade is automatically included.
# The glob /usr/lib/*-linux-gnu/ expands to the correct arch dir at build
# time (x86_64-linux-gnu on amd64, aarch64-linux-gnu on arm64).
# libc, libm, libgcc, libresolv are already in distroless/cc-debian12.
COPY --from=pgclient /usr/lib/*-linux-gnu/ /usr/lib/

USER nonroot:nonroot
EXPOSE 8080
# PITFALL #11: `shifter healthcheck` is the localhost GET — distroless has
# no curl/wget, so the binary itself is the probe. Plan 05 implemented the
# subcommand; it exits 0 on a 200 from /health and non-zero otherwise.
HEALTHCHECK --interval=10s --timeout=5s --retries=3 \
    CMD ["/usr/local/bin/shifter", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/shifter"]
CMD ["serve"]
