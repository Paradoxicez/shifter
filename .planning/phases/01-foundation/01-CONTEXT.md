# Phase 1: Foundation - Context

**Gathered:** 2026-04-27
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 1 delivers a scripted install that gets the operator from `docker compose up` to a logged-in Settings shell with a green "Test Connection" against ChirpStack v4 (gRPC + MQTT). No telemetry yet, no devices, no sites. The phase establishes the modal-first UX convention, the bilingual ChirpStack integration plumbing (gRPC control plane + MQTT subscription, both verified by Test Connection), the install-wizard and bootstrap mechanics, and the operational baseline (TLS, secrets, logs, health, version pinning) for every later phase.

In scope: AUTH-01..06, INST-01..06, CHIRP-01..03, OPS-01, UX-01, UX-02.
Explicitly out of scope (later phases): provisioning UI (gateways, devices), telemetry ingestion, dashboards, reports, alerts, full operational hardening (backup/restore, audit UI, secrets rotation runbook).

</domain>

<decisions>
## Implementation Decisions

### Repo Layout & Build Toolchain
- **D-01:** Single Go-canonical monorepo at the repo root: `cmd/shifter/` (binary entry), `internal/` (private packages), `web/` (Vite + React frontend). The Go binary embeds the built SPA via `go:embed`. No separate `backend/`, no separate `frontend/` repo.
- **D-02:** `Justfile` at the repo root is the canonical build/dev orchestrator. All common operations (`just dev`, `just build`, `just test`, `just migrate`, `just lint`) go through it. Document `just` as a one-line install in the README.
- **D-03:** `pnpm` is the only frontend package manager. `pnpm-lock.yaml` is committed; `package-lock.json` and `yarn.lock` must not appear.
- **D-04:** Hot reload uses `Air` for the Go binary plus `Vite` dev server for the SPA. In dev, Vite serves at `:5173` and proxies `/api` and `/sse` to the Go binary at `:8080`. In prod, Go serves the built SPA from the embed.

### Configuration & Secrets
- **D-05:** Configuration source order is `config.yaml` (canonical, default at `/etc/shifter/config.yaml`) overridden by environment variables (`SHIFTER_*` prefix). No flag-based config. Operator reads/writes `config.yaml`, ops scripts override via env when needed.
- **D-06:** All secrets — Postgres password, ChirpStack API token, session signing key — are mounted as Docker Compose `secrets` (file-based mount, paths referenced from `config.yaml`). `.env` files are forbidden for secrets. Set up Compose top-level `secrets:` block and `secrets:` references in the `shifter` and `chirpstack` services.
- **D-07:** `config-check` CLI command must validate `config.yaml` shape, ping the Postgres, ChirpStack gRPC, and MQTT endpoints, and exit non-zero on any failure. Install kit invokes this before starting `serve`.

### First-Run / Install Wizard Mechanics
- **D-08:** First-run is detected by absence of any admin user in the `user` table. No flag files, no env vars. Installing into an empty schema → wizard. Re-running after a completed install → straight to login. The check happens in middleware before any request reaches non-auth routes.
- **D-09:** No "default admin password". Wizard step 1 captures the operator's chosen `email` + `password` (with confirm + strength hint) and creates the admin user atomically as part of wizard commit. **AUTH-03 ("force change default admin password on first login") is therefore reframed as: applies only to admin-created secondary users (Phase 6 USER-04 inherits this) — the bootstrap admin sets their own password from the start.**
- **D-10:** Wizard is a 5-step stepped dialog (admin user → ChirpStack mode + creds → region default → install identity → review & finish), as described in the UI-SPEC. Each step writes to a draft `install_state` table; "Finish setup" commits all four into the canonical tables in one transaction.
- **D-11:** Wizard is reentrant: if the operator quits mid-wizard, refreshing the page shows the same step they were on (draft persisted). Once "Finish setup" commits, the draft row is deleted and the wizard becomes inaccessible for that install.

