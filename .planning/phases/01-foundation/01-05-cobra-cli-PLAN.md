---
phase: 01-foundation
plan: 05
type: execute
wave: 3
depends_on: [01, 02]
files_modified:
  - cmd/shifter/main.go
  - internal/cli/root.go
  - internal/cli/version.go
  - internal/cli/migrate.go
  - internal/cli/configcheck.go
  - internal/cli/healthcheck.go
  - internal/cli/createadmin.go
  - internal/cli/serve.go
autonomous: true
requirements: []
must_haves:
  truths:
    - "shifter --help lists all 6 subcommands: serve, migrate, version, create-admin, config-check, healthcheck (D-12)"
    - "shifter version prints the version, commit, build-time"
    - "shifter healthcheck makes a GET against http://localhost:$PORT/health and exits 0/1 (D-15)"
    - "shifter migrate up runs migrations and prints applied count (D-13)"
    - "shifter config-check loads config and prints PASS/FAIL per check (D-07; full impl deferred to Plan 17 connectivity probes)"
    - "shifter serve and shifter create-admin exist as command stubs that wire their dependencies but defer implementation to Plans 09/15 (auth) and 14/15 (install) "
  artifacts:
    - path: "cmd/shifter/main.go"
      provides: "Calls cli.Execute() (replaces the stub from Plan 01)"
      contains: "cli.Execute"
    - path: "internal/cli/root.go"
      provides: "Cobra root command with persistent --config flag"
      contains: "rootCmd"
    - path: "internal/cli/version.go"
      provides: "shifter version subcommand"
      contains: "versionCmd"
    - path: "internal/cli/healthcheck.go"
      provides: "shifter healthcheck (D-15) — localhost GET /health, exits 0/1"
      contains: "healthcheckCmd"
    - path: "internal/cli/migrate.go"
      provides: "shifter migrate up | down | force | version (D-16)"
      contains: "migrateCmd"
    - path: "internal/cli/configcheck.go"
      provides: "shifter config-check (D-07) — config syntax pass; connectivity probes added in Plan 17"
      contains: "configCheckCmd"
    - path: "internal/cli/createadmin.go"
      provides: "shifter create-admin --email --password [--reset] (D-14) — full implementation in Plan 09"
      contains: "createAdminCmd"
  key_links:
    - from: "cmd/shifter/main.go"
      to: "internal/cli"
      via: "package main calls cli.Execute()"
      pattern: "cli\\.Execute"
    - from: "internal/cli/healthcheck.go"
      to: "/health endpoint"
      via: "net/http GET to localhost:$HTTP_PORT/health"
      pattern: "http\\.Get"
    - from: "internal/cli/migrate.go"
      to: "internal/db.RunMigrations / ForceVersion"
      via: "imports + invokes"
      pattern: "db\\.RunMigrations"
---

<objective>
Wire the full Cobra CLI tree (D-12): `serve`, `migrate`, `version`, `create-admin`, `config-check`, `healthcheck`. Each subcommand is registered, parses flags, and calls into `internal/db`, `internal/config`, `internal/version`. The `serve` command file is a skeleton — its body is filled by later plans (mostly Plan 09 + Plan 14 wiring + Plan 18). `create-admin` body is filled by Plan 09. `config-check` connectivity probes are filled by Plan 17.

Purpose: D-12 (CLI surface), D-13 (`serve` auto-migrates), D-14 (`create-admin` recovery), D-15 (`healthcheck` for Docker `HEALTHCHECK`). Without this skeleton, the binary has nothing to run.

Output: `shifter --help` shows the canonical subcommand tree; `shifter version`, `shifter migrate up`, `shifter healthcheck` all functional today; `shifter serve` exists with imports wired but body marked TODO for Plan 09/14/18 to populate.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/01-foundation/01-CONTEXT.md
@.planning/phases/01-foundation/01-RESEARCH.md
@01-03-database-layer-PLAN.md
@01-04-config-secrets-PLAN.md

<interfaces>
RESEARCH §"Wiring serve" (lines 1390-1476) — the canonical skeleton; Plans 09/14/18 fill in auth + ChirpStack + http wiring.

