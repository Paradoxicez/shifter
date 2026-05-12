---
phase: 06-alerts-users-audit-operational-hardening
plan: 11
subsystem: [ops, doctor, alert, health, compose]
tags: [retention, prune, doctor, redact, health, compose-lint, runbook, OPS-05, OPS-06, OPS-07, OPS-08]
dependency_graph:
  requires: [06-01, 06-08, 06-09, 06-10]
  provides:
    - alerts-retention-prune-worker
    - /health/detailed-alert-worker-extension
    - /health/detailed-last-backup-extension
    - shifter-doctor-cli
    - email-redaction
    - compose-convention-lint-tests
    - operator-runbook-compose-conventions
    - operator-runbook-upgrading-shifter
  affects:
    - internal/alert (new worker)
    - internal/http/health.go (extended)
    - internal/doctor (new package)
    - internal/cli/root.go (new subcommand)
    - internal/compose (new lint test package)
    - docs/operator-runbook.md (2 new sections)
    - internal/db/migrations (0048)
tech_stack:
  added:
    - internal/doctor package (doctor.go + redact.go)
    - internal/compose test package (lint tests)
    - migration 0048_audit_vocab_alert_prune (alert.pruned action vocab)
  patterns:
    - River cron worker (AlertsPruneWorker)
    - PII redaction via regex walk over serialized JSON (RedactJSON)
    - Cobra --out flag pattern for CLI file/stdout output
    - YAML service-block stateful parser for compose lint
    - nonCommentLines helper for comment-aware grep tests
