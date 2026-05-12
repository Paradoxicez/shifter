---
phase: "06"
plan: "08"
subsystem: backup
tags: [backup, pg_dump, timescaledb, cli, cron, ofelia, ops]
dependency_graph:
  requires: [06-01-alert-engine-substrate, db-migrations-0046]
  provides: [backup-package, backup-cli-subcommand, backup-http-endpoints, ofelia-cron-sidecar]
  affects: [internal/backup, internal/cli, internal/http/router, compose/bundled, compose/external, Dockerfile]
tech_stack:
  added:
    - mcuadros/ofelia:v0.3.22 (cron sidecar, bundled compose only)
    - debian:bookworm-slim as pgclient stage (pg_dump/pg_restore/psql binaries)
    - archive/tar + compress/gzip stdlib (tarball construction)
    - crypto/sha256 stdlib (per-file + outer tarball checksums)
  patterns:
    - audit-in-transaction (backup.start committed before pg_dump; backup.complete/failed after)
    - BackupWithNotify channel pattern (run ID sent on channel before pg_dump starts for HTTP 202)
    - nil-guard router mounting (BackupDeps *backup.Deps in http.Deps)
    - exec.CommandContext with PGPASSWORD env (never CLI arg)
key_files:
  created:
    - internal/db/migrations/0046_backup_run.up.sql
    - internal/db/migrations/0046_backup_run.down.sql
    - internal/backup/doc.go
    - internal/backup/manifest.go
    - internal/backup/manifest_test.go
    - internal/backup/store.go
    - internal/backup/store_test.go
    - internal/backup/runner.go
    - internal/backup/runner_test.go
    - internal/backup/handler.go
    - internal/backup/handler_test.go
    - internal/cli/backup.go
    - internal/cli/backup_test.go
  modified:
    - internal/auth/authz.go (ActionBackupRun, ActionBackupRead, ActionBackupConfigure + roleBundles)
    - internal/config/config.go (BackupDir field + SHIFTER_BACKUP_DIR default)
    - internal/cli/root.go (backupCmd added to Execute() AddCommand list)
    - internal/http/router.go (BackupDeps field + backup.RegisterRoutes mount)
    - internal/db/migrations_test.go (version assertion 45→46)
    - internal/db/roundtrip_test.go (version assertion 45→46)
    - Dockerfile (pgclient stage: debian:bookworm-slim with postgresql-client-16)
    - compose/bundled.yml (ofelia sidecar + backups volume + SHIFTER_BACKUP_DIR env)
    - compose/external.yml (backups volume + SHIFTER_BACKUP_DIR env, no ofelia)
    - install/bundled/install.sh (mkdir -p /var/lib/shifter/backups + chown 65532)
    - install/external/install.sh (mkdir -p /var/lib/shifter/backups + chown 65532)
decisions:
  - "Migration 0046 used instead of plan's 0045 (0045 already owned by 06-02 device_gateway_link); downstream plans 06-10 and 06-11 must increment their migration numbers accordingly"
  - "BackupWithNotify() channel pattern: Runner sends run ID on buffered chan<- uuid.UUID after first tx commits (before pg_dump starts); HTTP handler selects on it to return 202 with job_id without double-inserting rows"
  - "debian:bookworm-slim chosen for pgclient stage (not Alpine) to ensure glibc ABI compatibility with distroless/static-debian12 runtime"
  - "pg_dump args hardcoded: --format=custom --no-owner --no-acl; --jobs/-j NEVER used (TimescaleDB PITFALL #1: parallel dump corrupts hypertable catalog)"
  - "TestCLIBackup_CommandIsRegistered rewritten to use isolated cobra root instead of calling package-level Execute() to prevent shared-state race with TestTestHarnessCmd_Help"
  - "ActionBackupRead granted to viewer role (D-46); ActionBackupRun admin-only (T-06-08-01)"
metrics:
  duration_minutes: 17
  completed_date: "2026-05-12"
  tasks_completed: 2
  files_created: 13
  files_modified: 11
  commits: 2
---

# Phase 06 Plan 08: Backup CLI + Cron + Ofelia Sidecar Summary

