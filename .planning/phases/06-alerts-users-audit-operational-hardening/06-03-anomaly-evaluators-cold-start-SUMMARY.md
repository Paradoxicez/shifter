---
phase: 06-alerts-users-audit-operational-hardening
plan: 03
subsystem: anomaly-evaluators-cold-start
tags: [phase-6, alerts, anomaly, p95, iqr, quiet-hour, cold-start, d-16, d-17, river-workers, install-tz]
requires:
  - phase-6-plan-01-alert-engine-substrate
  - phase-6-plan-02-threshold-offline-evaluators
provides:
  - sqlc-queries: alerts.sql (7 new named queries — IsMPEligibleForAnomaly, ListAnomalyWarmupRoster, GetMPAnomalyRules, P95BaselineForMPAndHour, IQRBaselineForMP, LatestInstantValueForMPAnomaly, NonZeroFlowDuringQuietWindow)
  - go-file: internal/alert/cold_start.go (D-16 21-day eligibility gate + warmup roster + per-MP rule state)
  - go-file: internal/alert/quiet_hour.go (EvalQuietHour with cross-midnight OR-form)
  - go-file: internal/alert/anomaly_worker.go (AnomalyWorker dispatching p95 / iqr / quiet-hour)
  - river-worker: 1 (alert_anomaly)
  - periodic-job: 1 (1h cadence)
  - requirement: ALERT-04 (statistical anomaly rules with cold-start gate)
affects:
  - internal/cli/serve.go (AnomalyWorker registration + 1h PeriodicJob)
tech-stack:
  added: []
  patterns:
    - "D-16 cold-start gate (IsMPEligibleForAnomaly) is the single source of truth for 'should this MP get anomaly alerts?' — called BEFORE the rule-kind switch so all 3 anomaly kinds (p95/iqr/quiet-hour) are gated together"
    - "Cross-midnight quiet-hour SQL uses the canonical OR-form from 06-RESEARCH §Pitfall 9: same-day branch (start < end) OR cross-midnight branch (start >= end with the explicit ≥start OR ≤end disjunction)"
    - "install_tz read fresh each cycle from install_identity.timezone — Settings change takes effect at next cycle without process restart (RESEARCH Open Q #4)"
    - "EXTRACT(HOUR FROM timestamptz) returns hour in session-tz (UTC for sqlc connections); evaluator uses time.Now().UTC().Hour() to match — using local hour silently misaligned baseline buckets on non-UTC deploy hosts"
    - "sqlc.arg() named parameters in NonZeroFlowDuringQuietWindow yield semantic Params struct fields (FlowThreshold/QuietWindowStart/QuietWindowEnd/InstallTz) instead of Column2/Column3/Column4"
    - "Anomaly evaluators silently no-op on percentile_cont NULL (zero-row baseline) — scan error treated as 'no baseline yet, skip MP' rather than RecordErr (the cold-start gate ensures at least 21d of data, but an hour bucket may still be empty)"
key-files:
  created:
    - internal/alert/cold_start.go
    - internal/alert/cold_start_test.go
    - internal/alert/quiet_hour.go
    - internal/alert/quiet_hour_test.go
    - internal/alert/anomaly_worker.go
    - internal/alert/anomaly_worker_test.go
  modified:
    - internal/db/queries/alerts.sql
    - internal/db/sqlc/alerts.sql.go
    - internal/db/sqlc/querier.go
    - internal/cli/serve.go
decisions:
  - D-16 cold-start threshold hardcoded at 21 days in v1 (constant AnomalyWarmupDays). Phase 7 may promote to retention_config once real-customer data shows whether 21 days is the right value.
  - Quiet-hour SQL parameters use sqlc.arg() named bindings rather than positional ($1..$5) so the generated Params struct fields read semantically rather than as Column2/Column3.
  - Anomaly worker uses time.Now().UTC().Hour() (not Local) for the P95 hour-of-day bucket because Postgres EXTRACT(HOUR FROM timestamptz) reports hour in session timezone (UTC) — using local hour silently misaligns the bucket on +07/+08 deploy hosts.
  - The three anomaly rule kinds share ONE AnomalyWorker (rather than three workers like the threshold subtypes) because install_tz is loaded once per cycle and the quiet-hour SQL is the only kind that needs it; consolidating avoids three reads per cycle.
  - percentile_cont on zero-row windows returns NULL → scan fails → worker silently skips ("no baseline yet"). Logged at Debug level only. Cold-start (21d) ensures overall data, but a specific hour bucket can still be empty (e.g. an MP that only reports during 08-18).
  - LatestInstantValueForMPAnomaly is a separate query from GetLatestMeasurementForMP (which the threshold worker uses) — different column projection avoids over-fetching cumulative_value when only instant_value is needed.
