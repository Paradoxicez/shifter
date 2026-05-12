---
phase: 05-aggregates-reports-map-floor-plans
plan: 03
subsystem: report
tags: [report, sqlc, csv, excel, handler, cagg, audit, timescaledb]
dependency_graph:
  requires: [05-02]
  provides: [report-data-layer, generate-handler, csv-writer, excel-writer]
  affects: [05-06-reports-pdf-river-worker, 05-09-reports-frontend]
tech_stack:
  added: [excelize/v2, encoding/csv]
  patterns: [sqlc-cagg-queries, D-03-yoy-silent-fallback, D-09-capability-filter, D-23-audit-atomicity, utf8-bom-csv]
key_files:
  created:
    - internal/db/migrations/0030_report.up.sql
    - internal/db/migrations/0030_report.down.sql
    - internal/db/migrations/0031_audit_vocab_phase5.up.sql
    - internal/db/migrations/0031_audit_vocab_phase5.down.sql
    - internal/db/queries/reports.sql
    - internal/db/sqlc/reports.sql.go
    - internal/report/doc.go
    - internal/report/assembler.go
    - internal/report/assembler_test.go
    - internal/report/delta.go
    - internal/report/delta_test.go
    - internal/report/csv.go
    - internal/report/csv_test.go
    - internal/report/excel.go
    - internal/report/excel_test.go
    - internal/report/handlers.go
    - internal/report/handlers_test.go
  modified:
    - internal/audit/log.go
    - internal/db/migrations_test.go
    - internal/db/roundtrip_test.go
decisions:
  - "Use ReportData (not Report) to avoid name conflict with sqlc-generated Report type"
  - "Period field in sqlc result is interface{}; timeFromInterface() helper handles pgtype.Timestamptz and time.Time cases"
  - "ComputeYoY takes *float64 (pre-fetched prior-year total) rather than querying DB inside the assembler; returns nil for D-03 silent fallback"
  - "csv.Reader requires FieldsPerRecord=-1 because the blank separator row produces a zero-field line that would otherwise cause a field-count mismatch"
  - "auth.GetUser(ctx, sm) is the correct handler auth pattern (not auth.UserFromCtx or auth.SetUser)"
  - "Integration test session injection uses sm.Put(ctx, key, val) directly (no HTTP round-trip needed)"
  - "EnqueuePDF hook on Deps is nil-tolerant in 05-03; plan 05-06 wires the River InsertTx call"
metrics:
  duration: "~3 hours"
  completed: "2026-05-12"
  tasks_completed: 5
  files_created: 17
  files_modified: 3
---

# Phase 05 Plan 03: Report Data Layer (sqlc + Assembler + CSV + Excel + Handler) Summary

**One-liner:** Synchronous report generation with CAGG-backed sqlc queries, CSV (UTF-8 BOM) and Excel (3-sheet excelize) writers, and an atomic HTTP handler (report INSERT + audit.WriteEntry in one pgx.Tx).

## Objective

Deliver the data layer for Phase 5 reports: sqlc queries against the CAGG hierarchy (daily/monthly/yearly x all-meters/site/single-meter x group-by-site/category), assembler that maps query results into a ReportData struct with prior-period and YoY deltas, CSV and Excel writers, and the synchronous POST /api/reports/generate handler.

## Tasks Completed

| Task | Description | Commit |
|------|-------------|--------|
| 1 | Migrations 0030 (report table) + 0031 (audit vocab), sqlc queries, sqlc code generation | 5f2e1c1 |
| 2 | Assembler + delta math (ComputeDelta, ComputeYoY, D-03 silent fallback) | 5f2e1c1 |
| 3 | CSV writer: UTF-8 BOM, ISO timestamps, 3-row metadata header | 1b5ce2a |
| 4 | Excel writer: 3 sheets (Summary/Period Detail/Meter Detail), date numFmt, bold+border totals row | c920514 |
| 5 | POST /api/reports/generate handler: auth, ToConfig validation, BuildReport, atomic tx, CSV+Excel to disk | c920514 |
| 6 | Migration test updates: version 29→31, step count corrections, report table assertion | 2fe9e30 |