CLI tree shape (D-12):
```
shifter
├── serve            — Run migrations then start HTTP server (Plan 09/18 fills body)
├── migrate
│   ├── up           — Apply pending migrations (calls db.RunMigrations)
│   ├── down N       — Roll back N migrations (dev only)
│   ├── force N      — Mark schema clean at version N (dirty recovery)
│   └── version      — Print current schema version
├── version          — Print binary version
├── create-admin     — Create admin user (Plan 09 fills body) [--email, --password, --reset]
├── config-check     — Validate config syntax + ping endpoints (Plan 17 fills probes)
└── healthcheck      — GET /health on localhost; exit 0/1 (D-15)
```

Shared dependencies pattern (every command uses):
```go
cfg, err := config.Load()
if err != nil { return err }
log := logging.New(cfg.LogLevel)
ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer cancel()
pool, err := db.NewPool(ctx, cfg.DB.DSN(), cfg.DB.MaxConns)
if err != nil { return err }
defer pool.Close()
```
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: Cobra root + version + healthcheck + migrate subcommands (fully implemented)</name>
  <files>cmd/shifter/main.go, internal/cli/root.go, internal/cli/version.go, internal/cli/healthcheck.go, internal/cli/migrate.go</files>
  <read_first>
    - .planning/phases/01-foundation/01-CONTEXT.md (D-12, D-13, D-14, D-15, D-16)
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pitfall 11: shifter healthcheck" (lines 1373-1377)
    - 01-03-database-layer-PLAN.md (db.RunMigrations, db.ForceVersion signatures)
    - 01-04-config-secrets-PLAN.md (config.Load and version.Info())
  </read_first>
  <action>
1. Replace `cmd/shifter/main.go`:
   ```go
   package main

   import (
       "os"

       "github.com/shifter-io/shifter/internal/cli"
   )

   func main() {
       if err := cli.Execute(); err != nil {
           os.Exit(1)
       }
   }
   ```

2. Create `internal/cli/root.go`:
   ```go
   package cli

   import (
       "fmt"
       "os"

       "github.com/spf13/cobra"
   )

   var rootCmd = &cobra.Command{
       Use:           "shifter",
       Short:         "Shifter — self-hosted LoRaWAN water/electricity monitoring",
       SilenceUsage:  true,
       SilenceErrors: true,
   }

   // Execute is the package entry point.
   func Execute() error {
       rootCmd.AddCommand(serveCmd, migrateCmd, versionCmd, createAdminCmd, configCheckCmd, healthcheckCmd)
       if err := rootCmd.Execute(); err != nil {
           fmt.Fprintln(os.Stderr, "error:", err)
           return err
       }
       return nil
   }
   ```

3. Create `internal/cli/version.go`:
   ```go
   package cli

   import (
       "encoding/json"
       "fmt"

       "github.com/shifter-io/shifter/internal/version"
       "github.com/spf13/cobra"
   )

   var (
       versionJSONOutput bool

       versionCmd = &cobra.Command{
           Use:   "version",
           Short: "Print the binary version",
           RunE: func(cmd *cobra.Command, _ []string) error {
               info := version.Info()
               if versionJSONOutput {
                   return json.NewEncoder(cmd.OutOrStdout()).Encode(info)
               }
               fmt.Fprintf(cmd.OutOrStdout(), "shifter %s\ncommit: %s\nbuilt: %s\n",
                   info.Version, info.Commit, info.BuildTime)
               return nil
           },
       }
   )

   func init() {
       versionCmd.Flags().BoolVar(&versionJSONOutput, "json", false, "Emit JSON output")
   }
   ```

4. Create `internal/cli/healthcheck.go` — D-15, PITFALL #11:
   ```go
   package cli

   import (
       "context"
       "fmt"
       "net/http"
       "os"
       "time"

       "github.com/spf13/cobra"
   )

   var healthcheckCmd = &cobra.Command{
       Use:   "healthcheck",
       Short: "Localhost HTTP GET /health for use as a Docker HEALTHCHECK",
       RunE: func(cmd *cobra.Command, _ []string) error {
           port := os.Getenv("SHIFTER_HTTP_PORT")
           if port == "" {
               port = "8080"
           }
           url := fmt.Sprintf("http://127.0.0.1:%s/health", port)

           ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
           defer cancel()
           req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
           if err != nil {
               return err
           }
           resp, err := http.DefaultClient.Do(req)
           if err != nil {
               return fmt.Errorf("healthcheck failed: %w", err)
           }
           defer resp.Body.Close()
           if resp.StatusCode >= 400 {
               return fmt.Errorf("healthcheck status %d", resp.StatusCode)
           }
           return nil
       },
   }
   ```