**One-liner:** pg_dump-based backup package with `shifter backup` CLI, HTTP run-now/list/last/jobs endpoints (OPS-02/OPS-03), and ofelia:v0.3.22 daily cron in bundled compose.

## What Was Built

### Task 1: Migration + internal/backup package + Dockerfile pgclient stage

**Migration 0046_backup_run** — `backup_run` table tracking every backup attempt:
- `status CHECK ('running','completed','failed')`, `trigger_kind CHECK ('cli','cron','api')`
- `manifest_json JSONB`, `sha256 TEXT`, `file_name`, `file_size_bytes`, `error_message`
- `triggered_by UUID` FK to users (nullable — cron has no user)

**internal/backup package:**
- `doc.go` — `DefaultBackupDir = "/var/lib/shifter/backups"`, package doc with tarball layout
- `manifest.go` — `Manifest` struct (11 fields), `ComputeFileSHA256`, `ComputeReaderSHA256`, `VerifyFileSHA256`
- `store.go` — `Store` with `InsertStartedTx`, `UpdateCompletedTx`, `UpdateFailedTx`, `ListRecent(n)`, `Last`, `Get`
- `runner.go` — `Runner.Backup(ctx, destDir, triggerKind, userID)` full orchestration:
  - Audit-in-transaction: `backup.start` committed before pg_dump; `backup.complete`/`backup.failed` after
  - `runPgDump()` via `exec.CommandContext` with PGPASSWORD env, `--format=custom --no-owner --no-acl`
  - 60-minute context timeout on pg_dump (T-06-08-08)
  - Bundled mode: dumps both Shifter DB and ChirpStack DB; external mode: Shifter-only
  - Tarball: `shifter-backup-{slug}-{YYYYMMDD-HHMM}-{schema}.tar.gz`
  - `BackupWithNotify(ctx, destDir, triggerKind, userID, idCh chan<- uuid.UUID)` for HTTP async dispatch
- `handler.go` — `Deps{Runner, Store, SessionMgr, Log}`, `RegisterRoutes(r, Deps)`:
  - `GET /api/backup/list` — 5 most recent rows (ActionBackupRead)
  - `GET /api/backup/last` — `{never_run: true}` or `{age_seconds, ...}` (ActionBackupRead)
  - `GET /api/backup/jobs/{id}` — poll by UUID (ActionBackupRead)
  - `POST /api/backup/run-now` — goroutine + channel → 202 `{job_id, status:"running"}` (ActionBackupRun)

**Dockerfile pgclient stage:** `debian:bookworm-slim` stage installs `postgresql-client-16`; COPY of pg_dump, pg_restore, psql binaries + required shared libraries into distroless runtime.

**Auth changes (authz.go):** `ActionBackupRun`, `ActionBackupRead`, `ActionBackupConfigure` constants; admin gets all three; viewer gets `ActionBackupRead` only (D-46, T-06-08-01).

**Config changes:** `BackupDir string` field with `SHIFTER_BACKUP_DIR` env binding and `/var/lib/shifter/backups` default.

### Task 2: CLI subcommand + HTTP router + compose + install scripts

**internal/cli/backup.go** — `shifter backup --to <dir> --trigger cli|cron|api`:
- Resolves destDir: `--to` flag > `cfg.BackupDir` > `backup.DefaultBackupDir`
- Validates trigger kind before opening DB connection
- Reads `install_identity` for `chirpstack_mode`/`slug`/`id`; reads `schema_migrations` version
- Prints JSON result `{backup_run_id, file, status, size_bytes}` to stdout

**internal/cli/root.go** — `backupCmd` added to `Execute()` `AddCommand` list (8 canonical subcommands).

