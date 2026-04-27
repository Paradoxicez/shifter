# Phase 1: Foundation — Research

**Researched:** 2026-04-27
**Domain:** Greenfield Go monorepo bootstrap — auth, install wizard, dual-channel ChirpStack v4 integration, scripted Compose deploy
**Confidence:** HIGH (stack, architecture, frontend patterns); MEDIUM (a few specific 2026 library APIs that required cross-check); LOW (none material — see Open Questions for one minor item)

---

## Summary

Phase 1 is the **trunk phase** of Shifter — a single Go binary at the repo root, with a Vite SPA embedded via `go:embed`, a TimescaleDB-backed Postgres, and two Compose flavors (bundled vs external) that share the same image. Every later phase inherits the conventions set here. The phase ships zero telemetry: it ends when the operator can install, sign in, run "Test Connection" against ChirpStack v4, and see two green status rows for gRPC and MQTT.

The heavy lifting is **convention setting**, not algorithmic novelty. The Go stack (chi + sqlc + pgx/v5 + alexedwards/scs + golang-migrate + Cobra + paho.mqtt.golang + chirpstack-api/go/v4) is a well-trodden, boring path. The frontend stack (Vite 7 + React 19 + Tailwind 4 + shadcn/ui new-york + slate + custom navy) is the canonical 2026 SPA shape. The traps are not in any individual library but in their composition: how `go:embed` falls through to `index.html` for SPA routes, how Vite proxies SSE without buffering, how Compose secrets actually land in `/run/secrets/`, how `golang-migrate` is embedded as a library (not invoked as a CLI), how `alexedwards/scs/pgxstore` (not `postgresstore`) is the right backend for pgx/v5, and how the install wizard's `install_state` draft persistence makes the wizard reentrant.

**Primary recommendation:** Treat Phase 1 as **plumbing + conventions**, not features. Every decision below should produce a pattern that Phases 2–7 inherit unmodified. The only place to spend creative effort is the install wizard's reentrant-draft mechanic and the v3-rejection install gate (PITFALLS §7) — everything else is "wire up the boring tech the way the docs say."

---

## User Constraints (from CONTEXT.md)

### Locked Decisions

**Repo Layout & Build Toolchain**
- **D-01:** Single Go-canonical monorepo at the repo root: `cmd/shifter/` (binary entry), `internal/` (private packages), `web/` (Vite + React frontend). The Go binary embeds the built SPA via `go:embed`. No separate `backend/`, no separate `frontend/` repo.
- **D-02:** `Justfile` at the repo root is the canonical build/dev orchestrator. All common operations (`just dev`, `just build`, `just test`, `just migrate`, `just lint`) go through it.
- **D-03:** `pnpm` is the only frontend package manager. `pnpm-lock.yaml` is committed; `package-lock.json` and `yarn.lock` must not appear.
- **D-04:** Hot reload uses `Air` for the Go binary plus `Vite` dev server for the SPA. In dev, Vite serves at `:5173` and proxies `/api` and `/sse` to the Go binary at `:8080`. In prod, Go serves the built SPA from the embed.

**Configuration & Secrets**
- **D-05:** Configuration source order is `config.yaml` (canonical, default at `/etc/shifter/config.yaml`) overridden by environment variables (`SHIFTER_*` prefix). No flag-based config.
- **D-06:** All secrets — Postgres password, ChirpStack API token, session signing key — are mounted as Docker Compose `secrets` (file-based mount, paths referenced from `config.yaml`). `.env` files are forbidden for secrets.
- **D-07:** `config-check` CLI command must validate `config.yaml` shape, ping the Postgres, ChirpStack gRPC, and MQTT endpoints, and exit non-zero on any failure.

**First-Run / Install Wizard Mechanics**
- **D-08:** First-run is detected by absence of any admin user in the `user` table. No flag files, no env vars. The check happens in middleware before any request reaches non-auth routes.
- **D-09:** No "default admin password". Wizard step 1 captures the operator's chosen `email` + `password`. **AUTH-03 is reframed: applies only to admin-created secondary users (Phase 6).**
- **D-10:** Wizard is a 5-step stepped dialog; each step writes to a draft `install_state` table; "Finish setup" commits all four into the canonical tables in one transaction.
- **D-11:** Wizard is reentrant: refreshing the page shows the same step (draft persisted). After commit, the draft row is deleted.

**CLI Surface**
- **D-12:** Cobra-based CLI. Subcommands: `serve`, `migrate`, `version`, `create-admin`, `config-check`, `healthcheck`.
- **D-13:** `shifter serve` auto-runs `migrate up` on startup before opening the listener. Logs "applied N migrations" or "schema up to date".
- **D-14:** `shifter create-admin` is a recovery escape hatch (with `--reset` flag).
- **D-15:** `shifter healthcheck` makes a localhost call to `/health` and exits 0/1. Used by Docker `HEALTHCHECK`.

**Migrations**
- **D-16:** `golang-migrate` library (called as a Go package — not the standalone CLI) runs plain SQL files at `internal/db/migrations/NNNN_<name>.{up,down}.sql`.
- **D-17:** Integer-prefix naming. One conceptual change per migration. Forward-only in production.

**/health Endpoint**
- **D-18:** `/health` is public (no auth) and returns `{ "status": "ok"|"degraded", "version": "<semver>", "uptime_seconds": <int> }`.
- **D-19:** Detailed health (DB, ChirpStack, MQTT, disk, last-uplink-age) at `/health/detailed`, **admin-auth required**. **REQUIREMENTS.md INST-06 wording must be updated during Phase 1.**

