---
phase: 06-alerts-users-audit-operational-hardening
plan: 09
type: execute
wave: 3
depends_on: [06-08]
files_modified:
  - internal/backup/restore.go
  - internal/backup/restore_test.go
  - internal/backup/roundtrip_test.go
  - internal/cli/restore.go
  - internal/cli/restore_test.go
  - internal/cli/root.go
  - .github/workflows/backup-restore-roundtrip.yml
  - docs/operator-runbook.md
autonomous: true
requirements: [OPS-04]
must_haves:
  truths:
    - "`shifter restore --from <tarball>` CLI exists; requires Shifter HTTP server to be DOWN (PG advisory lock check; refuses if Shifter is serving)"
    - "Restore sequence per RESEARCH §Decision A: 1) verify outer tarball sha256, 2) extract, 3) verify per-file sha256, 4) acquire pg_try_advisory_lock(0x5HIFTER1), 5) for each DB: drop/create + CREATE EXTENSION timescaledb + SELECT timescaledb_pre_restore() + pg_restore --no-owner --no-acl + SELECT timescaledb_post_restore(), 6) rsync floor plans, 7) release lock, 8) write audit row 'backup.restore'"
    - "NEVER passes -j/--jobs to pg_restore (Pitfall 1)"
    - "Manifest sha256 mismatch → abort with non-zero exit + clear error"
    - "CI workflow `.github/workflows/backup-restore-roundtrip.yml` runs same-version round-trip: seed → backup → drop+recreate DB → restore → smoke (row counts + 1 GET /api/sites/:id + 1 SELECT * FROM cagg_daily LIMIT 1)"
    - "Cross-version restore variant explicitly deferred to v1.1 per RESEARCH Open Question #1; the manifest's db_schema_version field is the forward-compat hook"
    - "Operator runbook section 'Backup & Restore' documents the exact `docker compose stop shifter` → `docker compose run --rm shifter restore --from <file>` → `docker compose start shifter` sequence"
  artifacts:
    - path: internal/backup/restore.go
      provides: "Restorer with pre_restore/post_restore wrapping + advisory lock + sha256 verify"
    - path: internal/cli/restore.go
      provides: "Cobra subcommand `shifter restore --from`"
    - path: .github/workflows/backup-restore-roundtrip.yml
      provides: "CI gate per D-45"
    - path: internal/backup/roundtrip_test.go
      provides: "TestBackupRestoreRoundtrip integration test"
  key_links:
    - from: internal/backup/restore.go
      to: pg_restore + timescaledb_pre_restore/post_restore
      via: "exec.CommandContext(\"pg_restore\", ...) wrapped by psql -c 'SELECT timescaledb_pre_restore()' BEFORE and 'SELECT timescaledb_post_restore()' AFTER"
      pattern: "timescaledb_pre_restore\\|timescaledb_post_restore"
    - from: .github/workflows/backup-restore-roundtrip.yml
      to: internal/backup/roundtrip_test.go
      via: "GitHub Actions runs `go test ./internal/backup/... -run TestBackupRestoreRoundtrip` against testcontainers"
      pattern: "TestBackupRestoreRoundtrip"
---

<objective>
Ship `shifter restore` CLI + CI round-trip job. Closes OPS-04 (restore round-trip tested in CI).

Purpose: the backup tarball produced by Plan 06-08 is only useful if a documented, CI-tested restore path exists. Plan 06-09 ships the inverse operation following the TimescaleDB-aware sequence (pre_restore + pg_restore + post_restore, NO --jobs), wraps in a PG advisory lock to refuse restore-while-running, and pins the round-trip-test gate in GitHub Actions.

Output: Restorer (pkg internal/backup) + Cobra subcommand + CI workflow YAML + operator runbook update.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-08-backup-cli-cron-PLAN.md
@internal/backup/manifest.go
@internal/backup/runner.go
@internal/cli/backup.go
@internal/cli/root.go
@docs/operator-runbook.md

<interfaces>
Plan 06-08 added:
- internal/backup/manifest.go::Manifest struct with `SHA256Sums map[string]string`, `Included []string`, ChirpStackMode, ChirpStackDBIncluded
- internal/backup/manifest.go::ComputeFileSHA256(path) (string, error)
- internal/backup/runner.go::RunnerConfig (DBHost/Port/User/Password/Name/etc.)
- internal/backup/store.go::Store with audit-tx helpers

Phase 1 install state via internal/install/state.go for chirpstack_mode.

PG advisory lock: `SELECT pg_try_advisory_lock(0x5348494654455231)` — 64-bit constant `0x5348494654455231` (hex of "SHIFTER1"). Decimal value 5,997,124,693,706,432,049 < INT64_MAX 9,223,372,036,854,775,807, so it fits BIGINT. Go declaration: `const ShifterAdvisoryLockID int64 = 0x5348494654455231`.