**internal/http/router.go** — `BackupDeps *backup.Deps` field; nil-guard mount of `backup.RegisterRoutes` before SPA fallback (PITFALL #4 preserved).

**compose/bundled.yml:**
- `backup-cron` ofelia service (`mcuadros/ofelia:v0.3.22`, Docker label-driven schedule `@daily`, overridable via `SHIFTER_BACKUP_SCHEDULE`)
- `backups:` named volume + `/var/lib/shifter/backups` mount on shifter service
- `SHIFTER_BACKUP_DIR: /var/lib/shifter/backups` env

**compose/external.yml:** `backups:` volume + mount + env (no ofelia — external operators own their cron per D-43).

**install scripts (both flavors):** `mkdir -p /var/lib/shifter/backups && chown 65532:65532` (distroless nonroot UID).

## Commits

| Hash | Message |
|------|---------|
| b17c622 | feat(06-08): migration 0046_backup_run + internal/backup package + Dockerfile pgclient stage |
| 6c2de84 | feat(06-08): backup CLI subcommand + HTTP endpoints + ofelia cron sidecar |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Migration number conflict: 0045 already taken by 06-02**
- **Found during:** Task 1
- **Issue:** Plan referenced `0045_backup_run` but `0045_device_gateway_link` was already created by plan 06-02. Using the same number would cause `golang-migrate` to silently skip or error.
- **Fix:** Used `0046_backup_run` instead. Updated `migrations_test.go` and `roundtrip_test.go` version assertions from 45→46.
- **Files modified:** `internal/db/migrations/0046_backup_run.{up,down}.sql`, `internal/db/migrations_test.go`, `internal/db/roundtrip_test.go`
- **Downstream impact:** Plans 06-09 and 06-11 must use 0047 and 0048 respectively for their migrations.
- **Commits:** b17c622

**2. [Rule 1 - Bug] Double-insert in RunNowHandler: pre-inserted row + Runner.Backup both created backup_run rows**
- **Found during:** Task 1 (handler design review)
- **Issue:** Original handler design pre-inserted a `status='running'` row, then called `Runner.Backup()` which also inserts its own start row — creating 2 rows per backup.
- **Fix:** Removed pre-insert from handler. Added `BackupWithNotify(ctx, dest, triggerKind, userID, idCh chan<- uuid.UUID)` to Runner: sends run ID on buffered channel after first tx commits (before pg_dump starts). Handler selects on channel for ID, returns 202.
- **Files modified:** `internal/backup/runner.go`, `internal/backup/handler.go`
- **Commits:** b17c622

**3. [Rule 1 - Bug] TestCLIBackup_CommandIsRegistered called Execute() causing shared-state race**
- **Found during:** Task 2 (test run)
- **Issue:** `Execute()` sets `TestHarnessCmd.parent = rootCmd`. `TestTestHarnessCmd_Help` then calls `root.AddCommand(TestHarnessCmd)` with the already-parented command — cobra corrupts help output, causing "clean_swap" not found in parallel run.
- **Fix:** Rewrote test to use an isolated cobra root with only `backupCmd` added; no call to package-level `Execute()`.
- **Files modified:** `internal/cli/backup_test.go`
- **Commits:** 6c2de84

## Known Stubs

None — all backup endpoints return real data from the `backup_run` table.

## Threat Flags

| Flag | File | Description |
|------|------|-------------|
| threat_flag: command-injection | internal/backup/runner.go | `pg_dump` is invoked via `exec.CommandContext` with args constructed from config fields (DBHost, DBName, DBUser). These values come from operator config (not user input), so injection risk is low, but callers must not interpolate user-controlled strings into these fields. PGPASSWORD is passed via env (not CLI arg) — correct. |
| threat_flag: file-write | internal/backup/runner.go | Tarball written to `destDir` which is operator-configured. The path is not sanitized for traversal; callers must ensure `destDir` comes from trusted config, not HTTP parameters. `RunNowHandler` currently hardcodes `DefaultBackupDir` rather than accepting a user-supplied path — correct. |

## Self-Check: PASSED

- internal/backup/handler.go: FOUND
- internal/backup/runner.go: FOUND
- internal/backup/store.go: FOUND
- internal/backup/manifest.go: FOUND
- internal/cli/backup.go: FOUND
- internal/db/migrations/0046_backup_run.up.sql: FOUND
- compose/bundled.yml (ofelia service): FOUND
- compose/external.yml (backups volume): FOUND
- Commits b17c622 and 6c2de84: FOUND