**TLS & Reverse Proxy**
- **D-20:** Caddy 2 bundled in both compose flavors.
- **D-21:** TLS modes config-driven via `tls.mode`: `acme` (default, Let's Encrypt; ZeroSSL fallback), `byo` (operator-mounted cert/key), `internal` (Caddy `tls internal` self-signed).
- **D-22:** Plain HTTP not allowed in production. Cookie security flags (`Secure`, `HttpOnly`, `SameSite=Lax`) always set.
- **D-23:** Dev mode runs plain HTTP on `localhost:5173`. Cookie `Secure` is auto-disabled when `SHIFTER_ENV=dev`.

**Logging & Observability**
- **D-24:** Structured JSON logs (one event per line) to stdout via `log/slog`. Operator's Docker `json-file` driver captures them.
- **D-25:** Levels: `info` default, `debug` toggled via `SHIFTER_LOG_LEVEL=debug`. No per-package level config in v1.

### Claude's Discretion

- Exact directory layout under `internal/` (e.g. `internal/auth`, `internal/chirpstack`, `internal/install`, `internal/web`)
- Cobra command file split (one file per subcommand vs. consolidated)
- Middleware order — standard chi layering
- Vite proxy config exact form
- Justfile recipe naming and grouping
- Caddyfile organization (one site block vs. per-route)
- `config.yaml` exact YAML shape (kebab-case vs. snake_case keys)

### Deferred Ideas (OUT OF SCOPE for Phase 1)

- Logging format toggle (text vs JSON) — Phase 6 polish item
- `/health/detailed` rich payload (DB+CS+MQTT+disk+last-uplink-age) — Phase 1 ships minimal version
- AS923 sub-plan catalog file (regulator-versioned data file) — Phase 7
- Internationalization — v2
- SMTP / password reset email — v2
- SSO / OAuth — v2
- Audit log UI — Phase 6 (audit middleware lands Phase 2)
- Per-package log levels — over-engineering for v1
- Backup script & restore CI test — Phase 6
- Detailed health auth and richer probe — Phase 6

---

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| AUTH-01 | User can log in with email and password | SCS+pgxstore session cookie + Argon2id verify (§Auth Stack) |
| AUTH-02 | Session persists across refresh, times out after configurable idle | `sessionManager.IdleTimeout` + `Lifetime` knobs in scs/v2 (§Auth Stack) |
| AUTH-03 | Force change default admin password on first login | **Reframed by D-09**: applies only to admin-created secondary users (Phase 6 USER-04). Bootstrap admin sets own password in wizard step 1. (§Wizard Schema) |
| AUTH-04 | Failed login attempts rate-limited | `golang.org/x/time/rate` per-IP + per-username token bucket, in-memory, single-instance OK (§Rate Limiting) |
| AUTH-05 | User can change own password from account menu | `/api/account/password` POST: verify current → re-hash new → update row (§Auth Stack) |
| AUTH-06 | Admin/viewer roles enforced across every API endpoint AND UI surface | `can(user, action, resource)` API per PITFALLS §14 (§Authorization Pattern) |
| INST-01 | First-run install wizard guides operator through creating initial admin | 5-step stepped dialog; `install_state` draft table (§Wizard Schema) |
| INST-02 | Wizard captures install identity (display name, logo, address, timezone, units) | Step 4 of wizard; logo upload to local volume (§Wizard Schema) |
| INST-03 | Wizard captures ChirpStack mode (bundled/external) + gRPC URL + API token + MQTT URL | Step 2 of wizard; v3-detection happens at submit-time (§ChirpStack Integration) |
| INST-04 | Regulator-aware LoRaWAN region picker (AS923 sub-plans for Thailand, EU868, US915, etc.) | Step 3; default AS923-2 for Thailand (PITFALLS §8); ChirpStack region `common_name` enum (§AS923) |
| INST-05 | System refuses to connect to ChirpStack v3 — only v4 supported, validated on first connect | `InternalService.GetVersion(Empty) → GetVersionResponse` is the canonical probe (§v3 Rejection) |
| INST-06 | `/health` reports DB, CS, MQTT, disk, last-uplink-age, and Shifter version | **Reframed by D-18/D-19**: `/health` public minimal `{status,version,uptime_seconds}`, `/health/detailed` admin-auth (§Health Endpoint) |
| CHIRP-01 | Backend connects to ChirpStack over gRPC for control plane | `chirpstack/api/go/v4` v4.17.0+ + `google.golang.org/grpc` v1.66+ (§ChirpStack Integration) |
| CHIRP-02 | Backend subscribes to ChirpStack MQTT integration topic for real-time uplinks | `paho.mqtt.golang` v1.5+; topic `application/+/device/+/event/up`; logs to stdout in Phase 1, no DB write (§MQTT Subscription) |
| CHIRP-03 | "Test connection" action verifies gRPC + MQTT reachability with clear error path | Two-channel probe behind one button; status-row UI (UI-SPEC); see §Test Connection |
| OPS-01 | Two Compose files — bundled and external — using same backend image | `docker-compose.bundled.yml` + `docker-compose.external.yml` + Caddy + Compose secrets (§Deployment) |
| UX-01 | All create/edit/delete via dialogs — no full-page CRUD | shadcn `<Dialog>` + `<ResponsiveDialog>` wrapper for mobile sheet swap (UI-SPEC §Dialog Conventions) |
| UX-02 | shadcn/ui blue/navy palette, modern minimal, English-only | new-york style + slate base + OKLCH navy `#1E40AF` primary (UI-SPEC §Color) |

---

## Project Constraints (from CLAUDE.md)

CLAUDE.md is comprehensive and treated as authoritative. Recommendations below align with these directives:

- **Backend language:** Go 1.24+ (Go 1.26 GA installed locally — verified `go1.26.0 darwin/arm64`)
- **Tech stack pinned:** PostgreSQL 16/17 + TimescaleDB 2.26, ChirpStack v4, Mosquitto 2.x, React 19, Vite 7, TypeScript 5.6+, Tailwind v4, Docker Compose
- **Backend libs (do not substitute):** `chirpstack/api/go/v4`, `google.golang.org/grpc`, `paho.mqtt.golang`, `pgx/v5`, `sqlc`, `chi/v5`, `golang-migrate/v4`, `golang.org/x/crypto/argon2`, `alexedwards/scs/v2`, `riverqueue/river`, `excelize/v2`, `maroto/v2`, stdlib `encoding/csv`, `cobra` + `viper`, `log/slog`
- **Frontend libs (do not substitute):** `shadcn/ui`, `tailwindcss@4`, `lucide-react`, `recharts`, `@tanstack/react-query`, `react-leaflet@5`, `leaflet@1.9`, `react-router-dom@7`, `zod`, `react-hook-form`, `date-fns@4`, `sonner`
- **NEVER use:** GORM, lib/pq, gofpdf, MapLibre/Mapbox, Next.js, MQTT-over-WebSocket exposed to browser, JWT in localStorage, mixed bcrypt+argon2, Kafka/RabbitMQ for ChirpStack events, Redis (Phase 1 has no Redis dependency), `.env` for secrets, `:latest` Docker tags, plain HTTP in production
- **GSD enforcement:** All file edits must originate from a GSD command. This is the active workflow.

CLAUDE.md and CONTEXT.md are aligned. No conflicts.

---

## Standard Stack

### Core (verified versions)

| Library | Version | Purpose | Why Standard | Provenance |
|---------|---------|---------|--------------|-----------|
| `go` | 1.24+ (1.26 GA installed) | Backend language | Single binary, native ChirpStack gRPC stubs, mature MQTT/Postgres/scheduler ecosystem | [VERIFIED: local `go version go1.26.0 darwin/arm64`] |
| `github.com/chirpstack/chirpstack/api/go/v4` | v4.17.0+ (March 2026) | ChirpStack gRPC client stubs | Official, generated from v4 protos, exposes `InternalService.GetVersion` for v3-rejection probe | [CITED: pkg.go.dev/github.com/chirpstack/chirpstack/api/go/v4] |
| `google.golang.org/grpc` | v1.66+ | gRPC transport | Required by chirpstack-api | [CITED: STACK.md] |
| `github.com/eclipse/paho.mqtt.golang` | v1.5+ | MQTT v3.1.1 client | Subscribe to `application/+/device/+/event/up` | [CITED: github.com/eclipse-paho/paho.mqtt.golang] |
| `github.com/jackc/pgx/v5` | v5.7+ | Postgres driver + pool | Production-grade; required by sqlc + scs/pgxstore + River | [CITED: pkg.go.dev/github.com/jackc/pgx/v5] |
| `github.com/sqlc-dev/sqlc` | v1.31+ | SQL → typed Go | TimescaleDB-aware queries; never abstracts SQL away | [CITED: docs.sqlc.dev/en/latest/guides/using-go-and-pgx.html] |
| `github.com/go-chi/chi/v5` | v5.1+ | HTTP router | Composable with stdlib middleware | [CITED: github.com/go-chi/chi] |
| `github.com/golang-migrate/migrate/v4` | v4.18+ | Migrations | Plain SQL files, used as **library** via `iofs` source + `pgx/v5` driver | [CITED: pkg.go.dev/github.com/golang-migrate/migrate/v4/source/iofs] |
| `golang.org/x/crypto/argon2` | latest stdlib-adjacent | Password hashing (Argon2id) | OWASP-recommended; PHC-format encoding | [CITED: cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html] |
| `github.com/alexedwards/scs/v2` | v2.8+ | Session manager | Server-side, httpOnly cookie, Postgres-backed | [CITED: pkg.go.dev/github.com/alexedwards/scs/v2] |
| `github.com/alexedwards/scs/pgxstore` | latest | scs backend for pgx/v5 pool | **Use this, not `postgresstore`** — pgxstore takes `*pgxpool.Pool` directly | [VERIFIED: pkg.go.dev/github.com/alexedwards/scs/pgxstore] |
| `golang.org/x/time/rate` | stdlib-adjacent | In-process token bucket | Per-IP + per-username login rate limit (AUTH-04) | [CITED: dev.to/ohugonnot/rate-limiter-in-go-per-ip-token-bucket-with-golangorgxtimerate-5ff8] |
| `github.com/spf13/cobra` | v1.8+ | CLI subcommands | `serve`/`migrate`/`version`/`create-admin`/`config-check`/`healthcheck` | [CITED: STACK.md] |
| `github.com/spf13/viper` | v1.19+ | Config layering | YAML + env-var precedence per D-05 | [CITED: STACK.md] |
| `log/slog` | stdlib | Structured logging | JSON to stdout per D-24 | [CITED: pkg.go.dev/log/slog] |

**Phase 1 deliberately does NOT install (deferred):**
- `riverqueue/river` — no jobs in Phase 1 (Phase 2+ for alert eval, CAGG refresh, PDF gen)
- `excelize/v2`, `maroto/v2` — no exports yet (Phase 5)
- `mockgen` — useful for testing the gRPC and MQTT boundaries; recommended for Phase 1 test scaffold
- `air-verse/air` — dev tool, install via `go install`
- `golangci-lint`, `gofumpt` — lint; pin a version in CI

### Frontend (verified peer-deps)

| Library | Version | Purpose | Provenance |
|---------|---------|---------|-----------|
| `react` + `react-dom` | 19.x | Framework (peer of shadcn + react-leaflet) | [CITED: STACK.md, UI-SPEC] |
| `vite` + `@vitejs/plugin-react` | 7.x | Build/dev server | [CITED: ui.shadcn.com/docs/installation/vite] |
| `typescript` | 5.6+ | Types | [CITED: STACK.md] |
| `tailwindcss` + `@tailwindcss/vite` | 4.x | Styling | [CITED: ui.shadcn.com/docs/tailwind-v4] |
| `shadcn` (CLI) | latest | Component installer | Components copied into repo, not deps | [CITED: ui.shadcn.com/docs/installation/vite] |
| `lucide-react` | latest | Icons | [CITED: shadcn defaults] |
| `react-router-dom` | v7.x | Routing | SPA mode (no framework mode); loader-based protected routes | [CITED: blog.logrocket.com/authentication-react-router-v7/] |
| `@tanstack/react-query` | v5.x | Server state | Mutation primitives matter for dialog-heavy UI | [CITED: STACK.md] |
| `react-hook-form` | v7.53+ | Form state | shadcn's official form recipe | [CITED: shadcn docs] |
| `zod` | v3.23+ | Runtime validation | Pairs with react-hook-form | [CITED: STACK.md] |
| `@hookform/resolvers` | latest | Bridge zod ↔ react-hook-form | [CITED: STACK.md] |
| `sonner` | latest | Toasts | shadcn-blessed | [CITED: shadcn docs] |
| `date-fns` | v4.x | Date math | Tree-shakeable | [CITED: STACK.md] |
| `@fontsource-variable/inter` | latest | Self-hosted Inter (variable, weights 400+600) | UI-SPEC explicit decision (no Google Fonts CDN) | [CITED: UI-SPEC §Typography] |
| `@fontsource/jetbrains-mono` | latest | Self-hosted JetBrains Mono for code/IDs | [CITED: UI-SPEC §Typography] |

**Phase 1 does NOT install:** `recharts` (Phase 4), `react-leaflet` + `leaflet` (Phase 5), `react-image-pin` (Phase 5).

### Tooling (install path — already audited)

| Tool | Status (audit) | Install path |
|------|---------------|--------------|
| `go` 1.26 | Available locally | — |
| `node` 22.11 | Available | — |
| `pnpm` 10.33 | Available | — |
| `docker` 28.3 | Available | — |
| `psql` 14 client | Available | (server runs in Compose; client is for ad-hoc) |
| `just` | **Missing** | `brew install just` (macOS) / `cargo install just` |
| `caddy` | **Missing** (only needed for local cert testing; bundled in Compose otherwise) | `brew install caddy` if local probe needed |
| `air` | **Missing** | `go install github.com/air-verse/air@latest` |
| `sqlc` | **Missing** | `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest` |
| `migrate` CLI | **Missing** (we use it as a library, but the CLI is handy for ad-hoc) | `go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest` |
| `golangci-lint` | **Missing** | `brew install golangci-lint` |
| `playwright` | **Missing** | `pnpm add -D playwright` (Phase 6 territory; OK to defer) |

### Installation commands

```bash
# Repo bootstrap
go mod init github.com/<org>/shifter

# Backend deps (Phase 1)
go get github.com/chirpstack/chirpstack/api/go/v4@latest
go get google.golang.org/grpc@latest
go get github.com/eclipse/paho.mqtt.golang@latest
go get github.com/jackc/pgx/v5@latest
go get github.com/jackc/pgx/v5/pgxpool@latest
go get github.com/go-chi/chi/v5@latest
go get github.com/golang-migrate/migrate/v4@latest
go get github.com/golang-migrate/migrate/v4/source/iofs@latest
go get github.com/golang-migrate/migrate/v4/database/pgx/v5@latest
go get golang.org/x/crypto/argon2
go get golang.org/x/time/rate
go get github.com/alexedwards/scs/v2@latest
go get github.com/alexedwards/scs/pgxstore@latest
go get github.com/spf13/cobra@latest
go get github.com/spf13/viper@latest

# sqlc as build-only tool
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

# Frontend bootstrap (Phase 1)
cd web
pnpm create vite@latest . -- --template react-ts
pnpm install
pnpm add tailwindcss @tailwindcss/vite
pnpm dlx shadcn@latest init       # style: new-york, base: slate, css-vars: yes
# shadcn components per UI-SPEC §Phase 1 inventory:
pnpm dlx shadcn@latest add button input label form card dialog alert alert-dialog \
  dropdown-menu avatar select checkbox separator skeleton sonner tabs progress badge tooltip sheet
pnpm add @tanstack/react-query react-hook-form @hookform/resolvers zod
pnpm add react-router-dom date-fns sonner lucide-react
pnpm add @fontsource-variable/inter @fontsource/jetbrains-mono
pnpm add -D @biomejs/biome
```

**Version verification performed (2026-04-27):** Versions referenced in CLAUDE.md and STACK.md were cross-checked against pkg.go.dev and npm where possible. ChirpStack `api/go/v4` is at v4.17.0+ as of March 2026 [CITED: STACK.md, pkg.go.dev]; TimescaleDB 2.26 (March 24 2026) [CITED: github.com/timescale/timescaledb/releases]; react-leaflet 5.0 requires React 19 [CITED: npmjs.com/package/react-leaflet]. None of the pins are stale.

### Alternatives Considered

Already exhaustively explored in STACK.md. For Phase 1 specifically the only judgment calls are:

| Instead of | Could Use | Tradeoff (and why we don't switch) |
|------------|-----------|-------------------------------------|
| `alexedwards/scs/pgxstore` | `alexedwards/scs/postgresstore` (with pgx/v5 stdlib adapter) | postgresstore is the older sql.DB-based store; pgxstore is purpose-built for `*pgxpool.Pool` and aligns with our pgx-native architecture [VERIFIED: pkg.go.dev/github.com/alexedwards/scs/pgxstore] |
| `golang.org/x/time/rate` (in-process token bucket) | `go-chi/httprate` (chi-aware middleware) | httprate is fine but adds a dep; x/time/rate is stdlib-adjacent and gives per-username keying easily [CITED: dev.to per-IP token bucket] |
| `paho.mqtt.golang` v1 | `eclipse/paho.golang/autopaho` (the v5 library) | autopaho is MQTT v5 only; ChirpStack publishes MQTT 3.1.1 by default. Stay on paho.mqtt.golang v1.5+. [CITED: github.com/eclipse-paho/paho.mqtt.golang] |

---

## Architecture Patterns

### Recommended Project Structure

```
shifter/
├── Justfile                       # canonical build/dev/test/lint orchestrator (D-02)
├── go.mod / go.sum
├── pnpm-workspace.yaml            # if/when needed; not strictly required for one web/ package
├── docker-compose.bundled.yml     # bundled CS flavor (OPS-01)
├── docker-compose.external.yml    # external CS flavor (OPS-01)
├── Caddyfile                      # config-driven TLS, env-templated (D-20..D-23)
├── .dockerignore
├── .gitignore                     # MUST exclude: .env, secrets/, web/dist, web/node_modules
├── README.md
├── config/
│   ├── config.example.yaml        # documented keys; install kit copies this
│   └── mosquitto.conf             # bundled mode only
├── secrets/                       # local dev (.gitignored). Compose secrets live here.
│   └── .gitkeep
├── cmd/
│   └── shifter/
│       └── main.go                # Cobra root entrypoint; delegates to internal/cli
├── internal/
│   ├── cli/                       # Cobra subcommand wiring (D-12)
│   │   ├── root.go
│   │   ├── serve.go               # serve = migrate-up + listen
│   │   ├── migrate.go             # migrate up/down/version/force
│   │   ├── version.go
│   │   ├── createadmin.go
│   │   ├── configcheck.go
│   │   └── healthcheck.go         # localhost call to /health (D-15)
│   ├── config/                    # viper YAML + SHIFTER_* env layering (D-05)
│   │   ├── config.go              # struct + Load() + Validate()
│   │   └── secrets.go             # /run/secrets/<name> reader (D-06)
│   ├── logging/                   # slog JSON handler bootstrap (D-24, D-25)
│   ├── db/
│   │   ├── pool.go                # pgxpool.New
│   │   ├── migrations/
│   │   │   ├── 0001_init.up.sql / .down.sql
│   │   │   ├── 0002_users.up.sql / .down.sql
│   │   │   ├── 0003_sessions.up.sql / .down.sql      # alexedwards/scs schema
│   │   │   ├── 0004_install_state.up.sql / .down.sql # wizard draft (D-10)
│   │   │   ├── 0005_install_identity.up.sql / .down.sql
│   │   │   └── 0006_chirpstack_connection.up.sql / .down.sql
│   │   ├── migrations.go          # //go:embed + iofs + migrate.New + Up()
│   │   └── queries/               # sqlc input
│   │       ├── users.sql
│   │       ├── install_state.sql
│   │       └── install_identity.sql
│   ├── auth/
│   │   ├── argon2id.go            # Hash + Verify; PHC-encoded
│   │   ├── ratelimit.go           # x/time/rate per-IP + per-username (AUTH-04)
│   │   ├── session.go             # scs setup; cookie config; SHIFTER_ENV=dev toggle
│   │   ├── handlers.go            # POST /api/auth/login, /api/auth/logout
│   │   ├── account.go             # POST /api/account/password (AUTH-05)
│   │   └── authz.go               # can(user, action, resource) (PITFALLS §14)
│   ├── install/
│   │   ├── middleware.go          # first-run gate (D-08): if no admin → 307 → /install
│   │   ├── handlers.go            # GET /api/install/state, POST /api/install/step/{n}, POST /api/install/finish
│   │   ├── state.go               # draft persistence + atomic commit (D-10, D-11)
│   │   └── regions.go             # AS923-x + EU868 + US915 catalog
│   ├── chirpstack/
│   │   ├── client.go              # gRPC dial + reconnect; only module that imports CS protos
│   │   ├── version.go             # InternalService.GetVersion → v3 rejection (INST-05)
│   │   ├── ping.go                # used by Test Connection (gRPC channel)
│   │   └── mqtt.go                # paho subscriber for application/+/device/+/event/up
│   ├── http/
│   │   ├── router.go              # chi.NewRouter + middleware chain
│   │   ├── middleware.go          # request-id, logger, recoverer, session, authz
│   │   ├── health.go              # /health (public) + /health/detailed (admin)
│   │   ├── testconn.go            # POST /api/settings/chirpstack/test (CHIRP-03)
│   │   └── spa.go                 # //go:embed of web/dist + SPA fallback
│   └── version/
│       └── version.go             # ldflags-injected build info; used by /health and `shifter version`
└── web/
    ├── package.json
    ├── pnpm-lock.yaml
    ├── vite.config.ts             # proxy /api + /sse → :8080 (D-04)
    ├── tsconfig.json
    ├── components.json            # shadcn config (style=new-york, base=slate)
    ├── biome.json
    ├── index.html
    ├── public/
    │   └── shifter-mark.svg
    └── src/
        ├── main.tsx
        ├── App.tsx                # router + react-query provider + theme provider + sonner
        ├── index.css              # tailwind imports + OKLCH custom palette per UI-SPEC
        ├── routes/
        │   ├── login.tsx
        │   ├── force-change-password.tsx
        │   ├── install.tsx        # the wizard route
        │   ├── settings.tsx
        │   ├── _root.tsx          # protected app shell
        │   └── _auth.tsx          # public layout for login/wizard
        ├── components/
        │   ├── ui/                # shadcn-installed components (auto-managed)
        │   ├── responsive-dialog.tsx     # <Dialog>↔<Sheet> swap (UI-SPEC mandatory)
        │   ├── status-row.tsx            # dot + label + mono detail (UI-SPEC)
        │   ├── stepper.tsx               # numbered horizontal + check icons
        │   ├── theme-provider.tsx        # light/dark/system
        │   └── shell/
        │       ├── topbar.tsx
        │       ├── sidebar.tsx
        │       └── account-menu.tsx
        ├── lib/
        │   ├── api.ts             # typed fetch wrapper; sends `Cookie` automatically
        │   ├── query-client.ts    # tanstack-query setup
        │   ├── auth.ts            # /api/auth/* helpers
        │   ├── install.ts         # /api/install/* helpers
        │   └── theme.ts
        └── assets/
            ├── shifter-logo.svg
            └── shifter-mark.svg
```

**Structure rationale:**
- One module under `internal/` per concern. `chirpstack/` is the **only** package that imports CS protos — Phase 2+ keeps this seam.
- `migrations/` lives at `internal/db/migrations/` per D-16.
- `web/` is a sibling of `internal/`, **not** under it — `go:embed web/dist` from `internal/http/spa.go` works cleanly with Go's relative-path embed semantics.
- `secrets/` directory at repo root holds local-dev secret files; gitignored. Compose mounts these into containers.

### Pattern 1: Two-Path Architecture (locked from ARCHITECTURE.md)

Phase 1 sets up the **API path**: HTTP/JSON CRUD for auth/install/settings + future SSE hub. The **ingestion path** (Phase 2) is stubbed in Phase 1 — `internal/chirpstack/mqtt.go` connects + subscribes + logs to stdout. **No DB writes from MQTT in Phase 1.** This proves the wire works without committing to a normalize/resolve/insert pipeline.

### Pattern 2: First-Run Detection in Middleware (D-08)

```go
// internal/install/middleware.go
func FirstRunGate(pool *pgxpool.Pool) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Whitelist: /install, /api/install/*, /health, static SPA assets
            if isInstallPath(r.URL.Path) || isHealthPath(r.URL.Path) || isStaticAsset(r.URL.Path) {
                next.ServeHTTP(w, r)
                return
            }
            adminExists, err := db.AdminUserExists(r.Context(), pool)
            if err != nil { http.Error(w, "bootstrap check failed", 500); return }
            if !adminExists {
                // SPA route — return JSON for /api/*, redirect for HTML
                if strings.HasPrefix(r.URL.Path, "/api/") {
                    httputil.WriteJSON(w, 409, map[string]any{"error":"install_required"})
                    return
                }
                http.Redirect(w, r, "/install", http.StatusTemporaryRedirect)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

The check is a single `SELECT EXISTS(SELECT 1 FROM "user" WHERE role='admin')` which Postgres serves in <1ms. Cache it in a `sync.Once` *only* after a positive result, because once the admin exists the answer never reverts.

### Pattern 3: Reentrant Install Wizard (D-10, D-11) — schema and commit

```sql
-- 0004_install_state.up.sql
CREATE TABLE install_state (
    id            INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),  -- singleton row
    started_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at  TIMESTAMPTZ,                               -- NULL = mid-wizard
    current_step  SMALLINT NOT NULL DEFAULT 1 CHECK (current_step BETWEEN 1 AND 5),
    -- Per-step drafts as JSONB (validated via zod on the client + manually here on POST):
    step1_admin       JSONB,    -- { email, name, password_hash } (already Argon2id'd at step submit)
    step2_chirpstack  JSONB,    -- { mode, grpc_url, api_token_ref, mqtt_url, mqtt_user, mqtt_pass_ref }
    step3_region      JSONB,    -- { common_name: "AS923", sub_band: "AS923_2" } etc.
    step4_identity    JSONB,    -- { display_name, logo_path, address, timezone, units }
    -- secrets are stored by REF (path on disk under /run/secrets) — never the raw value
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION touch_install_state() RETURNS trigger AS $$
BEGIN NEW.updated_at = now(); RETURN NEW; END $$ LANGUAGE plpgsql;
CREATE TRIGGER install_state_touch BEFORE UPDATE ON install_state
    FOR EACH ROW EXECUTE FUNCTION touch_install_state();
```

**Atomic commit at "Finish setup":**

```go
// internal/install/state.go (sketch)
func (s *Store) FinishSetup(ctx context.Context) error {
    tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
    if err != nil { return err }
    defer tx.Rollback(ctx)

    state, err := loadInstallState(ctx, tx)
    if err != nil { return err }
    if state.CompletedAt != nil { return ErrAlreadyCompleted }
    if !state.AllStepsValid() { return ErrIncompleteWizard }

    // 1. Create admin user
    if _, err := tx.Exec(ctx, `INSERT INTO "user" (email, name, password_hash, role) VALUES ($1,$2,$3,'admin')`,
        state.Step1.Email, state.Step1.Name, state.Step1.PasswordHash); err != nil { return err }
    // 2. ChirpStack connection (table sketch: id, mode, grpc_url, mqtt_url, default_region, ...)
    if _, err := tx.Exec(ctx, `INSERT INTO chirpstack_connection (...) VALUES (...)`, ...); err != nil { return err }
    // 3. Default region
    if _, err := tx.Exec(ctx, `INSERT INTO install_settings (...) VALUES (...)`, ...); err != nil { return err }
    // 4. Install identity
    if _, err := tx.Exec(ctx, `INSERT INTO install_identity (...) VALUES (...)`, ...); err != nil { return err }
    // 5. Mark complete (deletion is also acceptable; keeping the row for audit)
    if _, err := tx.Exec(ctx, `DELETE FROM install_state WHERE id = 1`); err != nil { return err }

    return tx.Commit(ctx)
}
```

**Why DELETE the draft row at commit:** D-11 says the draft row is deleted at completion. Combined with D-08's "first-run = no admin user", we get a clean two-state system: (a) `user` table empty → wizard, (b) `user` table has admin → app. The `install_state` table is *only* meaningful while the wizard is in flight. After commit, completed installations have no row.

**Reentrance:** On every `GET /api/install/state`, return the (singleton) install_state row. If absent, the SQL handler creates it with `INSERT INTO install_state (id) VALUES (1) ON CONFLICT (id) DO NOTHING RETURNING *`. Frontend renders the wizard at `current_step`.

### Pattern 4: ChirpStack v3 rejection probe (INST-05; PITFALLS §7)

The `chirpstack/api/go/v4` package exposes `InternalService` with `GetVersion(google.protobuf.Empty) returns (GetVersionResponse)` [VERIFIED: github.com/chirpstack/chirpstack/blob/master/api/proto/api/internal.proto].

```go
// internal/chirpstack/version.go
import (
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
    "google.golang.org/protobuf/types/known/emptypb"
    api "github.com/chirpstack/chirpstack/api/go/v4/api"
)

func ProbeVersion(ctx context.Context, conn *grpc.ClientConn) (string, error) {
    client := api.NewInternalServiceClient(conn)
    resp, err := client.GetVersion(ctx, &emptypb.Empty{})
    if err != nil {
        st, ok := status.FromError(err)
        // v3 has no InternalService.GetVersion → returns Unimplemented
        // a v3 server with the old proto returns codes.Unimplemented OR a NotFound on the
        // wire path. Treat both as "not v4."
        if ok && (st.Code() == codes.Unimplemented || st.Code() == codes.NotFound) {
            return "", ErrChirpStackV3OrUnknown
        }
        return "", fmt.Errorf("chirpstack version probe failed: %w", err)
    }
    return resp.Version, nil
}
```

**Note on the negative claim:** The ChirpStack v4 breaking-changes doc does not prescribe a "use this RPC to detect v3" pattern [CITED: chirpstack.io/docs/v4-breaking-changes.html — does not document a probe]. We are using `InternalService.GetVersion` because (a) it exists on v4 [VERIFIED: internal.proto], (b) it is named `Internal` but is *exposed on the same gRPC port* (`:8080` admin) and accessible with the same API token, (c) v3 does not implement this proto so the call returns `Unimplemented`. The "Internal" naming is a documentation flag — the service is callable; ChirpStack uses it from its own admin UI. **[ASSUMED]:** the explicit gRPC error code v3 returns is `Unimplemented`. If during implementation the actual code differs (e.g. `Unknown` or a transport-level RST), the planner should treat the implementation task as a one-cycle research+patch rather than a planning blocker. The remediation is one-line.

### Pattern 5: Argon2id with PHC encoding (AUTH-01, AUTH-05)

OWASP-current parameters for Argon2id [VERIFIED: cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html, fetched 2026-04-27]:

> Argon2id with a minimum configuration of 19 MiB of memory (m=19456), iteration count of 2 (t=2), and 1 degree of parallelism (p=1).
>
> Alternative configurations (all equal security):
> m=47104 (46 MiB), t=1, p=1
> m=19456 (19 MiB), t=2, p=1 ← **recommended minimum**
> m=12288 (12 MiB), t=3, p=1
> m=9216  (9 MiB),  t=4, p=1
> m=7168  (7 MiB),  t=5, p=1

**Parameter choice for Shifter:** Use `m=19456 KiB, t=2, p=1` (OWASP minimum). Per-customer single-tenant deploys run on modest hardware; the 19 MiB memory cost per login is negligible for Shifter's tens-of-logins-per-day scale.

**Salt + key length:** OWASP cheatsheet does not prescribe specific lengths in the Argon2id sub-section, but the broader doc (§Salting) requires "at least 16 bytes for the salt." For key length use 32 bytes (256-bit) which is the Argon2 RFC's strong default. **[CITED]:** [P-H-C/phc-string-format spec](https://github.com/P-H-C/phc-string-format) and the alexedwards/argon2id Go reference impl both default to 16-byte salt + 32-byte key.

```go
// internal/auth/argon2id.go
package auth

import (
    "crypto/rand"
    "crypto/subtle"
    "encoding/base64"
    "errors"
    "fmt"
    "strings"
    "golang.org/x/crypto/argon2"
)

const (
    argonMemKiB  uint32 = 19_456 // 19 MiB
    argonTime    uint32 = 2
    argonThreads uint8  = 1
    argonSaltLen        = 16
    argonKeyLen  uint32 = 32
)

// Hash returns a PHC-format string: $argon2id$v=19$m=...,t=...,p=...$<b64salt>$<b64hash>
func Hash(password string) (string, error) {
    salt := make([]byte, argonSaltLen)
    if _, err := rand.Read(salt); err != nil { return "", err }
    key := argon2.IDKey([]byte(password), salt, argonTime, argonMemKiB, argonThreads, argonKeyLen)
    b64 := base64.RawStdEncoding.EncodeToString
    return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
        argon2.Version, argonMemKiB, argonTime, argonThreads, b64(salt), b64(key)), nil
}

// Verify is constant-time. It also re-parses the encoded params, so we can
// migrate to stronger params later without breaking existing hashes.
func Verify(password, encoded string) (bool, error) {
    parts := strings.Split(encoded, "$")
    if len(parts) != 6 || parts[1] != "argon2id" { return false, errors.New("not argon2id") }
    var version int
    if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil { return false, err }
    if version != argon2.Version { return false, errors.New("argon2 version mismatch") }
    var m, t uint32; var p uint8
    if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil { return false, err }
    salt, err := base64.RawStdEncoding.DecodeString(parts[4])
    if err != nil { return false, err }
    want, err := base64.RawStdEncoding.DecodeString(parts[5])
    if err != nil { return false, err }
    got := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(want)))
    return subtle.ConstantTimeCompare(got, want) == 1, nil
}
```

**[CITED]:** PHC format spec [P-H-C/phc-string-format](https://github.com/P-H-C/phc-string-format/blob/master/phc-sf-spec.md), [alexedwards/argon2id](https://github.com/alexedwards/argon2id) reference impl, [alexedwards.net Argon2 hashing in Go](https://www.alexedwards.net/blog/how-to-hash-and-verify-passwords-with-argon2-in-go).

### Pattern 6: SCS sessions with pgxstore (AUTH-01, AUTH-02)

`alexedwards/scs/pgxstore` accepts `*pgxpool.Pool` directly [VERIFIED: pkg.go.dev/github.com/alexedwards/scs/pgxstore]. **Do NOT use `postgresstore` with the pgx stdlib adapter — pgxstore is purpose-built and avoids a redundant `database/sql` layer.**

```sql
-- 0003_sessions.up.sql (canonical schema from alexedwards/scs docs)
CREATE TABLE sessions (
    token  TEXT        PRIMARY KEY,
    data   BYTEA       NOT NULL,
    expiry TIMESTAMPTZ NOT NULL
);
CREATE INDEX sessions_expiry_idx ON sessions (expiry);
```

```go
// internal/auth/session.go
import (
    "net/http"
    "time"
    "github.com/alexedwards/scs/pgxstore"
    "github.com/alexedwards/scs/v2"
    "github.com/jackc/pgx/v5/pgxpool"
)

