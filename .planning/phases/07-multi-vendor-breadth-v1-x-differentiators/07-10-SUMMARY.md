---
phase: 07-multi-vendor-breadth-v1-x-differentiators
plan: 10
subsystem: alert
tags: [backtest, anomaly, timescaledb, cagg, recharts, react-query, sqlc, chi, rbac]

# Dependency graph
requires:
  - phase: 07-multi-vendor-breadth-v1-x-differentiators
    plan: 09b
    provides: profile-aware anomaly workers + anomaly_compatibility on device profiles

provides:
  - BacktestRun function (internal/alert/backtest.go) — reads measurement_hourly CAGG, returns 30-day fire counts for anomaly_p95 / anomaly_iqr / anomaly_quiet_hour
  - POST /api/alerts/backtest endpoint — RBAC-gated (ActionAlertRuleCreate), read-only, never writes alert rows
  - AddRuleDialog Phase 7 extensions — "Test against last 30 days" button + 30-bar sparkline + profile-aware rule kind filtering

affects:
  - 07-11a-saved-report-template-backend (reads alert_rule table; backtest endpoint is a read-only neighbor)
  - 07-14-doctor-probes-and-phase-closure (phase verifier should confirm backtest endpoint health)

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Two-pass CAGG query pattern: pass 1 computes aggregate (P95 or IQR bounds), pass 2 counts rows exceeding threshold — no window functions inside aggregate calls (TimescaleDB Pitfall §4)"
    - "Typed fill helpers (Checker M-1): fillDailyBucketsP95 / IQR / QuietHour each take a concrete sqlc slice type; no `any` erasure"
    - "fillDailyBucketsCore: shared zero-fill loop building exactly N daily buckets oldest-first"

key-files:
  created:
    - internal/alert/backtest.go
    - internal/alert/backtest_test.go
    - internal/api/backtest_handler.go
    - internal/api/backtest_handler_test.go
    - web/src/lib/backtest.ts
  modified:
    - internal/db/queries/alerts.sql
    - internal/db/sqlc/alerts.sql.go
    - internal/db/sqlc/querier.go
    - internal/http/router.go
    - web/src/routes/settings/AddRuleDialog.tsx
    - web/src/routes/settings/AddRuleDialog.test.tsx

key-decisions:
  - "Days capped at 90 (not 30) in BacktestRun — matching RESEARCH §T-07-10-01 DoS mitigation; handler defaults missing days to 30"
  - "Three typed fill helpers instead of one `any` fill function — satisfies Checker M-1; each helper takes a concrete sqlc row slice type"
  - "RBAC uses ActionAlertRuleCreate (same permission as creating a rule) — backtest is part of the configure-rule flow, not an admin-only debug tool"
  - "Backtest endpoint is read-only — no INSERT INTO alert anywhere in the call chain (T-07-10-04)"

patterns-established:
  - "Two-pass CAGG backtest: aggregate-then-count, no window functions inside aggregate calls"
  - "M-1 typed fill helpers: one concrete helper per sqlc result type, all delegating to a shared core"

requirements-completed: [ALERT-04]

# Metrics
duration: ~6min
completed: 2026-05-13
---

# Phase 07 Plan 10: Anomaly Backtest Summary

**Read-only backtest endpoint (POST /api/alerts/backtest) + 30-day sparkline in AddRuleDialog using two-pass CAGG queries and three M-1-compliant typed fill helpers for anomaly_p95, anomaly_iqr, and anomaly_quiet_hour**

## Performance

- **Duration:** ~6 min (Tasks 1-3 automated; Task 4 visual verification signed off by operator)
- **Started:** 2026-05-13T02:22:01Z
- **Completed:** 2026-05-13T02:27:37Z (code tasks) + visual approval
- **Tasks:** 4 (3 automated + 1 human-verify checkpoint)
- **Files modified:** 11

## Accomplishments

- BacktestRun with 5 sqlc queries targeting measurement_hourly CAGG — two-pass pattern for P95 and IQR, single-pass for quiet_hour; caps days at 90; read-only
- Three typed fill helpers (M-1): fillDailyBucketsP95 / IQR / QuietHour — no `any` erasure — delegating to shared fillDailyBucketsCore zero-fill loop
- 8 backend tests + 4 handler tests covering all rule kinds, invalid inputs, RBAC enforcement, and zero-fill correctness
- AddRuleDialog extended with "Test against last 30 days" button, Skeleton loading state, BacktestResultPanel (count + zero-fires copy + 30-bar Recharts sparkline), and profile-aware kind filtering (full / limited / unsupported)
- 4 vitest cases added for AddRuleDialog backtest flow; visual verification approved by operator

## Task Commits

Each task was committed atomically:

1. **Task 1: BacktestRun + sqlc two-pass queries + typed fill helpers** - `59cecae` (feat)
2. **Task 2: HTTP handler + router mount + API client** - `adfb9e2` (feat)
3. **Task 3: AddRuleDialog backtest button + sparkline + profile-aware kind filter** - `70e6e5a` (feat)
4. **Task 4: Visual verification** - approved by operator (no code commit; checkpoint signed off)

## Files Created/Modified

- `internal/alert/backtest.go` - BacktestRun dispatcher + 3 typed fill helpers + fillDailyBucketsCore
- `internal/alert/backtest_test.go` - 8 tests (P95, IQR, quiet_hour, zero-fill, sparse, invalid days, unsupported kind, M-1 proof)
- `internal/api/backtest_handler.go` - BacktestHandler HTTP handler (read-only, defaults days=30)
- `internal/api/backtest_handler_test.go` - 4 handler tests (happy path, invalid kind 400, viewer 403, default days)
- `web/src/lib/backtest.ts` - Typed runBacktest() client via apiFetch
- `internal/db/queries/alerts.sql` - 5 new queries: BacktestP95Pass1/Pass2, BacktestIQRPass1/Pass2, BacktestQuietHourCount
- `internal/db/sqlc/alerts.sql.go` - sqlc-generated Go code for the 5 backtest queries
- `internal/db/sqlc/querier.go` - Querier interface extended with 5 backtest methods
- `internal/http/router.go` - Route mount: POST /api/alerts/backtest with ActionAlertRuleCreate RBAC guard
- `web/src/routes/settings/AddRuleDialog.tsx` - Backtest button + result panel + profile-aware kind filter
- `web/src/routes/settings/AddRuleDialog.test.tsx` - 4 new vitest cases for backtest UI flow

## Decisions Made

- Days capped at 90 in BacktestRun (matching RESEARCH §T-07-10-01 DoS mitigation); handler defaults missing/zero days to 30 — operator always gets the 30-day window without specifying it
- Three concrete typed fill helpers instead of one `any`-typed fill function — satisfies Checker M-1; compiler enforces correct sqlc row types at each call site
- RBAC uses ActionAlertRuleCreate (same permission as creating a rule) — backtest is part of configure-rule flow, not a separate admin capability
- Backtest endpoint is strictly read-only — no INSERT INTO alert anywhere in the call chain (T-07-10-04 mitigated)

## Deviations from Plan

None - plan executed exactly as written. The typed fill helper split (M-1), two-pass query pattern, profile-aware filtering, and zero-fires copy were all specified in the plan and implemented as described.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Backtest endpoint ready for plan 07-11a (saved report templates) — the alert surface is stable
- AddRuleDialog profile-aware filtering mirrors plan 09b server-side guard — both layers consistent
- Plan 07-14 doctor probes should smoke-test POST /api/alerts/backtest as part of phase closure

---
*Phase: 07-multi-vendor-breadth-v1-x-differentiators*
*Completed: 2026-05-13*