5. Create `internal/cli/migrate.go` — D-13, D-16:
   ```go
   package cli

   import (
       "context"
       "fmt"
       "strconv"

       "github.com/shifter-io/shifter/internal/config"
       "github.com/shifter-io/shifter/internal/db"
       "github.com/shifter-io/shifter/internal/logging"
       "github.com/spf13/cobra"
   )

   var migrateCmd = &cobra.Command{
       Use:   "migrate",
       Short: "Database schema migrations",
   }

   var migrateUpCmd = &cobra.Command{
       Use:   "up",
       Short: "Apply all pending migrations",
       RunE: func(cmd *cobra.Command, _ []string) error {
           cfg, err := config.Load()
           if err != nil {
               return err
           }
           log := logging.New(cfg.LogLevel)
           ctx := cmd.Context()
           pool, err := db.NewPool(ctx, cfg.DB.DSN(), cfg.DB.MaxConns)
           if err != nil {
               return err
           }
           defer pool.Close()
           return db.RunMigrations(ctx, pool, log)
       },
   }

   var migrateForceCmd = &cobra.Command{
       Use:   "force <version>",
       Short: "Mark the schema clean at the given version (dirty-state recovery)",
       Args:  cobra.ExactArgs(1),
       RunE: func(cmd *cobra.Command, args []string) error {
           v, err := strconv.Atoi(args[0])
           if err != nil {
               return fmt.Errorf("version must be an integer")
           }
           cfg, err := config.Load()
           if err != nil {
               return err
           }
           ctx := context.Background()
           pool, err := db.NewPool(ctx, cfg.DB.DSN(), cfg.DB.MaxConns)
           if err != nil {
               return err
           }
           defer pool.Close()
           return db.ForceVersion(ctx, pool, v)
       },
   }

   func init() {
       migrateCmd.AddCommand(migrateUpCmd, migrateForceCmd)
   }
   ```
  </action>
  <verify>
    <automated>go build -o /tmp/shifter-cli ./cmd/shifter && /tmp/shifter-cli --help | grep -E '(serve|migrate|version|create-admin|config-check|healthcheck)' | wc -l | grep -q '6' && /tmp/shifter-cli version --json | grep -q '"version"' && rm /tmp/shifter-cli</automated>
  </verify>
  <acceptance_criteria>
    - File `cmd/shifter/main.go` calls `cli.Execute()` and exits non-zero on error
    - File `internal/cli/root.go` defines `rootCmd` and `Execute()` registers all 6 subcommands via `rootCmd.AddCommand`
    - File `internal/cli/version.go` defines `versionCmd` with `--json` flag
    - File `internal/cli/healthcheck.go` defines `healthcheckCmd` that GETs `http://127.0.0.1:$SHIFTER_HTTP_PORT/health` (default 8080)
    - File `internal/cli/migrate.go` defines `migrateCmd`, `migrateUpCmd`, `migrateForceCmd` and `migrateUpCmd` calls `db.RunMigrations`
    - Command `shifter --help` lists exactly 6 subcommands: `serve`, `migrate`, `version`, `create-admin`, `config-check`, `healthcheck`
    - Command `shifter version --json` outputs JSON containing `"version"` field
    - Command `shifter migrate --help` lists `up` and `force` subcommands
  </acceptance_criteria>
  <done>
    Top-level CLI tree wired. `version`, `healthcheck`, `migrate` are fully implemented. `serve`, `create-admin`, `config-check` are stubs in Task 2 below; their bodies fill in Plans 09 (auth/create-admin), 17 (config-check probes), 18+ (serve full wiring).
  </done>
</task>

<task type="auto">
  <name>Task 2: Stub commands (serve, create-admin, config-check) with TODO markers</name>
  <files>internal/cli/serve.go, internal/cli/createadmin.go, internal/cli/configcheck.go</files>
  <read_first>
    - .planning/phases/01-foundation/01-CONTEXT.md (D-07, D-12, D-13, D-14)
    - .planning/phases/01-foundation/01-RESEARCH.md §"Wiring serve (the big one)" (lines 1390-1476)
    - 01-04-config-secrets-PLAN.md (config.Load returns *Config with all secrets resolved)
  </read_first>
  <action>