func NewSessionManager(pool *pgxpool.Pool, devMode bool) *scs.SessionManager {
    sm := scs.New()
    sm.Store = pgxstore.New(pool) // background cleanup goroutine, 5min default
    sm.Lifetime    = 24 * time.Hour
    sm.IdleTimeout = 8 * time.Hour     // AUTH-02 idle period (configurable via SHIFTER_*)
    sm.Cookie.Name     = "shifter_session"
    sm.Cookie.HttpOnly = true
    sm.Cookie.Secure   = !devMode      // D-23: SHIFTER_ENV=dev disables Secure
    sm.Cookie.SameSite = http.SameSiteLaxMode
    sm.Cookie.Path     = "/"
    return sm
}
```

**Wrap chi router with `sm.LoadAndSave(router)` at the outermost layer** (before chi middleware) so every handler has access to `sm.Get(r.Context(), "user_id")` etc.

**[CITED]:** [pkg.go.dev/github.com/alexedwards/scs/pgxstore](https://pkg.go.dev/github.com/alexedwards/scs/pgxstore) and [pkg.go.dev/github.com/alexedwards/scs/v2](https://pkg.go.dev/github.com/alexedwards/scs/v2). Cookie config exactly as quoted from the pgxstore docs example.

### Pattern 7: golang-migrate as embedded library (D-13, D-16)

```go
// internal/db/migrations.go
package db

import (
    "context"
    "embed"
    "errors"
    "log/slog"

    "github.com/golang-migrate/migrate/v4"
    pgxdb "github.com/golang-migrate/migrate/v4/database/pgx/v5"
    "github.com/golang-migrate/migrate/v4/source/iofs"
    "github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func RunMigrations(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
    src, err := iofs.New(migrationsFS, "migrations")
    if err != nil { return err }

    // Convert pgxpool to *sql.DB-shape via the pgx/v5 migrate driver
    conn, err := pool.Acquire(ctx)
    if err != nil { return err }
    defer conn.Release()

    drv, err := pgxdb.WithInstance(conn.Conn(), &pgxdb.Config{})
    if err != nil { return err }

    m, err := migrate.NewWithInstance("iofs", src, "pgx", drv)
    if err != nil { return err }

    versionBefore, _, err := m.Version()
    if err != nil && !errors.Is(err, migrate.ErrNilVersion) { return err }

    if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
        // dirty state: log and surface — DO NOT auto-force in production
        return fmt.Errorf("migrate up: %w (run `shifter migrate force <prev_version>` if dirty)", err)
    }

    versionAfter, dirty, err := m.Version()
    if err != nil && !errors.Is(err, migrate.ErrNilVersion) { return err }
    log.Info("migrations applied",
        "from", versionBefore, "to", versionAfter, "dirty", dirty)
    return nil
}
```

**Dirty-state handling:** golang-migrate marks the schema dirty if any migration aborts mid-run. Recovery is the operator's job: `shifter migrate force <last_clean_version>` and rerun. Phase 1 ships the `force` subcommand under `shifter migrate force <N>` and documents the recovery procedure in the README. [CITED: pkg.go.dev/github.com/golang-migrate/migrate/v4/source/iofs and iofs example_test.go]

**Why pgx/v5 driver:** golang-migrate v4.18+ ships `database/pgx/v5` natively. Avoids the stdlib sql.DB indirection. [CITED: github.com/golang-migrate/migrate/tree/master/database/pgx]

### Pattern 8: `go:embed` SPA + history-mode fallback

```go
// internal/http/spa.go
package http

import (
    "embed"
    "io/fs"
    "net/http"
    "path"
    "strings"
)

//go:embed all:web/dist
var spaFS embed.FS

