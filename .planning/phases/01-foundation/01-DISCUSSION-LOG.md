# Phase 1: Foundation - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-27
**Phase:** 01-foundation
**Areas discussed:** Repo layout & dev toolchain, Config secrets & first-run, CLI surface & migrations, TLS & reverse proxy posture

---

## Repo Layout & Dev Toolchain

### Repo shape

| Option | Description | Selected |
|--------|-------------|----------|
| Monorepo, Go-canonical | `cmd/shifter/`, `internal/`, `web/` — 1 binary embeds SPA via `go:embed` (Recommended) | ✓ |
| Monorepo, separate dirs | `backend/` + `frontend/` root-level — clearer for FE devs | |
| Two repos | `shifter-backend` + `shifter-frontend` — small advantage, more deploy overhead | |

**User's choice:** Monorepo, Go-canonical.

### Build tool

| Option | Description | Selected |
|--------|-------------|----------|
| Makefile | Universal, every machine has it — verbose for parallel/help/dependencies | |
| Justfile (Recommended) | Modern, clean syntax, `just --list`, cross-platform — operator must install `just` | ✓ |
| Task (Taskfile.yml) | YAML-based, clear deps — operator must install `task` | |
| Go scripts only | `go run ./scripts/...` — no extra tooling | |

**User's choice:** Justfile.

### Frontend package manager

| Option | Description | Selected |
|--------|-------------|----------|
| pnpm (Recommended) | Fast, disk-efficient, strict lockfile, shadcn-standard | ✓ |
| npm | Bundled with Node, no extra install — slower | |
| yarn (modern) | Yarn 4, fast, optional plug'n'play | |

**User's choice:** pnpm.

### Hot reload

| Option | Description | Selected |
|--------|-------------|----------|
| Air (Go) + Vite (SPA), proxy | Vite proxies `/api` to Go (Recommended) — fastest dev experience | ✓ |
| Embed-rebuild only | Build SPA → `go run` — slower edit cycle | |
| Air only (no SPA HMR) | Hot reload Go only — SPA refreshes manually | |

**User's choice:** Air + Vite proxy.

---

## Config, Secrets & First-Run

### Config layer

| Option | Description | Selected |
|--------|-------------|----------|
| Env vars only | 12-factor pure — Compose `secrets` for sensitive values | |
| `config.yaml` + env override (Recommended) | `/etc/shifter/config.yaml` is source of truth, env overrides for secrets — readable, debuggable | ✓ |
| Flag-only at startup | `shifter serve --grpc-url=...` — verbose, awkward for self-host | |

**User's choice:** `config.yaml` + env override.

### Secrets source

| Option | Description | Selected |
|--------|-------------|----------|
| Compose `secrets` files (Recommended) | Docker secrets mount as files — never in `.env`, never leaks via `ps`/log | ✓ |
| `.env` file | Easy but leaks easily — bad habit-forming | |
| Mixed (DB pw via secrets, CS creds in DB after wizard) | Hybrid | |

**User's choice:** Compose `secrets` files.

### First-run detection

| Option | Description | Selected |
|--------|-------------|----------|
| DB has no admin user (Recommended) | Stateless, idempotent — wizard runs only when no admin exists | ✓ |
| Flag file (`/var/lib/shifter/.installed`) | Fragile if volume disappears | |
| Env `SHIFTER_INSTALLED=1` | User-managed flag — error-prone | |

**User's choice:** DB has no admin user.

### Default admin password

| Option | Description | Selected |
|--------|-------------|----------|
| Wizard step 1 sets it (Recommended) | Operator types email + password in wizard — no default password ever exists | ✓ |
| Auto-generate, print to stdout | Random pw written to docker logs — install kit grabs it | |
| `shifter create-admin` CLI | Operator runs `docker exec shifter create-admin` first — extra step | |

**User's choice:** Wizard step 1.
**Implication:** AUTH-03 ("force change default admin password on first login") was originally written for the bootstrap admin. With this decision, AUTH-03 reframes to apply only to admin-created secondary users (Phase 6 USER-04) — bootstrap admin sets their own password via the wizard from the start. CONTEXT.md notes this for downstream agents.

---

## CLI Surface & Migrations

### CLI commands (multiSelect)

| Option | Description | Selected |
|--------|-------------|----------|
| `serve`, `migrate`, `version` (minimal) | Bare minimum — ops favors small surface | ✓ |
| `+ create-admin` (recovery) | For password reset / lockout escape | ✓ |
| `+ config-check` (validate) | `shifter config-check` validates yaml schema, pings DB+CS — install kit uses it | ✓ |
| `+ healthcheck` (probe) | `shifter healthcheck` invokes `/health` — avoids bundling `curl` in image | ✓ |