### CLI Surface
- **D-12:** Cobra-based CLI. Subcommands shipping in Phase 1: `serve`, `migrate`, `version`, `create-admin`, `config-check`, `healthcheck`. All built into the same `shifter` binary.
- **D-13:** `shifter serve` auto-runs `migrate up` on startup before opening the listener. Migrations are idempotent and log a one-line "applied N migrations" or "schema up to date". Failure to migrate aborts startup with non-zero exit.
- **D-14:** `shifter create-admin` is a recovery / lockout escape hatch. It creates a new admin user (or resets an existing one's password if `--reset` is passed) without going through the wizard. Used by ops if the only admin gets locked out.
- **D-15:** `shifter healthcheck` makes a localhost call to the binary's `/health` endpoint and exits 0/1 based on status. Used by Docker `HEALTHCHECK` to avoid bundling `curl` into the image.

### Migrations
- **D-16:** `golang-migrate` library (vendored, called as a Go package — not the standalone CLI) runs plain SQL files. Migrations live at `internal/db/migrations/NNNN_<name>.{up,down}.sql`. Down migrations exist but are never auto-invoked.
- **D-17:** Migration names use the `golang-migrate` integer prefix convention (`0001_init.up.sql`, `0002_users.up.sql`, …). One conceptual change per migration. Forward-only in production; down migrations are for local dev rollback only.

### /health Endpoint
- **D-18:** `/health` is public (no auth required) and returns minimal info: `{ "status": "ok" | "degraded", "version": "<semver>", "uptime_seconds": <int> }`. This matches Docker / Caddy / external probe needs without leaking install internals to anonymous attackers.
- **D-19:** Detailed health (DB connection state, ChirpStack gRPC + MQTT reachability, last-uplink-age, disk free) is exposed at `/health/detailed` and **requires admin auth**. INST-06 claims `/health` reports DB+CS+MQTT+disk+last-uplink-age — that detail moves to `/health/detailed`; `/health` keeps the public summary. **Update REQUIREMENTS.md INST-06 wording during Phase 1 implementation to reflect this split.**

### TLS & Reverse Proxy
- **D-20:** Caddy 2 is bundled in both compose flavors (`bundled` and `external` ChirpStack). Caddy terminates TLS, fronts the Go binary, and routes `/`, `/api`, `/sse`, `/files`, `/health` to the appropriate handler.
- **D-21:** TLS modes are config-driven via `tls.mode`:
  - `acme` (default) — Caddy obtains certs from Let's Encrypt for `SHIFTER_DOMAIN`. ZeroSSL is the documented fallback ACME provider when LE is rate-limited.
  - `byo` — operator mounts `cert.pem` + `key.pem` into the Caddy container; Caddy serves them directly. Used for internal CAs and air-gapped LANs.
  - `internal` — Caddy `tls internal` (self-signed via Caddy's local CA). Used for LAN-only deployments without public DNS; surfaces a one-time browser warning.
- **D-22:** Plain HTTP (no TLS) is **not** an option in production. The config schema rejects `tls.mode: none`. Cookie security flags (`Secure`, `HttpOnly`, `SameSite=Lax`) are always set, predicated on TLS being terminated upstream.
- **D-23:** Dev mode (Vite + Air) runs plain HTTP on localhost (`http://localhost:5173`). Cookie `Secure` flag is automatically disabled in dev via a `SHIFTER_ENV=dev` toggle so sessions work locally.

### Logging & Observability (carried over to inform Phase 1 implementation)
- **D-24:** Structured JSON logs (one event per line) to stdout. Operator's Docker `json-file` driver with size+file-count caps captures them (set in compose). No file-based logging from inside the binary.
- **D-25:** Log levels: `info` default, `debug` toggled via `SHIFTER_LOG_LEVEL=debug` env. No per-package level config in v1 — keeps the surface small.

### Claude's Discretion
- Exact directory layout under `internal/` (e.g. `internal/auth`, `internal/chirpstack`, `internal/install`, `internal/web`) — planner picks idiomatic Go layout
- Cobra command file split (one file per subcommand vs. consolidated)
- Middleware order (auth → request-id → logger → recover) — standard chi layering
- Vite proxy config exact form
- Justfile recipe naming and grouping
- Caddyfile organization (one site block vs. per-route)
- `config.yaml` exact YAML shape (kebab-case vs. snake_case keys) — pick what reads best for ops

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project & Phase
- `.planning/PROJECT.md` — Vision, constraints, single-tenant-per-install posture, blue/navy + shadcn aesthetic, English-only, modal-first
- `.planning/REQUIREMENTS.md` §AUTH-01..06, INST-01..06, CHIRP-01..03, OPS-01, UX-01..02 — Phase 1 acceptance criteria
- `.planning/ROADMAP.md` — Phase 1 success criteria (5 observable behaviors), phase dependencies, coverage map

### Research synthesis
- `.planning/research/SUMMARY.md` — Cross-stream synthesis; backend = Go decision; two-channel ChirpStack integration; deployment topology
- `.planning/research/STACK.md` — Specific library versions and rationale (Cobra, chi, sqlc, pgx/v5, alexedwards/scs, Argon2id, Caddy, Vite, shadcn/ui, lucide-react, react-router-dom v7, react-hook-form, zod, sonner, TanStack Query, fontsource)
- `.planning/research/ARCHITECTURE.md` — Two-path architecture (ingestion vs API), `/health` model, dual-mode ChirpStack deployment topology, single-Postgres pattern (separate databases inside one instance for bundled mode)
- `.planning/research/PITFALLS.md` §7 (ChirpStack v3/v4 mismatch), §8 (AS923 Thailand sub-plan), §14 (two-role coarseness — design `can(user, action, resource)` API now), §15 (ops gaps — Compose secrets, pinned tags), §16 (codebase drift — one-branch rule)

### UI design contract
- `.planning/phases/01-foundation/01-UI-SPEC.md` — Frontend contract for Phase 1: shadcn `new-york` style + slate base + custom navy `#1E40AF`, OKLCH semantic palette, Inter + JetBrains Mono self-hosted, sidebar+topbar shell, dialog anatomy, stepped-dialog pattern (used by install wizard), Test Connection status-row, canonical verb table, dark/light/system theme

### External / vendor docs
- ChirpStack v4 gRPC API — `https://www.chirpstack.io/docs/chirpstack/api/grpc.html` (control plane)
- ChirpStack MQTT integration — `https://www.chirpstack.io/docs/chirpstack/integrations/mqtt.html` (event channel)
- ChirpStack v4 breaking changes — `https://www.chirpstack.io/docs/v4-breaking-changes.html` (informs the v3-rejection install gate)
- ChirpStack Docker reference compose — `https://github.com/chirpstack/chirpstack-docker` (basis for the bundled flavor)
- TimescaleDB releases — `https://github.com/timescale/timescaledb/releases` (pin 2.26)
- Caddy 2 docs — `https://caddyserver.com/docs/` (TLS modes, on-demand TLS, internal CA)
- LoRa Alliance regional parameters (AS923) — `https://www.thethingsnetwork.org/docs/lorawan/regional-parameters/` (Thailand sub-plan reference)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
None — Phase 1 is greenfield. The repo currently contains only `.planning/` artifacts.

### Established Patterns
None to inherit from existing code. Phase 1 establishes the patterns every later phase will reuse:
- **Layered Go module:** `cmd/shifter` → `internal/<domain>` → `internal/db` (sqlc-generated) → `internal/db/migrations` (plain SQL)
- **Modal-first frontend:** every CRUD via `<Dialog>` from shadcn; mobile responsive swap to `<Sheet>`
- **Stepped-dialog pattern:** the install wizard establishes the pattern reused in Phase 2 meter swap, Phase 3 bulk import, Phase 5 floor-plan upload, Phase 7 codec runner
- **Status-row pattern:** the Test Connection row is reused for alert center / SSE health / backup status in later phases
- **Canonical verb table:** locks vocabulary across all phases (Sign in, Set password, Save and test, Add, Edit, Delete, Revoke, Disable)

### Integration Points
Phase 1 only needs to talk to:
- **Postgres + TimescaleDB** (via `pgx/v5` + sqlc) — read/write users + install state + sessions
- **ChirpStack v4 gRPC** (via official `github.com/chirpstack/chirpstack/api/go/v4` stubs) — version probe + simple read calls (list devices/gateways) for Test Connection
- **MQTT broker** (via `paho.mqtt.golang`) — connect + subscribe to `application/+/device/+/event/up` and log to stdout (no DB write yet)
- **Frontend SPA** (served via `go:embed` in prod, Vite proxy in dev)

</code_context>

<specifics>
## Specific Ideas

- The product must feel like Shifter, never like a re-skinned ChirpStack — keep operator-facing copy in customer/site/device language; never expose "tenant" or "application" terminology in Phase 1 UI
- The install wizard's tone is conversational and confident: "We just need a few details to get Shifter running" rather than "Configure Shifter"
- Test Connection result panel uses the status-row pattern from UI-SPEC: dot + label + monospace detail (latency in ms, gRPC version, MQTT broker URL); failures show actionable next-step copy
- AS923-2 is the Thailand default in step 3 of the wizard; the picker shows other AS923 sub-plans plus EU868, US915, AU915, IN865 for international customers
- `config.yaml` example file lives at `config/config.example.yaml` and is the source of truth for documented keys; the install kit copies it to `/etc/shifter/config.yaml` if the file doesn't already exist

</specifics>

<deferred>
## Deferred Ideas

- **Logging format toggle (text vs JSON):** Phase 1 ships JSON-only. A `--text` mode for human-readable dev logs is a possible Phase 6 polish item but not in scope.
- **`/health/detailed` rich payload:** Phase 1 ships a minimal version. Full DB+CS+MQTT+disk+last-uplink-age payload lands in Phase 6 alongside backup status.
- **AS923 sub-plan catalog file:** Phase 1 hardcodes the four sub-plans (AS923-1, AS923-2, AS923-3, AS923-4) plus a Thailand-specific note. A regulator-versioned data file is a Phase 7 install-validation polish item.
- **Internationalization:** English-only in v1; not in any v1 phase. v2 work.
- **SMTP / password reset email:** v2. AUTH-03's "force change on first login" applies only to admin-created secondary users (USER-04 in Phase 6); Phase 1 avoids the SMTP dependency entirely.
- **SSO / OAuth:** v2. The `can(user, action, resource)` API in Phase 1 is forward-compatible with future role bundles, but no OAuth flow ships now.
- **Audit log UI:** Phase 1 records nothing yet (audit middleware lands in Phase 2 before meter swap UI). The audit-log browse + CSV export is Phase 6.
- **Per-package log levels:** Phase 1 ships single-level (`info`/`debug`). Per-package configurability is over-engineering for v1.
- **Backup script & restore CI test:** Phase 6 deliverable, not Phase 1.
- **Detailed health auth and richer probe:** moves to Phase 6 with backup status surface.

</deferred>

---

*Phase: 01-foundation*
*Context gathered: 2026-04-27*
