---
phase: 06-alerts-users-audit-operational-hardening
plan: 08
type: execute
wave: 2
depends_on: [06-01]
files_modified:
  - internal/db/migrations/0045_backup_run.up.sql
  - internal/db/migrations/0045_backup_run.down.sql
  - internal/backup/doc.go
  - internal/backup/runner.go
  - internal/backup/runner_test.go
  - internal/backup/manifest.go
  - internal/backup/manifest_test.go
  - internal/backup/store.go
  - internal/backup/store_test.go
  - internal/backup/handler.go
  - internal/backup/handler_test.go
  - internal/cli/backup.go
  - internal/cli/backup_test.go
  - internal/cli/root.go
  - internal/auth/authz.go
  - internal/http/router.go
  - internal/config/config.go
  - Dockerfile
  - compose/bundled.yml
  - compose/external.yml
  - install/bundled/install.sh
  - install/external/install.sh
autonomous: true
requirements: [OPS-02, OPS-03, SETT-05]
must_haves:
  truths:
    - "`shifter backup --to /path/to/dir` CLI subcommand exists; exits 0 on success, non-zero on failure"
    - "Backup produces a single tar.gz containing: db/shifter.dump (pg_dump --format=custom), db/chirpstack.dump (bundled mode only), floor-plans/ rsync, manifest.json"
    - "Manifest.json schema per RESEARCH §Decision A: {manifest_version, shifter_version, db_schema_version, chirpstack_mode, included, sha256_sums, started_at, finished_at}"
    - "Bundled mode (install_state.chirpstack_mode='bundled'): includes ChirpStack DB; external mode: Shifter-only"
    - "Backup CLI writes audit_log row 'backup.start' before pg_dump + 'backup.complete' (or 'backup.failed' with reason) after"
    - "POST /api/backup/run-now triggers the same CLI code path; returns 202 with backup_run.id; status polled via GET /api/backup/jobs/{id}"
    - "GET /api/backup/list returns the 5 most-recent backup_run rows for the Settings card"
    - "GET /api/backup/last returns {age_seconds, status, ...} for the freshness dot"
    - "Dockerfile bundles postgresql16-client (pg_dump + pg_restore + psql) in the runtime stage; image size impact ≤ +20MB"
    - "compose/bundled.yml has mcuadros/ofelia:v0.3.22 sidecar with label-driven schedule '0 2 * * *' (D-43, no :latest); volume /var/lib/shifter/backups mounted"
    - "compose/external.yml has the backups volume mounted but NO ofelia sidecar (external operators own their cron per D-43)"
  artifacts:
    - path: internal/cli/backup.go
      provides: "Cobra subcommand 'shifter backup --to <path>'"
    - path: internal/backup/runner.go
      provides: "Backup(ctx, destPath) runs pg_dump + tar + manifest + audit-in-tx"
    - path: internal/backup/manifest.go
      provides: "Manifest struct + sha256-of-every-file logic"
    - path: internal/db/migrations/0045_backup_run.up.sql
      provides: "backup_run history table"
    - path: compose/bundled.yml
      provides: "mcuadros/ofelia:v0.3.22 sidecar with backup schedule"
  key_links:
    - from: internal/cli/backup.go
      to: internal/backup/runner.go::Backup
      via: "CLI invokes Runner.Backup(ctx, dest) which writes backup_run row + manifest"
      pattern: "Runner.Backup"
    - from: internal/backup/runner.go
      to: pg_dump binary
      via: "exec.CommandContext(ctx, \"pg_dump\", ...) with PGPASSWORD env"
      pattern: "exec.CommandContext.*pg_dump"
---

<objective>
Ship the operator-owned backup surface: `shifter backup` CLI subcommand + `POST /api/backup/run-now` HTTP trigger + `mcuadros/ofelia:v0.3.22` cron sidecar in bundled-mode compose + Settings card data endpoints. Closes OPS-02 (TimescaleDB-aware backup script), OPS-03 (configurable destination), SETT-05 (Settings backup card data).

Purpose: every Shifter install must be backup-able with one CLI call, one HTTP POST, or a scheduled nightly job. Bundled mode includes both DBs in one tarball (operator runs `shifter backup` once → full-stack recovery artifact). The tarball format is the v1 contract; v1.x will add S3 + scheduled UI without changing the format.

Output: new `internal/backup/` package + Cobra subcommand + new migration `0045_backup_run` + Dockerfile bundles `postgresql16-client` + bundled compose adds ofelia sidecar + Settings backup API endpoints.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-01-alert-engine-substrate-PLAN.md
@internal/cli/root.go
@internal/cli/createadmin.go
@internal/cli/migrate.go
@internal/config/config.go
@internal/install/state.go
@internal/audit/log.go
@internal/auth/authz.go
@internal/http/router.go
@compose/bundled.yml
@compose/external.yml
@install/bundled/install.sh
@Dockerfile

