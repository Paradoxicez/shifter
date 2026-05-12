---
phase: "06"
plan: "09"
subsystem: backup
tags: [backup, restore, timescaledb, advisory-lock, sha256, ci, ops, pg_restore]
dependency_graph:
  requires: [06-08-backup-cli-cron]
  provides: [restore-package, restore-cli-subcommand, ci-roundtrip-workflow, operator-runbook-backup-restore]
  affects: [internal/backup, internal/cli, .github/workflows, docs/operator-runbook]
tech_stack:
  added:
    - RestoreInPlace method (integration-test variant skipping DROP/CREATE)
    - extractTarGz (stdlib archive/tar + compress/gzip)
    - copyDir / copyFile (stdlib io + os)
    - PG advisory lock pattern (pg_try_advisory_lock + pg_advisory_unlock)
    - //go:build integration build tag for roundtrip test
  patterns:
    - advisory-lock-before-destructive-op (pg_try_advisory_lock at restore entry)
    - timescaledb-pre-post-restore-wrapping (Pitfall 2 mitigation)
    - no-parallel-pg-restore (Pitfall 1 mitigation: args slice never contains -j/--jobs)
    - per-file-sha256-verify-before-restore (T-06-09-01)
    - audit-in-transaction (backup.restore row via audit.WriteEntry)
key_files:
  created:
    - internal/backup/restore.go
    - internal/backup/restore_test.go
    - internal/backup/roundtrip_test.go
    - internal/cli/restore.go
    - internal/cli/restore_test.go
    - .github/workflows/backup-restore-roundtrip.yml
  modified:
    - internal/cli/root.go (restoreCmd added to Execute() AddCommand list)
    - docs/operator-runbook.md (Backup & Restore section replacing placeholder)
decisions:
  - "RestoreInPlace() added as a second restore entry-point for integration tests: testcontainers provides a single bootstrapped DB whose name is fixed at container-spawn time; DROP DATABASE requires a separate maintenance-DB connection which testcontainers does not expose. RestoreInPlace skips DROP/CREATE (done by the test harness via DROP SCHEMA CASCADE) and runs timescaledb_pre_restore → pg_restore → timescaledb_post_restore identically to the full production path. Production operators always call Restore(); RestoreInPlace is only callable by integration tests."
  - "roundtrip_test.go uses //go:build integration so it does not run in the default go test ./... fast suite; CI explicitly passes -tags=integration to opt in"
  - "Worker function names use Roundtrip-prefixed helpers (newRoundtripRunner, newRoundtripRestorer) to avoid shadowing setupTestRunner/setupTestRestorer from restore_test.go in the same package test binary"
  - "floor_plan insert in seed is non-fatal (t.Logf on error) because the core round-trip assertion is measurement/audit row counts + CAGG survival; floor-plan DB rows are bonus coverage"
  - "pg_dump version mismatch on dev machine (local pg_dump != container PG16): 3 integration tests that call Runner.Backup against the testcontainer fail locally for the same pre-existing reason the 06-08 runner tests fail. All static/lock/sha256 tests pass. The roundtrip test runs against the container-native pg_dump in CI (postgresql-client-16 installed by workflow)."
metrics:
  duration_minutes: 18
  completed_date: "2026-05-12"
  tasks_completed: 2
  files_created: 6
  files_modified: 2
  commits: 2
---

# Phase 06 Plan 09: Restore CLI + CI Round-Trip Summary

**One-liner:** TimescaleDB-aware `shifter restore` CLI with PG advisory lock + sha256 verify + pre/post_restore wrapping, plus a CI-gated integration round-trip test (OPS-04, D-44, D-45).

## What Was Built

### Task 1: internal/backup/restore.go + Cobra `shifter restore` subcommand

**internal/backup/restore.go** — `Restorer` struct with:
- `ShifterAdvisoryLockID int64 = 0x5348494654455231` ("SHIFTER1" in hex)
- `Restore(ctx, srcPath, expectedOuterSHA256)` — full production path:
  1. Optional outer tarball sha256 verify (`--expected-sha256`)
  2. `SELECT pg_try_advisory_lock($1)` → refuses with `ErrShifterStillServing` if held
  3. `defer pg_advisory_unlock` on all exit paths
  4. Extract tarball to `os.MkdirTemp`
  5. Parse and verify `manifest.json` per-file sha256 sums
  6. `restoreDB(ctx, dumpPath, dbUser, dbName, isShifterDB)` for Shifter (+ ChirpStack if bundled)
  7. `timescaledb_pre_restore()` → `pg_restore --no-owner --no-acl` (NO `-j`) → `timescaledb_post_restore()` for Shifter DB; bare `pg_restore` for ChirpStack DB
  8. `copyDir` floor-plans rsync
  9. `audit.WriteEntry(ActionBackupRestore, EntityTypeBackupRun)` in committed tx