1. Create `internal/cli/serve.go` — skeleton with imports + outline matching RESEARCH §"Wiring serve". Plans 09/13/18 fill in auth/MQTT/HTTP wiring; this file just exists today so the CLI tree is complete:
   ```go
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

       "github.com/shifter-io/shifter/internal/config"
       "github.com/shifter-io/shifter/internal/db"
       "github.com/shifter-io/shifter/internal/logging"
       "github.com/spf13/cobra"
   )

   var serveCmd = &cobra.Command{
       Use:   "serve",
       Short: "Run migrations then start the HTTP server (D-13)",
       RunE: func(cmd *cobra.Command, _ []string) error {
           cfg, err := config.Load()
           if err != nil {
               return err
           }
           log := logging.New(cfg.LogLevel)
           ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
           defer cancel()

           // 1. DB pool
           pool, err := db.NewPool(ctx, cfg.DB.DSN(), cfg.DB.MaxConns)
           if err != nil {
               return fmt.Errorf("db: %w", err)
           }
           defer pool.Close()

           // 2. D-13: auto-migrate before listening
           if err := db.RunMigrations(ctx, pool, log); err != nil {
               return err
           }
           log.Info("startup", "version", cfg.HTTPPort)

           // TODO(plan-09 + plan-13 + plan-18): build http.Handler with:
           //   - session manager (Plan 08)
           //   - login + account routes (Plan 09)
           //   - install wizard handlers (Plan 14, 15)
           //   - test-connection (Plan 17)
           //   - chirpstack v3 probe + refusal (Plan 12)
           //   - mqtt subscriber start (Plan 13)
           //   - /health + /health/detailed (Plan 18)
           //   - SPA embed (Plan 19)
           // For now we serve a 503 placeholder so the listener exists end-to-end.
           mux := http.NewServeMux()
           mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
               http.Error(w, "shifter starting up — full wiring lands in Plans 09-19", http.StatusServiceUnavailable)
           })
           mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
               // Plan 18 replaces this with the canonical /health response.
               w.Header().Set("Content-Type", "application/json")
               w.WriteHeader(http.StatusOK)
               _, _ = w.Write([]byte(`{"status":"ok","version":"placeholder"}`))
           })

           srv := &http.Server{
               Addr:              ":" + cfg.HTTPPort,
               Handler:           mux,
               ReadHeaderTimeout: 10 * time.Second,
               IdleTimeout:       2 * time.Minute,
           }
           go func() {
               log.Info("listening", "addr", srv.Addr)
               if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
                   log.Error("listen failed", "err", err)
                   cancel()
               }
           }()

           <-ctx.Done()
           log.Info("shutting down")
           shutCtx, shutCancel := context.WithTimeout(context.Background(), 15*time.Second)
           defer shutCancel()
           return srv.Shutdown(shutCtx)
       },
   }
   ```

2. Create `internal/cli/createadmin.go` — Plan 09 fills in `auth.Hash` + DB insert; today this is a stub:
   ```go
   package cli

   import (
       "errors"

       "github.com/spf13/cobra"
   )

   var (
       createAdminEmail    string
       createAdminPassword string
       createAdminReset    bool

       createAdminCmd = &cobra.Command{
           Use:   "create-admin",
           Short: "Create or reset an admin user (recovery escape hatch — D-14)",
           RunE: func(cmd *cobra.Command, _ []string) error {
               if createAdminEmail == "" || createAdminPassword == "" {
                   return errors.New("--email and --password are required")
               }
               // TODO(plan-09): wire auth.Hash + db.InsertAdminUser / UpdateUserPassword
               // (see Plan 09 §"create-admin recovery flow" — once that plan implements
               //  auth.Hash and the user store, replace this body).
               return errors.New("create-admin: pending Plan 09 implementation")
           },
       }
   )

   func init() {
       createAdminCmd.Flags().StringVar(&createAdminEmail, "email", "", "Admin email")
       createAdminCmd.Flags().StringVar(&createAdminPassword, "password", "", "New password")
       createAdminCmd.Flags().BoolVar(&createAdminReset, "reset", false, "Reset existing admin's password instead of creating a new one")
   }
   ```