metrics:
  duration: 27min
  tasks: 2
  files: 9
  completed: 2026-05-12
---

# Phase 6 Plan 03: Anomaly Evaluators + Cold-Start Gate Summary

**One-liner:** Ships ALERT-04 (P95 + IQR + quiet-hour anomaly rule kinds) on top of the Plan 06-01 substrate and Plan 06-02 payload/expandScope helpers, with the D-16 21-day cold-start gate enforced as the single source of truth for "should this MP get anomaly alerts?".

## What shipped

### sqlc-generated queries (`internal/db/queries/alerts.sql`)

| Query | Purpose |
|-------|---------|
| `IsMPEligibleForAnomaly` | D-16: EXISTS(SELECT 1 ... time < now() - INTERVAL '21 days') |
| `ListAnomalyWarmupRoster` | D-16: per-MP days_until_eligible (0 = eligible, 21 = no data); ORDER BY days_until_eligible ASC, mp.name ASC. Schema bridge: uses mp.name (not mp.label) and mp.archived_at (not disabled_at). |
| `GetMPAnomalyRules` | Plan 06-04 MP detail card surface — returns the three anomaly rules (p95/iqr/quiet_hour) that target this MP or are global. |
| `P95BaselineForMPAndHour` | D-17 Rule 1 baseline — trailing-30d percentile_cont(0.95) filtered by EXTRACT(HOUR FROM time)::INT = hour_of_day. instant_value cast to DOUBLE PRECISION so sqlc emits float64. |
| `IQRBaselineForMP` | D-17 Rule 2 baseline — Q1 (percentile_cont(0.25)) + Q3 (percentile_cont(0.75)) over trailing 30 days. |
| `LatestInstantValueForMPAnomaly` | Anomaly-narrow projection (instant_value, time only). Separate from threshold worker's GetLatestMeasurementForMP. |
| `NonZeroFlowDuringQuietWindow` | D-17 Rule 3 — canonical cross-midnight OR-form from 06-RESEARCH Pitfall 9. sqlc.arg() named bindings produce semantic Params struct fields. |

### Go package additions

| File | Surface |
|------|---------|
| `internal/alert/cold_start.go` | `IsMPEligibleForAnomaly` (worker gate), `ListAnomalyWarmupRoster` (Plan 06-04/06-10 surface), `GetMPAnomalyState` (MP detail card per-rule toggle state). `AnomalyWarmupDays = 21` exported constant. |
| `internal/alert/quiet_hour.go` | `EvalQuietHour(ctx, q, mpID, rule, installTZ)` — wraps NonZeroFlowDuringQuietWindow; half-configured rules (NULL window) silently no-op rather than fire. `timeToMicros(t)` helper converts time.Time to pgtype.Time's microseconds-since-midnight. |
| `internal/alert/anomaly_worker.go` | `AnomalyWorker` + `AnomalyArgs` (Kind="alert_anomaly", MaxAttempts=3). `evalP95` + `evalIQR` per-rule-kind helpers. `fireAnomaly` opens tx → InsertAlert + audit.WriteEntry + TouchLastFiredAt in the same tx (D-23). `readInstallTimezone` reads fresh each cycle. |

### River wiring (`internal/cli/serve.go`)

- 1 new worker registration (`AnomalyWorker` sharing alertEng/alertRules/alertStore/alertWorkerStat with the threshold + offline workers)
- 1 new periodic job: `river.PeriodicInterval(1*time.Hour)` for `alert.AnomalyArgs{}` per 06-RESEARCH §Decision C
- No MaxWorkers bump needed (anomaly cadence is 1h; the existing 8-worker queue absorbs it)

## Deviations from Plan

### Rule 1 (auto-fix bug) — UTC hour-of-day for P95 bucket

**Found during:** Task 2 (TestAnomalyP95_FiresOnOutlier failed silently — no fire despite p95=10, latest=15)

**Issue:** First-draft `evalP95` used `time.Now().Hour()` (Go's local hour) for the bucket selection in `P95BaselineForMPAndHour`. The SQL filter `EXTRACT(HOUR FROM time)::INT = $2::INT` reads `time` (a `timestamptz`) in the **session timezone**, which for sqlc-driven Postgres sessions is **UTC**. On the test machine (TZ=+07, Bangkok), the test seeded measurements at `time.Date(..., curHour, 30, 0, 0, time.UTC)` — UTC hour-of-day — but the worker queried for `hour=local_hour` (UTC hour + 7). No rows matched → percentile_cont returned NULL → Scan into `float64` failed → evalP95 silently skipped.

**Fix:** `hour := time.Now().UTC().Hour()` (added comment explaining why local hour is wrong). This isn't just a test-side problem — production deploys on non-UTC hosts (the project's TECHNOLOGY.md targets +07 customers like Asia/Bangkok) would have silently broken anomaly_p95 on every cycle.