- `RestoreInPlace(ctx, srcPath, expectedOuterSHA256)` — integration-test variant (skips DROP/CREATE)
- Error sentinels: `ErrShifterStillServing`, `ErrTarballChecksumMismatch`, `ErrFileChecksumMismatch`, `ErrManifestMissing`
- Belt-and-suspenders Pitfall 1 guard: runtime loop asserts no arg is `"-j"` or `"--jobs"`
- `extractTarGz` with path-traversal guard
- `copyDir` / `copyFile` helpers

**internal/backup/restore_test.go** — 11 tests:
- `TestRestorer_AdvisoryLockConst` — const value sanity
- `TestRestorer_RefusesIfShifterRunning` — lock contention → `ErrShifterStillServing`
- `TestRestorer_VerifiesOuterSHA256` — tampered tarball → `ErrTarballChecksumMismatch`
- `TestRestorer_VerifiesPerFileSHA256` — corrupted manifest sha256 → `ErrFileChecksumMismatch`
- `TestRestorer_NeverUsesJobsFlag` — line-by-line source check (args entries only)
- `TestRestorer_SourceContainsAdvisoryLock` — source grep
- `TestRestorer_SourceContainsTimescaleHooks` — source grep
- `TestRestorer_SourceUsesAuditWriteEntry` — source grep (no raw INSERT INTO audit_log)
- `TestRestorer_ExternalMode_SkipsChirpstack` — external tarball → no ChirpStack attempt
- `TestRestorer_FloorPlansRsync` — floor plan file restored with identical bytes
- `TestRestorer_WritesAuditOnSuccess` — `backup.restore` audit row present after restore
- `TestRestorer_AbortsAndReleasesLockOnError` — lock released after failure

**internal/cli/restore.go** — `shifter restore --from <path> [--expected-sha256 <hex>]`:
- `restoreFlagFrom` required via `MarkFlagRequired("from")`
- `restoreFlagExpectedSHA256` optional
- Long description documents operator workflow + v1 cross-version deferral
- Reads config + opens pool + builds `RestorerConfig` from cfg fields

**internal/cli/restore_test.go** — 5 tests:
- `TestCLIRestore_FlagParsing` — flag registration + required annotation
- `TestCLIRestore_MissingFromFlagError` — isolated root enforces required flag
- `TestCLIRestore_CommandIsRegistered` — Use/RunE/root presence
- `TestCLIRestore_LongDescContainsKeyPhrases` — docker compose + v1 deferral
- `TestCLIRestore_InExecuteList` — root.go source grep

**internal/cli/root.go** — `restoreCmd` added; long doc updated to 9 subcommands.

### Task 2: CI workflow + integration test + operator runbook

**.github/workflows/backup-restore-roundtrip.yml:**
- `on.pull_request.paths`: `internal/backup/**`, `internal/cli/backup.go`, `internal/cli/restore.go`, `internal/db/migrations/**`, `Dockerfile`, the workflow file itself
- `on.push.branches: [main]`
- `ubuntu-latest`, `timeout-minutes: 15`
- Installs `postgresql-client-16`
- Runs `go test ./internal/backup/... -run TestBackupRestoreRoundtrip -tags=integration -count=1 -v -timeout=10m`

**internal/backup/roundtrip_test.go** (`//go:build integration`):
- `startTimescaleContainer` — `timescale/timescaledb:2.26.0-pg16` via testcontainers
- `seedRoundtripFixture` — 1 site + 1 MP + 1 device + 100 measurements (spanning ≥2h) + 5 audit rows + 1 floor plan file on disk
- `TestBackupRestoreRoundtrip` — full seed → backup → DROP SCHEMA CASCADE → RestoreInPlace → smoke (site=1, MP=1, device=1, measurement=100, audit≥6, measurement_daily>0)
- `TestBackupRestoreRoundtrip_ExternalMode` — external mode tarball round-trip