**User's choice:** All four. Final CLI surface = `serve, migrate, version, create-admin, config-check, healthcheck`.

### Migrations runtime

| Option | Description | Selected |
|--------|-------------|----------|
| Auto on `serve`, idempotent (Recommended) | `shifter serve` migrates if dirty, logs "applied N" — self-host friendly | ✓ |
| Explicit `shifter migrate` only | Operator runs migrations explicitly — safer but install kit must include the step | |
| Auto with `--auto-migrate` flag | Default no, opt-in | |

**User's choice:** Auto on `serve`.

### Migration runner

| Option | Description | Selected |
|--------|-------------|----------|
| `golang-migrate` embedded (Recommended) | Vendored Go package — no separate binary, plain SQL files | ✓ |
| Atlas (declarative) | Schema-as-code, drift detection — heavier mental load | |
| `goose` | Go-native, plain SQL or Go migrations — similar to golang-migrate | |

**User's choice:** `golang-migrate` embedded.

### `/health` endpoint

| Option | Description | Selected |
|--------|-------------|----------|
| Public, basic info (Recommended) | `{status, version}` — Docker/Caddy probe-friendly; details behind auth | ✓ |
| Public detailed | Full DB+CS+MQTT+disk reachability — leaks attack surface | |
| Auth-required entirely | Requires login — Docker healthcheck breaks | |

**User's choice:** Public basic.
**Implication:** REQUIREMENTS.md INST-06 says `/health` should report DB+CS+MQTT+disk+last-uplink-age. With this decision, that detail moves to `/health/detailed` (admin-auth required); `/health` keeps the public summary. CONTEXT.md notes the wording update for Phase 1 implementation.

---

## TLS & Reverse Proxy Posture

### TLS terminate where

| Option | Description | Selected |
|--------|-------------|----------|
| Caddy bundled, auto Let's Encrypt (Recommended) | Caddy in compose, auto-cert — operator just opens 80/443 | ✓ |
| Behind customer reverse proxy | Shifter listens `:8080` plain — customer fronts with nginx/traefik | |
| Both modes (toggle) | `tls.mode = caddy|external` — install kit asks at install | |

**User's choice:** Caddy bundled, auto LE.
**Note:** CONTEXT.md elaborates this into three TLS modes (`acme`, `byo`, `internal`) so the same bundled Caddy serves greenfield (LE), enterprise (BYO cert), and LAN-only (Caddy internal CA) without separate compose flavors.

### Dev mode TLS

| Option | Description | Selected |
|--------|-------------|----------|
| Plain HTTP on localhost (Recommended) | Vite at `http://localhost:5173` — no cert pain in dev | ✓ |
| Self-signed (`mkcert`) | HTTPS local — closer to prod but trust-cert overhead | |

**User's choice:** Plain HTTP on localhost.

### Caddy domain handling

| Option | Description | Selected |
|--------|-------------|----------|
| Domain via env, ACME default (Recommended) | `SHIFTER_DOMAIN=...` → Caddy + Let's Encrypt automatic | ✓ |
| Bring-your-own cert | Mount cert+key from operator — for internal CA | |
| Both | Env-default with optional cert mount override | |

**User's choice:** Domain via env, ACME default.
**Note:** CONTEXT.md combines this with `tls.mode: byo` so BYO cert is supported via mode switch, satisfying "both" effectively without exposing a third option.

### Internal LAN deployments without public DNS

| Option | Description | Selected |
|--------|-------------|----------|
| Caddy ZeroSSL fallback or self-signed (Recommended) | ACME-DNS / ZeroSSL for internal; fallback to self-signed with warning | ✓ |
| Plain HTTP allowed for LAN | Config flag disables TLS for internal — risks cookie security | |
| Bring-your-own cert only | Customer must supply cert — extra setup | |

**User's choice:** ZeroSSL fallback / self-signed.
**Note:** CONTEXT.md `tls.mode: internal` uses Caddy's local CA (self-signed via Caddy's built-in CA). ZeroSSL is documented as the alternative ACME provider when Let's Encrypt rate-limits.

---

## Claude's Discretion

- Exact directory layout under `internal/`
- Cobra command file organization
- Middleware ordering (chi)
- Vite proxy exact form
- Justfile recipe naming
- Caddyfile organization
- `config.yaml` key style (kebab-case vs snake_case)

## Deferred Ideas

- Text-mode logging toggle (v2 polish)
- `/health/detailed` full payload (Phase 6)
- AS923 sub-plan catalog as data file (Phase 7)
- i18n / localization (v2)
- SMTP / password reset email (v2)
- SSO / OAuth (v2)
- Audit log UI / CSV export (Phase 6)
- Per-package log levels (over-engineering for v1)
- Backup script + CI restore test (Phase 6)