## Key Design Points

**CAGG query selection:** daily range → measurement_hourly; monthly → measurement_daily; yearly → measurement_monthly. Raw measurement table is never queried by report logic.

**Scope × Group matrix:** 9 sqlc queries cover daily/monthly/yearly × by-MP/by-site/by-category. ListMetersInScope filters by scope and site_id/metering_point_id as appropriate.

**D-03 silent fallback:** ComputeYoY returns nil (not error) when no prior-year data exists. No error surface exposed to the user.

**D-09 capability filter:** capabilityMatches(utilityClass, cfg.Capabilities) silently drops rows for absent capabilities in single-capability installs.

**D-23 audit atomicity:** audit.WriteEntry is called inside the same pgx.Tx as CreateReport. If either fails, both roll back.

**D-07 artifact TTL:** report.expires_at is set to 24 hours from generation. Cleanup is deferred to plan 05-06.

**EnqueuePDF nil-tolerance:** Deps.EnqueuePDF hook is nil in this plan; plan 05-06 wires the River InsertTx call inside the same transaction.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] csv.Reader field-count mismatch from blank separator row**
- **Found during:** Task 3 (CSV test)
- **Issue:** `cw.Write([]string{})` emits a bare `\n`; csv.Reader with default FieldsPerRecord=3 rejects this as "wrong number of fields"
- **Fix:** Set `r.FieldsPerRecord = -1` in csv_test.go to allow variable field counts; adjusted row index expectations (blank row is skipped by csv.Reader so rows[3] is the data header, not rows[4])
- **Files modified:** internal/report/csv_test.go

**2. [Rule 1 - Bug] sqlc type casing typo in assembler.go**
- **Found during:** Task 2 (assembler compilation)
- **Issue:** Used `sqlc.ReportYearlybyCategoryParams` (lowercase 'b') but generated type is `ReportYearlyByCategoryParams`
- **Fix:** Corrected casing in assembler.go
- **Files modified:** internal/report/assembler.go

**3. [Rule 1 - Bug] Unused import and undefined auth symbols in handlers.go**
- **Found during:** Task 5 (handler compilation)
- **Issue:** Missing `"fmt"` import; helper `makeAuthRequest` referenced non-existent `auth.SetUser`; `install.FirstRunGate` symbol does not exist
- **Fix:** Added `"fmt"` import; removed `makeAuthRequest` helper (unused after switching to sm.Put-based injection); removed install package import
- **Files modified:** internal/report/handlers.go, internal/report/handlers_test.go

**4. [Rule 2 - Missing functionality] auth.GetUser pattern for session-backed handler**
- **Found during:** Task 5
- **Issue:** Initial handler draft attempted `auth.UserFromCtx(ctx)` which does not exist; the correct call requires the session manager reference
- **Fix:** Used `auth.GetUser(ctx, deps.SessionMgr)` throughout; propagated SessionMgr through Deps struct
- **Files modified:** internal/report/handlers.go

## Threat Surface Scan

No new network endpoints beyond POST /api/reports/generate (which is documented in the plan). The handler is auth-gated (401 without valid session). Artifact directory creation uses MkdirAll 0755; no world-writable paths. No new DB trust boundaries beyond the report/audit tables covered by the plan's threat_refs.

## Known Stubs

- `Deps.EnqueuePDF` is nil in this plan. The PDF status returned is always "pending" with no actual River job enqueued. Plan 05-06 wires the River InsertTx call.
- `GET /api/reports/{id}/file/{kind}` download route is documented in handlers.go but not mounted. Plan 05-06 adds this.
- `TestStatusHandler_PendingReadyFailed` is skipped (t.Skip) pending plan 05-06 implementation.
- `cfg.Capabilities` is hardcoded to "both" in the handler; plan 05-09 wires the real value from install_identity.capabilities.

## Self-Check: PASSED

Files exist: assembler.go, delta.go, csv.go, excel.go, handlers.go, 0030_report.up.sql, 0031_audit_vocab_phase5.up.sql, reports.sql — all confirmed present.

Commits exist: 5f2e1c1, 1b5ce2a, c920514, 2fe9e30 — all confirmed in git log.