<interfaces>
Plan 06-01 added audit constants ActionBackupStart, ActionBackupComplete, ActionBackupFailed, ActionBackupRestore + EntityTypeBackupRun.

internal/install/state.go reads install_state table: chirpstack_mode column (values 'bundled'|'external') — Plan 06-08 detects bundled to decide whether to dump ChirpStack DB.

Cobra subcommand pattern (internal/cli/createadmin.go): cmd.PersistentFlags(), Flags(), RunE function returning error → exit code.

internal/config/config.go (viper): config has SHIFTER_DB_HOST/PORT/USER/PASSWORD; Plan 06-08 reads these for pg_dump connection + adds new env var SHIFTER_BACKUP_DIR (default `/var/lib/shifter/backups`).

Dockerfile (line 1-30 read; final stage is distroless static-debian12:nonroot). Plan 06-08 changes the final stage to copy pg_dump + pg_restore + psql binaries from an alpine builder stage (RESEARCH §Decision A Dockerfile snippet).

D-39 manifest schema (RESEARCH §Decision A): JSON with manifest_version="1.0", shifter_version, db_schema_version, chirpstack_mode, install_id, install_slug, started_at, finished_at, included[], sha256_sums{}.

D-43 ofelia config snippet (RESEARCH §Decision I): service block with image v0.3.22 + Docker socket :ro + labels.

