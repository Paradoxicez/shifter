---
phase: 07-multi-vendor-breadth-v1-x-differentiators
plan: 09b
subsystem: alert
tags: [alert, profile-aware, offline-worker, anomaly, reverse-flow, tdd]
dependency_graph:
  requires: [07-02, 07-09a]
  provides: [profile-aware-offline-threshold, profile-aware-cold-start, anomaly-compat-guard, reverse-flow-worker]
  affects: [alert-engine, alert-rule-api]
tech_stack:
  added: []
  patterns:
    - Profile-aware alert thresholds via offline_threshold_multiplier from device_profile
    - Per-profile anomaly warmup: full=21d, limited=60d, unsupported=never
    - Server-side rule creation guard using pre-tx GetMPAnomalyCompatibility lookup
    - ReverseFlowIncreaseWorker using measurement.extra JSONB for reverse_flow_m3
    - DaysOfWeek repurposed as window_days for reverse_flow_increase rules
key_files:
  created:
    - internal/alert/reverse_flow_worker.go
    - internal/alert/reverse_flow_worker_test.go
    - internal/db/migrations/0052_anomaly_compat_check.up.sql
    - internal/db/migrations/0052_anomaly_compat_check.down.sql
  modified:
    - internal/alert/offline_worker.go
    - internal/alert/offline_worker_test.go
    - internal/alert/cold_start.go
    - internal/alert/cold_start_test.go
    - internal/alert/anomaly_worker.go
    - internal/alert/anomaly_worker_test.go
    - internal/alert/rule_handler.go
    - internal/alert/handler_test.go
    - internal/db/queries/alerts.sql
    - internal/db/sqlc/alerts.sql.go
    - internal/db/sqlc/querier.go
decisions:
  - IsMPEligibleForAnomaly now takes a ruleKind arg to allow kind-specific gating (quiet_hour blocked on limited profiles)
  - Hysteresis clear threshold changed from 2x to 1x interval_s so clear < fire invariant holds for any multiplier >= 1
  - reverse_flow_increase stores window_days in DaysOfWeek field (existing int32 column repurposed) and threshold in FlowThreshold
  - measurement.extra used for reverse_flow_m3 (not a raw column; extra is the JSONB vendor-extension field per CLAUDE.md)
  - migration 0052 also adds battery_low to CHECK constraint (was accepted by app but rejected by DB)
metrics:
  duration_minutes: 27
  completed_date: "2026-05-13"
  tasks_completed: 4
  tasks_total: 4
  files_created: 4
  files_modified: 11
---

# Phase 7 Plan 09b: Profile-Aware Alert Workers Summary

JWT auth with refresh rotation using jose library — one-liner template, actual: Profile-aware alert thresholds and anomaly compatibility gating for Itron+KINMY (daily-uplink) and Axioma W1 meters via offline_threshold_multiplier, per-profile cold-start warmup, server-side anomaly compat guard, and ReverseFlowIncreaseWorker.

## What Was Built

### Task 1: Profile-aware offline threshold (commit 286b9f7)

`ListOfflineDevicesWithGatewayStatus` query extended to return `dp.offline_threshold_multiplier`. `fireDeviceOffline` now computes `thresholdSecs = float64(interval) * c.OfflineThresholdMultiplier` instead of the hardcoded `3 * interval`.

`ListHysteresisClearOffline` changed from `2 × interval_s` to `1 × interval_s` clear threshold. This fixes the invariant: clear threshold must be strictly less than fire threshold for any multiplier ≥ 1. At multiplier=1.8 (Itron), fire=1.8×86400=43.2h; old clear=2×86400=48h would immediately clear a 44h-old alert (false negative). New clear=1×86400=24h is always below any multiplier ≥ 1 threshold.

New tests: `TestOfflineWorker_ProfileAwareThreshold_Itron` (interval=86400s, mult=1.8) and `TestOfflineWorker_ProfileAwareThreshold_Axioma` (interval=3600s, mult=3.0).

### Task 2: Profile-aware cold-start gate (commit 8695ab8)

`IsMPEligibleForAnomaly` signature changed from `(ctx, q, mpID)` to `(ctx, q, mpID, ruleKind string)`. New behavior:
- `unsupported` → always false
- `limited` + `anomaly_quiet_hour` → false; `limited` + p95/iqr → 60d warmup
- `full` → 21d warmup for all kinds

Added `GetMPAnomalyCompatibility` and `IsMPEligibleForAnomalyDays` SQL queries. The `binding` table (with `valid_to IS NULL` for active bindings) joins to `device_profile.anomaly_compatibility`.

Existing `TestColdStart_NewMPNotEligible` and `TestColdStart_OldMPIsEligible` updated to use `seedMPWithProfile` (profile+binding required for new eligibility logic). `anomaly_worker_test.go` default fixture updated to bind a `full` profile so all existing anomaly tests keep passing.

New tests: `TestColdStart_LimitedProfile_60Days`, `TestColdStart_UnsupportedProfile_NeverEligible`, `TestColdStart_FullProfile_21Days`, `TestAnomalyWorker_SkipsQuietHour_OnLimitedProfile`.

### Task 3: Server-side anomaly compat guard (commit 0e706e9)

`CreateRuleHandler` in `rule_handler.go` now fetches `GetMPAnomalyCompatibility` before opening a transaction when `scope_kind=metering_point` and the rule kind is an anomaly kind. Rejects with HTTP 400 + `{"code":"profile_anomaly_incompatible","detail":"..."}` for:
- Any anomaly kind on `unsupported` profile
- `anomaly_quiet_hour` on `limited` profile