func SPAHandler() http.Handler {
    sub, err := fs.Sub(spaFS, "web/dist")
    if err != nil { panic(err) }
    fileServer := http.FileServer(http.FS(sub))

    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Try the requested file first
        cleanPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
        if cleanPath == "" { cleanPath = "index.html" }

        f, err := sub.Open(cleanPath)
        if err == nil {
            f.Close()
            // Hashed assets get long-cache; HTML never cached.
            if strings.HasPrefix(cleanPath, "assets/") {
                w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
            } else {
                w.Header().Set("Cache-Control", "no-cache")
            }
            fileServer.ServeHTTP(w, r)
            return
        }
        // SPA history-mode fallback: serve index.html for any non-existent file
        // that doesn't look like a static asset (no extension or .html only).
        if path.Ext(cleanPath) == "" || path.Ext(cleanPath) == ".html" {
            r2 := r.Clone(r.Context())
            r2.URL.Path = "/"
            w.Header().Set("Cache-Control", "no-cache")
            fileServer.ServeHTTP(w, r2)
            return
        }
        http.NotFound(w, r)
    })
}
```

**Pin notes:**
- Use `//go:embed all:web/dist` (the `all:` prefix includes dotfiles like `.vite/` if Vite drops any). Without `all:`, dotfile assets are silently excluded.
- `fs.Sub(...)` strips the `web/dist/` prefix so the FS root is what the FileServer expects.
- The `Vary: Accept-Encoding` header is implicitly set by FileServer when content-negotiating; `text/html` and `application/javascript` MIME types are correctly inferred from extensions.
- Asset paths produced by Vite (`assets/index-<hash>.js`) are content-hashed and are safe to send with `immutable` cache header.