**Files modified:** `internal/alert/anomaly_worker.go`

**Commit:** 08c74df

### Rule 1 (auto-fix bug) — flaky days_until_eligible test

**Found during:** Task 1 second pass (TestWarmupRoster_DaysUntilEligible occasionally returned 17 instead of 16)

**Issue:** The roster SQL uses `EXTRACT(DAY FROM (now() - MIN(m.time)))::INT` which is integer-truncating. The original test seeded at `time.Now().UTC().Add(-5*24*time.Hour)` — exactly 5 days ago. As microseconds elapsed during the test (`pgx` round-trip + `Scan`), the actual elapsed interval could drift across the 5-day boundary: sometimes 5d 0.001s (EXTRACT(DAY)=5 → expected 16), sometimes 4d 23:59:59.999 (EXTRACT(DAY)=4 → 17). The first run passed; the post-Task-2 rerun hit the unlucky side.

**Fix:** Seed at `-(5*24+12)*time.Hour` (5d 12h ago) and `-(25*24+12)*time.Hour` (25d 12h ago) so the EXTRACT lands stably on the integer floor (5 and 25, respectively). 12-hour buffer eliminates any drift sensitivity.

**Files modified:** `internal/alert/cold_start_test.go`

**Commit:** 08c74df (folded into Task 2 commit because the rerun surfaced it)

### Rule 3 (auto-fix blocking) — schema column-name adaptations

**Found during:** Task 1 (first draft of queries.sql)

**Issue:** Plan body referenced `mp.label`, `mp.disabled_at`, `s.label`. Actual schema (from 0007_site + 0008_metering_point) uses `name` (not `label`), and metering_point uses `archived_at` (not `disabled_at`).

**Fix:** Adapted the new queries to actual columns:
- `mp.name AS metering_point_label`
- `mp.archived_at IS NULL` (instead of `disabled_at IS NULL`)
- `COALESCE(s.name, '') AS site_label` (the COALESCE handles MPs with no site row — defensive even though site_id is NOT NULL in 0008)

**Files modified:** `internal/db/queries/alerts.sql`

**Commit:** 0a8bb22 (Task 1)

### Rule 3 (auto-fix blocking) — instant_value NUMERIC → DOUBLE PRECISION cast

**Found during:** Task 1 (first draft of P95/IQR queries)

**Issue:** `measurement.instant_value` is declared `NUMERIC` in 0015. `percentile_cont(0.95) WITHIN GROUP (ORDER BY instant_value)` returns NUMERIC, which sqlc maps to `pgtype.Numeric` — making downstream Go arithmetic clunky.

**Fix:** Added `::DOUBLE PRECISION` casts in three places: the percentile_cont ORDER BY for both p95 and IQR, the outer expression for p95, and the `instant_value > $threshold` comparison in NonZeroFlowDuringQuietWindow. sqlc now emits clean `float64` types for the baselines.

**Files modified:** `internal/db/queries/alerts.sql`

**Commit:** 0a8bb22 (Task 1)

## Auth gates

None — fully autonomous execution.

## Test results

- `go build ./...`: clean
- `go vet ./...`: clean
- `go test ./internal/alert/... -count=1 -timeout=300s`: **61 passed** (50 prior from 06-01+06-02 + 11 new)
  - Cold-start: TestColdStart_NewMPNotEligible, TestColdStart_OldMPIsEligible, TestWarmupRoster_DaysUntilEligible, TestWarmupRoster_OrderingByDaysUntilEligible (4 tests)
  - Anomaly worker: TestAnomalyP95_FiresOnOutlier, TestAnomalyP95_BucketIsHourOfDay, TestAnomalyIQR_FiresAboveQ3PlusOnePointFiveIQR, TestAnomalyIQR_FiresBelowQ1MinusOnePointFiveIQR, TestAnomalyQuietHour_SameDayWindow, TestAnomalyQuietHour_CrossMidnight, TestAnomalyQuietHour_RespectsInstallTimezone, TestAnomaly_ColdStartGatesAllRuleKinds, TestAnomaly_OptInPerMP (9 tests)
  - Quiet-hour unit: TestEvalQuietHour_ReturnsFalseWhenWindowUnset, TestEvalQuietHour_ReturnsFalseWhenNoMatchingMeasurement (2 tests)
- `go test ./... -short -count=1 -timeout=180s`: **437 passed** (37 packages — no regression)

## Threat-model assertions verified

