---
phase: 06-alerts-users-audit-operational-hardening
plan: 01
subsystem: alert-engine-substrate
tags: [phase-6, alerts, audit-retention, security-definer, migrations, river-cron]
requires: [phase-1-foundation, phase-5-river, phase-5-retention-config]
provides:
  - migration: 0037_audit_vocab_phase6
  - migration: 0038_alert_rule
  - migration: 0039_alert
  - migration: 0040_retention_config_phase6
  - migration: 0042_alert_worker_state
  - migration: 0043_admin_prune_audit_rows
  - package: internal/alert (engine + rule_store + alert_store + worker_state + degraded + audit_prune_worker)
  - vocabulary: 27 audit actions + 6 entity types
  - function: admin_prune_audit_rows(INTEGER) SECURITY DEFINER
affects:
  - internal/audit/log.go (27 const + 6 entity constants)
  - internal/install/finish.go (alerts_days + audit_log_days seed)
  - internal/cli/serve.go (AuditPruneWorker + degraded subscriber wiring)
  - internal/db/migrations_test.go (chain length 37 → 43 with gap at 0041)
  - internal/db/roundtrip_test.go (uint(37) → uint(43))
  - internal/install/finish_test.go (asserts alerts_days/audit_log_days)
tech-stack:
  added:
    - github.com/robfig/cron/v3 v3.0.1
  patterns:
    - "Custom-GUC trigger bypass: shifter.allow_audit_prune via set_config(is_local=true) instead of SUPERUSER-only session_replication_role"
    - "Dedicated owner role for SECURITY DEFINER (shifter_audit_admin) — least-privilege chain"
    - "River subscriber for retry-exhaustion observability (EventKindJobFailed + JobStateDiscarded → degraded flag)"
    - "Idempotent fire pattern via partial unique index (alert_firing_unique_idx)"
key-files:
  created:
    - internal/db/migrations/0037_audit_vocab_phase6.up.sql
    - internal/db/migrations/0037_audit_vocab_phase6.down.sql
    - internal/db/migrations/0038_alert_rule.up.sql
    - internal/db/migrations/0038_alert_rule.down.sql
    - internal/db/migrations/0039_alert.up.sql
    - internal/db/migrations/0039_alert.down.sql
    - internal/db/migrations/0040_retention_config_phase6.up.sql
    - internal/db/migrations/0040_retention_config_phase6.down.sql
    - internal/db/migrations/0042_alert_worker_state.up.sql
    - internal/db/migrations/0042_alert_worker_state.down.sql
    - internal/db/migrations/0043_admin_prune_audit_rows.up.sql
    - internal/db/migrations/0043_admin_prune_audit_rows.down.sql
    - internal/alert/doc.go
    - internal/alert/engine.go
    - internal/alert/rule_store.go
    - internal/alert/alert_store.go
    - internal/alert/worker_state.go
    - internal/alert/degraded.go
    - internal/alert/audit_prune_worker.go
    - internal/alert/engine_test.go
    - internal/alert/rule_store_test.go
    - internal/alert/alert_store_test.go
    - internal/alert/worker_state_test.go
    - internal/alert/degraded_test.go
    - internal/alert/audit_prune_worker_test.go
    - internal/db/phase6_alert_schema_test.go
    - internal/audit/phase6_vocab_test.go
  modified:
    - internal/audit/log.go
    - internal/install/finish.go
    - internal/install/finish_test.go
    - internal/cli/serve.go
    - internal/db/migrations_test.go
    - internal/db/roundtrip_test.go
    - go.mod
    - go.sum
decisions:
  - D-30 vocabulary locked at 27 actions + 6 entity types; constants mirror migration literals
  - D-51 trigger bypass uses shifter.allow_audit_prune custom GUC marker (NOT session_replication_role) because the latter is SUPERUSER-only in PG13+ and conflicts with the restricted-owner design
  - shifter_audit_admin role granted SELECT in addition to DELETE+INSERT (the prune DELETE reads rows for the WHERE clause)
  - migration number 0041 reserved as a deliberate gap so admin_prune_audit_rows lands at terminal number 0043
  - Cooldown enforced in-engine via RuleRecord.CooledDown(now); test-fires (D-19) intentionally skip TouchLastFiredAt
  - Audit-prune worker is registered with River but NOT tracked by the degraded subscriber (alertWorkerKindFromJobKind returns false for "audit_prune") — defense-in-depth via MaxAttempts=3
metrics:
  duration: 38min
  tasks: 3
  files: 28
  completed: 2026-05-12
---

# Phase 6 Plan 01: Alert Engine Substrate Summary

**One-liner:** Lands the Phase 6 substrate — six new migrations (0037-0040, 0042, 0043) + the `internal/alert/` Go package (engine, rule_store, alert_store, worker_state, degraded subscriber, audit prune worker) — so every downstream Phase 6 plan adds code on top instead of touching the schema.