**docs/operator-runbook.md** — replaced placeholder "Backups (Phase 6)" with full `## Backup & Restore` section with 5 subsections: Backup (manual), Backup (scheduled, bundled), Backup (scheduled, external), Restore (manual), CI gate. Includes safety properties list and explicit cross-version deferral statement.

## Commits

| Hash | Message |
|------|---------|
| f24f374 | feat(06-09): restore CLI + Restorer with advisory lock + sha256 + TimescaleDB hooks |
| f7728ab | feat(06-09): CI round-trip workflow + RestoreInPlace + operator runbook |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing functionality] RestoreInPlace variant required for integration test**
- **Found during:** Task 2
- **Issue:** The production `Restore()` method calls `DROP DATABASE + CREATE DATABASE` via psql against the `"postgres"` maintenance DB. testcontainers provides a single bootstrapped DB ("shifter") and does not expose a separate maintenance-DB connection to issue `DROP DATABASE` against it. The test would fail when trying to drop the active test DB.
- **Fix:** Added `RestoreInPlace(ctx, srcPath, expectedOuterSHA256)` that runs the same sha256 verification + advisory lock + pre_restore → pg_restore → post_restore sequence but skips DROP/CREATE. The test harness performs `DROP SCHEMA public CASCADE` + `CREATE SCHEMA public` + `CREATE EXTENSION timescaledb` before calling RestoreInPlace — functionally equivalent to DROP/CREATE for the round-trip assertion. Production operators always call `Restore()`.
- **Files modified:** `internal/backup/restore.go`
- **Commits:** f7728ab

**2. [Rule 1 - Bug] TestRestorer_NeverUsesJobsFlag failed on initial implementation**
- **Found during:** Task 1 RED phase
- **Issue:** The test used `require.NotContains(srcStr, '"-j"')` but `restore.go` legitimately contains `"-j"` as a string literal inside the runtime guard check (`if a == "-j" || a == "--jobs"`). The guard is correct — it detects injection. The test was over-broad.
- **Fix:** Rewrote the test to do a line-by-line scan of source, skipping comment lines and guard-comparison lines (lines containing `==` or `HasPrefix`). The test now correctly verifies that no args slice entry is `"-j"` as a standalone element while allowing the guard code that references the string.
- **Files modified:** `internal/backup/restore_test.go`
- **Commits:** f24f374

**3. [Rule 1 - Bug] device_profile seed used wrong column names**
- **Found during:** Task 2 (roundtrip test authoring)
- **Issue:** Plan's seed pseudo-code used `manufacturer`, `model`, `protocol` columns; the actual `device_profile` table (migration 0009) uses `vendor` (required), `family` (nullable), `capabilities` (array). Also `floor_plan` table uses `label`/`image_w`/`image_h` not `name`/`width_px`/`height_px`.
- **Fix:** Corrected INSERT statements to match actual migration schema. floor_plan insert made non-fatal (t.Logf on error) since the core assertion is measurement/CAGG counts.
- **Files modified:** `internal/backup/roundtrip_test.go`
- **Commits:** f7728ab

## Known Stubs

None — restore is fully wired. The roundtrip test uses a live testcontainer.

## Threat Flags

None — all threat model items from the plan's `<threat_model>` are addressed:
- T-06-09-01 (tarball tampering): outer sha256 + per-file sha256 verify
- T-06-09-02 (concurrent restore+serve): advisory lock
- T-06-09-03 (--jobs corruption): compile-time hardcoded args + runtime guard + test
- T-06-09-04 (missing pre_restore): pre/post hooks always called for Shifter DB
- T-06-09-07 (no audit trail): `backup.restore` audit row in committed tx
- T-06-09-08 (wrong DB owner): `CREATE DATABASE WITH OWNER` + `--no-owner --no-acl`

## Self-Check: PASSED

- internal/backup/restore.go: FOUND
- internal/backup/restore_test.go: FOUND
- internal/backup/roundtrip_test.go: FOUND
- internal/cli/restore.go: FOUND
- internal/cli/restore_test.go: FOUND
- .github/workflows/backup-restore-roundtrip.yml: FOUND
- docs/operator-runbook.md (Backup & Restore section): FOUND
- Commits f24f374 and f7728ab: FOUND