internal/auth/authz.go: add ActionBackupRun, ActionBackupRead actions; admin only.
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Migration 0045 (backup_run) + internal/backup package (manifest + runner + store) + Dockerfile bundles pg client</name>
  <files>internal/db/migrations/0045_backup_run.up.sql, internal/db/migrations/0045_backup_run.down.sql, internal/backup/doc.go, internal/backup/runner.go, internal/backup/runner_test.go, internal/backup/manifest.go, internal/backup/manifest_test.go, internal/backup/store.go, internal/backup/store_test.go, internal/config/config.go, Dockerfile</files>
  <read_first>
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Decision A "Exact Backup Command" + "Manifest Schema (D-39)" + "Bundling pg client tools in Shifter image"
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Code Examples "Backup Tarball Construction"
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md §D-39, D-40, D-41, D-42
    - internal/install/state.go (chirpstack_mode lookup)
    - internal/audit/log.go (constants from Plan 06-01)
    - Dockerfile (current distroless final stage)
  </read_first>
  <behavior>
    - Test (TestMigration0044_CreatesBackupRunTable): migration applies; columns: id, status, destination_path, file_name, file_size_bytes, sha256, started_at, finished_at, manifest_json (JSONB), error_message, schema_version, chirpstack_mode, triggered_by (uuid → user.id), trigger_kind ('cli'|'cron'|'api').
    - Test (TestManifest_RoundTrip): build manifest → MarshalJSON → UnmarshalJSON → equal. sha256 of each "included" entry matches.
    - Test (TestManifest_VerifyChecksums): bytes-of-each-file → sha256_sums map; verify by reading from extracted tar matches.
    - Test (TestRunner_External_DumpsShifterOnly): chirpstack_mode='external' → tarball contains db/shifter.dump only, NO db/chirpstack.dump. Manifest.included = ["db/shifter.dump", "floor-plans/"]. Asserts no `chirpstack.dump` file in tarball.
    - Test (TestRunner_Bundled_IncludesChirpstack): chirpstack_mode='bundled' + chirpstack DB seeded → tarball contains both db/shifter.dump and db/chirpstack.dump.
    - Test (TestRunner_PgDumpFlags): assert the exec.Cmd has flags `--format=custom --no-owner --no-acl`; assert NO `-j` or `--jobs` flag anywhere (Pitfall 1).
    - Test (TestRunner_TarballNameConvention): produced tarball name matches `shifter-backup-{install_slug}-{YYYYMMDD-HHMM}-{schema_version}.tar.gz`.
    - Test (TestRunner_WritesAuditAndBackupRun): backup writes one backup_run row (status='completed') + audit rows 'backup.start' and 'backup.complete'; both audit rows have entity_type='backup_run' and entity_id=backup_run.id.
    - Test (TestRunner_FailureWritesAuditFailed): force pg_dump failure (bad DBNAME) → backup_run.status='failed', error_message populated; audit row 'backup.failed' present.
    - Test (TestDockerfile_BundlesPgClient): not a Go test — a shell-level check `docker build . && docker run --rm shifter pg_dump --version` exits 0 and prints `pg_dump (PostgreSQL) 16.x`. This is a CI gate, see Plan 06-09 task 3 wiring; here just verify Dockerfile lines exist.
    - Test (TestConfig_BackupDir): viper reads SHIFTER_BACKUP_DIR env var; default `/var/lib/shifter/backups`.
  </behavior>
  <action>
    **Migration coordination:** Plan 06-08 claims migration `0045` (Plan 06-01 owns 0037-0043, Plan 06-05 owns 0044, Plan 06-10 owns 0046, Plan 06-11 owns 0047).

    **Migration 0045_backup_run.up.sql:**
    ```sql
    CREATE TABLE backup_run (
        id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        trigger_kind     TEXT NOT NULL CHECK (trigger_kind IN ('cli','cron','api')),
        triggered_by     UUID REFERENCES "user"(id) ON DELETE SET NULL, -- NULL for cron
        status           TEXT NOT NULL CHECK (status IN ('running','completed','failed')),
        destination_dir  TEXT NOT NULL,
        file_name        TEXT,
        file_size_bytes  BIGINT,
        sha256           TEXT,
        manifest_json    JSONB,
        chirpstack_mode  TEXT,
        schema_version   TEXT,
        started_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
        finished_at      TIMESTAMPTZ,
        error_message    TEXT
    );
    CREATE INDEX backup_run_started_at_idx ON backup_run (started_at DESC);
    ```

    **internal/backup/manifest.go** (RESEARCH §Decision A Manifest Schema):
    ```go
    package backup

    type Manifest struct {
        ManifestVersion      string            `json:"manifest_version"`       // "1.0"
        ShifterVersion       string            `json:"shifter_version"`
        DBSchemaVersion      string            `json:"db_schema_version"`
        ChirpStackMode       string            `json:"chirpstack_mode"`        // "bundled"|"external"
        ChirpStackDBIncluded bool              `json:"chirpstack_db_included"`
        InstallID            string            `json:"install_id"`
        InstallSlug          string            `json:"install_slug"`
        StartedAt            time.Time         `json:"started_at"`
        FinishedAt           time.Time         `json:"finished_at"`
        Included             []string          `json:"included"`               // ["db/shifter.dump", "db/chirpstack.dump", "floor-plans/"]
        SHA256Sums           map[string]string `json:"sha256_sums"`            // per-file sha256
    }
    ```

    Helpers: `ComputeFileSHA256(path string) (string, error)`, `VerifyFileSHA256(path, expected string) error`.

    **internal/backup/runner.go** (RESEARCH §Code Examples "Backup Tarball Construction"):
    ```go
    type Runner struct {
        Pool             *pgxpool.Pool
        Queries          *sqlc.Queries
        Store            *Store
        Cfg              RunnerConfig
        Log              *slog.Logger
    }
    type RunnerConfig struct {
        DBHost           string
        DBPort           int
        DBUser           string
        DBName           string
        DBPassword       string
        ChirpStackDBName string         // typically "chirpstack"
        ChirpStackDBUser string         // typically "chirpstack"
        ChirpStackMode   string         // "bundled"|"external"
        FloorPlansDir    string         // /var/lib/shifter/floor-plans
        InstallSlug      string
        InstallID        string
        ShifterVersion   string
        SchemaVersion    string
    }

    // Backup builds the tarball at destDir/{tarball-name}.tar.gz.
    // Returns the backup_run.ID + the tarball path on success.
    func (r *Runner) Backup(ctx context.Context, destDir string, triggerKind string, userID *uuid.UUID) (backupID uuid.UUID, tarballPath string, err error) {
        // 1. Open tx, insert backup_run row with status='running' + trigger_kind, write 'backup.start' audit
        // 2. Commit (so external observers see the in-progress row)
        // 3. Build tarball name: shifter-backup-{slug}-{YYYYMMDD-HHMM}-{schema}.tar.gz
        // 4. Open file + gzip + tar writers
        // 5. pg_dump shifter DB via exec.CommandContext("pg_dump", "--host", r.Cfg.DBHost, "--port", strconv.Itoa(r.Cfg.DBPort), "--username", r.Cfg.DBUser, "--dbname", r.Cfg.DBName, "--format=custom", "--no-owner", "--no-acl", "--file", tmpShifterDump); cmd.Env = append(os.Environ(), "PGPASSWORD="+r.Cfg.DBPassword)
        //    Capture stdout/stderr to log; check exit code.
        //    ASSERT: never `--jobs` / `-j` in the args slice — Pitfall 1 lint.
        // 6. addFileToTar(tw, tmpShifterDump, "db/shifter.dump", manifest.SHA256Sums)
        // 7. IF chirpstack_mode == "bundled": pg_dump chirpstack DB → db/chirpstack.dump (same flags)
        // 8. addDirToTar(tw, r.Cfg.FloorPlansDir, "floor-plans/", manifest.SHA256Sums)
        // 9. manifest.FinishedAt = time.Now().UTC(); marshal + write to tar as last entry
        // 10. Close writers; stat tarball for size; compute outer sha256 of the whole tar.gz
        // 11. UPDATE backup_run row: status='completed', file_name, file_size_bytes, sha256, finished_at, manifest_json
        // 12. Write 'backup.complete' audit row in same tx
        // On any error: UPDATE row status='failed', error_message=err.Error(); write 'backup.failed' audit row.
    }
    ```

    **internal/backup/store.go:**
    ```go
    type BackupRunRow struct {
        ID               uuid.UUID
        TriggerKind      string
        TriggeredBy      *uuid.UUID
        Status           string
        DestinationDir   string
        FileName         *string
        FileSizeBytes    *int64
        SHA256           *string
        ManifestJSON     json.RawMessage
        ChirpStackMode   *string
        SchemaVersion    *string
        StartedAt        time.Time
        FinishedAt       *time.Time
        ErrorMessage     *string
    }
    func (s *Store) InsertStartedTx(ctx context.Context, tx pgx.Tx, params StartParams) (BackupRunRow, error)
    func (s *Store) UpdateCompletedTx(ctx context.Context, tx pgx.Tx, params CompleteParams) error
    func (s *Store) UpdateFailedTx(ctx context.Context, tx pgx.Tx, id uuid.UUID, errorMsg string) error
    func (s *Store) ListRecent(ctx context.Context, limit int) ([]BackupRunRow, error)
    func (s *Store) Last(ctx context.Context) (*BackupRunRow, error)
    func (s *Store) Get(ctx context.Context, id uuid.UUID) (*BackupRunRow, error)
    ```

    **internal/config/config.go** — add `BackupDir string` via viper key `SHIFTER_BACKUP_DIR`, default `/var/lib/shifter/backups`.

    **Dockerfile** — patch per RESEARCH §Decision A "Bundling pg client tools in Shifter image":
    ```dockerfile
    # Add a new stage that has the postgres client tools
    FROM alpine:3.20 AS pgclient
    RUN apk add --no-cache postgresql16-client tar gzip

    # In the existing distroless final stage, copy the binaries + needed libs
    FROM gcr.io/distroless/static-debian12:nonroot AS runtime
    # ... existing COPY of /out/shifter
    COPY --from=pgclient /usr/bin/pg_dump      /usr/bin/pg_dump
    COPY --from=pgclient /usr/bin/pg_restore   /usr/bin/pg_restore
    COPY --from=pgclient /usr/bin/psql         /usr/bin/psql
    # postgresql16-client needs some shared libs and the locale data — copy from pgclient
    COPY --from=pgclient /usr/lib/libpq.so.5         /usr/lib/libpq.so.5
    COPY --from=pgclient /usr/lib/libxml2.so.2       /usr/lib/libxml2.so.2
    COPY --from=pgclient /usr/lib/libgssapi_krb5.so.2 /usr/lib/libgssapi_krb5.so.2
    # ... add any other transitive .so files identified by `ldd /usr/bin/pg_dump` on the pgclient stage
    # Add tar + gzip (we use Go stdlib archive/tar+compress/gzip but Pitfall mitigations may need tar for ldconfig)
    ```
    NOTE: distroless does not include glibc/musl. Alpine's postgresql16-client is musl-linked. **Verify in Wave 0**: pg_dump from alpine likely won't run on debian distroless without musl libs. Mitigation: switch the pgclient stage to debian:bookworm-slim (`apt-get install postgresql-client-16`) so libraries match distroless's debian base; OR change the runtime stage to alpine:3.20 (gives up "distroless" branding but gains pg_dump compatibility cheaply). **PICK debian:bookworm-slim** for the pgclient stage:
    ```dockerfile
    FROM debian:bookworm-slim AS pgclient
    RUN apt-get update && apt-get install -y --no-install-recommends \
        postgresql-client-16 ca-certificates && rm -rf /var/lib/apt/lists/*

    FROM gcr.io/distroless/static-debian12:nonroot AS runtime
    COPY --from=pgclient /usr/bin/pg_dump    /usr/bin/pg_dump
    COPY --from=pgclient /usr/bin/pg_restore /usr/bin/pg_restore
    COPY --from=pgclient /usr/bin/psql       /usr/bin/psql
    # Copy all transitive libs the pg client binaries need (use ldd to discover)
    COPY --from=pgclient /usr/lib/x86_64-linux-gnu/libpq.so.5* /usr/lib/x86_64-linux-gnu/
    COPY --from=pgclient /usr/lib/x86_64-linux-gnu/libldap* /usr/lib/x86_64-linux-gnu/
    COPY --from=pgclient /usr/lib/x86_64-linux-gnu/libgssapi_krb5* /usr/lib/x86_64-linux-gnu/
    COPY --from=pgclient /usr/lib/x86_64-linux-gnu/libkrb5* /usr/lib/x86_64-linux-gnu/
    COPY --from=pgclient /usr/lib/x86_64-linux-gnu/libk5crypto* /usr/lib/x86_64-linux-gnu/
    COPY --from=pgclient /usr/lib/x86_64-linux-gnu/libcom_err* /usr/lib/x86_64-linux-gnu/
    COPY --from=pgclient /usr/lib/x86_64-linux-gnu/libkrb5support* /usr/lib/x86_64-linux-gnu/
    COPY --from=pgclient /lib/x86_64-linux-gnu/libsasl2* /lib/x86_64-linux-gnu/
    COPY --from=pgclient /lib/x86_64-linux-gnu/libssl* /lib/x86_64-linux-gnu/
    COPY --from=pgclient /lib/x86_64-linux-gnu/libcrypto* /lib/x86_64-linux-gnu/
    ```
    (Wave 0 task verifies the exact ldd output and trims/expands the COPY list. Document as a comment block in the Dockerfile.)

    `internal/backup/doc.go`: package doc explaining the tar.gz layout, the bundled-vs-external decision rule, and the audit-in-tx pattern.
  </action>
  <verify>
    <automated>go test ./internal/backup/... -count=1 -timeout=180s && docker build -t shifter:phase6-test . && docker run --rm --entrypoint pg_dump shifter:phase6-test --version</automated>
  </verify>
  <acceptance_criteria>
    - `internal/db/migrations/0045_backup_run.up.sql` contains `CREATE TABLE backup_run (` and `CHECK (status IN ('running','completed','failed'))` and `CHECK (trigger_kind IN ('cli','cron','api'))`
    - `internal/backup/manifest.go` exports `Manifest` struct with all 11 fields from RESEARCH §Decision A
    - `internal/backup/runner.go` contains `exec.CommandContext` call with literal flags `--format=custom`, `--no-owner`, `--no-acl`; grep proves NO `--jobs` or `-j` argument anywhere in the file (Pitfall 1 lint)
    - `internal/backup/store.go` exports `InsertStartedTx`, `UpdateCompletedTx`, `UpdateFailedTx`, `ListRecent`, `Last`, `Get`
    - `internal/config/config.go` reads `SHIFTER_BACKUP_DIR` env var with default `/var/lib/shifter/backups`
    - `Dockerfile` has a multi-stage `FROM debian:bookworm-slim AS pgclient` and COPY-from-pgclient lines for `pg_dump`, `pg_restore`, `psql`
    - `docker build .` succeeds AND `docker run --rm --entrypoint pg_dump shifter:phase6-test --version` exits 0 (Wave 0 verification gate; if it fails due to missing libs, expand the COPY list and document)
    - All 10 listed Go tests pass: `go test ./internal/backup/... -count=1 -timeout=180s` exits 0
    - `TestRunner_PgDumpFlags` test source grep proves it asserts the args slice contains no `-j` substring
  </acceptance_criteria>
  <done>The backup runner produces a valid TimescaleDB-compatible tarball in either bundled or external mode; the binary image has the pg client tools; the backup_run history table is queryable.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Cobra `shifter backup` subcommand + HTTP /api/backup/* endpoints + ofelia cron sidecar + compose volume</name>
  <files>internal/cli/backup.go, internal/cli/backup_test.go, internal/cli/root.go, internal/backup/handler.go, internal/backup/handler_test.go, internal/auth/authz.go, internal/http/router.go, compose/bundled.yml, compose/external.yml, install/bundled/install.sh, install/external/install.sh</files>
  <read_first>
    - internal/cli/createadmin.go (Cobra subcommand pattern; flag parsing; viper integration)
    - internal/cli/root.go (root command + subcommand registration)
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Decision I "Cron Sidecar for Bundled-Mode Backup" — exact compose snippet
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md §D-42, D-43
    - compose/bundled.yml (current top; note the x-logging anchor pattern + secrets pattern)
    - compose/external.yml (mirror the same conventions)
  </read_first>
  <behavior>
    - Test (TestCLIBackup_FlagParsing): `shifter backup --to /tmp/x --trigger=cli` parses; missing `--to` returns usage error.
    - Test (TestCLIBackup_RunsToCompletion): with testcontainer DB seeded, `shifter backup --to /tmp/dest` exits 0; `/tmp/dest/shifter-backup-*.tar.gz` exists; `tar -tzf <file>` lists `manifest.json` + `db/shifter.dump`.
    - Test (TestCLIBackup_NonZeroOnFailure): bad DB host → exits non-zero; backup_run row status='failed' present.
    - Test (TestAuthz_BackupActions): RoleAdmin has ActionBackupRun + ActionBackupRead; viewer has neither.
    - Test (TestHandler_RunNow_ReturnsBackupRunID): POST /api/backup/run-now → 202 `{job_id: uuid, status:"running"}`; subsequent GET /api/backup/jobs/{id} returns the row.
    - Test (TestHandler_ListRecent_TopFive): seed 7 backup_run rows → GET /api/backup/list?limit=5 returns 5 sorted by started_at DESC.
    - Test (TestHandler_LastAndAge): GET /api/backup/last returns `{age_seconds: int, status: string, started_at, sha256}`; when no backup exists returns `{never_run: true}`.
    - Test (TestHandler_ViewerCanRead): viewer GET /api/backup/list / /last → 200 (D-46 viewer sees status); viewer POST /run-now → 403.
    - Test (TestComposeBundled_HasOfelia): grep `compose/bundled.yml` for `image: mcuadros/ofelia:v0.3.22` (exact tag, no :latest) AND `ofelia.job-exec.shifter-backup.schedule: "0 2 * * *"`.
    - Test (TestComposeExternal_HasBackupsVolumeButNoOfelia): grep `compose/external.yml` for `backups:` volume AND NO `mcuadros/ofelia` reference.
    - Test (TestComposeBundled_OfeliaUsesJsonLogging): the backup-cron service has `logging: *json-logging` anchor.
    - Test (TestComposeBundled_BackupsVolumeMounted): the shifter service has `/var/lib/shifter/backups` volume mount referencing the named `backups` volume.
  </behavior>
  <action>
    **internal/auth/authz.go:** Add:
    ```go
    // Phase 6 Plan 06-08 backup actions:
    const (
        ActionBackupRun  Action = "backup.run"
        ActionBackupRead Action = "backup.read"
        ActionBackupConfigure Action = "backup.configure" // for SETT-05 threshold edit (used in Plan 06-10)
    )
    ```
    `roleBundles[RoleAdmin]` gets all three true. `roleBundles[RoleViewer]` gets `ActionBackupRead: true` only (per D-46 viewer sees status + history but cannot Run-now or configure thresholds).

    **internal/cli/backup.go** (Cobra subcommand mirroring createadmin.go):
    ```go
    var backupCmd = &cobra.Command{
        Use:   "backup",
        Short: "Create a Shifter backup tarball",
        Long:  `Runs pg_dump on the Shifter DB (and ChirpStack DB if bundled mode), rsyncs the floor-plan volume, and produces a single tar.gz manifest-verified backup. The tarball is written to --to (default $SHIFTER_BACKUP_DIR or /var/lib/shifter/backups).`,
        RunE:  runBackupCmd,
    }
    var backupFlagTo string
    var backupFlagTrigger string
    func init() {
        backupCmd.Flags().StringVar(&backupFlagTo, "to", "", "Destination directory for the tarball (defaults to SHIFTER_BACKUP_DIR or /var/lib/shifter/backups)")
        backupCmd.Flags().StringVar(&backupFlagTrigger, "trigger", "cli", "Trigger kind (cli|cron|api) — written to backup_run.trigger_kind")
        rootCmd.AddCommand(backupCmd)
    }
    func runBackupCmd(cmd *cobra.Command, args []string) error {
        // 1. Load config via viper (DB creds, install_state)
        // 2. Resolve destDir = flag OR env OR default
        // 3. Open pgxpool, load install_state for chirpstack_mode + slug + id
        // 4. Build Runner; call Backup(ctx, destDir, backupFlagTrigger, nil /* no userID for cli */)
        // 5. Print { "backup_run_id": ..., "file": ..., "size_bytes": ..., "sha256": ... } to stdout
        // 6. Return non-zero on err
    }
    ```

    Register the command in `internal/cli/root.go` (cobra `rootCmd.AddCommand(backupCmd)` — confirm the existing `init()` pattern picks it up).

    **internal/backup/handler.go:**
    ```go
    func RunNowHandler(deps Deps) http.HandlerFunc {
        // POST /api/backup/run-now
        // 1. Pull current user from context
        // 2. Spawn goroutine running Runner.Backup with trigger_kind="api", userID=current user
        //    BUT: backup is heavy + long-running; instead, kick it off via a River one-shot job
        //    so the HTTP request returns immediately with 202 + backup_run.ID
        // 3. Return 202 with {job_id: backup_run.id, status:"queued"}
        //    Pattern: enqueue a `backup_run_now` River job; Plan 06-08 registers it.
    }

    func ListRecentHandler(deps Deps) http.HandlerFunc {
        // GET /api/backup/list?limit=5 → store.ListRecent(limit)
    }

    func LastHandler(deps Deps) http.HandlerFunc {
        // GET /api/backup/last → {never_run: true} OR {age_seconds, status, sha256, file_name, finished_at}
    }

    func GetJobHandler(deps Deps) http.HandlerFunc {
        // GET /api/backup/jobs/{id} → store.Get(id)
    }
    ```

    Optionally, register `BackupRunNowWorker` River worker (kind="backup_run_now") that calls `Runner.Backup(...)`. This keeps the HTTP request snappy (202 returned in ms) while the actual backup runs in the worker queue. Adds a River Insert at the end of RunNowHandler.

    Add to `internal/cli/serve.go`:
    ```go
    river.AddWorker(riverWorkers, &backup.BackupRunNowWorker{Pool: pool, Store: backupStore, Cfg: backupCfg, Log: log})
    ```

    **internal/http/router.go:**
    ```go
    r.Route("/api/backup", func(r chi.Router) {
        r.With(auth.RequireAction(sm, auth.ActionBackupRead)).Get("/list", backup.ListRecentHandler(deps))
        r.With(auth.RequireAction(sm, auth.ActionBackupRead)).Get("/last", backup.LastHandler(deps))
        r.With(auth.RequireAction(sm, auth.ActionBackupRead)).Get("/jobs/{id}", backup.GetJobHandler(deps))
        r.With(auth.RequireAction(sm, auth.ActionBackupRun)).Post("/run-now", backup.RunNowHandler(deps))
    })
    ```

    **compose/bundled.yml** — add at the bottom of services (RESEARCH §Decision I verbatim, with project conventions):
    ```yaml
      backup-cron:
        image: mcuadros/ofelia:v0.3.22
        restart: unless-stopped
        depends_on:
          shifter:
            condition: service_started
        command: ["daemon", "--docker"]
        labels:
          ofelia.job-exec.shifter-backup.schedule: "0 2 * * *"
          ofelia.job-exec.shifter-backup.container: "shifter"
          ofelia.job-exec.shifter-backup.command: "shifter backup --to /var/lib/shifter/backups --trigger=cron"
        volumes:
          - /var/run/docker.sock:/var/run/docker.sock:ro
        logging: *json-logging
        networks: [shifter]
    ```

    Also add a `backups:` named volume at the bottom volumes block, and mount it in the shifter service: `/var/lib/shifter/backups`. Confirm the existing floor-plans volume mount pattern as the reference.

    **compose/external.yml:** Add the same `backups:` named volume and mount in shifter service, BUT do NOT add the backup-cron service. Add a comment block above the volume mount referencing the runbook section for "How to add your own backup cron in external mode" (Plan 06-11 writes that section).

    **install/bundled/install.sh** + **install/external/install.sh:** ensure the `backups` directory is created under the install volume with correct ownership before `docker compose up`. Update both scripts to include `mkdir -p /var/lib/shifter/backups && chown 65532:65532 /var/lib/shifter/backups` (65532 is the nonroot UID in distroless).
  </action>
  <verify>
    <automated>go test ./internal/cli/... ./internal/backup/... -run "TestCLIBackup|TestAuthz_BackupActions|TestHandler_RunNow|TestHandler_ListRecent|TestHandler_LastAndAge|TestHandler_ViewerCanRead" -count=1 && grep -c "mcuadros/ofelia:v0.3.22" compose/bundled.yml | grep -q "1" && grep -c "mcuadros/ofelia" compose/external.yml | grep -q "0"</automated>
  </verify>
  <acceptance_criteria>
    - `internal/cli/backup.go` exists with `var backupCmd = &cobra.Command{Use: "backup"...}` and `rootCmd.AddCommand(backupCmd)` in init
    - `internal/cli/backup.go` has `Flags().StringVar(&backupFlagTo, "to"` and `StringVar(&backupFlagTrigger, "trigger"`
    - `internal/auth/authz.go` declares all 3 new backup actions: grep returns 3 for `ActionBackupRun\|ActionBackupRead\|ActionBackupConfigure`
    - `roleBundles[RoleViewer]` has `ActionBackupRead: true` but NOT ActionBackupRun nor ActionBackupConfigure
    - `internal/http/router.go` mounts /api/backup/* with the 4 routes
    - `compose/bundled.yml` contains exactly one `image: mcuadros/ofelia:v0.3.22` line AND `ofelia.job-exec.shifter-backup.schedule: "0 2 * * *"` AND the backups volume mounted in the shifter service
    - `compose/bundled.yml` backup-cron service uses `logging: *json-logging` anchor
    - `compose/external.yml` does NOT contain `mcuadros/ofelia` AND DOES contain the `backups:` named volume + shifter service mount
    - Both install scripts (`install/bundled/install.sh`, `install/external/install.sh`) contain `mkdir -p /var/lib/shifter/backups`
    - All 11 listed tests pass: `go test ./internal/cli/... ./internal/backup/... -count=1` exits 0
    - `shifter backup --to /tmp/test --trigger=cli` (run inside the test container) exits 0 and produces a non-zero-size .tar.gz
  </acceptance_criteria>
  <done>Operator can: run `shifter backup` from CLI; POST /api/backup/run-now via the Settings card (Plan 06-10 wires the UI); rely on a nightly 02:00 cron in bundled mode. External mode operators add their own cron per runbook.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| client→/api/backup/run-now | Admin-only; viewer 403 |
| Docker socket mounted in ofelia sidecar | Read-only (`:ro`); ofelia only invokes `docker exec shifter shifter backup` (no API mutation) |
| pg_dump shell-out | exec.CommandContext with fixed binary path + parameterized args; PGPASSWORD in env (not command line) |
| backup tarball | Contains every domain row + floor-plan images; treat as full-data exfil if leaked |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-06-08-01 | Elevation of Privilege | viewer runs backup via curl | mitigate | `auth.RequireAction(sm, auth.ActionBackupRun)` on POST /api/backup/run-now; ActionBackupRun is admin-only. Test: TestHandler_ViewerCanRead asserts viewer POST → 403. |
| T-06-08-02 | Tampering | pg_restore --jobs accidentally injected | mitigate | Plan 06-08's Runner has hardcoded args slice; the test `TestRunner_PgDumpFlags` greps the slice for `-j` / `--jobs` and fails if present. Pitfall 1. |
| T-06-08-03 | Information Disclosure | backup tarball contains password hashes (user.password_hash) | accept | The tarball is the operator's complete backup; it MUST contain user.password_hash to be restorable. Mitigation is at the filesystem layer: SHIFTER_BACKUP_DIR is owned by 65532:65532 (nonroot); operator is responsible for filesystem encryption (LUKS/ZFS) per CONTEXT.md deferred ideas. |
| T-06-08-04 | Tampering | Docker socket escape via ofelia sidecar | mitigate | Docker socket mounted `:ro` (read-only API surface — list + exec but not write); ofelia config is label-driven (no arbitrary command execution via env var); the sidecar's only privileged action is `docker exec shifter shifter backup` (a binary inside the Shifter image). |
| T-06-08-05 | DoS | runaway backup fills disk | accept | SHIFTER_BACKUP_DIR is a Docker volume — disk full causes the backup to fail with a clear error message + audit row 'backup.failed'; the next attempt would just fail again. Operator manages rotation manually (deferred to v1.x per CONTEXT.md). The Settings card surfaces the most recent backup status so operators see persistent failures. |
| T-06-08-06 | Information Disclosure | tarball includes session tokens | mitigate | The `sessions` table is in the Shifter DB; pg_dump captures it. ON RESTORE, the session tokens point at the restored user.id values — which means restored DB resumes from the backup point, which is correct. Operationally: restoring a backup older than session_timeout means all sessions are expired anyway. Not a real disclosure issue. |
| T-06-08-07 | Tampering | tarball replay (substitute an old or malicious tarball) | mitigate (Plan 06-09 restore path) | Manifest contains per-file sha256; the outer tarball sha256 is in backup_run.sha256. Restore (Plan 06-09) verifies sha256 BEFORE running pg_restore. The current plan: just produces the artifact + checksums. Replay defense is at restore time. |
| T-06-08-08 | DoS | pg_dump hangs forever | mitigate | exec.CommandContext is wrapped with `context.WithTimeout(parent, 60*time.Minute)` (60-min cap); on timeout the cmd is killed and backup_run row goes status='failed' with error_message='timeout'. |
| T-06-08-09 | Repudiation | backup ran but no record | mitigate | Every backup writes one backup_run row (with status='running' first, then 'completed'/'failed') + two audit_log rows ('backup.start' + 'backup.complete'|'backup.failed'). Test: TestRunner_WritesAuditAndBackupRun. |
</threat_model>

<verification>
- `shifter backup` CLI smoke runs against the testcontainer DB
- /api/backup/run-now returns 202 + the backup_run.id
- /api/backup/list + /last power the Settings card (Plan 06-10)
- bundled compose has ofelia cron sidecar; external compose has volume but no cron
- Docker image bundles pg_dump + pg_restore + psql (verified by `docker run --rm shifter pg_dump --version`)
- `go test ./internal/cli/... ./internal/backup/... -count=1` passes
- Manifest schema matches RESEARCH §Decision A verbatim
</verification>

<success_criteria>
- OPS-02 covered: TimescaleDB-aware backup using pg_dump --format=custom + per-file sha256 + manifest + floor-plan rsync
- OPS-03 covered: destination configurable via SHIFTER_BACKUP_DIR + --to flag + Settings (Plan 06-10)
- SETT-05 partial: /api/backup/last + /list endpoints ready; Settings card UI is Plan 06-10
- D-39, D-40, D-41, D-42, D-43 (a + b) implemented; D-43 (c) Settings "Run backup now" button is Plan 06-10
</success_criteria>

<output>
After completion, create `.planning/phases/06-alerts-users-audit-operational-hardening/06-08-SUMMARY.md`
</output>