3. Create `internal/cli/configcheck.go` — D-07. Plan 17 adds connectivity probes (gRPC ProbeVersion, MQTT connect):
   ```go
   package cli

   import (
       "fmt"

       "github.com/shifter-io/shifter/internal/config"
       "github.com/spf13/cobra"
   )

   var configCheckCmd = &cobra.Command{
       Use:   "config-check",
       Short: "Validate config.yaml syntax and probe endpoints (D-07)",
       RunE: func(cmd *cobra.Command, _ []string) error {
           cfg, err := config.Load()
           if err != nil {
               fmt.Fprintf(cmd.OutOrStdout(), "FAIL config: %v\n", err)
               return err
           }
           fmt.Fprintf(cmd.OutOrStdout(), "PASS config syntax (env=%s, tls.mode=%s)\n", cfg.Env, cfg.TLS.Mode)

           // TODO(plan-17): connectivity probes
           //   - PASS/FAIL postgres ping (db.NewPool().Ping)
           //   - PASS/FAIL chirpstack gRPC ProbeVersion (Plan 12)
           //   - PASS/FAIL MQTT PingMQTT (Plan 13)
           // Until Plan 17 lands, config-check only verifies syntax + secret resolution.
           fmt.Fprintln(cmd.OutOrStdout(), "SKIP connectivity probes (pending Plan 17)")
           return nil
       },
   }
   ```
  </action>
  <verify>
    <automated>go build -o /tmp/shifter-cli2 ./cmd/shifter && /tmp/shifter-cli2 --help 2>&1 | grep -q 'create-admin' && /tmp/shifter-cli2 create-admin 2>&1 | grep -q 'required' && rm /tmp/shifter-cli2</automated>
  </verify>
  <acceptance_criteria>
    - File `internal/cli/serve.go` defines `serveCmd` and calls `db.RunMigrations` after pool creation (D-13)
    - File `internal/cli/serve.go` registers a placeholder `/health` handler that returns 200 (replaced by Plan 18)
    - File `internal/cli/createadmin.go` defines `createAdminCmd` with `--email`, `--password`, `--reset` flags
    - File `internal/cli/configcheck.go` defines `configCheckCmd` and prints `PASS config syntax`
    - Command `shifter create-admin` (no flags) prints an error mentioning `--email and --password are required`
    - Command `shifter --help` shows `serve`, `create-admin`, `config-check` (verifying registration)
    - `go build ./cmd/shifter` exits 0
    - File `internal/cli/createadmin.go` contains a `TODO(plan-09)` marker so search finds the wiring point
    - File `internal/cli/configcheck.go` contains a `TODO(plan-17)` marker
    - File `internal/cli/serve.go` contains `TODO(plan-09 + plan-13 + plan-18)` marker
  </acceptance_criteria>
  <done>
    All 6 subcommands registered. `version`, `healthcheck`, `migrate up/force` work today. `serve` opens a listener with a placeholder /health (Plan 18 replaces). `create-admin` and `config-check` exist as stubs that error politely until Plan 09/17 land.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| operator → CLI | flags + env vars; create-admin and migrate force are privileged ops |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-05-01 | Spoofing | `shifter create-admin --reset` runnable by anyone with shell access | accept | Self-hosted single-tenant; shell access = root access. Documented as a recovery tool (D-14). ASVS V4. |
| T-05-02 | Information Disclosure | `--password` flag visible in process listings (`ps`) | mitigate | Plan 09 implementation reads from prompt or `SHIFTER_NEW_PASSWORD` env var when `--password=-`; documented in README. |
| T-05-03 | Tampering | `migrate force <N>` rolls back the dirty-state guard, could mask data corruption | accept | Operator-explicit recovery; Plan 24 README documents that operator should investigate root cause first. ASVS V11. |
| T-05-04 | Denial of Service | `healthcheck` retries against a never-listening binary | accept | 5-second context timeout; Docker `HEALTHCHECK` retries are bounded. |
</threat_model>

<verification>
- `shifter --help` lists the 6 canonical subcommands
- `shifter version --json` outputs JSON with `version` key
- `shifter migrate up` exits 0 against a migrated DB (no-op)
- `shifter healthcheck` exits 1 when nothing is listening, 0 when /health returns 200
- `shifter create-admin` errors when flags are missing
- All TODO markers visible to grep for downstream plans
</verification>

<success_criteria>
- D-12 fully realized: 6-subcommand tree
- D-13 enforced: `serve` calls `db.RunMigrations` before listening
- D-14 enforced: `create-admin` skeleton accepts `--reset`
- D-15 enforced: `healthcheck` is a localhost HTTP probe (no curl needed)
- D-16 enforced: `migrate force <N>` exists for dirty recovery
- TODO markers point downstream plans (09, 13, 17, 18) at exact insertion sites
</success_criteria>

<output>
After completion, create `.planning/phases/01-foundation/01-05-SUMMARY.md` documenting:
- Subcommand tree
- Where each subcommand body lives
- TODO markers and their downstream plan
- ldflags command for production build
</output>