CI testcontainers pattern (Phase 1 already uses): testcontainers-go/postgres with `WithImage("timescale/timescaledb:2.26.0-pg16")`.

D-45 acceptance test sequence (RESEARCH §Decision B): seed 1 site / 1 MP / 1 device / 100 measurements / 1 floor plan / 5 audit rows → backup → drop schema → restore → smoke (per-table SELECT count(*) + 1 cagg_daily SELECT).
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: internal/backup/restore.go + Cobra `shifter restore` subcommand</name>
  <files>internal/backup/restore.go, internal/backup/restore_test.go, internal/cli/restore.go, internal/cli/restore_test.go, internal/cli/root.go</files>
  <read_first>
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Decision A "Exact Restore Procedure (full sequence)" — the in-shifter-restore pseudocode at lines ~388-410
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Pitfall 1 "pg_restore --jobs=N corrupts TimescaleDB catalogs" + §Pitfall 2 "timescaledb_pre_restore() forgotten"
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md §D-44
    - internal/audit/log.go (Plan 06-01 added `ActionBackupRestore` + `EntityTypeBackupRun` constants — use these instead of raw INSERT)
    - internal/backup/runner.go + manifest.go (Plan 06-08 substrate)
    - internal/cli/backup.go (Plan 06-08; mirror the subcommand pattern)
  </read_first>
  <behavior>
    - Test (TestRestorer_RefusesIfShifterRunning): acquire ShifterAdvisoryLockID from a parallel goroutine; call Restorer.Restore() → fails with `ErrShifterStillServing` and a helpful error message; backup_run rows untouched.
    - Test (TestRestorer_VerifiesOuterSHA256): tamper one byte of the tarball after backup but before restore → restore fails with `ErrTarballChecksumMismatch`.
    - Test (TestRestorer_VerifiesPerFileSHA256): tamper db/shifter.dump inside the tarball (rebuild gz with edited file) → restore fails with `ErrFileChecksumMismatch` referencing the changed filename.
    - Test (TestRestorer_RunsTimescaleHookSequence): mock the pg layer (testcontainers DB) → assert the SQL sequence runs IN ORDER: DROP DATABASE → CREATE DATABASE → CREATE EXTENSION timescaledb → SELECT timescaledb_pre_restore() → pg_restore (without -j) → SELECT timescaledb_post_restore(). Use a SQL capture hook OR inspect `pg_stat_statements` after restore.
    - Test (TestRestorer_NeverUsesJobsFlag): grep the args slice (or use a test double) to assert no `-j` or `--jobs` in any pg_restore invocation. (Pitfall 1 lint repeated for the restore path.)
    - Test (TestRestorer_FloorPlansRsync): tarball with `floor-plans/01H.png` + `floor-plans/02K.jpg` → after restore, `${FloorPlansDir}` contains both files with matching bytes.
    - Test (TestRestorer_ExternalMode_SkipsChirpstack): tarball with `chirpstack_db_included: false` → only the Shifter DB restored; no attempt to connect to a ChirpStack DB.
    - Test (TestRestorer_BundledMode_RestoresBoth): chirpstack_db_included: true → both DBs restored, Shifter FIRST then ChirpStack (D-44 ordering).
    - Test (TestRestorer_WritesAuditOnSuccess): one audit row 'backup.restore' present after success with entity_id = manifest install_id; user_id = NULL for CLI-trigger.
    - Test (TestRestorer_AbortsAndReleasesLockOnError): mid-restore failure → advisory lock released; manifest verification + tx rollback complete; the next restore attempt succeeds without manual intervention.
    - Test (TestCLIRestore_FlagParsing): `shifter restore --from /tmp/x.tar.gz` → reads flag; missing --from → usage error.
    - Test (TestCLIRestore_ExitsZeroOnSuccess + NonZeroOnFailure): roundtrip-tested via the integration test below.
  </behavior>
  <action>
    **internal/backup/restore.go** (RESEARCH §Decision A "Inside `shifter restore`" pseudocode):
    ```go
    package backup

    import (
        "archive/tar"
        "compress/gzip"
        "context"
        "encoding/json"
        "errors"
        "fmt"
        "io"
        "os"
        "os/exec"
        "path/filepath"
        "strings"
        "time"
        "github.com/jackc/pgx/v5"
        "github.com/jackc/pgx/v5/pgxpool"
        "log/slog"
    )

    const ShifterAdvisoryLockID int64 = 0x5348494654455231

    var (
        ErrShifterStillServing    = errors.New("restore: shifter is still serving — stop the shifter container before restoring (acquire advisory lock failed)")
        ErrTarballChecksumMismatch = errors.New("restore: outer tarball sha256 mismatch")
        ErrFileChecksumMismatch    = errors.New("restore: per-file sha256 mismatch")
        ErrManifestMissing         = errors.New("restore: manifest.json not found in tarball")
    )

    type Restorer struct {
        Pool          *pgxpool.Pool
        Cfg           RestorerConfig
        Log           *slog.Logger
    }
    type RestorerConfig struct {
        DBHost, DBUser, DBPassword, DBName string
        DBPort                              int
        ChirpStackDBName, ChirpStackDBUser  string
        FloorPlansDir                       string
    }

    // Restore reads the tarball at srcPath, verifies sha256s, runs the TimescaleDB
    // restore sequence for each DB in the manifest, rsyncs floor plans, and writes
    // a 'backup.restore' audit row. Refuses if Shifter is still serving (PG
    // advisory lock test).
    func (r *Restorer) Restore(ctx context.Context, srcPath string, expectedOuterSHA256 string) error {
        // 1. Verify outer tarball sha256 (if expected provided — operator can pass nothing to skip)
        if expectedOuterSHA256 != "" {
            actual, err := ComputeFileSHA256(srcPath)
            if err != nil { return fmt.Errorf("compute outer sha256: %w", err) }
            if actual != expectedOuterSHA256 { return fmt.Errorf("%w: expected %s got %s", ErrTarballChecksumMismatch, expectedOuterSHA256, actual) }
        }

        // 2. Acquire advisory lock; refuse if not free
        var acquired bool
        if err := r.Pool.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, ShifterAdvisoryLockID).Scan(&acquired); err != nil {
            return fmt.Errorf("acquire lock: %w", err)
        }
        if !acquired { return ErrShifterStillServing }
        defer func() {
            _, _ = r.Pool.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, ShifterAdvisoryLockID)
        }()

        // 3. Extract tarball to temp dir
        tmpDir, err := os.MkdirTemp("", "shifter-restore-")
        if err != nil { return err }
        defer os.RemoveAll(tmpDir)
        if err := extractTarGz(srcPath, tmpDir); err != nil { return fmt.Errorf("extract: %w", err) }

        // 4. Parse manifest
        manifestPath := filepath.Join(tmpDir, "manifest.json")
        manifestBytes, err := os.ReadFile(manifestPath)
        if err != nil { return ErrManifestMissing }
        var manifest Manifest
        if err := json.Unmarshal(manifestBytes, &manifest); err != nil { return fmt.Errorf("parse manifest: %w", err) }

        // 5. Verify per-file sha256s against manifest.SHA256Sums
        for relPath, expectedSum := range manifest.SHA256Sums {
            if relPath == "manifest.json" { continue }
            fullPath := filepath.Join(tmpDir, relPath)
            // For directories (floor-plans/), iterate files
            info, err := os.Stat(fullPath)
            if err != nil { return fmt.Errorf("file %s missing: %w", relPath, err) }
            if info.IsDir() { continue } // dirs aren't in sha256 map; per-file entries cover floor-plans/<uuid>.png
            actual, err := ComputeFileSHA256(fullPath)
            if err != nil { return fmt.Errorf("sha256 %s: %w", relPath, err) }
            if actual != expectedSum {
                return fmt.Errorf("%w: file=%s expected=%s actual=%s", ErrFileChecksumMismatch, relPath, expectedSum, actual)
            }
        }

        // 6. Restore each DB
        if err := r.restoreDB(ctx, filepath.Join(tmpDir, "db/shifter.dump"), r.Cfg.DBUser, r.Cfg.DBName); err != nil {
            return fmt.Errorf("restore shifter DB: %w", err)
        }
        if manifest.ChirpStackDBIncluded {
            chirpDumpPath := filepath.Join(tmpDir, "db/chirpstack.dump")
            if err := r.restoreDB(ctx, chirpDumpPath, r.Cfg.ChirpStackDBUser, r.Cfg.ChirpStackDBName); err != nil {
                return fmt.Errorf("restore chirpstack DB: %w", err)
            }
        }

        // 7. Rsync floor plans
        srcDir := filepath.Join(tmpDir, "floor-plans")
        if _, err := os.Stat(srcDir); err == nil {
            if err := copyDir(srcDir, r.Cfg.FloorPlansDir); err != nil { return fmt.Errorf("rsync floor-plans: %w", err) }
        }

        // 8. Write audit row via audit.WriteEntry (use Plan 06-01 constants).
        // Goes through the shared writer for consistency with every other audit
        // call site in Shifter; matches Plan 06-08's audit-in-tx pattern.
        tx, err := r.Pool.BeginTx(ctx, pgx.TxOptions{})
        if err != nil { return err }
        defer tx.Rollback(ctx)
        if err := audit.WriteEntry(ctx, tx, audit.Entry{
            Action:     audit.ActionBackupRestore,            // Plan 06-01 const "backup.restore"
            EntityType: audit.EntityTypeBackupRun,            // Plan 06-01 const "backup_run"
            EntityID:   mustUUIDFromString(manifest.InstallID),
            Notes:      fmt.Sprintf("restored from %s (size=%d, install_slug=%s, schema=%s)", filepath.Base(srcPath), len(manifestBytes), manifest.InstallSlug, manifest.DBSchemaVersion),
        }); err != nil { return fmt.Errorf("audit: %w", err) }
        return tx.Commit(ctx)
    }

    // restoreDB does the TimescaleDB-aware sequence for one DB.
    // Pitfall 2 mitigation: ALWAYS wrap pg_restore in pre/post.
    // Pitfall 1 mitigation: NEVER use -j / --jobs.
    func (r *Restorer) restoreDB(ctx context.Context, dumpPath, dbUser, dbName string) error {
        // 1. Drop + recreate database (must be done from a different DB — connect to "postgres")
        if err := r.execPsql(ctx, "postgres", fmt.Sprintf(`DROP DATABASE IF EXISTS %q;`, dbName)); err != nil {
            return fmt.Errorf("drop db %s: %w", dbName, err)
        }
        if err := r.execPsql(ctx, "postgres", fmt.Sprintf(`CREATE DATABASE %q WITH OWNER %q;`, dbName, dbUser)); err != nil {
            return fmt.Errorf("create db %s: %w", dbName, err)
        }

        // 2. CREATE EXTENSION timescaledb (only Shifter's DB has it; ChirpStack DB skips it)
        if dbName == r.Cfg.DBName { // is the Shifter DB
            if err := r.execPsql(ctx, dbName, `CREATE EXTENSION IF NOT EXISTS timescaledb;`); err != nil {
                return fmt.Errorf("create extension timescaledb: %w", err)
            }
            // 3. SELECT timescaledb_pre_restore();
            if err := r.execPsql(ctx, dbName, `SELECT timescaledb_pre_restore();`); err != nil {
                return fmt.Errorf("pre_restore: %w", err)
            }
        }

        // 4. pg_restore (NO -j)
        args := []string{
            "--host", r.Cfg.DBHost,
            "--port", fmt.Sprintf("%d", r.Cfg.DBPort),
            "--username", dbUser,
            "--dbname", dbName,
            "--no-owner",
            "--no-acl",
            dumpPath,
        }
        // SANITY: enforce Pitfall 1 — refuse if any caller injects -j
        for _, a := range args {
            if a == "-j" || a == "--jobs" || strings.HasPrefix(a, "--jobs=") {
                return fmt.Errorf("restore: -j/--jobs forbidden (TimescaleDB Pitfall 1)")
            }
        }
        cmd := exec.CommandContext(ctx, "pg_restore", args...)
        cmd.Env = append(os.Environ(), "PGPASSWORD="+r.Cfg.DBPassword)
        if out, err := cmd.CombinedOutput(); err != nil {
            return fmt.Errorf("pg_restore %s: %w: %s", dbName, err, string(out))
        }

        // 5. SELECT timescaledb_post_restore() (Shifter DB only)
        if dbName == r.Cfg.DBName {
            if err := r.execPsql(ctx, dbName, `SELECT timescaledb_post_restore();`); err != nil {
                return fmt.Errorf("post_restore: %w", err)
            }
        }
        return nil
    }

    func (r *Restorer) execPsql(ctx context.Context, dbName string, sql string) error {
        cmd := exec.CommandContext(ctx, "psql",
            "--host", r.Cfg.DBHost,
            "--port", fmt.Sprintf("%d", r.Cfg.DBPort),
            "--username", r.Cfg.DBUser,
            "--dbname", dbName,
            "--command", sql,
        )
        cmd.Env = append(os.Environ(), "PGPASSWORD="+r.Cfg.DBPassword)
        if out, err := cmd.CombinedOutput(); err != nil {
            return fmt.Errorf("psql failed: %w: %s", err, string(out))
        }
        return nil
    }
    ```

    **internal/cli/restore.go** mirrors backup.go pattern:
    ```go
    var restoreCmd = &cobra.Command{
        Use:   "restore",
        Short: "Restore a Shifter backup tarball (Shifter must be stopped)",
        Long:  `Verifies tarball sha256, acquires PG advisory lock (refuses if Shifter is serving), runs timescaledb_pre_restore -> pg_restore --no-jobs -> timescaledb_post_restore for the Shifter DB and optionally the ChirpStack DB (bundled mode), rsyncs floor plans, and writes a 'backup.restore' audit row.