key_files:
  created:
    - internal/db/migrations/0048_audit_vocab_alert_prune.up.sql
    - internal/db/migrations/0048_audit_vocab_alert_prune.down.sql
    - internal/alert/alerts_prune_worker.go
    - internal/alert/alerts_prune_worker_test.go
    - internal/doctor/doctor.go
    - internal/doctor/doctor_test.go
    - internal/doctor/redact.go
    - internal/doctor/redact_test.go
    - internal/cli/doctor.go
    - internal/cli/doctor_test.go
    - internal/compose/conventions_test.go
    - internal/compose/doc.go
  modified:
    - internal/audit/log.go (ActionAlertPruned constant)
    - internal/http/health.go (AlertWorkerHealth + LastBackupHealth types + extended HealthDetailed)
    - internal/http/health_test.go (3 new tests)
    - internal/cli/serve.go (AlertsPruneWorker registration + cron schedule)
    - internal/cli/root.go (doctorCmd registered)
    - internal/db/migrations_test.go (version assertion 47 -> 48)
    - internal/db/roundtrip_test.go (version assertion 47 -> 48)
    - docs/operator-runbook.md (## Compose conventions + ## Upgrading Shifter sections)
decisions:
  - "Plain DELETE used for alert prune (no SECURITY DEFINER needed — alert table has no INSERT-ONLY trigger unlike audit_log)"
  - "Doctor package does its own DB queries rather than importing internal/http to avoid cross-package dependency cycle"
  - "Docker socket log tail deferred to v1.x per RESEARCH Open Question #3 (D-50) — static fallback note in bundle logs field"
  - "Migration 0048 (not 0047) — 0047 was claimed by 06-10 backup_thresholds; Rule 3 deviation documented"
  - "serviceNames() stateful parser stops at next top-level YAML key to avoid matching nested keys as service names"
metrics:
  duration_minutes: 120
  completed_date: "2026-05-12"
  tasks_completed: 3
  tasks_total: 3
  new_files: 12
  modified_files: 8
---

# Phase 06 Plan 11: Ops Hardening, Doctor CLI, and Runbook Summary

**One-liner:** Alerts retention prune worker (River 03:30 cron) + `shifter doctor` support-diagnostic CLI with email redaction + `/health/detailed` alert_worker/last_backup extension + 11-test compose convention lint suite + Compose conventions and Upgrading Shifter runbook sections.

## Objective

Close OPS-05/06/07/08: compose hygiene audit with automated enforcement; `shifter doctor` support bundle with PII redaction; alert observability in `/health/detailed`; daily alert retention prune worker. Final plan of Phase 6 — after this, Phase 6 is shippable.

## Tasks Completed

| Task | Description | Commit | Key Files |
|------|-------------|--------|-----------|
| 1 | Alerts prune worker + migration 0048 + /health/detailed extension | 71f4fa1 | alerts_prune_worker.go, 0048_audit_vocab_alert_prune.up.sql, health.go |
| 2 | shifter doctor CLI + redaction + diagnostic bundle | ba9728c | doctor/doctor.go, doctor/redact.go, cli/doctor.go |
| 3 | Compose conventions lint test + operator-runbook sections | eb21a3f | compose/conventions_test.go, docs/operator-runbook.md |

## What Was Built

### Task 1: Alerts Prune Worker + /health/detailed Extension

**Migration 0048** extends the `audit_log` action CHECK constraint to include `'alert.pruned'` using the DROP+re-ADD pattern established in prior plans. Carries all 50+ existing action vocabulary entries.

**AlertsPruneWorker** (River cron, 03:30 install_tz) reads `retention_config.alerts_days`, executes a plain DELETE in a transaction, and writes a `alert.pruned` audit row with a `gen_random_uuid()` meta entity ID. No SECURITY DEFINER needed — the alert table lacks the INSERT-ONLY trigger that required the bypass in the audit prune worker.

**`ActionAlertPruned = "alert.pruned"`** constant added to `internal/audit/log.go`.

**/health/detailed extended** with two new fields:
- `alert_workers`: array of `AlertWorkerHealth` rows (kind, last_run_at, rules_evaluated, fires_emitted, cleared, duration_ms, degraded, last_error) sourced from `alert_worker_state` ordered by worker_kind
- `last_backup`: `LastBackupHealth` row (file_name, started_at, age_seconds, status, sha256) or null

Status transitions to `"degraded"` when any `alert_workers[*].degraded = true` OR `last_backup.age_seconds > backup_crit_threshold_hours * 3600`.

**`internal/cli/serve.go`** wires `AlertsPruneWorker` into the River worker registry and registers a daily periodic job at `CRON_TZ=<install_tz> 30 3 * * *`.

7 unit tests for the prune worker; 3 new integration tests in `health_test.go`.

### Task 2: shifter doctor CLI + Redaction

**`internal/doctor` package** (new):
- `doctor.go`: `Doctor` struct with `Pool`, `CSDialer`, `StartedAt` fields. `SnapshotBundle(ctx)` assembles all 9 bundle fields; `Bundle.MarshalRedacted()` calls `json.MarshalIndent` then `RedactJSON`.
- `redact.go`: `MaskEmail(email)` → `j***@example.com` (first char + `***` + `@domain`); `RedactJSON(b)` applies a compiled email regex walk over all JSON bytes.

**Bundle fields (9):** `generated_at`, `shifter` (version + schema_version + uptime_seconds), `config_check` (db bool), `health_detailed` (same shape as HTTP handler), `alert_workers`, `last_backup`, `chirpstack_grpc_ping` (reachable + version + error), `recent_audit` (last 100 rows, emails masked), `logs` (static D-50 fallback note).

**Docker log tail deferred (D-50):** The `logs` field contains a static note: `"<docker socket not mounted — run \`docker compose logs --tail=200 shifter\` from host>"`. No Docker SDK dependency in v1.

**`internal/cli/doctor.go`** (`shifter doctor` subcommand):
- `--out` flag (default: stdout)
- `Long` help documents v1 deferral and the `docker compose logs` fallback command
- `runDoctorCmd` loads config, creates pool, constructs Doctor, calls SnapshotBundle + MarshalRedacted, writes to file or stdout

**`internal/cli/root.go`** updated to register `doctorCmd`.

4 CLI tests (registration, --out flag, Long content, bundle shape round-trip); 4 doctor package integration tests.

### Task 3: Compose Conventions Lint + Runbook

**`internal/compose/conventions_test.go`** (11 tests):

| Test | What it asserts |
|------|-----------------|
| TestCompose_NoLatestTag | No non-comment line contains `:latest` in either compose file |
| TestCompose_PinnedImageVersions | Every `image:` line has a tag (no bare `image: name`) |
| TestCompose_EveryServiceHasJsonLogging | Count of `logging: *json-logging` refs == number of services |
| TestCompose_NoEnvCredentials | No `PASSWORD/API_TOKEN/SECRET/SIGNING_KEY/API_KEY: ${...}` in non-comment lines |
| TestCompose_AllSecretsMountedViaFiles | All 4 `*_FILE` env vars present in each compose file |
| TestCompose_BackupsVolumeMounted | `backups:` volume declared + mounted at `/var/lib/shifter/backups` |
| TestCompose_NoExposedInternalServices | postgres/mosquitto/redis blocks contain no `ports:` (comment lines stripped) |
| TestCompose_TopLevelSecretsDeclaration | All 4 secrets declared at top-level |
| TestRunbook_HasComposeConventions | `docs/operator-runbook.md` contains `## Compose conventions` heading |
| TestRunbook_HasUpgradingSection | Runbook contains `## Upgrading Shifter` + 5 numbered steps |
| TestRunbook_BackupRestoreSection | `## Backup & Restore` section preserved from Plan 06-09 |

Key helper `serviceNames(content)` uses a stateful parser that collects names only within the `services:` block, stopping at the next top-level YAML key — prevents matching `depends_on:`, `environment:`, etc.

**`docs/operator-runbook.md`** gained two sections:
- `## Compose conventions` — 5 convention bullets (pinned tags, json-logging, secrets via files, no exposed internals, backups volume) + `go test ./internal/compose/...` gate note
- `## Upgrading Shifter` — 5-step procedure (backup → bump image tag → pull+up → verify → rollback) + per-release migration notes subsection (v0.6.0: migrations 0037–0048 all additive)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Migration 0048 instead of 0047**
- **Found during:** Task 1 setup
- **Issue:** Plan reserved `0047_audit_vocab_alert_prune` but 06-10 had already used 0047 for `0047_backup_thresholds`. Using 0047 again would corrupt the migration chain.
- **Fix:** Used `0048_audit_vocab_alert_prune` for all migration file names and updated all references (migrations_test.go, roundtrip_test.go).
- **Files modified:** internal/db/migrations/0048_*.sql, internal/db/migrations_test.go, internal/db/roundtrip_test.go
- **Commit:** 71f4fa1

**2. [Rule 1 - Bug] No SECURITY DEFINER needed for alert prune**
- **Found during:** Task 1 implementation
- **Issue:** Plan said "mirrors audit prune pattern / calls admin_prune_alerts SECURITY DEFINER function" but the alert table has no INSERT-ONLY trigger (unlike audit_log). A plain DELETE suffices; a SECURITY DEFINER function would be unnecessary complexity.
- **Fix:** Implemented plain DELETE within a transaction. No stored function created.
- **Files modified:** internal/alert/alerts_prune_worker.go
- **Commit:** 71f4fa1

**3. [Rule 1 - Bug] Doctor package avoided http package dependency**
- **Found during:** Task 2 implementation
- **Issue:** Initially considered reusing health.go helpers by importing internal/http from internal/doctor but this would create a cross-package dependency. Doctor is a support tool that should stand alone.
- **Fix:** Doctor package performs its own direct DB queries (loadAlertWorkers, loadLastBackupHealth, loadRecentAudit) rather than importing internal/http.
- **Files modified:** internal/doctor/doctor.go
- **Commit:** ba9728c

**4. [Rule 1 - Bug] serviceNames() parser false positives**
- **Found during:** Task 3 test failures (TestCompose_EveryServiceHasJsonLogging)
- **Issue:** Initial regex `^  [a-z][a-z0-9_-]+:\s*$` matched ALL two-space-indented YAML keys (depends_on:, environment:, volumes:, etc.), counting 25 "services" instead of 9.
- **Fix:** Implemented stateful `serviceNames()` helper that tracks `inServices` flag, collects names only between `services:` and the next top-level key.
- **Files modified:** internal/compose/conventions_test.go
- **Commit:** eb21a3f

**5. [Rule 1 - Bug] Comment lines in compose files triggered :latest and ports: tests**
- **Found during:** Task 3 test failures (TestCompose_NoLatestTag, TestCompose_NoExposedInternalServices)
- **Issue:** Compose files contain documentation comments like `# All image tags PINNED — no \`:latest\`` and `# No \`ports:\` block`. Naive string-search tripped on these.
- **Fix:** TestCompose_NoLatestTag iterates line-by-line and skips lines starting with `#` after trim. TestCompose_NoExposedInternalServices filters comment lines from each service block before the `NotContains` check. `nonCommentLines()` helper used for TestCompose_NoEnvCredentials.
- **Files modified:** internal/compose/conventions_test.go
- **Commit:** eb21a3f

## Known Stubs

None. All fields in the doctor bundle and /health/detailed are wired to live DB queries. No placeholder text or empty fallbacks flow to operators.

The `logs` field intentionally returns a static fallback note (D-50 deferral) — this is documented design, not a stub.

## Threat Flags

None. This plan adds read-only DB queries and a diagnostic CLI that runs offline. No new network endpoints, auth paths, or schema changes at trust boundaries.

## Self-Check: PASSED

Files created and committed:
- FOUND: internal/db/migrations/0048_audit_vocab_alert_prune.up.sql
- FOUND: internal/db/migrations/0048_audit_vocab_alert_prune.down.sql
- FOUND: internal/alert/alerts_prune_worker.go
- FOUND: internal/doctor/doctor.go
- FOUND: internal/doctor/redact.go
- FOUND: internal/cli/doctor.go
- FOUND: internal/compose/conventions_test.go
- FOUND: docs/operator-runbook.md (## Compose conventions + ## Upgrading Shifter present)

Commits verified:
- FOUND: 71f4fa1 (feat(06-11): alerts retention prune worker + migration 0048)
- FOUND: ba9728c (feat(06-11): shifter doctor CLI + redaction + diagnostic bundle)
- FOUND: eb21a3f (feat(06-11): compose conventions lint test + 3 operator-runbook sections)
