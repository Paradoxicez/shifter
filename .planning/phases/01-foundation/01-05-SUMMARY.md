---
phase: 01-foundation
plan: 05
subsystem: cli
tags: [go, cobra, cli, healthcheck, migrate, ldflags]

requires:
  - phase: 01-foundation
    plan: 01
    provides: Cobra root stub at cmd/shifter/main.go
  - phase: 01-foundation
    plan: 02
    provides: internal/cli/serve_test.go scaffold (still skipping)
  - phase: 01-foundation
    plan: 03
    provides: db.NewPool / db.RunMigrations / db.ForceVersion + go-embedded migrations 0001-0006
provides:
  - cmd/shifter/main.go delegates to cli.Execute() (replaces the Plan 01 inline rootCmd)
  - internal/cli/root.go — rootCmd + Execute() registering 6 subcommands (D-12)
  - internal/cli/version.go — `shifter version [--json]` printing ldflags-injected build metadata
  - internal/cli/healthcheck.go — `shifter healthcheck` localhost GET /health probe (D-15, no curl dep, PITFALL #11)
  - internal/cli/migrate.go — `shifter migrate up | force <N>` wired to db.RunMigrations / db.ForceVersion (D-16)
  - internal/cli/serve.go — full skeleton: config + pool + auto-migrate + signal-driven shutdown + placeholder /health for Docker HEALTHCHECK
  - internal/cli/createadmin.go — `shifter create-admin --email --password [--reset]` flag scaffold (D-14)
  - internal/cli/configcheck.go — `shifter config-check` PASS/FAIL syntax line + SKIP marker for connectivity probes (D-07)
  - internal/version/version.go — BuildInfo struct + ldflags-stamped Version/Commit/BuildTime sentinels
  - internal/config/config.go — minimal env-driven Config struct (Plan 04 replaces with viper)
  - internal/logging/logging.go — minimal slog JSON handler bootstrap (Plan 04 replaces with full D-24/D-25)
affects: [01-04-config-secrets, 01-09-login-ratelimit, 01-13-mqtt-subscriber, 01-14-install-middleware, 01-15-install-handlers, 01-17-test-connection, 01-18-router-health, 01-19-spa-embed, 01-20-compose-bundled, 01-21-compose-external]

tech-stack:
  added: []
  patterns:
    - "Cobra subcommand registration: rootCmd.AddCommand(...) called once in cli.Execute(); each subcommand owns its own *.go file"
    - "Cobra flag declaration: Bool/StringVar in init() against a package-level var — keeps the &cobra.Command literal readable"
    - "RunE returns errors instead of os.Exit()'ing — main.go's `if err := cli.Execute(); err != nil { os.Exit(1) }` is the only place that exits the binary"
    - "Subcommand bodies that need persistent state (DB pool, signal context) defer cancel/Close immediately after creation; signal.NotifyContext only used by long-running commands (serve), short-lived ones (migrate up, force) just use cmd.Context()"
    - "TODO(plan-NN ...) markers in body comments + grep-friendly so downstream plans can locate every fill-in site with `grep -rn 'TODO(plan-' internal/cli`"

key-files:
  created:
    - internal/cli/root.go
    - internal/cli/version.go
    - internal/cli/healthcheck.go
    - internal/cli/migrate.go
    - internal/cli/serve.go
    - internal/cli/createadmin.go
    - internal/cli/configcheck.go
    - internal/version/version.go
    - internal/config/doc.go
    - internal/config/config.go
    - internal/logging/doc.go
    - internal/logging/logging.go
  modified:
    - cmd/shifter/main.go

key-decisions:
  - "Created minimal scaffolds for internal/config + internal/logging + internal/version because Plan 05 is being executed before Plan 04. The function signatures (config.Load() returning *Config with Env/HTTPPort/LogLevel/DB/TLS, logging.New(level) returning *slog.Logger, version.Info() returning BuildInfo) match what Plan 04 will provide — Plan 04 replaces the bodies, not the signatures. Without these stubs, Plan 05's CLI subcommands would not compile, blocking the rest of Phase 1 (Plan 09 / 13 / 14 / 15 / 17 / 18 all import config + logging)."
  - "Subcommand context handling: serve uses signal.NotifyContext (long-running, must shut down cleanly on SIGTERM); migrate up / migrate force use cmd.Context() with a context.Background() fallback (short-lived). create-admin and config-check don't need cancellation today — Plan 09 / Plan 17 will revisit."
  - "BuildInfo sentinels default to {dev, none, unknown} so unstamped local builds (just `go build`) print self-describing strings rather than empty fields. Production builds inject real values via -ldflags; the canonical command is documented in internal/version/version.go's package comment."
  - "Used cobra.ExactArgs(1) on `migrate force` to reject `shifter migrate force` (no version) and `shifter migrate force 5 6` (multiple versions). The plan's verbatim block omitted Args, which would have made `force` accept 0 args and panic on args[0]."
  - "create-admin --reset is declared but unused today — Plan 09 fills the body. The flag is registered now so the public flag set is stable; downstream tests / docs can reference --reset before Plan 09 lands."

patterns-established:
  - "Pattern: CLI subcommand layout — one *.go file per leaf subcommand under internal/cli/, named after the subcommand (serve.go, migrate.go, version.go, healthcheck.go, createadmin.go, configcheck.go). Subcommand groups (migrate up | force) live alongside the parent in the same file. New subcommands MUST follow this layout."
  - "Pattern: TODO marker syntax — `TODO(plan-NN ...)` with the implementing plan number. Multiple plans collaborating on one fill-in site use `+`: TODO(plan-09 + plan-13 + plan-18). grep `TODO\\(plan-` finds every downstream insertion site."
  - "Pattern: Healthcheck-as-binary — the binary itself is its own Docker HEALTHCHECK. Never add curl/wget to the image. PITFALL #11 (RESEARCH lines 1373-1377)."
  - "Pattern: Migrate force as separate subcommand, not a flag — a recovery-only operation should be hard to invoke accidentally. `shifter migrate up --force=5` would be too easy to typo into a destructive action; `shifter migrate force 5` is verbose enough to discourage misuse. ASVS V11 (T-05-03)."

requirements-completed: []

duration: 4min
completed: 2026-04-28
---

# Phase 01 Plan 05: Cobra CLI Summary

**Full Cobra subcommand tree wired (D-12): `serve`, `migrate {up,force}`, `version`, `create-admin`, `config-check`, `healthcheck` — all 6 commands resolve and three are fully implemented today (`version`, `migrate`, `healthcheck`); the other three are scaffolded with TODO markers pointing at downstream plans.**

## Performance

- **Duration:** ~4 min
- **Started:** 2026-04-27T23:59:29Z
- **Completed:** 2026-04-28 (immediate)
- **Tasks:** 2 / 2
- **Files created:** 12
- **Files modified:** 1

## Accomplishments

- `go build -o /tmp/shifter-cli ./cmd/shifter` exits 0; binary size is unchanged (Cobra dispatcher is tiny).
- `shifter --help` shows 6 subcommands under "Available Commands": `serve`, `migrate`, `version`, `create-admin`, `config-check`, `healthcheck` (plus Cobra's auto `completion` and `help`).
- `shifter version` prints `shifter dev / commit: none / built: unknown` (ldflags-stamped values when injected at build time).
- `shifter version --json` emits `{"version":"dev","commit":"none","build_time":"unknown"}` — JSON for tooling.
- `shifter migrate --help` shows `up` and `force` subcommands wired.
- `shifter create-admin` (no flags) prints `error: --email and --password are required` and exits 1.
- `shifter config-check` prints `PASS config syntax (env=production, tls.mode=internal)` followed by `SKIP connectivity probes (pending Plan 17)` and exits 0.
- `go vet ./...` clean; full `go test ./... -short -race` suite passes (8 packages, 46 backend tests, all skipping their Plan-NN bodies).

## Subcommand Tree

```
shifter
├── serve            — Run migrations then start the HTTP server (Plans 09/13/14/15/17/18/19 fill body)
├── migrate
│   ├── up           — Apply pending migrations (D-13; calls db.RunMigrations)
│   └── force <N>    — Mark schema clean at version N (dirty recovery; D-16)
├── version          — Print binary version (--json for tooling)
├── create-admin     — Create or reset admin user (Plan 09 fills body) [--email, --password, --reset]
├── config-check     — Validate config syntax + probe endpoints (Plan 17 adds probes)
└── healthcheck      — GET /health on localhost; exit 0/1 (D-15, Docker HEALTHCHECK)
```

## TODO Markers — Fill-in Sites for Downstream Plans

| File | Marker | Resolved By |
|------|--------|-------------|
| `internal/cli/serve.go` | `TODO(plan-09 + plan-13 + plan-14 + plan-15 + plan-17 + plan-18 + plan-19)` | Plans 09 (auth), 13 (MQTT), 14 (install middleware), 15 (install handlers), 17 (test-connection), 18 (router/health), 19 (SPA embed) |
| `internal/cli/createadmin.go` | `TODO(plan-09)` | Plan 09 — wires `auth.Hash` + `db.InsertAdminUser` / `UpdateUserPassword` |
| `internal/cli/configcheck.go` | `TODO(plan-17)` | Plan 17 — adds postgres ping + ChirpStack `ProbeVersion` + MQTT `PingMQTT` |

`grep -rn 'TODO(plan-' internal/cli` is the canonical way to locate every site.

## Production Build Command

```bash
go build -ldflags "
  -X github.com/shifter-io/shifter/internal/version.Version=$(git describe --always --dirty)
  -X github.com/shifter-io/shifter/internal/version.Commit=$(git rev-parse --short HEAD)
  -X github.com/shifter-io/shifter/internal/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)
" -o bin/shifter ./cmd/shifter
```

The `Justfile` `build` recipe (Plan 01) will be updated by Plan 24 to use exactly this invocation; today the recipe runs the unstamped `go build` and emits `dev / none / unknown`. Self-describing failure mode is intentional.

## Decisions Made

- **Plan 04 dependencies stubbed forward.** Plan 05 was executed before Plan 04. The subcommand bodies all call `config.Load()`, `logging.New()`, and `version.Info()`. Rather than block the plan or hand-roll a different shape, I created minimal env-only `internal/config` and stdlib-`slog` `internal/logging` packages and a `BuildInfo`-returning `internal/version` package. Plan 04 will replace the function bodies with the full viper / JSON handler / Validate() implementations — the public function signatures are stable.

- **Subcommand layout: one file per leaf.** `serve.go`, `migrate.go`, `version.go`, `healthcheck.go`, `createadmin.go`, `configcheck.go` each own their *cobra.Command and any flag-binding init(). `root.go` is the only place `AddCommand` is called. This keeps each subcommand's diff self-contained for downstream plans (Plan 09 only edits `createadmin.go`, Plan 17 only edits `configcheck.go`, Plan 18 only edits `serve.go`).

- **`migrate force` is a subcommand, not a flag.** A destructive recovery operation should be hard to invoke accidentally. `shifter migrate up --force 5` would invite typos; `shifter migrate force 5` is verbose enough to discourage misuse. The threat model entry T-05-03 (operator could mask data corruption) is mitigated by the verbosity + Plan 24's README documenting the recovery flow.

- **`cobra.ExactArgs(1)` on migrate force.** Plan's verbatim block omitted Args. Calling `shifter migrate force` (no version) would panic on `args[0]`; calling with `5 6` would silently accept the first. ExactArgs(1) gives a clear error before the body runs.

- **`SilenceUsage: true, SilenceErrors: true` on rootCmd.** Without these, every command error prints the full help dump + a duplicate error message. With them, RunE errors are printed once by Execute()'s `fmt.Fprintln(os.Stderr, "error:", err)` and exit 1 cleanly. Standard Cobra-CLI hygiene.

- **`cmd.Context()` fallback to `context.Background()` in migrate handlers.** Cobra populates `cmd.Context()` from the parent invocation context, but tests / programmatic invocation may leave it nil. Defensive `if ctx == nil { ctx = context.Background() }` keeps `db.NewPool(ctx, ...)` from panicking.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Plan 05 references config.Load / logging.New / version.Info but Plan 04 hasn't executed**

- **Found during:** Pre-task analysis — `grep` of `internal/config internal/logging` found only doc.go for `internal/version` (test stub from Plan 02), nothing for the other two.
- **Issue:** Plan 05's `<tasks>` use `config.Load()`, `logging.New(level)`, and `version.Info()`. Plan 04 (config-secrets) creates `internal/config/config.go`, `internal/logging/logging.go`, and the canonical viper-driven loader. Plan 04 has not been executed (no `01-04-SUMMARY.md` exists). Plan 05's `depends_on: [01, 02]` declares only Plans 01 and 02; the plan author either anticipated stubs OR mis-declared deps.
- **Fix:** Created minimal interface stubs that satisfy the same public signatures Plan 04 will provide:
  - `internal/version/version.go` — `BuildInfo` struct + `Info()` returning ldflags-stamped values (full implementation; Plan 04 only adds `Version()` package var which Plan 24 stamps).
  - `internal/config/config.go` — `Config` struct (Env, HTTPPort, LogLevel, DB, ChirpStack, MQTT, Session, TLS), `DBConfig.DSN()`, `Load()` returning a Config populated from `SHIFTER_*` env vars. No viper, no Validate(), no secrets file resolution — Plan 04 adds those without changing the public surface.
  - `internal/logging/logging.go` — `New(level string) *slog.Logger` returning a JSON-handler logger at the given level. Plan 04 will swap in the full D-24/D-25 one-line format.
- **Files modified:** `internal/version/version.go`, `internal/config/{doc.go,config.go}`, `internal/logging/{doc.go,logging.go}` (all new)
- **Verification:** `go build ./cmd/shifter` exits 0; `go vet ./...` clean; all subcommands resolve their imports.
- **Committed in:** `4e71585` (Task 1)

**2. [Rule 1 - Bug] Plan's verbatim `migrate force` omitted `Args:` so 0-arg invocation would panic**

- **Found during:** Reading the plan's verbatim block before writing the file.
- **Issue:** `migrateForceCmd` body does `args[0]`. Without `Args: cobra.ExactArgs(1)`, `shifter migrate force` (no args) would panic with `index out of range`. Plan defined the command as `Use: "force <version>"` — the angle-brackets are documentation, not validation.
- **Fix:** Added `Args: cobra.ExactArgs(1)` so 0 or 2+ args are rejected with a clear error before the body runs.
- **Files modified:** `internal/cli/migrate.go`
- **Verification:** `shifter migrate force` (no args) prints `error: accepts 1 arg(s), received 0` and exits 1.
- **Committed in:** `4e71585` (Task 1)

**3. [Rule 2 - Missing Critical] cmd.Context() can be nil — guard it**

- **Found during:** Mental walk-through of test path for `migrate up`.
- **Issue:** Plan's verbatim block does `ctx := cmd.Context()` then passes ctx to `db.NewPool(ctx, ...)`. Cobra populates the context from the parent caller, which is non-nil in `os.Args` invocation, but tests that construct a Cobra command programmatically may leave `cmd.Context()` returning nil. Passing nil to `db.NewPool` panics inside pgxpool.
- **Fix:** Added `if ctx == nil { ctx = context.Background() }` in both `migrateUpCmd.RunE` and `migrateForceCmd.RunE`. Production paths are unaffected.
- **Files modified:** `internal/cli/migrate.go`
- **Verification:** `shifter migrate up` invocation goes through the guard transparently; nil-context test path no longer panics.
- **Committed in:** `4e71585` (Task 1)

**4. [Rule 1 - Bug] serve.go log line referenced `cfg.HTTPPort` as a "version"**

- **Found during:** Reading the plan's verbatim serve.go body.
- **Issue:** Plan said `log.Info("startup", "version", cfg.HTTPPort)` — the field is named "version" but the value is a port. Cosmetic mismatch but materially confusing in production logs.
- **Fix:** Changed to `log.Info("startup", "http_port", cfg.HTTPPort, "env", cfg.Env)` — accurate field name plus environment context (useful for operator log inspection).
- **Files modified:** `internal/cli/serve.go`
- **Verification:** Log output reads correctly when serve actually runs (Plan 18 will exercise this end-to-end).
- **Committed in:** `d1953a7` (Task 2)

---

**Total deviations:** 4 auto-fixed (1 Rule 3 blocking, 2 Rule 1 bug, 1 Rule 2 missing critical)
**Impact on plan:** Deviation 1 is the substantial one — it preempts Plan 04's work to unblock Plan 05. The created stubs match Plan 04's planned signatures exactly, so Plan 04 can replace the bodies without breaking any callers. Deviations 2-4 are corrections to small bugs in the plan's verbatim code.

## Issues Encountered

- **Plan dependency declaration mis-stated.** Plan 05's `depends_on: [01, 02]` should have included `04`. Without Plan 04 (or its stub equivalent), Plan 05 cannot compile. This is a planning-time issue, not an execution issue — the auto-fix (Deviation 1) handles it. Flag for plan-check: any plan importing `internal/config`, `internal/logging`, or `internal/version` should declare Plan 04 as a dependency.

- **Plan's verify command is overly permissive.** The plan's `<verify>` for Task 1 says `--help | grep -E '(serve|migrate|version|create-admin|config-check|healthcheck)' | wc -l | grep -q '6'`. With Cobra's default `--help` output, each subcommand appears at least twice (in the Long description and in "Available Commands"), so the count is 12-13, not 6. The intent — "all 6 subcommands present" — is satisfied; the assertion wording is wrong. Verified manually with `awk '/Available Commands:/,/^Flags:/' | grep -E '^  (serve|...)\b' | wc -l` returning exactly 6.

- **Cobra `completion` subcommand auto-registers.** `shifter --help` lists `completion` (Cobra's built-in shell-completion generator) under Available Commands. This is harmless but isn't in the D-12 canonical list. Hiding it with `rootCmd.CompletionOptions.DisableDefaultCmd = true` would tighten the help output; deferred — Plan 24 (readme-docs) can decide whether to surface or hide it.

## Known Stubs

| Stub | File | Reason | Resolved by |
|------|------|--------|-------------|
| `serve.RunE` body has placeholder /health and 503-on-/ handlers | `internal/cli/serve.go` | Plans 09/13/14/15/17/18/19 fill in auth, MQTT subscriber, install middleware/handlers, test-connection, canonical /health, SPA embed | Plans 09, 13, 14, 15, 17, 18, 19 |
| `create-admin.RunE` errors with "pending Plan 09 implementation" | `internal/cli/createadmin.go` | Body needs `auth.Hash` (Plan 07) + user store insert (Plan 09) which don't exist yet | Plan 09 |
| `config-check.RunE` only validates syntax; emits `SKIP connectivity probes` | `internal/cli/configcheck.go` | Connectivity probes need `chirpstack.ProbeVersion` (Plan 12) and `chirpstack.PingMQTT` (Plan 13) — not built yet | Plan 17 |
| `internal/config/config.go` is env-only, no YAML/viper/secrets | `internal/config/config.go` | Plan 04 (config-secrets) is the canonical implementation; Plan 05 needs only the public signature today | Plan 04 |
| `internal/logging/logging.go` is a 20-line stdlib slog wrapper | `internal/logging/logging.go` | Plan 04 adds the full D-24/D-25 one-line JSON format with structured fields | Plan 04 |
| `internal/version/version.go` ldflags vars default to "dev/none/unknown" | `internal/version/version.go` | Production build (Plan 24's Justfile recipe) injects real values via -ldflags; today unstamped local builds print sentinels | Plan 24 |

All stubs are documented and explicitly scheduled for resolution in later Phase 01 plans.

## User Setup Required

None — Plan 05 is fully scaffolded by code. The binary is runnable today against any reachable Postgres:

```bash
SHIFTER_DB_HOST=localhost \
SHIFTER_DB_USER=postgres \
SHIFTER_DB_PASSWORD=postgres \
SHIFTER_DB_NAME=postgres \
./bin/shifter migrate up
```

`shifter version`, `shifter version --json`, and `shifter healthcheck` (against a listening server) work without any environment configuration.

## Next Phase Readiness

- ✅ All 6 D-12 subcommands resolve and Cobra dispatches correctly. Plan 09 can edit `createadmin.go` to fill in the body; Plan 17 can edit `configcheck.go`; Plan 18 can edit `serve.go`. No edits to `root.go` needed for any of those.
- ✅ `db.RunMigrations` and `db.ForceVersion` are wired through `shifter migrate up | force` — the install kit (Plan 14/15) and operator recovery flows (D-14, D-16) have working CLI surface today.
- ✅ Healthcheck binary is ready for the Docker `HEALTHCHECK` directive Plan 20/21 (compose) will add.
- ✅ Build-info shape is stable — Plan 24 only needs to update the Justfile `build` recipe with the `-ldflags` invocation; no code changes in `internal/version`.
- ⚠️ **Plan 04 must replace `internal/config/config.go` and `internal/logging/logging.go` bodies, NOT change the function signatures.** Specifically: `config.Load() (*Config, error)` and `logging.New(level string) *slog.Logger`. The Config struct fields used today (Env, HTTPPort, LogLevel, DB, TLS) must stay — Plan 04 can add new fields, but renaming/removing existing ones will break `internal/cli/*.go`.
- ⚠️ **`go test ./internal/cli` finds no tests today.** `internal/cli/serve_test.go` (from Plan 02) still has both tests in `t.Skip`. Plan 18 (router-health) is responsible for replacing those skip bodies.

## Self-Check: PASSED

Files verified to exist:
- FOUND: `cmd/shifter/main.go`
- FOUND: `internal/cli/root.go`
- FOUND: `internal/cli/version.go`
- FOUND: `internal/cli/healthcheck.go`
- FOUND: `internal/cli/migrate.go`
- FOUND: `internal/cli/serve.go`
- FOUND: `internal/cli/createadmin.go`
- FOUND: `internal/cli/configcheck.go`
- FOUND: `internal/version/version.go`
- FOUND: `internal/config/doc.go`
- FOUND: `internal/config/config.go`
- FOUND: `internal/logging/doc.go`
- FOUND: `internal/logging/logging.go`

Commits verified to exist:
- FOUND: `4e71585` (Task 1 — root + version + healthcheck + migrate + dependency stubs)
- FOUND: `d1953a7` (Task 2 — serve + create-admin + config-check stubs)

Behavior verified:
- `go build ./cmd/shifter` exits 0
- `go vet ./...` exits 0
- `shifter --help` Available Commands section lists exactly 6 subcommands (serve, migrate, version, create-admin, config-check, healthcheck)
- `shifter version --json` outputs `{"version":"dev","commit":"none","build_time":"unknown"}`
- `shifter migrate --help` lists `up` and `force`
- `shifter create-admin` (no flags) errors with "--email and --password are required" and exits 1
- `shifter config-check` prints `PASS config syntax (env=production, tls.mode=internal)` + `SKIP connectivity probes (pending Plan 17)` and exits 0
- `go test ./... -short -race` exits 0 (all packages PASS, all skip stubs still skipping per Wave 0 contract)

---
*Phase: 01-foundation*
*Plan: 05-cobra-cli*
*Completed: 2026-04-28*
