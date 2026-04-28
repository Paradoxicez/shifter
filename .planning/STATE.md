---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
status: Ready to execute
last_updated: "2026-04-28T01:42:56.791Z"
progress:
  total_phases: 7
  completed_phases: 0
  total_plans: 24
  completed_plans: 11
  percent: 46
---

# Project State: Shifter

**Last Updated:** 2026-04-28 (after Plan 01-11 execution — Account UI shipped: typed auth client (`web/src/lib/auth.ts`), rootLoader gating protected routes on `/api/account/me`, ChangePasswordDialog with UI-SPEC verbatim copy, AccountInfoHandler at the API. AUTH-05 frontend complete; AUTH-06 frontend hiding scaffolding plumbed (userRole prop ready for Phase 2+ admin-only menu items).)

## Project Reference

**Core Value:** The operator runs their entire LoRaWAN water/electricity monitoring operation — provisioning, placement, monitoring, reporting — from Shifter alone, and meter swaps never break historical continuity.

**Current Focus:** Phase 01 — foundation

## Current Position

Phase: 01 (foundation) — EXECUTING
Plan: 11 of 24 complete (Plans 01, 02, 03, 04, 05, 06, 07, 08, 09, 10, 11)

| Field | Value |
|-------|-------|
| **Phase** | 1 — Foundation |
| **Plan** | 12 — chirpstack-grpc (next) |
| **Status** | Plans 01–11 complete; Plan 11 shipped the account UI: backend `AccountInfoHandler` at GET /api/account/me (returns `{user: {id, email, role, must_change_password}}`, 401 for missing session AND for disabled-mid-session admins via `ErrUserNotFound` short-circuit); typed frontend auth client at `web/src/lib/auth.ts` (fetchSessionUser / login / logout / changePassword + SessionUser type, ApiError re-export) — every `/api/auth/*` and `/api/account/*` consumer now goes through this module; `rootLoader` in `web/src/routes/_root.tsx` calls fetchSessionUser before any protected route renders and `throw redirect('/login?next=...')` on 401; `ChangePasswordDialog` (ResponsiveDialog wrapper, UI-SPEC verbatim copy strings, 401/422 inline error mapping, Cancel-LEFT/primary-RIGHT footer) wired into RootLayout via the AccountMenu's "Change password" item; sonner success toast "Password changed" on commit + revalidator.revalidate(). AUTH-05 frontend complete; AUTH-06 frontend hiding scaffolding plumbed (userRole prop reaches AccountMenu; Phase 1 has no admin-only menu items but Phase 2+ adds will be one-line `userRole === 'admin' && …` guards). D-09 verified at the API: `TestAccountInfo_ReturnsUser` asserts `must_change_password=false` for the create-admin / wizard admin. |
| **Progress (plans)** | `[█████░░░░░] 11/24 (46%)` |
| **Progress (phases)** | `[░░░░░░░░░░] 0/7 phases` |

**Next action:** `/gsd-execute-plan 01 12` (or `/gsd-execute-phase 01` to continue the chain)

## Performance Metrics

| Metric | Value |
|--------|-------|
| Phases complete | 0 / 7 |
| v1 requirements mapped | 99 / 99 (100%) |
| Plans complete | 11 / 24 (01, 02, 03, 04, 05, 06, 07, 08, 09, 10, 11) |
| Open blockers | 0 |

### Per-plan execution log

| Plan | Duration | Tasks | Files |
|------|----------|-------|-------|
| 01-01 repo-scaffold | 23 min | 2 | 21 |
| 01-02 test-harness | 7 min | 2 | 44 |
| 01-03 database-layer | 33 min | 1 | 29 |
| 01-05 cobra-cli | 4 min | 2 | 13 |
| 01-06 frontend-shell | 6 min | 3 | 38 |
| 01-04 config-secrets | 10 min | 2 | 11 |
| 01-07 argon2id | 3 min | 2 | 6 |
| 01-08 session-manager | 5 min | 1 | 5 |
| 01-09 login-ratelimit | 11 min | 3 | 13 |
| 01-10 authz | 3 min | 1 | 3 |
| 01-11 account-ui | 6 min | 2 | 9 |

## Accumulated Context

### Key Decisions (locked at roadmap creation)

- **Backend language:** Go 1.24+ (single-binary deploy, native ChirpStack gRPC stubs, mature MQTT/Postgres ecosystem) — locked in research, executed in Phase 1.
- **Database:** PostgreSQL 16/17 + TimescaleDB 2.26 (one DB for relational metadata + telemetry hypertable + CAGGs).
- **ChirpStack integration:** gRPC for control plane + MQTT for events; v4 only (refuses v3 on first connect).
- **Realtime delivery:** SSE backed by Postgres `LISTEN/NOTIFY` from a hypertable insert trigger — never directly off MQTT, so the dashboard only sees persisted data.
- **Frontend:** Vite + React 19 + TypeScript + Tailwind v4 + shadcn/ui (no Next.js — no SSR/SEO benefit). Served by the Go binary via `go:embed`.
- **Domain anchor:** Metering point + time-windowed device assignment + reading offset is the schema invariant. Every report, chart, alert queries by `metering_point_id`, never `dev_eui`.
- **Canonical measurement schema:** Hybrid wide+JSONB hypertable (canonical first-class columns + `extra` JSONB + `raw` JSONB for the original payload). Codecs run inside ChirpStack's QuickJS sandbox; Shifter only maps decoded objects to canonical fields.
- **Floor-plan placement:** Normalized fractions (`x_frac`, `y_frac` ∈ [0, 1]), never pixel integers. PROJECT.md wording will be updated during Phase 1 or Phase 5.
- **Map:** Leaflet 1.9 + react-leaflet 5 + `leaflet.markercluster` from day 1; OpenStreetMap tiles only (no paid API).
- **Deployment:** Two Docker Compose flavors (`bundled` + `external`) sharing the same backend image. File-based Compose secrets, pinned image tags, Caddy reverse proxy.
- **Audit middleware:** Ships in Phase 2 before the meter-swap UI so swaps are auditable from day 1; UI + CSV export in Phase 6.