## What shipped

### Migrations (6 new)

| # | Name | Purpose |
|---|------|---------|
| 0037 | `audit_vocab_phase6` | 27 new actions + 6 new entity types — D-30 auth events + user-mgmt + alerts + D-35 audit.export + D-51 audit.prune + backup verbs |
| 0038 | `alert_rule` | Persistent rule definitions. 8 rule_kinds, 5 scope_kinds, 3 severities. D-04 soft-delete, D-05 cooldown, D-17 quiet-window columns. CHECK enforces `scope_kind='global' OR scope_id IS NOT NULL`. |
| 0039 | `alert` | Fired-alert lifecycle. D-12 payload JSONB shape. State machine `firing → acknowledged|snoozed|muted|cleared`. Partial unique `alert_firing_unique_idx` for idempotent fire. |
| 0040 | `retention_config_phase6` | Adds `alerts_days INT NOT NULL DEFAULT 365` (D-13) + `audit_log_days INT NOT NULL DEFAULT 1825` (D-38) to the existing retention_config row. |
| 0042 | `alert_worker_state` | One observability row per worker_kind (5 kinds seeded). D-21 `/health/detailed` surface for Plan 06-04. Note: 0041 is a deliberate gap. |
| 0043 | `admin_prune_audit_rows` | SECURITY DEFINER prune function. Dedicated owner role `shifter_audit_admin`. Trigger bypass via custom GUC marker (`shifter.allow_audit_prune`) — NOT `session_replication_role`. Writes meta `audit.prune` audit row. |

### Go package: `internal/alert/`

| File | Surface |
|------|---------|
| `doc.go` | Package-level overview of the three-worker / one-queue pattern (D-03) |
| `engine.go` | `EvaluateContext` dependency bundle (Pool/Queries/Hub/InstallTZ/Log) + `CompareBound(value, high, low)` helper |
| `rule_store.go` | `RuleStore` over `alert_rule`: `CreateRule`, `GetRuleByID`, `ListActiveRulesByKind`, `ListAllRules`, `DisableRule`, `EnableRule`, `TouchLastFiredAt`. `RuleRecord.CooledDown(now)` helper. |
| `alert_store.go` | `AlertStore` over `alert`: `InsertAlert` (returns `ErrDuplicateFire` on partial-unique violation), `ClearAlert`, `AckAlert`, `SnoozeAlert`, `MuteAlert`, `ListFiringByRuleTarget`, `ListRecent`. |
| `worker_state.go` | `WorkerStateStore`: `UpsertWorkerState` + `MarkWorkerDegraded` + `GetWorkerState`. `RunState` accumulator with `RecordErr` (first-error only). |
| `degraded.go` | `StartDegradedSubscriber(ctx, riverClient, store, log)` goroutine. Listens for `EventKindJobFailed` + `JobStateDiscarded`, maps `job.Kind` to `worker_kind` via `alertWorkerKindFromJobKind`, flips `degraded=true`. Non-alert kinds silently ignored (T-06-01-06 mitigation). |
| `audit_prune_worker.go` | `AuditPruneWorker` River worker. Reads `retention_config.audit_log_days` each cycle, SELECTs `admin_prune_audit_rows()`. MaxAttempts=3 cap. |

### Wiring

- `internal/cli/serve.go`: registers `AuditPruneWorker` + adds a River `PeriodicJob` with `CRON_TZ=<install_tz> 0 3 * * *` (D-43 daily 03:00 install_tz). Starts `StartDegradedSubscriber` after `riverClient.Start`.
- `internal/install/finish.go`: explicit `UPDATE alerts_days = 365, audit_log_days = 1825` in the Serializable retention_config seed tx (belt-and-suspenders on top of the 0040 NOT NULL DEFAULTs).

## Deviations from Plan

### Rule 1 (auto-fix bug) — D-51 trigger bypass mechanism

**Found during:** Task 2 (TestAdminPruneAuditRows_BypassesTrigger ran red after first migration draft)
**Issue:** Plan body proposed `SET LOCAL session_replication_role = 'replica'` inside the SECURITY DEFINER function. In PostgreSQL 13+, this parameter is **SUPERUSER-only** — even setting it via SET LOCAL fails with `42501 permission denied`. This directly contradicts the same plan body's requirement that the function be owned by a restricted (non-superuser) role `shifter_audit_admin`. Both claims can't be true simultaneously.
**Fix:** Replaced the bypass with a custom-GUC marker. The 0016 trigger function `audit_log_reject_modification()` is rewritten in 0043 to honor `current_setting('shifter.allow_audit_prune', true) = 'true'`. The prune function uses `PERFORM set_config('shifter.allow_audit_prune', 'true', is_local := true)` (works for every role on user-defined GUCs, scoped to the transaction). The mitigation chain is preserved — every other code path still hits the RAISE EXCEPTION branch.
**Files modified:** `internal/db/migrations/0043_admin_prune_audit_rows.up.sql`, `internal/db/migrations/0043_admin_prune_audit_rows.down.sql` (restores original trigger function body)
**Commit:** 5806207