[CITED]: [embed package docs](https://pkg.go.dev/embed), [dev.to "One of the coolest features of Go"](https://dev.to/pacholoamit/one-of-the-coolest-features-of-go-embed-reactjs-into-a-go-binary-41e9), [Feng's Go Embed Vite notes](https://ofeng.org/posts/go-embed-vite/).

### Pattern 9: Vite proxy for `/api` and `/sse` (D-04)

The naïve `proxy: { '/api': 'http://localhost:8080' }` works for REST. **For SSE the response stream is buffered by http-proxy by default**, breaking real-time delivery. The fix is twofold: (a) backend sets `X-Accel-Buffering: no` on the SSE response (which the dev proxy respects), and (b) Vite proxy uses a `bypass` rule that flushes for `text/event-stream` requests with extended timeouts. [CITED: github.com/vitejs/vite/discussions/10851]

```ts
// web/vite.config.ts
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,   // sets Origin header to target host
        secure: false,
      },
      '/sse': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        secure: false,
        // Long-lived SSE streams: prevent proxy timeout, prevent buffering
        configure: (proxy, _opts) => {
          proxy.on('proxyRes', (proxyRes) => {
            // Backend already sets X-Accel-Buffering: no; nothing to add here,
            // but we leave the hook for future tweaks (e.g. CORS in dev).
          })
        },
        proxyTimeout: 60 * 60 * 1000, // 1h
        timeout:      60 * 60 * 1000,
      },
    },
  },
  build: { outDir: 'dist', sourcemap: true },
})
```

Phase 1 has no `/sse` endpoint yet (Phase 4), but the proxy block is set up now so Phase 4 doesn't have to revisit `vite.config.ts`.

### Pattern 10: MQTT subscriber lifecycle (CHIRP-02)

```go
// internal/chirpstack/mqtt.go
package chirpstack

import (
    "context"
    "fmt"
    "log/slog"
    "time"
    mqtt "github.com/eclipse/paho.mqtt.golang"
)

type MQTTSubscriber struct {
    client mqtt.Client
    log    *slog.Logger
}

func NewMQTTSubscriber(brokerURL, user, pass, clientID string, log *slog.Logger) (*MQTTSubscriber, error) {
    opts := mqtt.NewClientOptions().
        AddBroker(brokerURL).
        SetClientID(clientID).
        SetUsername(user).
        SetPassword(pass).
        SetCleanSession(false).                            // resume QoS1 inflight
        SetAutoReconnect(true).
        SetConnectRetry(true).
        SetMaxReconnectInterval(30 * time.Second).
        SetKeepAlive(30 * time.Second).
        SetPingTimeout(10 * time.Second).
        SetOrderMatters(false)

    sub := &MQTTSubscriber{log: log}

    // Re-subscribe in OnConnect — single source of truth for subscriptions
    opts.OnConnect = func(c mqtt.Client) {
        log.Info("mqtt connected", "broker", brokerURL)
        // QoS 1: at-least-once. ChirpStack publishes uplinks at QoS 0 by default
        // but we subscribe at 1 so any retained / queued events are delivered.
        if t := c.Subscribe("application/+/device/+/event/up", 1, sub.handleUplink); t.Wait() && t.Error() != nil {
            log.Error("mqtt subscribe failed", "err", t.Error())
        }
    }
    opts.OnConnectionLost = func(_ mqtt.Client, err error) {
        log.Warn("mqtt connection lost", "err", err)
    }

    // Mandatory: prevent silent inflight-limit breaches on disconnect
    opts.SetDefaultPublishHandler(func(_ mqtt.Client, m mqtt.Message) {
        log.Warn("mqtt unrouted message", "topic", m.Topic(), "len", len(m.Payload()))
    })

    sub.client = mqtt.NewClient(opts)
    if t := sub.client.Connect(); t.Wait() && t.Error() != nil {
        return nil, fmt.Errorf("mqtt connect: %w", t.Error())
    }
    return sub, nil
}

func (s *MQTTSubscriber) handleUplink(_ mqtt.Client, m mqtt.Message) {
    // Phase 1: log only. Phase 2 normalizes + persists.
    s.log.Info("uplink received",
        "topic", m.Topic(),
        "payload_bytes", len(m.Payload()))
}

func (s *MQTTSubscriber) Shutdown(timeout time.Duration) {
    s.client.Disconnect(uint(timeout.Milliseconds())) // graceful: lets inflight publishes drain
    s.log.Info("mqtt subscriber stopped")
}
```

**Pin points from research [CITED]:**
- Subscribing in `OnConnect` is the canonical pattern for re-subscribing on reconnect [CITED: github.com/eclipse-paho/paho.mqtt.golang/issues/22 + autopaho docs].
- A `DefaultPublishHandler` is **mandatory** to prevent inflight-message-limit deadlocks per the upstream warning.
- `client.Disconnect(timeout_ms)` is the graceful shutdown call. Reusing a Client after Disconnect is unsafe — call `mqtt.NewClient` if reconnecting.

### Pattern 11: Caddyfile with config-driven TLS (D-20..D-23)

```caddy
# Caddyfile (mounted into bundled + external compose)
{
    email {$SHIFTER_TLS_EMAIL}
    # tls.mode=internal in dev → Caddy uses its local CA
    {$CADDY_GLOBAL_TLS_BLOCK}
}

{$SHIFTER_DOMAIN:localhost} {
    encode zstd gzip

    # Security headers (always on)
    header {
        Strict-Transport-Security "max-age=31536000; includeSubDomains"
        X-Content-Type-Options "nosniff"
        Referrer-Policy "strict-origin-when-cross-origin"
        X-Frame-Options "DENY"
        # CSP locked down — SPA assets only, no inline scripts (Vite doesn't emit any in prod)
        Content-Security-Policy "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; connect-src 'self'"
    }

    # TLS mode is selected via env-templated block injection.
    # For tls.mode=acme: leave the per-site `tls` block empty — Caddy uses ACME.
    # For tls.mode=byo:  `tls /etc/caddy/cert.pem /etc/caddy/key.pem`
    # For tls.mode=internal: `tls internal`
    {$CADDY_TLS_BLOCK}

    # SSE must NOT be buffered by the proxy
    @sse path /sse /sse/*
    handle @sse {
        reverse_proxy shifter:8080 {
            flush_interval -1
            transport http {
                read_buffer 0
                response_header_timeout 0
            }
        }
    }

    # All other API + SPA traffic
    reverse_proxy shifter:8080
}
```

The `{$VAR}` syntax is Caddy's standard env-var interpolation [CITED: caddyserver.com/docs/caddyfile/options]. Templating is done by the install kit / Compose by exporting the right `CADDY_TLS_BLOCK` value before `caddy run`:

```yaml
# fragment of docker-compose.bundled.yml
caddy:
  image: caddy:2.8       # PINNED — D-20, OPS-07 enforces no :latest
  environment:
    SHIFTER_DOMAIN: ${SHIFTER_DOMAIN:-localhost}
    SHIFTER_TLS_EMAIL: ${SHIFTER_TLS_EMAIL:-}
    CADDY_TLS_BLOCK: ${CADDY_TLS_BLOCK:-tls internal}    # default = internal
    CADDY_GLOBAL_TLS_BLOCK: ${CADDY_GLOBAL_TLS_BLOCK:-}
  volumes:
    - ./Caddyfile:/etc/caddy/Caddyfile:ro
    - caddy_data:/data
    - ./secrets/cert.pem:/etc/caddy/cert.pem:ro    # only used in byo mode
    - ./secrets/key.pem:/etc/caddy/key.pem:ro
  ports: ["80:80", "443:443"]
```

The install kit picks one of three preset env values:

| `tls.mode` | `CADDY_TLS_BLOCK` value |
|------------|--------------------------|
| `acme` | (empty — Caddy auto-issues from LE) |
| `byo` | `tls /etc/caddy/cert.pem /etc/caddy/key.pem` |
| `internal` | `tls internal` |

[CITED: caddyserver.com/docs/caddyfile/directives/tls, caddyserver.com/docs/automatic-https]

### Pattern 12: Compose secrets idiom (D-06; OPS-01)

```yaml
# docker-compose.bundled.yml — fragment
secrets:
  postgres_password:
    file: ./secrets/postgres_password.txt
  chirpstack_api_token:
    file: ./secrets/chirpstack_api_token.txt
  session_signing_key:
    file: ./secrets/session_signing_key.txt
  mqtt_password:
    file: ./secrets/mqtt_password.txt

services:
  shifter:
    image: shifter:0.1.0          # PINNED tag
    secrets:
      - postgres_password
      - chirpstack_api_token
      - session_signing_key
      - mqtt_password
    environment:
      SHIFTER_DB_PASSWORD_FILE: /run/secrets/postgres_password
      SHIFTER_CHIRPSTACK_API_TOKEN_FILE: /run/secrets/chirpstack_api_token
      SHIFTER_SESSION_KEY_FILE: /run/secrets/session_signing_key
      SHIFTER_MQTT_PASSWORD_FILE: /run/secrets/mqtt_password
      SHIFTER_DB_HOST: postgres
      SHIFTER_DB_USER: shifter
      # ... all non-secret config via env or config.yaml ...
    depends_on:
      postgres: { condition: service_healthy }
      mosquitto: { condition: service_started }
      chirpstack: { condition: service_started }

  postgres:
    image: timescale/timescaledb:2.26.0-pg16  # PINNED
    secrets:
      - postgres_password
    environment:
      POSTGRES_USER: shifter
      POSTGRES_DB: shifter
      POSTGRES_PASSWORD_FILE: /run/secrets/postgres_password
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U shifter -d shifter"]
      interval: 10s; timeout: 5s; retries: 5

volumes:
  postgres_data:
  caddy_data:
```

**Reading secrets in Go** — pattern is "the env var ending in `_FILE` points to a file path; read once at startup":

```go
// internal/config/secrets.go
func ReadSecret(name string) (string, error) {
    if v := os.Getenv(name); v != "" { return v, nil }                  // direct override (dev only)
    if path := os.Getenv(name + "_FILE"); path != "" {
        b, err := os.ReadFile(path)
        if err != nil { return "", fmt.Errorf("read %s_FILE: %w", name, err) }
        return strings.TrimRight(string(b), "\n"), nil
    }
    return "", fmt.Errorf("secret %s not set (neither %s nor %s_FILE)", name, name, name)
}
```

[CITED: docs.docker.com/compose/how-tos/use-secrets/]

**External-mode Compose** drops `chirpstack`, `mosquitto`, `redis`, `chirpstack-gateway-bridge` and replaces the env with operator-provided URLs:

```yaml
# docker-compose.external.yml — fragment
services:
  shifter:
    environment:
      SHIFTER_CHIRPSTACK_GRPC_URL: ${SHIFTER_CHIRPSTACK_GRPC_URL}    # required
      SHIFTER_MQTT_URL:            ${SHIFTER_MQTT_URL}                # required
      # secrets file references identical to bundled
```

### Pattern 13: AS923 region picker (INST-04; PITFALLS §8)

ChirpStack v4 region naming uses `(name, common_name)` where `common_name` is the LoRa Alliance enum and `name` is operator-assigned (e.g., `as923_2` to disambiguate sub-bands) [CITED: chirpstack.io/docs/chirpstack/configuration.html]. The MQTT topic prefix matches the `name` field (so `as923` and `as923_3` produce different topic trees in multi-region installs) [CITED: forum.chirpstack.io/t/multi-region-with-v4-and-docker-compose/15934].

```ts
// web/src/lib/regions.ts (Phase 1 hardcoded catalog; data file in Phase 7)
export type LoraRegion = {
  name: string                  // operator-side stable id
  display: string               // UI label
  common_name: string           // ChirpStack `common_name` enum value
  group: 'asia' | 'europe' | 'americas' | 'oceania' | 'india'
  default_for_country?: string  // ISO 3166-1 alpha-2
  note?: string
}

export const REGIONS: LoraRegion[] = [
  { name: 'as923',   display: 'AS923-1',                 common_name: 'AS923',   group: 'asia' },
  { name: 'as923_2', display: 'AS923-2 (Thailand)',       common_name: 'AS923_2', group: 'asia',
    default_for_country: 'TH', note: 'Required by Thai regulator NBTC.' },
  { name: 'as923_3', display: 'AS923-3',                 common_name: 'AS923_3', group: 'asia' },
  { name: 'as923_4', display: 'AS923-4',                 common_name: 'AS923_4', group: 'asia' },
  { name: 'eu868',   display: 'EU868 (Europe)',           common_name: 'EU868',   group: 'europe' },
  { name: 'us915_0', display: 'US915 sub-band 1 (ch 0–7)', common_name: 'US915',   group: 'americas' },
  { name: 'au915_0', display: 'AU915 sub-band 1',          common_name: 'AU915',   group: 'oceania' },
  { name: 'in865',   display: 'IN865 (India)',             common_name: 'IN865',   group: 'india' },
]
```

Phase 1 stores `(name, common_name, sub_band)` in `chirpstack_connection.default_region` and surfaces both fields when calling ChirpStack to create gateways (Phase 3). The wizard step pre-selects AS923-2 if the install address country is Thailand (per UI-SPEC step 3 description copy). For Phase 1 the country detection is operator-confirmed at step 4; if step 4 happens after step 3, the planner should flip step ordering OR offer a "based on your address" hint card after step 4 saves. The UI-SPEC currently has step 3 before step 4 — accept the small UX tradeoff (no country at step 3) and just default the dropdown to AS923-2 with a "Thailand" note as the canonical hint.

### Pattern 14: Test Connection two-channel probe (CHIRP-03)

```go
// internal/http/testconn.go — POST /api/settings/chirpstack/test
type TestConnRequest struct {
    GrpcURL  string `json:"grpc_url"`
    APIToken string `json:"api_token"`
    MqttURL  string `json:"mqtt_url"`
    MqttUser string `json:"mqtt_user,omitempty"`
    MqttPass string `json:"mqtt_pass,omitempty"`
}
type ChannelResult struct {
    Status   string  `json:"status"`               // "reachable" | "unreachable" | "skipped"
    LatencyMs *int   `json:"latency_ms,omitempty"`
    Detail   string  `json:"detail,omitempty"`     // version on success, error on failure
}
type TestConnResponse struct {
    GRPC ChannelResult `json:"grpc"`
    MQTT ChannelResult `json:"mqtt"`
}

func TestConnHandler(...) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
        defer cancel()
        var req TestConnRequest
        // ... decode + validate ...
        resp := TestConnResponse{}

        // Channel 1: gRPC version probe
        t0 := time.Now()
        conn, err := dialChirpstack(ctx, req.GrpcURL, req.APIToken)
        if err != nil {
            resp.GRPC = ChannelResult{Status:"unreachable", Detail: err.Error()}
            // UI-SPEC: skip MQTT if gRPC failed first
            resp.MQTT = ChannelResult{Status:"skipped", Detail:"gRPC failed first"}
            httputil.WriteJSON(w, 200, resp)
            return
        }
        defer conn.Close()
        version, err := chirpstack.ProbeVersion(ctx, conn)
        ms := int(time.Since(t0).Milliseconds())
        if err != nil {
            resp.GRPC = ChannelResult{Status:"unreachable", LatencyMs:&ms, Detail: err.Error()}
            resp.MQTT = ChannelResult{Status:"skipped", Detail:"gRPC failed first"}
            httputil.WriteJSON(w, 200, resp)
            return
        }
        resp.GRPC = ChannelResult{Status:"reachable", LatencyMs:&ms, Detail: "v"+version}

        // Channel 2: MQTT connect-only ping
        t1 := time.Now()
        if err := chirpstack.PingMQTT(ctx, req.MqttURL, req.MqttUser, req.MqttPass); err != nil {
            ms := int(time.Since(t1).Milliseconds())
            resp.MQTT = ChannelResult{Status:"unreachable", LatencyMs:&ms, Detail: err.Error()}
        } else {
            ms := int(time.Since(t1).Milliseconds())
            resp.MQTT = ChannelResult{Status:"reachable", LatencyMs:&ms, Detail: "Mosquitto reachable"}
        }
        httputil.WriteJSON(w, 200, resp)
    }
}
```

`PingMQTT` is "connect, optionally subscribe to a noop topic, disconnect" with a 5-second deadline.

### Pattern 15: `/health` and `/health/detailed` (INST-06; D-18, D-19)

```go
// internal/http/health.go
var startedAt = time.Now()

func Health(version string) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        httputil.WriteJSON(w, 200, map[string]any{
            "status":          "ok",                // "degraded" if a startup-time check failed
            "version":         version,
            "uptime_seconds":  int(time.Since(startedAt).Seconds()),
        })
    }
}

// /health/detailed — REQUIRES admin auth via the chi route group
func HealthDetailed(pool *pgxpool.Pool, csClient ChirpStackPinger, mqttClient MQTTPinger) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Phase 1 minimal version per D-19: DB ping + version + uptime.
        // CS+MQTT+disk+last-uplink-age expand in Phase 6.
        dbOK := pool.Ping(r.Context()) == nil
        httputil.WriteJSON(w, 200, map[string]any{
            "status":  ternary(dbOK, "ok", "degraded"),
            "checks":  map[string]any{"db": dbOK},
            "version": version.Version(),
        })
    }
}
```

The minimal Phase 1 detailed-health is intentionally narrow per D-19. Phase 6 will fold in `chirpstack_grpc`, `chirpstack_mqtt`, `disk_free`, `last_uplink_age` once those signals exist (Phase 2 introduces the uplink-age column; Phase 6 surfaces backup status).

### Pattern 16: Authorization — `can(user, action, resource)` (PITFALLS §14)

```go
// internal/auth/authz.go
type Action string
const (
    ActionDeviceCreate    Action = "device.create"
    ActionDeviceUpdate    Action = "device.update"
    ActionDeviceDelete    Action = "device.delete"
    ActionUserManage      Action = "user.manage"
    ActionConnectionEdit  Action = "connection.edit"
    ActionAccountSelfEdit Action = "account.self.edit"
    ActionHealthDetailed  Action = "health.detailed"
    // ...phase 2+ adds more
)

type Role string
const (
    RoleAdmin  Role = "admin"
    RoleViewer Role = "viewer"
)

// roleBundles is the only thing that needs to grow when new roles ship in v2.
var roleBundles = map[Role]map[Action]bool{
    RoleAdmin: { /* every action true */ },
    RoleViewer: {
        ActionAccountSelfEdit: true,
        // viewer has no write actions, no health-detailed
    },
}

func Can(user *User, action Action, resource any) bool {
    if user == nil { return false }
    bundle, ok := roleBundles[user.Role]
    if !ok { return false }
    return bundle[action]
}

// chi middleware factory
func RequireAction(action Action) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            u, _ := UserFromContext(r.Context())
            if !Can(u, action, nil) {
                http.Error(w, "forbidden", http.StatusForbidden)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

**Phase 1 enforces only `RoleAdmin` and `RoleViewer`** with one shared admin (the bootstrap user) and no viewers yet. The pattern is set up so Phase 6 USER-04 can add multi-user role assignments without refactoring call sites. **Every** route under `/api/*` other than `/api/auth/login` and `/api/install/*` must run through `RequireAction(...)`.

### Pattern 17: Rate-limited login (AUTH-04)

```go
// internal/auth/ratelimit.go
import (
    "sync"
    "time"
    "golang.org/x/time/rate"
)

type LoginLimiter struct {
    perIP       *limiterMap   // 5 attempts / 5 minutes per IP
    perUsername *limiterMap   // 5 attempts / 5 minutes per username
}

type limiterMap struct {
    sync.Mutex
    m   map[string]*entry
}
type entry struct {
    lim     *rate.Limiter
    lastSeen time.Time
}

func NewLoginLimiter() *LoginLimiter {
    ll := &LoginLimiter{
        perIP:       &limiterMap{m: map[string]*entry{}},
        perUsername: &limiterMap{m: map[string]*entry{}},
    }
    go ll.cleanup()
    return ll
}

// 5 attempts in any 5-minute window: refill rate = 1/60s, burst = 5
func (lm *limiterMap) get(key string) *rate.Limiter {
    lm.Lock(); defer lm.Unlock()
    e, ok := lm.m[key]
    if !ok {
        e = &entry{lim: rate.NewLimiter(rate.Every(time.Minute), 5)}
        lm.m[key] = e
    }
    e.lastSeen = time.Now()
    return e.lim
}

func (ll *LoginLimiter) Allow(ip, username string) (allowedIP, allowedUser bool) {
    return ll.perIP.get(ip).Allow(), ll.perUsername.get(strings.ToLower(username)).Allow()
}

func (ll *LoginLimiter) cleanup() {
    t := time.NewTicker(15 * time.Minute); defer t.Stop()
    for range t.C {
        for _, lm := range []*limiterMap{ll.perIP, ll.perUsername} {
            lm.Lock()
            for k, e := range lm.m {
                if time.Since(e.lastSeen) > time.Hour { delete(lm.m, k) }
            }
            lm.Unlock()
        }
    }
}
```

Login handler refuses *both* failures: if either IP or username is over-budget, return 429 with `Retry-After: 300`. UI-SPEC's copy "Too many failed attempts. Try again in 5 minutes." [CITED: dev.to/ohugonnot/rate-limiter-in-go-per-ip-token-bucket-with-golangorgxtimerate-5ff8]

### Anti-Patterns to Avoid

- **`postgresstore` with pgx/v5 stdlib adapter** — use `pgxstore` (purpose-built for `*pgxpool.Pool`).
- **Subscribing to MQTT before `OnConnect` fires** — silent failure on reconnect.
- **`pgxstore.NewWithCleanupInterval(pool, 0)` to "disable cleanup"** — leaks expired sessions in the table; only acceptable in tests.
- **Embedding without `all:` prefix** — silently drops dotfile assets.
- **`Cookie.Domain`** — leave unset; sessions stay scoped to the host that issued them.
- **One `docker-compose.yml` that hardcodes bundled services** — diverges per customer fast (PITFALLS §16). Two compose files share the same backend image.
- **Using ChirpStack's REST gateway** — v4 has none; gRPC + MQTT only.
- **`Disable frame-counter validation` as a default** — replay-attack class vulnerability (PITFALLS §13).
- **JWT in localStorage** — XSS-exfiltratable. We're using server-side sessions in Postgres + httpOnly cookie.
- **`if user.is_admin` scattered** — use `Can(user, action, resource)` exclusively.
- **`bg-blue-700` or `bg-[#1E40AF]` in JSX** — UI-SPEC requires `bg-primary` etc.; CSS variable layer is the single source of truth.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Sessions / cookies | Custom cookie + DB-backed lookup | `alexedwards/scs/v2` + `pgxstore` | Battle-tested cookie security, idle-vs-absolute timeout, automatic cleanup, token rotation |
| Password hashing | bcrypt/scrypt fallback combo | `golang.org/x/crypto/argon2` IDKey + PHC encoding | OWASP-current; constant-time verify; future param migration |
| SQL migrations | bash + psql | `golang-migrate/v4` library + `iofs` source | Versioned, idempotent, dirty-state recovery |
| Typed SQL access | hand-written `Scan` rows | `sqlc` v1.31+ | Pure typing, TimescaleDB-aware (`time_bucket` etc. survive) |
| HTTP routing | stdlib `http.ServeMux` | `chi/v5` | Composable middleware, route groups with auth scoping |
| MQTT client | net.Dial + protocol parser | `paho.mqtt.golang` v1.5+ | Auto-reconnect, ordered/unordered delivery, QoS handling, OnConnect re-subscribe |
| ChirpStack gRPC types | grpcurl + manual proto compile | `chirpstack/api/go/v4` | Generated stubs match server; bumps tracked with Compose tag |
| CLI surface | flag.Parse + sub-switch | `cobra` | Subcommands, help, completion, version flag |
| Config layering | manual env reading | `viper` | YAML + env precedence, file watching (not used in v1) |
| Structured logging | log + json marshal | stdlib `log/slog` | Built-in JSON handler, level toggle, context propagation |
| Forms (frontend) | controlled inputs + state | `react-hook-form` + `zod` + `@hookform/resolvers` | shadcn's official recipe, accessible, performant |
| UI components | hand-built buttons/dialogs | shadcn `add` CLI | Components copied into repo (auditable, customizable) |
| TLS certs | manual openssl | Caddy 2 | Auto-ACME, ZeroSSL fallback, internal CA mode |
| Session storage table schema | custom | shadcn-quoted SCS schema | `(token TEXT PK, data BYTEA, expiry TIMESTAMPTZ)` is the canonical shape |
| Argon2id PHC parsing | regex | `strings.Split` on `$` per PHC spec | Boring; the format is fixed |
| Rate limiter | custom map + timestamps | `golang.org/x/time/rate` token bucket | Thread-safe; well-tested |
| First-run gate | flag file in volume | `SELECT EXISTS(SELECT 1 FROM "user")` | DB is authoritative; no flag drift across restarts |

**Key insight:** Phase 1 is dense with library wiring. Every "Don't hand-roll" above is a place where the natural temptation is to "just write it ourselves" — and in every case the boring library is the durable choice. The code in `internal/auth/argon2id.go` is 60 lines. Custom-implementing it would be 60 lines plus weeks of audit anxiety.

---

## Common Pitfalls

### Pitfall 1: ChirpStack v3 connection accepted (PITFALLS §7)

**What goes wrong:** Operator runs ChirpStack v3 in production; install wizard step 2 succeeds because we don't probe; first MQTT subscription returns malformed events; debugging takes a week.

**Why it happens:** v3 and v4 share gRPC port and superficial proto names. Without an explicit version probe, the gRPC dial succeeds.

**How to avoid:** Call `InternalService.GetVersion(Empty)` at wizard step 2 submit. Reject with destructive banner per UI-SPEC copy ("We detected ChirpStack v3 at this URL. Shifter requires v4 or newer."). Repeat the same probe at `shifter serve` startup; refuse to subscribe MQTT if probe fails.

**Warning signs:** Test fixtures use `chirpstack:3` image; absence of `GetVersion` call in chirpstack package.

### Pitfall 2: AS923 sub-band defaulted to "AS923" instead of "AS923_2" for Thailand (PITFALLS §8)

**What goes wrong:** Operator picks AS923 (sub-band 1) by default; gateway is configured AS923-2; first device join attempts go nowhere; "RF debugging" actually a 30-second config issue.

**How to avoid:** Wizard step 3 dropdown shows "Thailand (AS923-2)" as the **first option** with a "Thailand pre-selected" hint label. Other AS923 sub-bands and global plans are alternatives. UI-SPEC §Wizard Step 3 has the canonical copy.

### Pitfall 3: Sessions table grows unbounded if `pgxstore.NewWithCleanupInterval(pool, 0)` is used

**Why:** Disabling cleanup leaves expired session rows. After months, table grows; queries slow.

**Prevention:** Use `pgxstore.New(pool)` (5-min default cleanup) in production. Tests can disable. [CITED: pgxstore docs]

### Pitfall 4: SPA fallback serves `index.html` for `/api/*` 404s

**Why:** Order matters in the chi router. If the SPA handler is mounted at `/` *before* the `/api/*` routes are mounted, every API 404 returns the HTML shell.

**Prevention:** Mount `/api` and `/sse` route groups *before* the SPA handler. The SPA handler is a fallthrough at the very end:

```go
r.Route("/api", apiRoutes)
r.Route("/sse", sseRoutes)        // Phase 4
r.Get("/health", healthHandler)
r.Get("/health/detailed", auth.RequireAction(ActionHealthDetailed)(detailedHealthHandler))
r.Handle("/*", SPAHandler())      // last
```

### Pitfall 5: `Cookie.Secure=true` in dev breaks login

**Why:** Browsers reject Secure cookies on plain HTTP. The dev proxy is `http://localhost:5173` → Go binary on `:8080`, all plain HTTP. If `Cookie.Secure` is hard-coded `true`, login appears successful (cookie sent in response) but the browser silently drops it; subsequent requests are unauthenticated.

**Prevention:** D-23 toggle. `Cookie.Secure = !devMode`, where `devMode := os.Getenv("SHIFTER_ENV") == "dev"`. Also `cookie.SameSite = SameSiteLaxMode` (not Strict — breaks the SPA navigation flows after login redirect).

### Pitfall 6: Migrations dirty after a failed startup

**Why:** golang-migrate marks a version "dirty" if the migration aborts. On the next `serve`, `m.Up()` errors with `Dirty database version N. Fix and force version`.

**Prevention:** `shifter migrate force <prev>` recovery subcommand documented in README. Better: every up migration is wrapped in a single `BEGIN/COMMIT` transaction so a syntax error rolls back cleanly. Caveat: some PG DDL statements (e.g., `CREATE INDEX CONCURRENTLY`) cannot run in a transaction — for those, split them into their own migration with the body marked `-- migrate:no-transaction` (golang-migrate convention). Phase 1 has no concurrent-index migrations, so this is forward-looking.

### Pitfall 7: Vite proxy buffers SSE responses

**Why:** http-proxy (under Vite's hood) buffers responses by default; SSE breaks. [CITED: github.com/vitejs/vite/discussions/10851]

**Prevention:** (a) Backend always sets `X-Accel-Buffering: no` and `Cache-Control: no-cache` on every SSE response. (b) Vite proxy block sets `proxyTimeout`/`timeout` to `60*60*1000`. Phase 1 has no SSE endpoint yet, but the proxy is configured now.

### Pitfall 8: Compose secrets file CRLF line endings on Windows

**Why:** Windows-edited secret files contain `\r\n`. The Go `os.ReadFile` returns the raw bytes; the trailing `\r` becomes part of the password and authentication fails opaquely.

**Prevention:** `strings.TrimRight(string(b), "\n")` is insufficient. Use `strings.TrimRight(string(b), "\r\n")` or `strings.TrimSpace`. Add a unit test that asserts a CRLF-formatted secret file parses to the right value.

### Pitfall 9: `slog.NewJSONHandler` writes pretty-printed JSON (multi-line)

**Why:** Operator's `json-file` log driver expects one JSON object per line. Default `slog` JSON handler is one-line — confirm in test.

**Prevention:** Pin the handler config: `slog.HandlerOptions{Level: slog.LevelInfo, AddSource: false}` and emit one event per line. Phase 1 verification includes a smoke test that `shifter serve` for 1 second produces ≥1 valid newline-delimited JSON line.

### Pitfall 10: First-run middleware runs on every request and adds DB latency

**Why:** Naïve implementation does `SELECT EXISTS(...)` on every HTTP request.

**Prevention:** Cache the "admin exists" answer in-memory after the first positive result. Use `sync.Once` or an atomic bool. The cache is only set on success (admin found) — never invalidated, because adding admins later doesn't remove the bootstrap one.

### Pitfall 11: `shifter healthcheck` fails because the binary in the container doesn't have curl

**Why:** Distroless or `FROM scratch` images don't include `curl`. Without `shifter healthcheck` we can't write a Docker `HEALTHCHECK` line.

**Prevention:** Per D-15, ship `shifter healthcheck` as a sub-command that does a localhost HTTP GET to `/health`. The Dockerfile sets `HEALTHCHECK CMD ["/usr/local/bin/shifter", "healthcheck"]`.

### Pitfall 12: shadcn `init` overwrites the custom OKLCH palette in `index.css`

**Why:** Re-running `shadcn init` (e.g., during onboarding) regenerates `src/index.css` with the slate preset — overwriting the navy primary tokens from UI-SPEC.

**Prevention:** Two strategies: (a) document "do not re-run `shadcn init` after the first time — to add components use `shadcn add <name>`," (b) keep custom tokens in a separate `src/theme.css` imported after `index.css`, with `@layer` rules that override the shadcn defaults. Strategy (b) is more robust.

---

## Code Examples

### Wiring `serve` (the big one)

```go
// internal/cli/serve.go
package cli

import (
    "context"
    "errors"
    "fmt"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/spf13/cobra"
    "github.com/<org>/shifter/internal/auth"
    "github.com/<org>/shifter/internal/chirpstack"
    "github.com/<org>/shifter/internal/config"
    "github.com/<org>/shifter/internal/db"
    httpapi "github.com/<org>/shifter/internal/http"
    "github.com/<org>/shifter/internal/logging"
)

func newServeCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "serve",
        Short: "Run migrations then start the HTTP server",
        RunE: func(cmd *cobra.Command, _ []string) error {
            cfg, err := config.Load(); if err != nil { return err }
            log := logging.New(cfg.LogLevel)
            ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
            defer cancel()

            // 1. DB pool
            pool, err := pgxpool.New(ctx, cfg.DBURL); if err != nil { return err }
            defer pool.Close()
            if err := pool.Ping(ctx); err != nil { return fmt.Errorf("db ping: %w", err) }

            // 2. D-13: auto-migrate before listening
            if err := db.RunMigrations(ctx, pool, log); err != nil { return err }

            // 3. ChirpStack adapters (only after wizard completes; Phase 1 starts them lazily)
            csConn, csErr := chirpstack.Dial(ctx, cfg.ChirpStack)
            if csErr != nil { log.Warn("chirpstack not reachable on boot — degraded mode", "err", csErr) }
            // INST-05: probe on connect; refuse v3
            if csConn != nil {
                if _, err := chirpstack.ProbeVersion(ctx, csConn); errors.Is(err, chirpstack.ErrChirpStackV3OrUnknown) {
                    return fmt.Errorf("refusing to start: ChirpStack v3 detected at %s — Shifter requires v4+", cfg.ChirpStack.GrpcURL)
                }
            }
            mqttSub, mqttErr := chirpstack.NewMQTTSubscriber(cfg.MQTT.URL, cfg.MQTT.User, cfg.MQTT.Pass, "shifter-"+cfg.Version, log)
            if mqttErr != nil { log.Warn("mqtt not reachable on boot — degraded mode", "err", mqttErr) }
            if mqttSub != nil { defer mqttSub.Shutdown(5 * time.Second) }

            // 4. HTTP
            sm := auth.NewSessionManager(pool, cfg.Env == "dev")
            limiter := auth.NewLoginLimiter()
            r := httpapi.NewRouter(httpapi.Deps{
                Pool: pool, SessionMgr: sm, LoginLimiter: limiter, Log: log,
                ChirpStackClient: chirpstack.NewClient(csConn),
                Version: cfg.Version,
            })

            srv := &http.Server{
                Addr:              ":" + cfg.HTTPPort,
                Handler:           sm.LoadAndSave(r),
                ReadHeaderTimeout: 10 * time.Second,
                IdleTimeout:       2 * time.Minute,
            }
            go func() {
                log.Info("listening", "addr", srv.Addr)
                if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
                    log.Error("listen failed", "err", err); cancel()
                }
            }()

            <-ctx.Done()
            log.Info("shutting down")
            shutCtx, shutCancel := context.WithTimeout(context.Background(), 15*time.Second)
            defer shutCancel()
            return srv.Shutdown(shutCtx)
        },
    }
}
```

### shadcn theme override (UI-SPEC §Color)

```css
/* web/src/theme.css — imported AFTER shadcn's index.css generation */
@layer base {
  :root {
    /* Override slate base with the navy primary system */
    --background:        oklch(1 0 0);
    --foreground:        oklch(0.18 0.025 240);
    --card:              oklch(0.985 0.005 240);
    --card-foreground:   oklch(0.18 0.025 240);
    --popover:           oklch(1 0 0);
    --popover-foreground:oklch(0.18 0.025 240);
    --primary:           oklch(0.42 0.17 255);     /* navy 800 #1E40AF */
    --primary-foreground:oklch(0.98 0 0);
    --secondary:         oklch(0.96 0.005 240);
    --secondary-foreground:oklch(0.18 0.025 240);
    --muted:             oklch(0.96 0.005 240);
    --muted-foreground:  oklch(0.45 0.020 240);
    --accent:            oklch(0.96 0.005 240);
    --accent-foreground: oklch(0.18 0.025 240);
    --destructive:       oklch(0.55 0.22 27);      /* red 600 */
    --destructive-foreground: oklch(0.98 0 0);
    --success:           oklch(0.62 0.18 145);    /* custom token */
    --success-foreground:oklch(0.98 0 0);
    --warning:           oklch(0.75 0.16 70);     /* custom token */
    --warning-foreground:oklch(0.18 0.025 240);
    --info:              oklch(0.55 0.15 240);    /* custom token */
    --info-foreground:   oklch(0.98 0 0);
    --border:            oklch(0.92 0.008 240);
    --input:             oklch(0.92 0.008 240);
    --ring:              oklch(0.42 0.17 255 / 0.4);
    --radius:            0.5rem;
  }
  .dark {
    --background:        oklch(0.18 0.025 240);
    --foreground:        oklch(0.97 0.005 240);
    --card:              oklch(0.22 0.025 240);
    --card-foreground:   oklch(0.97 0.005 240);
    --popover:           oklch(0.22 0.025 240);
    --popover-foreground:oklch(0.97 0.005 240);
    --primary:           oklch(0.65 0.18 250);     /* lifted blue 500 */
    --primary-foreground:oklch(0.18 0.025 240);
    --secondary:         oklch(0.30 0.020 240);
    --secondary-foreground: oklch(0.97 0.005 240);
    --muted:             oklch(0.30 0.020 240);
    --muted-foreground:  oklch(0.65 0.015 240);
    --accent:            oklch(0.30 0.020 240);
    --accent-foreground: oklch(0.97 0.005 240);
    --destructive:       oklch(0.62 0.22 27);
    --destructive-foreground: oklch(0.97 0.005 240);
    --success:           oklch(0.68 0.18 145);
    --success-foreground:oklch(0.18 0.025 240);
    --warning:           oklch(0.78 0.16 70);
    --warning-foreground:oklch(0.18 0.025 240);
    --info:              oklch(0.65 0.15 235);
    --info-foreground:   oklch(0.18 0.025 240);
    --border:            oklch(0.30 0.020 240);
    --input:             oklch(0.30 0.020 240);
    --ring:              oklch(0.65 0.18 250 / 0.5);
  }
}

/* Custom utility tokens (not in shadcn defaults) */
@theme inline {
  --color-success:           var(--success);
  --color-success-foreground:var(--success-foreground);
  --color-warning:           var(--warning);
  --color-warning-foreground:var(--warning-foreground);
  --color-info:              var(--info);
  --color-info-foreground:   var(--info-foreground);
}
```

[CITED: ui.shadcn.com/docs/theming, ui.shadcn.com/docs/tailwind-v4]

### React Router v7 protected route pattern

```tsx
// web/src/App.tsx (sketch)
import { createBrowserRouter, RouterProvider, redirect } from 'react-router-dom'
import { fetchSessionUser } from './lib/auth'
import { fetchInstallState } from './lib/install'

const router = createBrowserRouter([
  {
    path: '/install',
    lazy: () => import('./routes/install'),
  },
  {
    path: '/login',
    lazy: () => import('./routes/login'),
  },
  {
    path: '/',
    loader: async ({ request }) => {
      // First-run gate redirect (client-side mirror of server middleware)
      const install = await fetchInstallState()
      if (!install.completed) throw redirect('/install')
      const user = await fetchSessionUser()
      if (!user) {
        const url = new URL(request.url)
        throw redirect(`/login?next=${encodeURIComponent(url.pathname)}`)
      }
      return { user }
    },
    children: [
      { path: 'settings', lazy: () => import('./routes/settings') },
      // Phase 2+ routes added here
    ],
  },
])

export default function App() {
  return <RouterProvider router={router} />
}
```

[CITED: blog.logrocket.com/authentication-react-router-v7/, dev.to/ra1nbow1/building-reliable-protected-routes-with-react-router-v7-1ka0]

### Stepped wizard (sketch using react-hook-form + zod)

```tsx
// web/src/routes/install.tsx (sketch)
import { z } from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery } from '@tanstack/react-query'