`anomaly_p95` and `anomaly_iqr` are allowed on `limited` profiles (only quiet_hour is blocked). Non-anomaly rule kinds (`threshold_hourly`, etc.) are not gated.

Added `isAnomalyKind(k string) bool` helper. Four tests in `handler_test.go` cover all rejection + allow paths (checker I-2 satisfied).

### Task 4: ReverseFlowIncreaseWorker + migration 0052 (commit f7f4a25)

New `ReverseFlowIncreaseWorker` in `reverse_flow_worker.go`. Evaluates `measurement.extra->>'reverse_flow_m3'` delta over a configurable window for all `reverse_flow_increase` rules. Window and threshold parameters stored in existing `DaysOfWeek` (int32 days) and `FlowThreshold` (m³) columns.

`GetReverseFlowDelta` SQL query uses `measurement.extra` JSONB (not a `raw` column — the schema's vendor-extension field is `extra`). Both COALESCE to 0.0 so missing fields are treated as zero delta.

Migration 0052 drops and recreates the `alert_rule.rule_kind` CHECK constraint to include `reverse_flow_increase` and `battery_low` (the latter was accepted by the app layer but rejected by the DB constraint since Phase 6 — this migration tightens both).

T-07-09b-01 mitigations: `window_days` clamped to [1, 365], threshold forced positive.

Three tests: fires on delta > threshold, no-fire on delta < threshold, missing `reverse_flow_m3` in `extra` is safe (delta=0, no fire, no crash).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Hysteresis clear threshold caused immediate auto-clear of Itron alerts**
- **Found during:** Task 1 GREEN phase
- **Issue:** Clear threshold was `2 × interval_s = 48h`. With Itron (mult=1.8), fire threshold = 43.2h. A 44h-old device would fire and immediately be cleared (44h < 48h clear threshold). The invariant requires clear < fire for all multipliers.
- **Fix:** Changed `ListHysteresisClearOffline` to `1 × interval_s` (always below fire threshold since mult ≥ 1 by design).
- **Files modified:** `internal/db/queries/alerts.sql`
- **Commit:** 286b9f7

**2. [Rule 1 - Bug] Wrong table name in GetMPAnomalyCompatibility SQL**
- **Found during:** Task 2 SQL authoring
- **Issue:** Plan pseudo-code used `device_metering_point_binding` (non-existent) and `unbound_at IS NULL`. Actual table is `binding` with `valid_to IS NULL`.
- **Fix:** Used correct `binding` table with `valid_to IS NULL` predicate and `valid_from` in INSERT.
- **Files modified:** `internal/db/queries/alerts.sql`
- **Commit:** 8695ab8

**3. [Rule 1 - Bug] Existing anomaly_worker tests failed after IsMPEligibleForAnomaly signature change**
- **Found during:** Task 2 GREEN verification
- **Issue:** Old tests (TestColdStart_NewMPNotEligible, TestColdStart_OldMPIsEligible) seeded MPs without a profile binding, so `GetMPAnomalyCompatibility` returned no rows → `false`. They expected `true` for 22d-old MP.
- **Fix:** Updated both tests to use `seedMPWithProfile("full")`. Also added `full` profile binding to `newAnomalyTestEnv` so all 7 pre-existing anomaly worker tests keep passing.
- **Files modified:** `internal/alert/cold_start_test.go`, `internal/alert/anomaly_worker_test.go`
- **Commit:** 8695ab8

**4. [Rule 2 - Missing] reverse_flow_m3 stored in extra, not raw column**
- **Found during:** Task 4 SQL authoring
- **Issue:** Plan pseudo-code used `measurement.raw->>'reverse_flow_m3'` but no `raw` column exists. CLAUDE.md describes `extra` as "the JSONB field where vendor-specific data lands."
- **Fix:** Used `measurement.extra->>'reverse_flow_m3'` in `GetReverseFlowDelta` query and test fixtures.
- **Files modified:** `internal/db/queries/alerts.sql`, `internal/alert/reverse_flow_worker_test.go`
- **Commit:** f7f4a25

**5. [Rule 2 - Missing] battery_low missing from alert_rule CHECK constraint**
- **Found during:** Task 4 migration authoring
- **Issue:** `battery_low` rule kind was registered in Phase 6 app layer but never added to the `rule_kind` CHECK constraint in migration 0038. DB would reject any battery_low rule INSERT at the constraint level.
- **Fix:** Migration 0052 includes `battery_low` alongside `reverse_flow_increase` in the new CHECK.
- **Files modified:** `internal/db/migrations/0052_anomaly_compat_check.up.sql`
- **Commit:** f7f4a25

## Known Stubs

None. All four tasks wire real data paths.

## Deferred Issues

Pre-existing failures in `TestAlertsPruneWorker_PrunesPerRetention` and `TestAlertsPruneWorker_NeverPrunesFiringByAge` reference `column "threshold_value" of relation "alert_rule"` — this column does not exist in any migration. These failures pre-date this plan and are out of scope. Logged to deferred-items.

## Threat Flags

None. All surfaces introduced (GetReverseFlowDelta, CreateRuleHandler guard) were in the plan's threat register and mitigations were implemented.

## Self-Check: PASSED

All key files found on disk. All 4 task commits verified in git log.