### Phase 01 Execution Decisions

- **Plan 01-01 — Justfile is the canonical entry point.** Every dev/CI/install task goes through a `just` recipe; raw `go`/`pnpm`/`golangci-lint`/`migrate` calls in docs or CI are forbidden going forward (D-02).
- **Plan 01-01 — `@vitejs/plugin-react` pinned to ^5.** v6 requires Vite 8; we pin Vite 7. Re-evaluate at phase wrap-up if Vite 8 has stabilized.
- **Plan 01-01 — Biome 2.x config schema.** `assist.actions.source.organizeImports`, `files.includes` with negative globs. Plan template referenced legacy 1.9 schema; future plans must use 2.x.
- **Plan 01-01 — Cobra root carries a Long description.** Lets `--help` print lowercase "shifter" (verification grep) and seeds the help template for upcoming `serve` / `migrate` / `create-admin` subcommands.
- **Plan 01-01 — `web/package.json` uses `@types/node` and `type: "module"`.** Required by Vite 7 + ESM `vite.config.ts`.
- **Plan 01-01 — Host prerequisites:** Go 1.24+, Node 22.12+ (or 20.19+) per Vite 7, pnpm 10.x, `just` (host-installed). Plan 02 (test harness) should add `engines.node` to `web/package.json` and a `.nvmrc`.
- **Plan 01-02 — Wave 0 stub-then-fill is the canonical pattern.** Every Phase 1 plan's `<verify>` block points at an existing test file (`t.Skip` / `describe.skip`); the implementing plan replaces only the body. Future plans MUST NOT create new test files outside the `internal/{auth,install,chirpstack,db,http,cli,version}/*_test.go` and `web/src/**/*.test.{ts,tsx}` scaffold from this plan.
- **Plan 01-02 — Pinned testcontainer image tags.** T-02-01 mitigation: `timescale/timescaledb:2.26.0-pg16` and `eclipse-mosquitto:2.0.18`. No `:latest` tags allowed in tests; supply-chain hygiene per ASVS V10/OPS-07.
- **Plan 01-02 — engines.node >=22.12 + .nvmrc=22.12.** Resolves Plan 01-01's open todo. Caveat: jsdom 29 itself requires Node 22.13+, so the CI runner must run >=22.13 even though the project floor is 22.12 — flagged for the CI plan.
- **Plan 01-02 — pgx/v5 v5.9.2 added to go.mod.** Required by `internal/testsupport/postgres.go`'s `*pgxpool.Pool` return. Plan 03 (database-layer) reuses this same version when wiring `db.RunMigrations`.
- **Plan 01-03 — Migration runner uses dedicated `*sql.DB`, not `stdlib.OpenDBFromPool`.** Sharing connections via `OpenDBFromPool` wedges `puddle.Pool.Close` at teardown because the migrate driver's connection-release semantics conflict with puddle's WaitGroup. Fix: `sql.Open("pgx", pool.Config().ConnConfig.ConnString())` with anon import of `pgx/v5/stdlib` for driver registration. The migration `*sql.DB` is fully independent of the application pgxpool.
- **Plan 01-03 — `user.email` is TEXT (not CITEXT).** Plan's verbatim 0002 created CITEXT then ALTER'd to TEXT, which fails at CREATE TABLE because we don't load the citext extension. Schema goes straight to `email TEXT NOT NULL` with `CHECK (email = lower(email))` — same lowercase invariant, no broken intermediate state. Application code (Plans 09, 15) MUST `lower()` email before insert; CHECK is a backstop.
- **Plan 01-03 — `touch_updated_at()` is the canonical updated_at trigger function.** Defined once in `0002_users`; `install_state`, `install_identity`, `chirpstack_connection` all reuse it. Future tables with `updated_at` MUST NOT redeclare the function — only attach a new trigger.
- **Plan 01-03 — Singleton tables use `id INT PRIMARY KEY DEFAULT 1 CHECK (id = 1)` + INSERT .. ON CONFLICT (id) DO UPDATE pattern.** `install_state`, `install_identity`, `chirpstack_connection` all use this. Future singletons MUST use this pattern; do not invent alternatives.
- **Plan 01-03 — sqlc generates to `internal/db/sqlc` (Go import: `github.com/shifter-io/shifter/internal/db/sqlc`).** Path is locked. Plans 09/11/14/15/17 import directly — no aliasing.
- **Plan 01-03 — Secrets stored by reference.** `*_ref` columns hold a path under `/run/secrets/`, never the raw value. Applies to `chirpstack_connection.api_token_ref` and `chirpstack_connection.mqtt_password_ref`. Plan 04 wires the read side.
- **Plan 01-05 — Subcommand layout: one *.go file per leaf subcommand under `internal/cli/`.** `serve.go`, `migrate.go`, `version.go`, `healthcheck.go`, `createadmin.go`, `configcheck.go` each own their `*cobra.Command` and any flag-binding init(). `root.go` is the only place `AddCommand` is called. New subcommands MUST follow this layout — no monolithic command files.
- **Plan 01-05 — `migrate force` is a verbose subcommand, not a flag.** `shifter migrate force <N>` instead of `shifter migrate up --force <N>`. Recovery operations should be hard to invoke accidentally (T-05-03 mitigation).
- **Plan 01-05 — Healthcheck is the binary itself.** `shifter healthcheck` is a localhost GET /health probe. Docker `HEALTHCHECK` directives MUST use this — never add curl/wget to the image (PITFALL #11).
- **Plan 01-05 — Plan 04 dependency stubs created early.** `internal/config/config.go`, `internal/logging/logging.go`, and `internal/version/version.go` were scaffolded by Plan 05 because Plan 05 was executed before Plan 04. Plan 04 MUST replace the function bodies (full viper / Validate / JSON handler implementations) WITHOUT changing the public function signatures: `config.Load() (*Config, error)`, `logging.New(level string) *slog.Logger`, `version.Info() BuildInfo`. The Config struct fields used today (Env, HTTPPort, LogLevel, DB, TLS) must stay; new fields can be added.
- **Plan 01-05 — TODO marker convention: `TODO(plan-NN ...)` with the implementing plan number.** Multi-plan collaborations use `+`: `TODO(plan-09 + plan-13 + plan-18)`. `grep -rn 'TODO(plan-' internal/cli` locates every downstream insertion site.
- **Plan 01-05 — BuildInfo sentinels:** Unstamped builds default to `{Version: "dev", Commit: "none", BuildTime: "unknown"}` so local `go build` is self-describing. Production injection via `-ldflags "-X github.com/shifter-io/shifter/internal/version.{Version,Commit,BuildTime}=..."` is documented in `internal/version/version.go`'s package comment; Plan 24 wires the Justfile recipe.
- **Plan 01-06 — shadcn 4.5.0 init bypassed; components.json authored directly.** style=new-york / baseColor=slate / cssVariables=true / iconLibrary=lucide. End state matches the plan's intended init flow. Future plans use `pnpm dlx shadcn@latest add <name>` only — no re-init required (PITFALL #12).
- **Plan 01-06 — Theme tokens locked in `web/src/theme.css`** (separate from shadcn's regenerable `index.css` per PITFALL #12). `@theme inline` mappings register the custom `--color-success` / `--color-warning` / `--color-info` tokens AND the standard core tokens (background / foreground / primary / etc.) with Tailwind v4 so utility classes like `bg-primary`, `text-success`, `border-warning` compile.
- **Plan 01-06 — 20 shadcn components installed (plan inventory says "21" but lists 20).** button, input, label, form, card, dialog, alert, alert-dialog, dropdown-menu, avatar, select, checkbox, separator, skeleton, sonner, tabs, progress, badge, tooltip, sheet. Plan typo — concrete inventory is 20.
- **Plan 01-06 — Foundational components ResponsiveDialog / StatusRow / Stepper / ThemeProvider are MANDATORY for Phase 1+ CRUD surfaces.** Plans 11/16/17/23 import these directly; never instantiate raw shadcn `<Dialog>` / `<Sheet>` for CRUD. Phase 2+ (Add device, Meter swap, Floor-plan upload) inherit too.
- **Plan 01-06 — `apiFetch` contract: every `/api/*` request sends `X-Requested-With: shifter`** (CSRF mitigation per RESEARCH §Security Domain). Backend (Plan 11) will reject state-changing requests without it. SameSite=Lax cookies + custom header is the canonical pattern.
- **Plan 01-06 — Theme persistence key locked to `localStorage['shifter-theme']`.** Plan 11+ MUST NOT change the key — operator-set theme survives across logins.
- **Plan 01-06 — Self-hosted fonts via @fontsource (no Google Fonts CDN).** Inter Variable + JetBrains Mono 400/600 imported from `@fontsource-variable/inter` + `@fontsource/jetbrains-mono` per UI-SPEC §Design System; satisfies the self-hosted constraint.
- **Plan 01-06 — App.tsx provider order: `<ThemeProvider><QueryClientProvider><RouterProvider/><Toaster/></QueryClientProvider></ThemeProvider>`.** RouterProvider MUST be inside QueryClientProvider; ThemeProvider is outermost so theme switches don't blow away query cache.
- **Plan 01-04 — Layered config loader (env > YAML > defaults).** viper.AutomaticEnv + SetEnvPrefix("SHIFTER") + SetEnvKeyReplacer(".", "_") so dotted YAML keys (`db.host`) overlay onto `SHIFTER_DB_HOST` env vars (D-05). Future plans MUST read configuration only via `*config.Config`, never `os.Getenv` directly for runtime config. `SHIFTER_CONFIG_FILE` overrides the `/etc/shifter/config.yaml` default; missing file is tolerated (env+defaults can satisfy a valid Config).
- **Plan 01-04 — Compose-secrets idiom locked.** Every secret has both a `SHIFTER_<NAME>` direct env (dev paths only) and a `SHIFTER_<NAME>_FILE` env pointing into `/run/secrets/<name>` (production). `ReadSecret(name)` prefers direct then file, errors when neither is set. `ReadSecretOrEmpty(name)` returns `("", nil)` for "neither set" but propagates real read errors. `Session.Key` uses `ReadSecret` (T-04-03 hard requirement); DB password / CS token / MQTT password use `ReadSecretOrEmpty`. Future plans adding secrets MUST follow this pattern (both env vars; pick required vs optional; never put raw secrets in `config.yaml`).
- **Plan 01-04 — CRLF-safe file read.** `strings.TrimRight(content, "\r\n")` so a Windows-edited secret file does not silently corrupt the password (PITFALL #8). Tested by `TestReadSecret_CRLF`. Plan reference's `\n`-only trim was insufficient; corrected to drop both `\r` and `\n`.
- **Plan 01-04 — Validate() rejects `tls.mode=none` AND empty.** D-22 forbids plain HTTP. Empty string also rejected because a hand-edited config with the line removed could otherwise reach an unsafe runtime state with a confusing error message. `tls.mode=acme` additionally requires `tls.domain` to be set.
- **Plan 01-04 — Session key length floor enforced.** `len(Session.Key) >= 32` per OWASP ASVS V6 / T-04-03. Plan listed this as a must_haves "truth" but plan-verbatim Validate() omitted the check; added explicitly with a regression test (`TestValidate_RejectsShortSessionKey`).
- **Plan 01-04 — slog JSON to stdout (not stderr).** `slog.NewJSONHandler(os.Stdout, &HandlerOptions{Level: lvl, AddSource: false})` — D-24 calls for stdout so the Docker `json-file` driver captures it under operator-configured rotation caps. Plan 05's stub used stderr; corrected. `AddSource: false` keeps event size small (file:line strings have low operational value when logs aggregate across containers). PITFALL #9 (slog pretty-print) is defused by relying on the handler's documented one-event-one-line contract.
- **Plan 01-04 — Log-level whitelist.** D-25 limits operator-configurable levels to `info|debug`; `debug` → LevelDebug, `info`/empty → LevelInfo, anything else (warn, error, trace, garbage) silently falls back to LevelInfo. Future logging changes MUST NOT add warn/error/trace as configurable levels without amending D-25.
- **Plan 01-04 — `version.ImageTag = Version` single source of truth.** Top-level package var (not function) so the OPS-01 unit test (`TestImageTagPinned`) is `require.Equal(t, Version, ImageTag)` and the same `-ldflags -X version.Version=...` injection updates both at link time. Compose plans (20/21) MUST read this constant rather than hardcode a tag string.
- **Plan 01-04 — Build sentinel defaults.** `Version = "dev" / Commit = "none" / BuildTime = "unknown"` so an unstamped local `go build` is self-describing rather than printing empty strings. Production builds inject via `-ldflags "-X github.com/shifter-io/shifter/internal/version.Version=$(git describe --always --dirty) -X .Commit=$(git rev-parse --short HEAD) -X .BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)"`. Plan 24 wires the Justfile recipe.
- **Plan 01-07 — Argon2id PHC encoding is the only password hash.** `argonMemKiB=19456 / argonTime=2 / argonThreads=1 / salt=16 / key=32` are pinned package-level constants per OWASP-2025; future hardening updates the constants and lazy-rehashes on login (Verify re-parses params from the encoded string, so old hashes keep working). No bcrypt anywhere; CLAUDE.md "do not mix" enforced by absence (`grep -rn 'golang.org/x/crypto/bcrypt' internal/` = 0 hits). `subtle.ConstantTimeCompare` is the only key comparison primitive (T-07-01). Plans 09 / 11 / 15 MUST call `auth.Hash` + `auth.Verify`; no raw `argon2.IDKey` calls outside this package.
- **Plan 01-07 — Strength enum starts at iota 0 = StrengthWeak (defensive default).** `PasswordStrength` is stateless: length floor 12 + character-class diversity heuristic across upper/lower/digit/punct|symbol. Length dominates: 16+ chars with 3 classes is `StrengthStrong` even without a 4th class — matches NIST 800-63B § 5.1.1.2. UI consumes via API (Plans 11 / 16 will expose `/api/auth/strength`); the evaluator is never recomputed client-side. JSON-encoding the zero value defaults to "weak" — the safest UI default.
- **Plan 01-07 — Verify wraps every error path with `argon2id:` prefix.** Deviation from plan-verbatim bare `errors.New` / unwrapped `err`. Production logging via slog needs a stable namespace to filter parse failures from unrelated subsystem errors; cost is one `fmt.Errorf` per branch. Plan 09 login handler will treat any non-nil `Verify` error as "invalid credentials" for the user but log the wrapped chain for triage.
- **Plan 01-07 — Long-password DoS cap is Plan 09's responsibility, not Plan 07's.** T-07-05 mitigation note: Plan 09 (login) and Plan 11 (change password) MUST reject `len(password) > 256` BEFORE calling `Hash` / `Verify`. The crypto primitive itself does not enforce a length cap because the cost belongs at the API boundary.
- **Plan 01-07 — `golang.org/x/crypto` promoted from indirect to direct.** Bumped v0.48.0 → v0.50.0; transitively bumped `x/sync` v0.20.0, `x/sys` v0.43.0, `x/text` v0.36.0, added `x/term` v0.42.0. All stdlib-extension packages with stable APIs; no other code changes.
- **Plan 01-08 — Cookie attribute set is locked.** `Name="shifter_session"`, `HttpOnly=true`, `SameSite=Lax`, `Path="/"`, `Domain=""` always; `Secure=!devMode` (D-23). No future plan may flip any of these values without amending D-23 — `SameSite=Strict` would break the SPA login redirect (PITFALL §5); `Domain` set to anything would scope the cookie wider than the issuing host.
- **Plan 01-08 — `auth.User` is the canonical session-bound identity.** Stores ONLY `(ID, Role)`. Plans 09/10/11/14/15 use this struct via `PutUser`/`GetUser`/`UserFromContext`; future plans MUST NOT add complex types or PII (email, display_name, audit fields) to the session payload. Profile data is joined on demand from the `user` table — T-08-06 mitigation. The two role values are `"admin"` and `"viewer"` (matches the `user_role` enum from migration 0002).
- **Plan 01-08 — `GetUser` is panic-safe.** SCS panics with `"scs: no session data in context"` when its context key is absent (bare `context.Background()`, unauthenticated requests). `GetUser` recovers and returns `(zero, false)` instead — matches the comma-ok idiom of stdlib `m[k]`. Required by the plan's own acceptance criterion (`TestGetUser_NoSession_ReturnsZero`); without it every Plan 09/11 handler would need a panic guard around every call.
- **Plan 01-08 — `pgxstore.New(pool)` default 5-minute cleanup is the T-08-05 mitigation.** Production code MUST use the default constructor; `NewWithConfig(CleanUpInterval=0)` is reserved for tests that need to stop the goroutine. The session store reuses the application pgxpool — no second pool, no Redis (D-06 honored).
- **Plan 01-08 — Two rotate entry points.** `PutUser` bundles `Put + RenewToken` for the common login path (Plans 09 + 15); `RotateOnLogin` exposes pure `RenewToken` for Plan 11 password-change where the user blob isn't changing but the session ID must rotate. Both call `sm.RenewToken` under the hood — call sites stay readable. Plan 11 will additionally `DELETE FROM sessions WHERE ...` to invalidate other devices on password change.
- **Plan 01-08 — `sm.LoadAndSave` is wired exactly once, in Plan 09's chi router setup.** No plan beyond 09 should call `LoadAndSave` again; double-wrapping would double-write the cookie. `auth.UserFromContext + ErrNoUser` is the only sanctioned way for handlers to fetch the current user — Plan 10's role middleware and Plan 14's install middleware both build on top of it.
- **Plan 01-08 — `EnsureCSRFToken` ships pre-emptively.** Phase 1 enforcement is `SameSite=Lax + X-Requested-With` (RESEARCH §Security; Plan 06 apiFetch sends the header, Plan 11/15 will reject POST/PUT/DELETE without it). Adding the per-session token now means later phases can adopt token-pair CSRF without a session-data migration. base64.RawURLEncoding chosen for URL-safe transport.
- **Plan 01-09 — Per-IP + per-username login rate limiter via golang.org/x/time/rate.** 5 burst, `rate.Every(time.Minute)` refill. Both buckets must allow before login proceeds. Username key is lowercased before bucket lookup so `Alice@`, `alice@`, `ALICE@` cannot multiply the per-username budget. The cleanup goroutine evicts entries older than 1h every 15m so the map stays bounded under attack. AUTH-04 satisfied.
- **Plan 01-09 — `RetryAfter` cancels its `Reserve()`.** Plan-verbatim used `Reserve().Delay()` without canceling, which would silently consume one token per query — turning the 429 path into a feedback loop where every error response stole an extra attempt from the budget. `Reserve+Cancel` is the canonical query-without-consuming idiom in `golang.org/x/time/rate`. Future rate-limited endpoints (test-connection probes, per-route limiters) MUST follow this pattern.
- **Plan 01-09 — `X-Requested-With: shifter` is required on every state-changing POST.** csrfHeaderPresent(r) check sits at the top of every handler (login, logout, change-password). Combined with SameSite=Lax cookies (Plan 08), this defeats classic cross-site form CSRF without per-request token plumbing. T-09-03 mitigated. Plan 11/14/15/17 handlers MUST start with the same guard.
- **Plan 01-09 — 256-byte password length cap at the API boundary.** `maxPasswordLength = 256` constant in handlers.go. LoginHandler and ChangePasswordHandler reject oversize passwords with 400 BEFORE calling Hash/Verify. Argon2id cost scales with input length; this is the T-07-05 mitigation Plan 07 deferred to the API layer. Plan 11/16 password forms inherit the same cap.
- **Plan 01-09 — Constant-time-ish login: `dummyHash()` Verify on user-not-found.** When `GetUserByEmail` returns ErrUserNotFound, the handler still calls `Verify(password, dummyHash())` so wall-clock between "no such email" and "wrong password" is comparable. dummyHash() is a static valid PHC string (not a runtime-computed hash — computing on start would burn ~20ms per cold start for zero security gain). T-09-02 mitigated.
- **Plan 01-09 — `clientIP` honors X-Forwarded-For first hop.** Caddy / Compose deployments terminate TLS in front of shifter; the operator-controlled reverse proxy is the trust boundary. `clientIP(r)` returns the first XFF entry when present, else `net.SplitHostPort(r.RemoteAddr)`. Plan 22 (Caddyfile) MUST configure XFF correctly. Future rate-limited endpoints reuse this helper.
- **Plan 01-09 — `auth.Store` is the narrow user-table facade.** 5 methods + Pool() accessor: `GetUserByEmail` / `GetUserByID` / `AdminExists` / `InsertAdminUser` / `UpdatePassword`. Email lowercased in BOTH `InsertAdminUser` SQL (`lower($1)`) AND in callers — belt + suspenders defense for the user_email_lowercase CHECK. Disabled users (disabled_at IS NOT NULL) are filtered out — login path treats them as non-existent. Plans 10/11/14/15 MUST import this Store; raw queries against `"user"` from outside the package are forbidden.
- **Plan 01-09 — `iterateAndRevoke` defense-in-depth on password change.** AUTH-05: changing a password drops every OTHER active session for that user. Implementation uses scs.SessionManager.Iterate to decode each session's user_id (SCS payloads are gob-encoded; using SCS's iterator gives us pre-loaded ictx for free) and DELETEs matching tokens via raw pgx. The current session stays valid (operator's own device). T-09-08 mitigated.
- **Plan 01-09 — `ChangePasswordHandler` does NOT rotate the current session token after success.** The session was already authenticated; password just changed → no fixation scenario. Skipping the rotate avoids one extra session-store write. Plan 11 may revisit if a UI need surfaces.
- **Plan 01-09 — `shifter create-admin --reset` refuses to promote viewer → admin.** If a viewer row already exists at the email and the operator runs --reset, the command errors with "user exists but is not an admin (role=viewer) — refusing to promote". Recovery escape hatch must NOT silently change roles; promotion is an explicit operator action that belongs in the future admin UI.
- **Plan 01-09 — `must_change_password=FALSE` is the only path that creates admins.** Both `Store.InsertAdminUser` (create-admin CLI + Plan 15 wizard finish) hardcode `must_change_password=FALSE`. D-09 reframes AUTH-03: bootstrap admins set their own password — there is no force-change UI gate today. TestWizardAdmin_NoForceChange asserts the schema invariant directly so future regressions surface in CI.
- **Plan 01-09 — Testcontainer port flake noted.** Two separate runs in this session hit `postgres dsn: port "5432/tcp" not found` on a single test case under default-parallelism `go test ./internal/... -short`. Re-running the affected test always passed. Per-package runs (`go test ./internal/auth ...`) are stable. Logged to Open Todos for the CI plan to address with `-p 1` or per-package serialization.
- **Plan 01-10 — `roleBundles` is package-private.** External consumers MUST go through `Can()` only — exposing the map would let downstream plans iterate it and accidentally introduce a divergent permission check. Plans 11 / 17 / 18 import only the Action constants and `Can` / `RequireAction` functions; the map stays an implementation detail.
- **Plan 01-10 — 9 Action constants declared (4 Phase 1 + 5 forward-declared Phase 2+).** Per PITFALLS §14: declaring `ActionUserManage` / `ActionDevice{Create,Update,Delete}` / `ActionAuditView` now (alongside the wired-today set `ActionConnectionEdit` / `ActionConnectionTest` / `ActionAccountSelfEdit` / `ActionHealthDetailed`) locks the API surface so Phase 6 USER-04 and Phase 2/3 device CRUD only extend `roleBundles` — call sites already point at the right constants.
- **Plan 01-10 — `Action` / `Role` are string-aliased types, not int enums.** Audit logs (Phase 6) record the action verbatim (`connection.edit`, not `4`); future per-namespace policy can prefix-match by string without an additional registry. Trade-off accepted: a typo in a string literal at a call site won't be caught at compile time, but every plan uses the exported `auth.ActionX` constants so a typo would be on a constant identifier the compiler does check.
- **Plan 01-10 — `RequireAction(sm, action)` is the chi-friendly middleware factory shape `func(http.Handler) http.Handler`.** Plans 11 / 17 / 18 wrap routes via `auth.RequireAction(sm, ActionX)(handler)`; never call `Can()` directly in handlers. The shape composes naturally with `sm.LoadAndSave` (which is also `func(http.Handler) http.Handler`); chi's `r.Method`, `r.With`, and `chi.Chain` all consume this signature.
- **Plan 01-10 — Viewer keeps `connection.test` (probe is read-only) but is denied `connection.edit` / `health.detailed` / `user.manage` / `device.{create,update,delete}` / `audit.view`.** Locked into `roleBundles` source code with inline comments so future plans cannot tighten/loosen by accident. Test Connection probe leaks only "reachable / unreachable" — same info the dashboard already shows; tightening would add friction for zero security benefit.
- **Plan 01-10 — `health.detailed` is admin-only even though `/health` (basic) is unauthenticated.** Plan 18 mounts both: `/health` stays public for monitoring systems, `/health/detailed` (DB connection counts, MQTT broker status, migration version) goes behind `RequireAction(sm, ActionHealthDetailed)` because it leaks operational fingerprint useful to an attacker.
- **Plan 01-10 — `Can(user, action, resource any)` keeps the third parameter even though Phase 1 ignores it.** Reserved for Phase 6 / Phase 7 per-row authz ("user can manage own dashboard") so the body extends without re-signing every call site. The cost is one ignored parameter at every call site (`Can(&u, ActionX, nil)`); the benefit is API stability across two future phases.
- **Plan 01-10 — `RequireAction` returns 401 / 403 with JSON `{"error": "unauthorized" | "forbidden"}`.** Mirrors Plan 09's handler error shape (same `errorResp` JSON envelope from `handlers.go`) but uses a private `writeAuthzError` helper inside `authz.go` rather than re-using `writeJSON`. Self-contained; if a future refactor splits `internal/auth` into sub-packages, `authz.go` does not need to import its sibling. Cost: 4 duplicated lines.
- **Plan 01-11 — Backend-hydrated `/api/account/me`.** AccountInfoHandler reads email + must_change_password from the user table on every call rather than storing them in the session payload (Plan 08's deliberate slimness — id + role only — per T-08-06). Single indexed lookup per protected page nav is cheap; the alternative (denormalize email into the session) would force a session-store migration on every email change. Bonus: `errors.Is(err, ErrUserNotFound)` short-circuits to 401 so a disabled-mid-session admin's next page nav bounces them to /login without an explicit revoke step — symmetric with Plan 09's user-store `disabled_at IS NULL` filter.
- **Plan 01-11 — Loader-thrown redirect + apiFetch redirect overlap is intentional.** rootLoader (`web/src/routes/_root.tsx`) calls fetchSessionUser and `throw redirect('/login?next=...')` on failure. apiFetch ALSO does `window.location.assign('/login?...')` on 401. Both end at the same /login URL; the loader-thrown redirect is the react-router-native control-flow signal that prevents the protected layout from rendering with `useLoaderData() === undefined` between apiFetch's window-level redirect and the browser navigation completing. Belt-and-suspenders: apiFetch handles non-loader 401s (SSE reconnect after session expiry); the loader handles initial-mount 401 cleanly.
- **Plan 01-11 — `web/src/lib/auth.ts` is the canonical client.** Every Phase 1+ feature consuming `/api/auth/*` or `/api/account/*` MUST go through fetchSessionUser / login / logout / changePassword. Raw `fetch('/api/auth/...')` from any other file is forbidden going forward — apiFetch's CSRF header injection (X-Requested-With: shifter, locked at Plan 06) and ApiError envelope are non-negotiable. Plans 16 / 17 / 23 inherit this contract.
- **Plan 01-11 — UI-SPEC verbatim copy lives in JSX, not in a strings table.** Phase 1 is English-only (UX-02 lock). Strings table is V2-I18N-01 ceremony with zero Phase 1 payoff. The future i18n pass is one mechanical sweep replacing literals with `t(key)` calls — easier than maintaining a strings table while it has only one consumer. ChangePasswordDialog title / submit labels / error mapping / strength hint live in `web/src/routes/change-password-dialog.tsx`'s JSX.
- **Plan 01-11 — Dialog-Submit-Error-Toast pattern locked.** Mutation dialogs follow this skeleton: `useState(inputs + busy + error); onSubmit setBusy(true) → try { await mutate(); onSuccess(); onOpenChange(false); reset() } catch ApiError → setError(byStatus(err.status)) finally setBusy(false)`. Parent owns the success toast (sonner) and `revalidator.revalidate()`. Plans 16 (wizard step submits), 17 (Edit Connection), Phase 2 (Add device, Meter swap) MUST follow this shape.
- **Plan 01-11 — Cancel-LEFT, primary-RIGHT footer.** UI-SPEC §Dialog Conventions lock. ResponsiveDialog's `footer` prop receives a `<>...<button cancel /><button primary /></>` fragment — the order in JSX IS the visual order. Every CRUD dialog inherits.
- **Plan 01-11 — AccountInfoHandler is NOT wrapped in `RequireAction(sm, …)`.** The 401-on-no-session path IS the entire authz contract for "self read" — every authenticated user has it. Wrapping in RequireAction would require declaring a `ActionAccountSelfRead` and registering it in `roleBundles` for both roles, which is ceremony with no security gain. Other "every authenticated user" endpoints (V2 saved views, V2 personalization) will reuse this "authenticated → 200, unauthenticated → 401, no role gate" pattern. State-changing self-edit endpoints (e.g. POST /api/account/password from Plan 09) still get RequireAction(sm, ActionAccountSelfEdit) — the read/write split is the boundary.
- **Plan 01-11 — `Object.defineProperty(window, 'location', { configurable: true, writable: true, value: { ...window.location, assign: vi.fn() } })` is the canonical jsdom 29 location stub.** jsdom 29 sealed `window.location.assign` (non-configurable accessor); direct `window.location.assign = vi.fn()` throws in strict mode. The whole-object replacement keeps the spy interceptable. Reused in any future test that asserts apiFetch's 401 redirect path; lives in `web/src/lib/auth.test.ts`'s `stubLocationAssign()` helper.
- **Plan 01-11 — Sonner success toast string is verbatim "Password changed".** UI-SPEC §"Phase 1 copy table" locks it. Resisting the upgrade to "Password changed successfully" / "Your password has been updated" — the shorter form is louder, and richColors styling already implies success via the green check. Same principle applies to all future operator-facing success toasts: terse + verbatim.
- **Plan 01-11 — RootLayout success-side calls `revalidator.revalidate()` on password change even though the response payload doesn't change.** Pattern locks for mutations that DO produce new server state (e.g. Plan 16 region pick → revalidates capabilities; Plan 17 connection edit → revalidates /api/health). The cost on Plan 11 (one extra `/api/account/me` round-trip) is negligible; the consistency win is "every mutation dialog ends with a revalidator pulse."

### Open Todos

- **Plan 02 — Biome OOM workaround.** `pnpm exec biome` is OOM'ing the linter daemon in this sandbox. Investigate `BIOME_LOG_PATH` / heap flags or fall back to `biome ci` mode if pre-commit hooks fail. *(Carried from Plan 01-01; Plan 02 did not need biome at runtime, deferring resolution to whichever plan first wires biome into pre-commit/CI.)*
- **CI plan — Node version >=22.13.** jsdom 29 (vitest worker) requires Node 22.13+ even though the project floor is 22.12. Whichever plan lands the GitHub Actions / CI config must pin the runner image accordingly.
- **Plan 12 — `mockgen` on PATH.** `just bootstrap` should add `$(go env GOPATH)/bin` to PATH or document the requirement so contributors don't get "mockgen not found" after `go install`.
- **Plan-check enhancement — verify command wording.** Plans whose `<verify>` uses `grep -q 'PASS'` against `go test ./...` (non-verbose) silently fail; either use `-v` mode or change the assertion to `grep -E 'PASS|ok\s'`. Flag during plan-check.
- **Plan-check enhancement — depends_on accuracy.** Plan 05's frontmatter declared `depends_on: [01, 02]` but the plan's task code requires Plan 04's outputs (config.Load, logging.New, version.Info). Future plan-check passes should grep for cross-package imports (`internal/config`, `internal/logging`, `internal/version`) and require the providing plan to be in `depends_on`.
- **Plan 24 — Justfile build recipe.** Update `just build` to use the production -ldflags invocation documented in `internal/version/version.go`'s package comment so release artifacts ship with real Version / Commit / BuildTime.
- **Plan 18 — Cobra completion subcommand visibility.** `shifter --help` lists `completion` (Cobra's auto-registered shell completion). Decide whether to keep visible (useful for ops), hide via `rootCmd.CompletionOptions.DisableDefaultCmd = true`, or move to a `tools` group.
- **Plan 19 — Bundle size review.** Frontend bundle jumped from 193 KB to 463 KB after Plan 06 (react-router-dom v7 + @tanstack/react-query + radix primitives). Plan 19 (spa-embed) should consider route-level code splitting if the size becomes a concern at install time.
- **CI plan — Testcontainer port-mapping race.** Default-parallel `go test ./internal/... -short` occasionally fails one test case with `postgres dsn: port "5432/tcp" not found` when many TimescaleDB containers spin up simultaneously. Re-running the affected test always passes; per-package runs are stable. CI plan should use `-p 1` or per-package serialization for full-suite verification.
- **Bootstrap docs — Node version pre-flight check.** Local Node `<22.12` (e.g. 22.11) silently breaks vitest's forks pool with `ERR_REQUIRE_ESM` from `html-encoding-sniffer@6.0.0` requiring `@exodus/bytes`'s ESM `encoding-lite.js` — Node 22.12+ added the `node:diagnostics_channel` `TracingChannel.traceSync` interop machinery jsdom 29 transitively depends on. Reproduced during Plan 11 resume. The .nvmrc=22.12 floor is correct; the operator's shell needs `nvm use` (or fnm/asdf equivalent) before `pnpm test:run`. Plan 24 (readme-docs) or whichever plan lands developer onboarding instructions should document an explicit `node --version` pre-flight check or wire a Justfile recipe (`just bootstrap-check`) that fails fast on a stale local Node.

### Open Blockers

(none)

### Pre-Flight Notes

- **PROJECT.md wording fix pending:** "Pixel-coordinate device placement" should read "normalized fractional coordinates on the floor plan" — apply during Phase 1 or Phase 5, whichever lands the floor-plan storage code first.
- **Research flags for downstream planning:** Phases 2, 3, 5, 6, 7 should run `/gsd-research-phase` before planning (see ROADMAP.md "Research Flags"). Phases 1 and 4 use standard patterns.
- **AS923 sub-plan default:** Region picker in Phase 1 install wizard pre-selects the Thailand-correct sub-plan. This is a Phase 1 acceptance check.

## Session Continuity

**If resuming after interruption:**

1. Re-read `.planning/PROJECT.md` (core value + constraints)
2. Re-read `.planning/REQUIREMENTS.md` (v1 scope + traceability)
3. Re-read `.planning/ROADMAP.md` (phase structure + success criteria)
4. Re-read `.planning/research/SUMMARY.md`, `ARCHITECTURE.md`, `PITFALLS.md` for technical context
5. Run `/gsd-plan-phase 1` to begin Phase 1 planning

**Files of record:**

- `.planning/PROJECT.md` — vision + constraints + key decisions
- `.planning/REQUIREMENTS.md` — v1 + v2 + out-of-scope + traceability
- `.planning/ROADMAP.md` — 7-phase structure with success criteria
- `.planning/STATE.md` — this file (current position + accumulated context)
- `.planning/research/SUMMARY.md` — research synthesis
- `.planning/research/ARCHITECTURE.md` — component dependencies
- `.planning/research/PITFALLS.md` — pitfall→phase mapping
- `.planning/config.json` — granularity + workflow settings

---
*State initialized: 2026-04-27 after roadmap creation*
*Last session: 2026-04-28T01:42Z — Stopped at: Completed 01-11-account-ui-PLAN.md*