const Step1Schema = z.object({
  email: z.string().email(),
  name: z.string().min(1).max(120),
  password: z.string().min(12, 'At least 12 characters'),
  confirm: z.string(),
}).refine(d => d.password === d.confirm, { path: ['confirm'], message: "Passwords don't match." })

export default function InstallWizard() {
  const { data: state } = useQuery({ queryKey: ['install-state'], queryFn: fetchInstallState })
  const stepIdx = state?.current_step ?? 1

  switch (stepIdx) {
    case 1: return <AdminStep prev={state?.step1_admin} />
    case 2: return <ChirpStackStep prev={state?.step2_chirpstack} />
    case 3: return <RegionStep prev={state?.step3_region} />
    case 4: return <IdentityStep prev={state?.step4_identity} />
    case 5: return <ReviewStep state={state!} />
  }
}

function AdminStep({ prev }: { prev?: any }) {
  const form = useForm<z.infer<typeof Step1Schema>>({
    resolver: zodResolver(Step1Schema),
    defaultValues: { email: prev?.email ?? '', name: prev?.name ?? '', password: '', confirm: '' },
  })
  const save = useMutation({
    mutationFn: (vals: any) => postInstallStep(1, vals),
    onSuccess: () => navigateToStep(2),
  })
  // ... shadcn <Form> wrapper renders the fields ...
}
```

---

## State of the Art

| Old Approach | Current (Phase 1) | When Changed | Impact |
|--------------|-------------------|--------------|--------|
| `lib/pq` Postgres driver | `jackc/pgx/v5` + `pgxpool` | 2021 (lib/pq maintenance mode) | Native binary protocol; better perf; required by sqlc 1.18+ |
| `gorilla/mux` HTTP router | `chi/v5` | 2022 (gorilla archived briefly) | Cleaner middleware composition; route groups |
| `database/sql` ORMs | `sqlc` (codegen) + raw SQL | 2024 ecosystem shift | TimescaleDB-aware queries survive; performance preserved |
| `migrate` CLI in container | `migrate` Go library + `iofs` embed | 2022 (iofs added) | Migrations bake into binary; no separate container |
| HSL Tailwind palette | OKLCH via Tailwind v4 | March 2025 (shadcn+Tailwind v4 default) | Better dark-mode contrast; perceptual uniformity |
| shadcn HSL CSS variables | shadcn OKLCH + `@theme inline` | March 2025 | Same visual system, modern color space |
| Cookie-based JWT | Server-side sessions (SCS) | 2024 (alexedwards's auth guides for 2026) | Easier revocation; no XSS-via-localStorage exposure |
| `gofpdf` for PDF | `maroto/v2` | 2021 (gofpdf archived) | Active maintenance; not Phase 1 but pinned for Phase 5 |
| MQTT v3 paho v1.4 | paho.mqtt.golang v1.5+ (or autopaho for v5) | 2024 paho 1.5 release | Better reconnect ergonomics; ChirpStack still v3.1.1 |
| Phoenix-style WebSocket | SSE via pg_notify | 2025 dashboard consensus | One-way push fits; no broker-state in browser |

**Deprecated/outdated:**
- `gofpdf` — archived 2021. Listed in CLAUDE.md "do not use."
- `lib/pq` — maintenance mode 2021. Use pgx.
- ChirpStack `as_external/api/...` proto paths — v3 only. v4 uses `api/...`.
- Next.js for an internal dashboard — see PROJECT.md and STACK.md rationales.
- shadcn HSL preset — superseded by OKLCH in March 2025 update.

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | ChirpStack v3 returns `codes.Unimplemented` for `InternalService.GetVersion` | Pattern 4 (v3 Rejection) | If v3 returns a different code or transport-level error, the planner's "v3 rejection" task adds a one-line code-path branch. No structural impact. |
| A2 | ChirpStack v4's `InternalService.GetVersion` is callable with the operator's API token | Pattern 4 | If `InternalService` is gateway-bridge-only (it is named `Internal`), the probe needs an alternative: call `TenantService.List` or `DeviceProfileService.List` with the API token instead. This is a minor implementation pivot; the planner should write the task as "v4-detection probe — preferred = `InternalService.GetVersion`, fallback = `TenantService.List` with empty filter." |
| A3 | ChirpStack v3's `as_external/api/...` proto namespace is wire-incompatible with the v4 client (i.e. v4 client doesn't accidentally succeed against v3) | Pattern 4 | If the v4 client gets a response on v3, the version check is the only safety net. Implementation must error if the response Version field is missing or has v3 prefix. |
| A4 | Setting `X-Accel-Buffering: no` on SSE responses is respected by Vite's http-proxy | Pattern 9 | If proxy still buffers, the `bypass`-with-overrideOptions workaround from the GitHub discussion is the fallback. |
| A5 | `pgxstore.New(pool)` cleans up sessions every 5 minutes by default (matches `postgresstore`) | Pattern 6 | pgxstore docs show same `NewWithCleanupInterval` API; assumed default matches. Worst case: tune in code. |
| A6 | OWASP `m=19456 KiB, t=2, p=1` is the right tradeoff for Shifter's tens-of-logins-per-day load on a 4-vCPU VPS | Pattern 5 | Wrong direction = login takes 1+ second under load (acceptable). Tunable via config later. |
| A7 | The `chirpstack/api/go/v4` Go module exposes `InternalService` (some SDKs strip "Internal" prefixed services) | Pattern 4 | If stripped, plan accordingly — likely `chirpstack-rest-api/api` exposes a Settings/Version endpoint that's a layer over the gRPC. |
| A8 | Caddy 2.8 honors `flush_interval -1` correctly for SSE (no buffering) | Pattern 11 | Standard Caddy reverse-proxy SSE pattern; verified in Caddy docs. Low risk. |
| A9 | The bootstrap admin user's first session login is just a regular login (no "force change" gate, since they set their own password in the wizard) | D-09 reframing of AUTH-03 | Rethink only if security review wants a defense-in-depth re-prompt. Not material. |

If any A1-A3 prove wrong at implementation time, the affected task is a one-cycle research+adjust, not a planning re-do. The planner should mark these tasks as "verify probe RPC at task start; if Unimplemented variant differs, document and adjust" rather than locking in code.

---

## Open Questions (RESOLVED)

1. **Should `install_state` step values be encrypted at rest?**
   - What we know: SQLite WAL/PG WAL contains the wizard draft, which includes the password hash (already Argon2id'd) and ChirpStack API token (stored by file ref, not raw value).
   - What's unclear: Whether step2_chirpstack JSONB references the *path* to the secret file (safe — install kit pre-populates `/run/secrets/chirpstack_api_token`) or contains the raw token (concerning — would land in WAL/backups).
   - **RESOLVED:** Recommendation: **the JSONB stores secret REFs, never raw values.** A reference is `{"api_token_ref": "/run/secrets/chirpstack_api_token"}` and the install kit ensures the file exists with the operator-provided value before the wizard starts. If the operator types a token in the UI, the backend writes it to `/run/secrets/...` (mode 0600, owned by the shifter user), and only stores the path in JSONB.

2. **How does the wizard handle changing ChirpStack URLs after Phase 1 ships?**
   - Phase 1 supports SETT-03 (admin can update ChirpStack credentials at any time) per UI-SPEC §Settings. The Edit Connection dialog re-runs the v3 probe on save (UI-SPEC copy: "Save and test" button label).
   - Open: should re-test happen *before* the DB write or *after*? If after, the operator can save a broken config and lock themselves out (no v3 probe before save = saving a v3 URL succeeds, then MQTT subscriber fails). If before, the dialog blocks on the probe and the UX feels heavier.
   - **RESOLVED:** Recommendation: **probe before save**. The dialog uses the same Test Connection two-channel probe (gRPC version + MQTT connect-only); if either fails, save is rejected with the failure message. The operator must Test Connection (in-place panel) before Save is enabled. This is a UI affordance worth specifying in the plan.

3. **Is the install kit a separate repo, or part of this monorepo?**
   - PROJECT.md says "install must be a single, scripted operation we run for each customer." That's an artifact (compose files + secrets template + install.sh + version pin file).
   - **RESOLVED:** Recommendation: Phase 1 ships the install kit *in this repo* under `install/` (or similar). The kit is `install/bundled/` (with docker-compose + secrets templates) and `install/external/` (similar). A top-level `install.sh` is a thin wrapper that copies the right Compose file + reminds the operator to populate the secrets directory.

4. **Should `/health/detailed` ping ChirpStack on every request?**
   - Phase 1 minimal version (D-19) only checks DB. That's fine for v1.
   - For Phase 6's full version, a per-request ChirpStack gRPC ping might be slow (gRPC dial cost). **RESOLVED:** Recommendation for the planner now: structure the detailed-health to read from a 30s-cached probe result, not a live ping. This is forward-looking; Phase 1 doesn't have to implement it.

---

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go 1.24+ | Backend build | ✓ | 1.26.0 darwin/arm64 | — |
| Node.js 18+ | Frontend build | ✓ | 22.11.0 | — |
| pnpm 9+ | Frontend deps | ✓ | 10.33.2 | — |
| Docker (Compose v2) | Local dev + deploy | ✓ | 28.3.2 | — |
| psql (client) | DB ad-hoc inspection | ✓ | 14.17 | Use container `docker compose exec postgres psql` |
| `just` | Build orchestrator (D-02) | ✗ | — | `brew install just` (one-shot install in README) |
| `caddy` (CLI on host) | TLS local probing | ✗ | — | Bundled in Compose; only needed standalone for cert ad-hoc — NOT a phase-blocker |
| `air` | Dev hot reload (D-04) | ✗ | — | `go install github.com/air-verse/air@latest` |
| `sqlc` | SQL → Go codegen | ✗ | — | `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest` |
| `migrate` CLI | Ad-hoc migration commands | ✗ | — | Library-mode migration is built into `shifter migrate`; standalone CLI is optional |
| `golangci-lint` | Lint | ✗ | — | `brew install golangci-lint` |
| `mockgen` | Test mocks for CS/MQTT | ✗ | — | `go install go.uber.org/mock/mockgen@latest` |
| `playwright` | E2E tests (Phase 6) | ✗ | — | `pnpm add -D playwright` (deferred) |

**Missing dependencies with no fallback:** None — every missing tool has a `go install` or `brew install` one-liner.

**Missing dependencies with fallback:** The full list above (`just`, `air`, `sqlc`, etc.). The planner should include a "tooling install" task as the first task in Wave 0 (or include it in the Justfile bootstrap recipe `just bootstrap`).

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Backend framework | Go stdlib `testing` + `testify/assert` (recommended for ergonomics; CLAUDE.md doesn't pin) |
| Backend integration | `testcontainers-go` for Postgres+TimescaleDB and Mosquitto. ChirpStack tests use a recorded-fixtures gRPC mock (mockgen generates the interface). |
| Frontend framework | `vitest` (Vite-native) for unit; `@testing-library/react` for component; `playwright` for E2E (deferred to Phase 6) |
| Backend config file | `go.mod` (no separate test config); migrations applied per test via `db.RunMigrations` against testcontainer pool |
| Frontend config file | `vitest.config.ts` (extends `vite.config.ts`) |
| Quick backend run | `go test ./internal/... -short -race` |
| Quick frontend run | `pnpm --dir web test --run` |
| Full backend suite | `go test ./... -race -count=1` (incl. testcontainer-backed integration) |
| Full frontend suite | `pnpm --dir web test --run && pnpm --dir web build` (build is a smoke gate) |
| Phase gate | Full backend + frontend + `just lint` green before `/gsd-verify-work` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| AUTH-01 | Login with email+password produces a session cookie | integration | `go test ./internal/auth -run TestLogin_Success -race` | ❌ Wave 0 |
| AUTH-01 | Login with bad password is rejected (constant-time) | unit + integration | `go test ./internal/auth -run TestVerify_BadPassword_ConstantTime` | ❌ Wave 0 |
| AUTH-02 | Session persists across two HTTP requests with the same cookie | integration | `go test ./internal/http -run TestSessionPersistence` | ❌ Wave 0 |
| AUTH-02 | Session expires after `IdleTimeout` of inactivity | integration | `go test ./internal/auth -run TestSession_IdleTimeout` (uses `time.Now()` injection) | ❌ Wave 0 |
| AUTH-03 | Bootstrap admin (created via wizard) does NOT see force-change-password gate on first login | integration | `go test ./internal/auth -run TestWizardAdmin_NoForceChange` | ❌ Wave 0 |
| AUTH-04 | 6th failed login from same IP within 1m returns 429 | integration | `go test ./internal/auth -run TestLogin_RateLimit_PerIP` | ❌ Wave 0 |
| AUTH-04 | 6th failed login for same username from different IPs returns 429 | integration | `go test ./internal/auth -run TestLogin_RateLimit_PerUsername` | ❌ Wave 0 |
| AUTH-05 | Authenticated user can change own password | integration | `go test ./internal/auth -run TestAccount_ChangePassword` | ❌ Wave 0 |
| AUTH-05 | Changing password invalidates other sessions (defense-in-depth) | integration | `go test ./internal/auth -run TestAccount_ChangePassword_RevokeOtherSessions` | ❌ Wave 0 |
| AUTH-06 | Viewer cannot POST to admin-only endpoints | integration | `go test ./internal/http -run TestRBAC_ViewerForbidden` | ❌ Wave 0 |
| AUTH-06 | Admin can POST to admin-only endpoints | integration | `go test ./internal/http -run TestRBAC_AdminAllowed` | ❌ Wave 0 |
| AUTH-06 | Frontend hides admin-only UI controls from viewer | component | `pnpm --dir web test components/account-menu.test.tsx` | ❌ Wave 0 |
| INST-01 | First-run middleware redirects to /install when no admin user exists | integration | `go test ./internal/install -run TestFirstRun_Gate` | ❌ Wave 0 |
| INST-01 | After wizard finish, /install becomes inaccessible (302 to /) | integration | `go test ./internal/install -run TestPostFinish_NoWizardAccess` | ❌ Wave 0 |
| INST-02 | Wizard step 4 persists install_identity row | integration | `go test ./internal/install -run TestStep4_PersistsIdentity` | ❌ Wave 0 |
| INST-03 | Wizard step 2 captures CS mode + creds and v3 probe | integration | `go test ./internal/install -run TestStep2_CapturesCS` (uses mock gRPC) | ❌ Wave 0 |
| INST-04 | Wizard step 3 region picker defaults AS923-2 for Thailand | component | `pnpm --dir web test routes/install/region-step.test.tsx` | ❌ Wave 0 |
| INST-04 | Saved region's `(name, common_name)` are written to chirpstack_connection | integration | `go test ./internal/install -run TestStep3_PersistsRegion` | ❌ Wave 0 |
| INST-05 | Wizard rejects v3 ChirpStack with destructive banner | integration | `go test ./internal/install -run TestStep2_RejectsV3` (mock returns Unimplemented) | ❌ Wave 0 |
| INST-05 | `serve` startup refuses to launch against v3 | integration | `go test ./internal/cli -run TestServe_RefusesV3` | ❌ Wave 0 |
| INST-06 | `/health` returns `{status, version, uptime_seconds}` without auth | integration | `go test ./internal/http -run TestHealth_Public` | ❌ Wave 0 |
| INST-06 | `/health/detailed` requires admin and returns DB ping result (Phase 1 minimum) | integration | `go test ./internal/http -run TestHealthDetailed_RequiresAdmin` | ❌ Wave 0 |
| CHIRP-01 | gRPC client dials and lists devices (uses mock) | integration | `go test ./internal/chirpstack -run TestClient_ListDevices_Mock` | ❌ Wave 0 |
| CHIRP-01 | `ProbeVersion` returns version string for v4 mock | integration | `go test ./internal/chirpstack -run TestProbeVersion_v4` | ❌ Wave 0 |
| CHIRP-01 | `ProbeVersion` returns ErrChirpStackV3OrUnknown when GetVersion is Unimplemented | integration | `go test ./internal/chirpstack -run TestProbeVersion_v3` | ❌ Wave 0 |
| CHIRP-02 | MQTT subscriber connects and re-subscribes on reconnect | integration | `go test ./internal/chirpstack -run TestMQTT_ReconnectResubscribe` (testcontainer mosquitto) | ❌ Wave 0 |
| CHIRP-02 | Uplink topic publish lands as a logged event | integration | `go test ./internal/chirpstack -run TestMQTT_UplinkLogged` | ❌ Wave 0 |
| CHIRP-03 | Test Connection POST returns gRPC=reachable + MQTT=reachable for v4 + reachable Mosquitto | integration | `go test ./internal/http -run TestTestConn_Happy` | ❌ Wave 0 |
| CHIRP-03 | Test Connection POST returns gRPC=unreachable + MQTT=skipped for v3 | integration | `go test ./internal/http -run TestTestConn_V3Refused` | ❌ Wave 0 |
| CHIRP-03 | Frontend renders status-row component with green/red dots correctly | component | `pnpm --dir web test components/status-row.test.tsx` | ❌ Wave 0 |
| OPS-01 | `docker-compose.bundled.yml` brings up all 6 services healthy | smoke (CI) | `just compose-smoke-bundled` (script: `up -d`, wait for `/health`, then down) | ❌ Wave 0 |
| OPS-01 | `docker-compose.external.yml` brings up only Postgres+Caddy+shifter; reads CS URL from env | smoke (CI) | `just compose-smoke-external` | ❌ Wave 0 |
| OPS-01 | Both flavors share the same `shifter:<version>` image | unit | `go test ./internal/version -run TestImageTagPinned` (read from `.env.example`) | ❌ Wave 0 |
| UX-01 | Every Phase 1 CRUD surface is a `<Dialog>` (no full-page edit screens) | manual + lint | Code-review checklist + Biome custom rule scanning for `<Dialog>` usage in route files | manual-only (acceptable; pattern enforced via UI-SPEC §Dialog Conventions and code review) |
| UX-02 | Login screen renders with shadcn navy primary, Inter font, English copy | component | `pnpm --dir web test routes/login.test.tsx` (asserts CSS variables resolve to expected OKLCH and copy strings match UI-SPEC) | ❌ Wave 0 |
| UX-02 | Dark mode toggle persists in localStorage and applies `class="dark"` to root | component | `pnpm --dir web test components/theme-provider.test.tsx` | ❌ Wave 0 |

### Sampling Rate

- **Per task commit:** `go test ./<package> -race && pnpm --dir web test <relevant-files> --run`
- **Per wave merge:** `go test ./... -race -count=1 && pnpm --dir web test --run && pnpm --dir web build && just lint`
- **Phase gate:** Full suite green plus `just compose-smoke-bundled` and `just compose-smoke-external` before `/gsd-verify-work`. Manual UI-SPEC checker pass on dialog/copy conformance.

### Wave 0 Gaps

- [ ] `internal/auth/argon2id_test.go` — Hash + Verify round-trip; constant-time guarantee; PHC parsing edge cases
- [ ] `internal/auth/session_test.go` — IdleTimeout behavior, dev-mode Secure toggle
- [ ] `internal/auth/ratelimit_test.go` — per-IP and per-username buckets, cleanup goroutine
- [ ] `internal/auth/authz_test.go` — `Can()` matrix for admin and viewer
- [ ] `internal/install/state_test.go` — draft persistence, atomic finish, idempotency under concurrent finish
- [ ] `internal/install/middleware_test.go` — first-run redirect; post-install passthrough
- [ ] `internal/chirpstack/version_test.go` — v3 (Unimplemented) and v4 (success) paths
- [ ] `internal/chirpstack/mqtt_test.go` — testcontainer Mosquitto connect, subscribe, reconnect, graceful shutdown
- [ ] `internal/db/migrations_test.go` — clean run from empty schema; idempotent re-run; dirty-state surfacing
- [ ] `internal/http/health_test.go` — public minimum payload; admin-required detailed endpoint
- [ ] `internal/http/testconn_test.go` — happy path, gRPC-fail-skip-MQTT, both-fail
- [ ] `internal/http/spa_test.go` — fallback to index.html for unknown route, no fallback for `/api/*`, 404 for missing static assets
- [ ] `internal/cli/serve_test.go` — startup migration; v3-rejection-on-boot
- [ ] `web/src/lib/auth.test.ts` — fetch wrapper, redirect-on-401
- [ ] `web/src/routes/install/region-step.test.tsx` — Thailand → AS923-2 default
- [ ] `web/src/components/status-row.test.tsx` — three states (reachable/unreachable/skipped) render
- [ ] `web/src/components/responsive-dialog.test.tsx` — viewport-driven Dialog↔Sheet swap
- [ ] `web/src/components/theme-provider.test.tsx` — light/dark/system + localStorage persistence
- [ ] Framework install commands (Wave 0 first task):
  ```bash
  # Backend test deps
  go get github.com/stretchr/testify/assert
  go get github.com/testcontainers/testcontainers-go
  go get github.com/testcontainers/testcontainers-go/modules/postgres
  # Frontend test deps
  pnpm --dir web add -D vitest @testing-library/react @testing-library/jest-dom @testing-library/user-event jsdom
  ```
- [ ] `web/vitest.config.ts` — extends `vite.config.ts` with `test.environment = 'jsdom'`, `test.setupFiles = ['./src/test-setup.ts']`
- [ ] `web/src/test-setup.ts` — jest-dom matcher import
- [ ] CI smoke harness: `just compose-smoke-bundled` and `just compose-smoke-external` recipes

---

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | yes | Argon2id with OWASP-2025 params; password complexity hint at ≥12 chars (UI-SPEC); rate limiting per AUTH-04 |
| V3 Session Management | yes | alexedwards/scs server-side; httpOnly + Secure (prod) + SameSite=Lax cookie; idle + absolute timeouts |
| V4 Access Control | yes | `can(user, action, resource)` middleware + UI hide; viewers cannot read admin fields server-side |
| V5 Input Validation | yes | zod (frontend) + manual validation in handlers (backend); pgx parameterized queries via sqlc — **no string interpolation in SQL** |
| V6 Cryptography | yes | argon2id from `golang.org/x/crypto/argon2` — **never hand-roll**; TLS via Caddy auto-ACME or BYO; secret signing via SCS's built-in cookie auth (HMAC) |
| V7 Error Handling | yes | slog JSON; never leak stack traces to client (chi `Recoverer`); tracking IDs in errors |
| V8 Data Protection | yes | Compose secrets via files at `/run/secrets/`; never `.env` (D-06); SOPS-style at-rest is operator's choice |
| V9 Communication Security | yes | TLS-only in production (D-22); HSTS header in Caddyfile; no plaintext fallback |
| V10 Malicious Code | yes | go modules pinned in go.sum; pnpm-lock.yaml; pinned image tags (OPS-07; D-20+) |
| V11 Business Logic | yes | First-run gate is a single SQL query; install_state is singleton (`CHECK id=1`); finish is one transaction |
| V12 Files and Resources | partial | Phase 1 has logo upload (INST-02). Whitelist PNG/JPG, max 256KB; re-encode through `image/png`+`image/jpeg` decoders to strip EXIF/scripts |
| V13 API Security | yes | All `/api/*` routes are auth-scoped (except `/api/auth/login` and `/api/install/*`); CSRF mitigated by SameSite=Lax + custom header (TBD; SCS doesn't enforce; recommend adding `X-Requested-With: shifter` check on state-changing methods) |
| V14 Configuration | yes | All secrets in Compose secrets; pinned tags; no `:latest`; documented `tls.mode` choices |

### Known Threat Patterns for {Go + Postgres + ChirpStack stack}

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| SQL injection | Tampering | sqlc-generated queries with parameterized args; never string-concatenate SQL |
| Session fixation | Spoofing | SCS rotates the session token on Login (built-in) |
| CSRF on state-changing endpoints | Tampering | SameSite=Lax cookie + custom request header (`X-Requested-With`) check on POST/PUT/DELETE |
| XSS in install identity display name | Tampering | React auto-escapes; never `dangerouslySetInnerHTML` for operator-supplied strings |
| ChirpStack API token leakage to logs | Information Disclosure | slog `slog.Attr` with redaction wrapper; secrets read from files, never logged |
| MQTT broker credential exposure | Information Disclosure | Compose secrets file mount; broker bound to `127.0.0.1` in bundled mode (not exposed externally per CONTEXT.md note "ports for Mosquitto are not exposed externally") |
| TLS downgrade (mixed-content / plain HTTP) | Tampering | D-22: production rejects `tls.mode: none`; HSTS header; Caddy enforces redirect from `:80` → `:443` |
| Brute-force login | Repudiation/Spoofing | AUTH-04: per-IP + per-username token bucket, 5 attempts / 5 min |
| Pickled session deserialization | Various | SCS uses gob encoding; we never store custom-serializable types in the session — only primitives (`user_id` int64, `csrf_token` string) |
| Path traversal in logo upload | Tampering | Re-save uploaded image through Go's `image/jpeg` and `image/png` decoders to a UUID-named file in a known directory; never accept the operator-provided filename |
| First-run race (two operators starting wizard simultaneously) | Tampering | `install_state` is a singleton (`CHECK (id=1)`); commit is `Serializable` isolation; second concurrent `Finish` returns ErrAlreadyCompleted |
| ChirpStack v3 acceptance | Spoofing | `InternalService.GetVersion` probe at wizard submit and at every server start |

---

## Sources

### Primary (HIGH confidence — verified or cited from authoritative source)

- [chirpstack/chirpstack — internal.proto](https://github.com/chirpstack/chirpstack/blob/master/api/proto/api/internal.proto) — `InternalService` RPCs including `GetVersion(Empty) → GetVersionResponse` [VERIFIED via WebFetch 2026-04-27]
- [chirpstack/chirpstack-docker — docker-compose.yml](https://github.com/chirpstack/chirpstack-docker) — image tags: `chirpstack/chirpstack:4`, `chirpstack/chirpstack-gateway-bridge:4`, `chirpstack/chirpstack-rest-api:4`, `postgres:14-alpine`, `redis:7-alpine`, `eclipse-mosquitto:2`; ports 8080/8090/1700-UDP/1883 [VERIFIED via WebFetch]
- [chirpstack.io/docs/v4-breaking-changes.html](https://www.chirpstack.io/docs/v4-breaking-changes.html) — v3-incompatibility statement; UUIDs end-to-end; gRPC-only API surface [VERIFIED via WebFetch]
- [chirpstack.io/docs/chirpstack/configuration.html](https://www.chirpstack.io/docs/chirpstack/configuration.html) — region `(name, common_name)` distinction; AS923 sub-bands [CITED]
- [chirpstack.io/docs/chirpstack/integrations/mqtt.html](https://www.chirpstack.io/docs/chirpstack/integrations/mqtt.html) — `application/+/device/+/event/up` topic [CITED]
- [pkg.go.dev/github.com/alexedwards/scs/pgxstore](https://pkg.go.dev/github.com/alexedwards/scs/pgxstore) — `pgxstore.New(*pgxpool.Pool)`; cookie config; SQL schema [VERIFIED via WebFetch]
- [pkg.go.dev/github.com/alexedwards/scs/v2](https://pkg.go.dev/github.com/alexedwards/scs/v2) — session manager API; `LoadAndSave` middleware; idle/lifetime timeouts [CITED]
- [cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html) — Argon2id `m=19456,t=2,p=1` minimum [VERIFIED via WebFetch]
- [pkg.go.dev/github.com/golang-migrate/migrate/v4/source/iofs](https://pkg.go.dev/github.com/golang-migrate/migrate/v4/source/iofs) — `iofs.New(fs, dir)` pattern with `embed.FS` [CITED]
- [github.com/eclipse-paho/paho.mqtt.golang](https://github.com/eclipse-paho/paho.mqtt.golang) — OnConnect re-subscribe pattern; DefaultPublishHandler; graceful Disconnect [CITED]
- [P-H-C/phc-string-format](https://github.com/P-H-C/phc-string-format/blob/master/phc-sf-spec.md) — PHC encoding `$argon2id$v=19$m=...,t=...,p=...$<salt>$<hash>` [CITED]
- [docs.docker.com/compose/how-tos/use-secrets/](https://docs.docker.com/compose/how-tos/use-secrets/) — `/run/secrets/<name>` mount; top-level `secrets:` block [CITED]
- [caddyserver.com/docs/caddyfile/directives/tls](https://caddyserver.com/docs/caddyfile/directives/tls) — `tls internal`; `tls cert key`; `tls` (auto-ACME default) [CITED]
- [caddyserver.com/docs/automatic-https](https://caddyserver.com/docs/automatic-https) — ACME flow + ZeroSSL fallback [CITED]
- [ui.shadcn.com/docs/installation/vite](https://ui.shadcn.com/docs/installation/vite) — canonical Vite + React + TS bootstrap with Tailwind v4 [CITED]
- [ui.shadcn.com/docs/theming](https://ui.shadcn.com/docs/theming) — OKLCH CSS variables; `@theme inline` for custom tokens [CITED]
- [ui.shadcn.com/docs/dark-mode](https://ui.shadcn.com/docs/dark-mode) — `.dark` class strategy [CITED]
- [github.com/vitejs/vite/discussions/10851](https://github.com/vitejs/vite/discussions/10851) — Vite SSE proxy quirks; `proxyTimeout`; `X-Accel-Buffering: no` workaround [VERIFIED via WebFetch]
- [pkg.go.dev/embed](https://pkg.go.dev/embed) — `//go:embed all:dir` for hidden files; `fs.Sub` to strip prefix [CITED]
- [github.com/go-chi/chi](https://github.com/go-chi/chi) — middleware order: RequestID → RealIP → Logger → Recoverer [CITED]

### Secondary (MEDIUM confidence — corroborated by multiple sources)

- [blog.logrocket.com/authentication-react-router-v7/](https://blog.logrocket.com/authentication-react-router-v7/) — protected route loaders; redirect with `next` param
- [dev.to/ra1nbow1/building-reliable-protected-routes-with-react-router-v7-1ka0](https://dev.to/ra1nbow1/building-reliable-protected-routes-with-react-router-v7-1ka0) — loader pattern + Outlet
- [dev.to/ohugonnot/rate-limiter-in-go-per-ip-token-bucket-with-golangorgxtimerate-5ff8](https://dev.to/ohugonnot/rate-limiter-in-go-per-ip-token-bucket-with-golangorgxtimerate-5ff8) — `x/time/rate` per-key cleanup pattern
- [www.alexedwards.net/blog/how-to-hash-and-verify-passwords-with-argon2-in-go](https://www.alexedwards.net/blog/how-to-hash-and-verify-passwords-with-argon2-in-go) — Go Argon2id encoding+verify
- [github.com/alexedwards/argon2id](https://github.com/alexedwards/argon2id) — reference Go Argon2id wrapper (PHC format)
- [riverqueue.com/docs/periodic-jobs](https://riverqueue.com/docs/periodic-jobs) — River cron schedule (deferred to Phase 2+)
- [forum.chirpstack.io/t/multi-region-with-v4-and-docker-compose/15934](https://forum.chirpstack.io/t/multi-region-with-v4-and-docker-compose/15934) — region naming `as923` vs `as923_3` topic prefix split
- [pkg.go.dev/log/slog](https://pkg.go.dev/log/slog) — structured logging stdlib
- [docs.sqlc.dev/en/latest/guides/using-go-and-pgx.html](https://docs.sqlc.dev/en/latest/guides/using-go-and-pgx.html) — sqlc 1.31 + pgx/v5 integration

### Tertiary (LOW confidence — single source, marked for verification at task time)

- ChirpStack v3 returning `codes.Unimplemented` exactly for `InternalService.GetVersion` — assumed (A1) based on standard gRPC behavior for missing RPCs; verify at first task touching this code path. If the actual error is `codes.NotFound` or `codes.Unknown`, the planner's mitigation is a one-line additional case in the v3-detection branch.
- Vite proxy honoring `X-Accel-Buffering: no` end-to-end — assumed (A4) based on http-proxy convention; if buffering still occurs, the `bypass` workaround from the GitHub discussion is the documented fallback.

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — every library and version cross-checked against pkg.go.dev, npm, official docs, or local environment audit.
- Architecture: HIGH — patterns inherit from synthesized SUMMARY.md / ARCHITECTURE.md / CONTEXT.md, with no contradictions surfaced.
- Pitfalls: HIGH — every pitfall has a concrete prevention rooted in either CLAUDE.md, PITFALLS.md, or this research's verified sources.
- ChirpStack v3 detection: MEDIUM — the canonical RPC (`InternalService.GetVersion`) is verified to exist in v4 protos but the exact gRPC error code v3 returns is assumed; impact is one-line.
- Validation Architecture: HIGH — test types map cleanly to Phase 1 boundaries; testcontainers + mockgen + vitest are well-supported.

**Research date:** 2026-04-27
**Valid until:** 2026-05-27 (30 days — stable boring tech; ChirpStack v4.18+ release would warrant a re-check; Tailwind v4 minor releases could affect the OKLCH `@theme inline` syntax)

---

*Phase: 01-foundation*
*Researched: 2026-04-27*