- **T-06-03-01** (DoS via anomaly worker firing constantly on new install): `TestAnomaly_ColdStartGatesAllRuleKinds` — an MP with only 5 days of history + breaching latest measurement + all three anomaly rules active produces ZERO fires. The gate is `IsMPEligibleForAnomaly` called BEFORE the rule-kind switch (anomaly_worker.go:101 vs switch at :122).
- **T-06-03-02** (Tampering: cross-midnight quiet-hour SQL bypass): `TestAnomalyQuietHour_SameDayWindow` + `TestAnomalyQuietHour_CrossMidnight` exercise both branches of the OR-form. The TIME parameters are bound positionally via sqlc-generated code; the install_tz parameter is bound as TEXT and applied via `AT TIME ZONE` operator (Postgres rejects invalid tz names at query parse time).
- **T-06-03-04** (DoS via percentile_cont scan cost): performance gate accepted-defer per RESEARCH §Decision E A4 — Wave 0 EXPLAIN deferred to Phase 7 when real-customer data is available. Estimated 1.44M rows/hour total scan at 500 MPs is trivially fast on the existing `(metering_point_id, time DESC)` hot-path index.
- **T-06-03-05** (Repudiation: anomaly fire/clear untracked): `TestAnomalyP95_FiresOnOutlier` asserts payload presence; `fireAnomaly` writes audit_log row (`audit.ActionAlertFired`) inside the same pgx.Tx as the alert insert (D-23). Mirrors the threshold/offline pattern verified by Plan 06-02's TestThresholdAuditTransactionAtomicity.

## Known Stubs / Deferred wiring

- **GetMPAnomalyState API endpoint**: Plan 06-03 ships the helper function (`alert.GetMPAnomalyState`) but does NOT add an HTTP route. Plan 06-04 will mount it at `/api/metering-points/{id}/anomaly-state` for the MP detail card. The function is exercised indirectly via the worker path (rules with scope_kind='metering_point' resolve to the same MP) but has no direct test in this plan — added when the route lands.
- **Warmup roster API endpoint**: Same — `alert.ListAnomalyWarmupRoster` ships as a function; Plan 06-10 will mount it under Settings → Alerts. No route added in this plan.
- **install_tz timezone validation**: The worker passes `install_identity.timezone` through to Postgres as TEXT. Postgres rejects invalid IANA names at query time with a SQL error. We treat this as a configuration error (Plan 22 install wizard validates the value), not a hot-path concern — but if a deploy ever ends up with `timezone='Banana/Hello'` the anomaly worker will RecordErr and bail. No defensive Go-side validation added in v1.

## Self-Check: PASSED

- `internal/alert/cold_start.go` exists and exports `IsMPEligibleForAnomaly`, `ListAnomalyWarmupRoster`, `GetMPAnomalyState`, `AnomalyEligibility{DaysUntilEligible int32}`, `MPAnomalyRuleState`, `AnomalyWarmupDays` ✓
- `internal/alert/quiet_hour.go` exists and contains `func EvalQuietHour(` and `cross-midnight` ✓
- `internal/alert/anomaly_worker.go` exists and contains `IsMPEligibleForAnomaly` call BEFORE the rule-kind switch ✓ (line 101 vs 122)
- All 3 rule kinds in the switch: `"anomaly_p95"`, `"anomaly_iqr"`, `"anomaly_quiet_hour"` ✓
- `internal/cli/serve.go` registers `&alert.AnomalyWorker{` ✓ and adds `river.PeriodicInterval(1*time.Hour)` with `alert.AnomalyArgs{}` ✓
- `internal/db/queries/alerts.sql` contains `name: IsMPEligibleForAnomaly :one`, `name: ListAnomalyWarmupRoster :many`, `INTERVAL '21 days'` exact string ✓
- `internal/db/queries/alerts.sql` contains `P95BaselineForMPAndHour`, `IQRBaselineForMP`, `NonZeroFlowDuringQuietWindow` ✓
- `NonZeroFlowDuringQuietWindow` SQL contains both cross-midnight branches (`TIME < ... TIME` AND `TIME >= ... TIME`) ✓
- All 11 listed tests exist in `cold_start_test.go` + `anomaly_worker_test.go` + `quiet_hour_test.go` ✓
- `go build ./...` clean ✓
- `go vet ./...` clean ✓
- 61 alert tests pass (4 + 9 + 2 new + 46 unchanged from 06-01/06-02) ✓
- 437 short tests pass project-wide (no regression) ✓
- All 2 task commits exist in `git log`:
  - 0a8bb22 — Task 1: cold-start gate + warmup roster (sqlc + Go)
  - 08c74df — Task 2: AnomalyWorker + P95/IQR/quiet-hour + serve.go wiring