Operator workflow:
  docker compose stop shifter
  docker compose run --rm shifter shifter restore --from /var/lib/shifter/backups/<file>.tar.gz
  docker compose start shifter`,
        RunE:  runRestoreCmd,
    }
    var restoreFlagFrom string
    var restoreFlagExpectedSHA256 string
    func init() {
        restoreCmd.Flags().StringVar(&restoreFlagFrom, "from", "", "Tarball path (.tar.gz) to restore")
        restoreCmd.Flags().StringVar(&restoreFlagExpectedSHA256, "expected-sha256", "", "Optional: expected outer tarball sha256 (refuses restore on mismatch)")
        _ = restoreCmd.MarkFlagRequired("from")
        rootCmd.AddCommand(restoreCmd)
    }
    func runRestoreCmd(cmd *cobra.Command, args []string) error {
        // ... build Restorer + call Restore + print result
    }
    ```

    Add restore command registration in `internal/cli/root.go` (or rely on init() pattern Plan 06-08 used).
  </action>
  <verify>
    <automated>go test ./internal/backup/... ./internal/cli/... -run "TestRestorer|TestCLIRestore" -count=1 -timeout=180s</automated>
  </verify>
  <acceptance_criteria>
    - `internal/backup/restore.go` declares `const ShifterAdvisoryLockID int64 = 0x5348494654455231`
    - `internal/backup/restore.go` contains the literal string `pg_try_advisory_lock` and `pg_advisory_unlock`
    - `internal/backup/restore.go` calls `timescaledb_pre_restore` AND `timescaledb_post_restore` for the Shifter DB ONLY (grep both strings; verify they wrap `exec.CommandContext(ctx, "pg_restore"`)
    - `internal/backup/restore.go` source contains a sanity check `if a == "-j" || a == "--jobs"` (Pitfall 1 belt-and-suspenders)
    - `internal/cli/restore.go` (CLI orchestrator) AND `internal/backup/restore.go` audit-write paths use `audit.WriteEntry` with `audit.ActionBackupRestore` constant — NO raw `INSERT INTO audit_log` statements: `grep "audit.WriteEntry" internal/backup/restore.go` returns ≥ 1 AND `grep "INSERT INTO audit_log" internal/backup/restore.go` returns 0
    - `internal/cli/restore.go` declares `restoreFlagFrom` + `MarkFlagRequired("from")`
    - All 12 listed tests pass: `go test ./internal/backup/... ./internal/cli/... -run "TestRestorer|TestCLIRestore" -count=1 -timeout=180s` exits 0
    - `TestRestorer_NeverUsesJobsFlag` source asserts the args slice contains no `-j` substring
    - `TestRestorer_RefusesIfShifterRunning` creates the lock contention scenario and asserts `errors.Is(err, ErrShifterStillServing)`
    - `TestRestorer_RunsTimescaleHookSequence` verifies the SQL sequence via `pg_stat_statements` or a captured exec.Command spy
  </acceptance_criteria>
  <done>Restorer ships with all safety properties: advisory lock + sha256 verify + TimescaleDB pre/post wrapping + Pitfall 1 lint + audit row write.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: CI round-trip workflow + integration test + operator runbook section</name>
  <files>internal/backup/roundtrip_test.go, .github/workflows/backup-restore-roundtrip.yml, docs/operator-runbook.md</files>
  <read_first>
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Decision B "Exact CI Job Shape" + "Test Implementation"
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md §D-45
    - docs/operator-runbook.md (current content; this plan appends "Backup & Restore" section)
    - internal/cli/testharness.go OR internal/db/migrations seed pattern (for the test fixture seed function)
  </read_first>
  <behavior>
    - Test (TestBackupRestoreRoundtrip): testcontainers spin up Postgres+TimescaleDB → apply all migrations 0001..0047 (Phase 6 ships 0037-0047; 06-09 depends on 06-08's 0045 backup_run migration) → seed (1 site, 1 MP, 1 device, 100 measurements, 1 floor plan file on disk, 5 audit rows) → call Runner.Backup → assert tarball size > 1024 → DROP SCHEMA public CASCADE + CREATE SCHEMA public + CREATE EXTENSION timescaledb → re-apply migrations → call Restorer.Restore → assert: SELECT count(*) FROM site = 1, FROM metering_point = 1, FROM device = 1, FROM measurement = 100, FROM audit_log >= 5 (the +1 backup.restore audit row is added during restore; expect 5 original + 1 restore row = 6), 1 floor-plan file exists, SELECT count(*) FROM measurement_daily > 0 (CAGG state survived).
    - Test (TestBackupRestoreRoundtrip_CAGGSurvival): seed measurements spanning ≥ 2 hours, manually `CALL refresh_continuous_aggregate('measurement_hourly', ...)` before backup; after restore + post_restore the CAGG row count matches pre-backup count.
    - Test (TestBackupRestoreRoundtrip_BundledMode): set chirpstack_mode='bundled' + seed 1 ChirpStack DB row → backup → drop both DBs → restore → both DBs present.
    - Test (TestBackupRestoreRoundtrip_ExternalMode): chirpstack_mode='external' → tarball has no chirpstack.dump; restore skips chirpstack DB.
  </behavior>
  <action>
    **internal/backup/roundtrip_test.go** (RESEARCH §Decision B test verbatim, adapted to the actual seeded fixture):
    ```go
    //go:build integration
    // +build integration

    package backup_test

    import (
        "context"
        "os"
        "path/filepath"
        "testing"
        "github.com/jackc/pgx/v5/pgxpool"
        "github.com/stretchr/testify/require"
        "github.com/testcontainers/testcontainers-go"
        tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
        "github.com/shifter-io/shifter/internal/backup"
        "github.com/shifter-io/shifter/internal/db"
    )

    // TestBackupRestoreRoundtrip enforces OPS-04 in CI:
    //   seed → backup → fresh DB → restore → smoke
    func TestBackupRestoreRoundtrip(t *testing.T) {
        ctx := context.Background()

        // 1. Spin up PG+TimescaleDB
        pg, err := tcpostgres.RunContainer(ctx,
            testcontainers.WithImage("timescale/timescaledb:2.26.0-pg16"),
            tcpostgres.WithDatabase("shifter"),
            tcpostgres.WithUsername("shifter"),
            tcpostgres.WithPassword("test"),
        )
        require.NoError(t, err)
        defer pg.Terminate(ctx)

        connStr, _ := pg.ConnectionString(ctx, "sslmode=disable")
        pool, err := pgxpool.New(ctx, connStr)
        require.NoError(t, err)
        defer pool.Close()

        require.NoError(t, db.RunMigrations(ctx, pool))

        // 2. Seed fixture
        seedRoundtripFixture(t, ctx, pool)

        // 3. Backup
        tmpDir := t.TempDir()
        floorPlansDir := filepath.Join(tmpDir, "floor-plans")
        require.NoError(t, os.MkdirAll(floorPlansDir, 0o755))
        require.NoError(t, os.WriteFile(filepath.Join(floorPlansDir, "test.png"), []byte("test-png-bytes"), 0o644))
        runner := backup.NewRunner(pool, /* RunnerConfig with floorPlansDir + db creds */)
        backupID, tarballPath, err := runner.Backup(ctx, tmpDir, "cli", nil)
        require.NoError(t, err)
        _ = backupID
        fi, err := os.Stat(tarballPath)
        require.NoError(t, err)
        require.Greater(t, fi.Size(), int64(1024))

        // 4. Drop + recreate (disaster simulation).
        // The integration test uses DROP SCHEMA (not DROP DATABASE) because
        // testcontainers gives the test a single bootstrapped database whose
        // name is fixed at container-spawn time — there is no superuser-owned
        // 'postgres' DB connection available to issue DROP DATABASE against
        // the test DB without re-orchestrating the container. DROP SCHEMA
        // public CASCADE removes every table/index/extension-schema-object in
        // the test DB; the subsequent pg_restore reconstructs the schema from
        // the dump (pg_restore --format=custom replays CREATE TABLE statements
        // and re-installs the timescaledb extension's schema artifacts). The
        // PRODUCTION restore path (internal/backup/restore.go) uses the full
        // DROP DATABASE + CREATE DATABASE sequence per RESEARCH §Decision A
        // because it has the postgres maintenance-DB connection available.
        // Both paths achieve the same end-state: an empty target ready for
        // pg_restore. The test's DROP SCHEMA is a tighter operation but is
        // semantically equivalent for the round-trip assertion.
        _, err = pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`)
        require.NoError(t, err)

        // 5. Restore
        restorer := backup.NewRestorer(pool, /* RestorerConfig with floorPlansDir for the restored dir */)
        require.NoError(t, restorer.Restore(ctx, tarballPath, "" /* no expected sha — skip outer verify */))

        // 6. Smoke assertions
        var n int
        require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM site`).Scan(&n))
        require.Equal(t, 1, n)
        require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM metering_point`).Scan(&n))
        require.Equal(t, 1, n)
        require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM device`).Scan(&n))
        require.Equal(t, 1, n)
        require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM measurement`).Scan(&n))
        require.Equal(t, 100, n)
        require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM audit_log`).Scan(&n))
        require.GreaterOrEqual(t, n, 6) // 5 seeded + 1 backup.restore from the restorer
        // CAGG survival
        require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM measurement_daily`).Scan(&n))
        require.Greater(t, n, 0)
    }

    func seedRoundtripFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
        // 1 site, 1 metering_point, 1 device, 100 measurements (over 2 hours so CAGG has data),
        // 1 floor plan (referenced by row + file already exists in floorPlansDir),
        // 5 audit_log rows with valid vocabulary
        // CALL refresh_continuous_aggregate('measurement_hourly', NULL, NULL); + measurement_daily
    }
    ```

    **.github/workflows/backup-restore-roundtrip.yml** (RESEARCH §Decision B verbatim):
    ```yaml
    name: backup-restore-roundtrip

    on:
      pull_request:
        paths:
          - 'internal/backup/**'
          - 'internal/cli/backup.go'
          - 'internal/cli/restore.go'
          - 'internal/db/migrations/**'
          - 'Dockerfile'
          - '.github/workflows/backup-restore-roundtrip.yml'
      push:
        branches: [main]

    jobs:
      roundtrip:
        runs-on: ubuntu-latest
        timeout-minutes: 15
        steps:
          - uses: actions/checkout@v4

          - uses: actions/setup-go@v5
            with:
              go-version: '1.26'
              cache: true

          - name: Install postgres client (for pg_dump/restore tests)
            run: sudo apt-get update && sudo apt-get install -y postgresql-client-16

          - name: Run round-trip integration test
            run: go test ./internal/backup/... -run TestBackupRestoreRoundtrip -tags=integration -count=1 -v -timeout=10m
            env:
              TESTCONTAINERS_RYUK_DISABLED: "false"
    ```

    **docs/operator-runbook.md** — append a new section "Backup & Restore":
    ```markdown
    ## Backup & Restore

    ### Backup (manual)

    Bundled mode:
    ```sh
    docker compose -f compose/bundled.yml exec shifter shifter backup --to /var/lib/shifter/backups
    ```
    External mode:
    ```sh
    docker compose -f compose/external.yml exec shifter shifter backup --to /var/lib/shifter/backups
    ```

    Output: `/var/lib/shifter/backups/shifter-backup-<slug>-<YYYYMMDD-HHMM>-<schema>.tar.gz` + `backup_run` row in DB.

    ### Backup (scheduled, bundled mode)

    Bundled compose ships a `backup-cron` sidecar (mcuadros/ofelia:v0.3.22) that runs `shifter backup` daily at 02:00 install_tz. To change the schedule, edit `compose/bundled.yml` and update the `ofelia.job-exec.shifter-backup.schedule` label, then `docker compose up -d backup-cron`.

    ### Backup (scheduled, external mode)

    External mode does NOT include the cron sidecar. Set up your preferred cron (system cron, k8s CronJob, etc.) to run:
    ```sh
    docker compose -f compose/external.yml exec -T shifter shifter backup --to /var/lib/shifter/backups --trigger=cron
    ```

    ### Restore (manual; Shifter must be stopped)

    ```sh
    docker compose -f compose/bundled.yml stop shifter
    docker compose -f compose/bundled.yml run --rm shifter shifter restore --from /var/lib/shifter/backups/<file>.tar.gz
    docker compose -f compose/bundled.yml start shifter
    ```

    Restore safety:
    - Refuses to run if Shifter HTTP server is still up (PG advisory lock).
    - Verifies per-file sha256 against the manifest.
    - Wraps pg_restore in `SELECT timescaledb_pre_restore()` and `SELECT timescaledb_post_restore()` (required for TimescaleDB metadata).
    - Restores the Shifter DB first; in bundled mode, ChirpStack DB second.
    - Rsyncs the floor-plans volume from the tarball.
    - Writes a `backup.restore` audit row.

    Cross-version restore (e.g., v0.5.0 backup → v0.6.0 install) is **not supported in v1**. The manifest's `db_schema_version` field is the forward-compat hook for v1.1.

    ### CI gate

    `.github/workflows/backup-restore-roundtrip.yml` runs the full round-trip (seed → backup → drop → restore → smoke) on every PR touching `internal/backup/**` or `internal/db/migrations/**` and on every push to main. The workflow fails CI if backup or restore produces invalid output.
    ```

    Wire the integration test build tag (`//go:build integration`) so it only runs in CI by default; local fast tests use `go test ./internal/backup/...` without the tag.
  </action>
  <verify>
    <automated>go test -tags=integration ./internal/backup/... -run TestBackupRestoreRoundtrip -count=1 -v -timeout=10m</automated>
  </verify>
  <acceptance_criteria>
    - `.github/workflows/backup-restore-roundtrip.yml` exists at the exact path
    - Workflow has `on.pull_request.paths` including `'internal/backup/**'` and `'internal/cli/backup.go'` and `'internal/cli/restore.go'`
    - Workflow installs `postgresql-client-16` and runs `go test ./internal/backup/... -run TestBackupRestoreRoundtrip -tags=integration`
    - `internal/backup/roundtrip_test.go` contains `//go:build integration` build tag
    - Roundtrip test seeds: 1 site, 1 MP, 1 device, 100 measurements, 1 floor plan, 5 audit rows; asserts post-restore counts; asserts measurement_daily count > 0 (CAGG survival)
    - `docs/operator-runbook.md` contains a new section `## Backup & Restore` with the 4 sub-headings (Backup manual, scheduled bundled, scheduled external, Restore)
    - The operator runbook explicitly states cross-version restore is not supported in v1
    - Integration test `go test -tags=integration ./internal/backup/... -run TestBackupRestoreRoundtrip -count=1 -v -timeout=10m` passes locally (assumes Docker is running)
  </acceptance_criteria>
  <done>OPS-04 is satisfied — the CI gate fails any change that breaks backup/restore round-trip semantics. Operators have an exact written procedure for restore.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| restore CLI → DB | Restorer requires the Shifter HTTP server to be DOWN; acquires PG advisory lock to enforce |
