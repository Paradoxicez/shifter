---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
status: Ready to execute
last_updated: "2026-04-28T00:43:47.198Z"
progress:
  total_phases: 7
  completed_phases: 0
  total_plans: 24
  completed_plans: 7
  percent: 29
---

# Project State: Shifter

**Last Updated:** 2026-04-28 (after Plan 01-07 execution — Argon2id Hash/Verify + PasswordStrength shipped; AUTH-01 unblocked)

## Project Reference

**Core Value:** The operator runs their entire LoRaWAN water/electricity monitoring operation — provisioning, placement, monitoring, reporting — from Shifter alone, and meter swaps never break historical continuity.

**Current Focus:** Phase 01 — foundation

## Current Position

Phase: 01 (foundation) — EXECUTING
Plan: 7 of 24 complete (Plans 01, 02, 03, 04, 05, 06, 07)

| Field | Value |
|-------|-------|
| **Phase** | 1 — Foundation |
| **Plan** | 08 — session-manager (next) |
| **Status** | Plans 01–07 complete; Plan 07 shipped Argon2id Hash/Verify with PHC encoding (OWASP-2025 params m=19456 / t=2 / p=1 / salt=16 / key=32) plus a stateless PasswordStrength tier evaluator. AUTH-01 unblocked for Plan 09 login handler and Plan 11 change-password. No bcrypt anywhere; subtle.ConstantTimeCompare grep-verified. |
| **Progress (plans)** | `[███░░░░░░░] 7/24 (29%)` |
| **Progress (phases)** | `[░░░░░░░░░░] 0/7 phases` |

**Next action:** `/gsd-execute-plan 01 08` (or `/gsd-execute-phase 01` to continue the chain)

## Performance Metrics

| Metric | Value |
|--------|-------|
| Phases complete | 0 / 7 |
| v1 requirements mapped | 99 / 99 (100%) |
| Plans complete | 7 / 24 (01, 02, 03, 04, 05, 06, 07) |
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

### Open Todos

- **Plan 02 — Biome OOM workaround.** `pnpm exec biome` is OOM'ing the linter daemon in this sandbox. Investigate `BIOME_LOG_PATH` / heap flags or fall back to `biome ci` mode if pre-commit hooks fail. *(Carried from Plan 01-01; Plan 02 did not need biome at runtime, deferring resolution to whichever plan first wires biome into pre-commit/CI.)*
- **CI plan — Node version >=22.13.** jsdom 29 (vitest worker) requires Node 22.13+ even though the project floor is 22.12. Whichever plan lands the GitHub Actions / CI config must pin the runner image accordingly.
- **Plan 12 — `mockgen` on PATH.** `just bootstrap` should add `$(go env GOPATH)/bin` to PATH or document the requirement so contributors don't get "mockgen not found" after `go install`.
- **Plan-check enhancement — verify command wording.** Plans whose `<verify>` uses `grep -q 'PASS'` against `go test ./...` (non-verbose) silently fail; either use `-v` mode or change the assertion to `grep -E 'PASS|ok\s'`. Flag during plan-check.
- **Plan-check enhancement — depends_on accuracy.** Plan 05's frontmatter declared `depends_on: [01, 02]` but the plan's task code requires Plan 04's outputs (config.Load, logging.New, version.Info). Future plan-check passes should grep for cross-package imports (`internal/config`, `internal/logging`, `internal/version`) and require the providing plan to be in `depends_on`.
- **Plan 24 — Justfile build recipe.** Update `just build` to use the production -ldflags invocation documented in `internal/version/version.go`'s package comment so release artifacts ship with real Version / Commit / BuildTime.
- **Plan 18 — Cobra completion subcommand visibility.** `shifter --help` lists `completion` (Cobra's auto-registered shell completion). Decide whether to keep visible (useful for ops), hide via `rootCmd.CompletionOptions.DisableDefaultCmd = true`, or move to a `tools` group.
- **Plan 19 — Bundle size review.** Frontend bundle jumped from 193 KB to 463 KB after Plan 06 (react-router-dom v7 + @tanstack/react-query + radix primitives). Plan 19 (spa-embed) should consider route-level code splitting if the size becomes a concern at install time.

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
*Last session: 2026-04-28T00:42Z — Stopped at: Completed 01-07-argon2id-PLAN.md*