### Rule 2 (auto-add missing critical functionality) — SELECT grant on audit_log

**Found during:** Task 2 (TestAdminPruneAuditRows_BypassesTrigger reran red on first GUC-marker draft)
**Issue:** `DELETE FROM audit_log WHERE time < $1` implicitly requires `SELECT` on the table (PostgreSQL needs to read the WHERE clause). The plan listed only DELETE + INSERT grants on `shifter_audit_admin`.
**Fix:** Added `GRANT SELECT ON audit_log TO shifter_audit_admin`. UPDATE + TRUNCATE remain ungranted — even if the role is hijacked it cannot modify in place or wipe the table.
**Files modified:** `internal/db/migrations/0043_admin_prune_audit_rows.up.sql`
**Commit:** 5806207

### Rule 3 (auto-fix blocking issue) — Migration step-count tests

**Found during:** Task 1 (TestRunMigrations_Clean asserted version=36)
**Issue:** Adding migrations 0037..0043 (with gap at 0041) bumps the highest schema version from 36 to 43. Multiple tests in `migrations_test.go` and `roundtrip_test.go` had `require.Equal(t, 36, version)` / `runMigrateSteps(..., -17)` constants. Without updating, they break.
**Fix:** Updated 9 test assertions to reflect chain length 43 (with the 0041 gap). Comments now explicitly document the gap and the deliberate Phase 6 plan-coordination.
**Files modified:** `internal/db/migrations_test.go`, `internal/db/roundtrip_test.go`
**Commits:** 415cf74 (Task 1, +1 step adjustments) and 5806207 (Task 2, +5 step adjustments)

### Pre-existing failures NOT fixed (scope boundary)

**TestPhase3Migrations_0018_Down** and **TestPhase3Migrations_0019_Down**
have an off-by-one step count error that pre-dates this plan (failing on
`main` commit `72b0b6c`). They are NOT regressions caused by Plan 06-01.
Documented in
`.planning/phases/06-alerts-users-audit-operational-hardening/deferred-items.md`
along with a suggested rewrite (assert version, not step constant).

## Auth gates

None — fully autonomous execution.

## Test results

- `go build ./...`: clean
- `go vet ./...`: clean
- `go test ./internal/alert/... -count=1 -race -timeout=300s`: 27 passed
- `go test ./... -short -count=1 -timeout=120s`: 426 passed (35 packages)
- `go test ./internal/db/... -count=1 -timeout=300s`: 30 passed, 2 pre-existing failures (documented above)
- `go test ./internal/audit/... ./internal/install/... ./internal/cli/... -count=1 -timeout=300s`: all relevant tests pass (boot-flow tests preserved)

## Threat-model assertions verified

- **T-06-01-01** (audit_log INSERT-ONLY bypass): TestAdminPruneAuditRows_BypassesTrigger asserts direct DELETE still 23502s the trigger after the function exits.
- **T-06-01-03** (audit prune is itself audited): TestAuditPruneWorker_DeletesOldRows asserts the meta `audit.prune` row appears after every cycle (even zero-row prunes via TestAuditPruneWorker_NoOldRows).
- **T-06-01-04** (DoS via runaway prune): retention_config CHECK BETWEEN 90 AND 18250 + function-level `cutoff_days < 0` rejection (TestAdminPruneAuditRows_RejectsNegativeCutoff) + River MaxAttempts=3 cap.
- **T-06-01-06** (subscriber crash on malformed event): TestDegradedSubscriber_IgnoresUnrelatedJobs asserts a non-alert job failure does NOT flip any alert_worker_state row.
- **T-06-01-02** (`session_replication_role` escape): rendered moot — that mechanism is no longer used. Documented in deferred-items.md.

## Self-Check: PASSED

- All claimed files exist on disk ✓
- All 3 task commits exist in `git log` ✓
- Acceptance-criteria greps return non-empty for every required literal ✓
- `go vet` and `go build` clean ✓
- 27 alert-package tests + 27 audit-vocab + schema tests pass ✓
- Phase 6 vocabulary constants (27 + 6) all wired into 0037 CHECK ✓
- `internal/cli/serve.go` references `alert.AuditPruneArgs{}` (line 384) + `cron.ParseStandard("CRON_TZ=...")` + `alert.StartDegradedSubscriber` (line 411) ✓
- `go.mod` contains `github.com/robfig/cron/v3 v3.0.1` (direct require, line 18) ✓