| tarball file → restore | Untrusted bytes; verified against manifest sha256 BEFORE pg_restore touches the DB |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-06-09-01 | Tampering | malicious tarball replays old / forged data | mitigate | Outer sha256 verification (optional via --expected-sha256 flag) + per-file sha256 from manifest.json (mandatory). Manifest itself is inside the same tarball, so the outer sha256 covers it. Operator captures the manifest sha at backup time (printed by `shifter backup`); they pass it to `shifter restore --expected-sha256` for full chain-of-custody. Test: TestRestorer_VerifiesOuterSHA256 + TestRestorer_VerifiesPerFileSHA256. |
| T-06-09-02 | Tampering | concurrent restore + serve | mitigate | PG `pg_try_advisory_lock(0x5348494654455231)` at restore start; the Shifter serve.go ALSO acquires this lock on boot (Plan 06-09 sub-extension: ADD lock acquisition in cli/serve.go's init phase). If serve already has it, restore fails with `ErrShifterStillServing` and a helpful message ("docker compose stop shifter"). Test: TestRestorer_RefusesIfShifterRunning. |
| T-06-09-03 | Tampering | restore with --jobs corrupts TimescaleDB catalogs | mitigate | Pitfall 1 lint at TWO layers: (a) restore.go's args slice never contains -j/--jobs (compile-time hard-coded); (b) runtime sanity check loops over args and errors if any caller injects them. Plus a grep test in CI workflow line. |
| T-06-09-04 | Tampering | restore skips pre_restore → CAGG corruption | mitigate | restore.go's restoreDB ALWAYS calls timescaledb_pre_restore BEFORE pg_restore and timescaledb_post_restore AFTER. Test TestRestorer_RunsTimescaleHookSequence asserts the SQL sequence. Pitfall 2. |
| T-06-09-05 | Information Disclosure | restore writes plaintext SQL data to disk | accept | Restore extraction is to a TempDir which is RemoveAll'd via defer; on success the bytes only reach the DB. On crash/panic, OS auto-cleans /tmp on reboot. |
| T-06-09-06 | DoS | restore takes forever | accept | pg_restore is single-threaded for TimescaleDB (Pitfall 1 forbids --jobs); restore time scales linearly with backup size. CI workflow has 15-min timeout; production operator scales to backup size empirically. |
| T-06-09-07 | Repudiation | restore happens with no record | mitigate | restoreDB writes a 'backup.restore' audit row in the restored DB (mandated by the audit_log INSERT-ONLY trigger which is restored as part of pg_restore). Test: TestRestorer_WritesAuditOnSuccess. |
| T-06-09-08 | Elevation of Privilege | restore creates DB with wrong owner | mitigate | DROP DATABASE + CREATE DATABASE WITH OWNER clauses use parameterized identifiers from RestorerConfig (operator-trusted); pg_restore --no-owner --no-acl preserves data without GRANT/REVOKE changes. |
</threat_model>

<verification>
- TestBackupRestoreRoundtrip passes locally + in CI
- The full restore sequence is in place (sha256 → advisory lock → pre_restore → pg_restore → post_restore → audit row)
- Operator runbook documents the exact procedure including cross-version-restore deferral note
- D-45 same-version round-trip is CI-gated
- Cross-version variant explicitly filed as v1.1 prep
</verification>

<success_criteria>
- OPS-04 covered: restore round-trip tested in CI on every backup-relevant change
- D-44 implemented: advisory lock + manifest verify + pre/post restore + rsync + audit row
- D-45 same-version variant shipped + cross-version deferred per Open Question #1
- Pitfall 1 + Pitfall 2 both mitigated with belt-and-suspenders linting
</success_criteria>

<output>
After completion, create `.planning/phases/06-alerts-users-audit-operational-hardening/06-09-SUMMARY.md`
</output>
