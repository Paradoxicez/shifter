---
phase: 06-alerts-users-audit-operational-hardening
plan: "07"
subsystem: audit
tags: [audit, csv-export, cursor-pagination, react-virtual, tanstack-table, river, timescaledb, tdd]

# Dependency graph
requires:
  - phase: 06-01-alert-engine-substrate
    provides: audit_log table schema, AuditStore Write/WriteInTx, ActionAuditRead/ActionAuditExport constants
  - phase: 06-06-auth-event-audit-retrofit
    provides: audit-in-tx pattern for auth events; validates audit_log rows exist pre-browse
  - phase: 04-09-metering-point-detail
    provides: JsonTree component reused with new highlightKeys prop for before/after diff
provides:
  - cursor-paginated GET /api/audit with filters (from, to, user_id, entity_type[], action[], request_id)
  - GET /api/audit/count + /api/audit/distincts supporting endpoints
  - GET /api/audit/export: inline CSV (<=50k rows) with UTF-8 BOM + ISO-8601 + CSV-injection mitigation
  - POST /api/audit/export-async: River AuditExportWorker for >50k rows, 24h TTL filesystem cleanup
  - /audit React route: admin-only, URL-state filter chips, virtualized TanStack Table, side-by-side JsonTree diff
  - JsonTree highlightKeys prop for D-34 diff highlighting of changed keys
affects: [06-08, 06-09, 06-10, 06-11]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Row-comparison cursor pagination: (time DESC, id DESC) < ($1, $2) for stable paging under concurrent inserts (D-36)"
    - "CSV export: UTF-8 BOM + ISO-8601 in install_tz + apostrophe-prefix CSV-injection mitigation (REPT-03)"
    - "Mtime-based CSV TTL cleanup via PruneExpiredCSVExports (no DB row; differs from PDF cleanup which is DB-driven)"
    - "Optional RiverInserter interface on audit.Deps — nil = degraded (skip enqueue, still return 202)"
    - "URL-state schema: zod with .catch() fallbacks for robust deep-link handling (D-15 pattern)"
    - "TanStack Virtual + TanStack Table for virtualized audit log (600px scrollable container)"

key-files:
  created:
    - internal/audit/browse_store.go
    - internal/audit/browse_store_test.go
    - internal/audit/export.go
    - internal/audit/export_test.go
    - internal/audit/export_worker.go
    - internal/audit/export_worker_test.go
    - web/src/lib/auditParams.ts
    - web/src/hooks/useAudit.ts
    - web/src/routes/audit/index.tsx
    - web/src/routes/audit/AuditFilterChips.tsx
    - web/src/routes/audit/AuditTable.tsx
    - web/src/routes/audit/AuditRowExpand.tsx
    - web/src/routes/audit/AuditExportButton.tsx
    - web/playwright/specs/audit-export.spec.ts
  modified:
    - internal/audit/handler.go (added RiverClient to Deps, browse/count/distincts handlers)
    - internal/cli/serve.go (AuditExportWorker registration, RiverClient wiring, ReportsDir)
    - internal/report/cleanup.go (ReportsDir field + PruneExpiredCSVExports call)
    - internal/report/pdf_worker_test.go (TestCleanupExpiredReportsWorker_PrunesCSV added)
    - web/src/components/metering-point/JsonTree.tsx (highlightKeys prop + data-highlight attr)
    - web/src/App.tsx (/audit lazy route registration)

key-decisions:
  - "D-33: Default browse window = last 7 days when from/to both absent (applied server-side and client-side zod .catch)"
  - "D-34: JsonTree highlightKeys prop — bg-warning/20 + data-highlight=true on changed keys, passes down recursively"
  - "D-35: 50k inline / >50k async split — ExportHandler writes audit meta-row; ExportAsyncHandler enqueues River job"
  - "D-36: Row-comparison cursor pagination (time, id) < ($1, $2) — stable under concurrent inserts, O(1) cursor decode"
  - "D-37: No live-tail / no SSE — manual Refresh button + refetchOnWindowFocus:false"
  - "RiverClient is optional (nil = skip River enqueue, still return 202 with job_id) — allows degraded operation without River"
  - "PruneExpiredCSVExports uses mtime-based 24h TTL on filesystem; no DB row for audit CSVs unlike PDF reports"
  - "audit.Deps.RiverClient uses rivertype.JobInsertResult (sub-package) not river.JobInsertResult (not exported)"

patterns-established:
  - "Cursor pagination: (time DESC, id DESC) tuple cursor encodes as base64 JSON in next_cursor response field"
  - "CSV export pipeline: BOM write → timezone comment header → column header → streaming rows via pgx.Rows"
  - "Export cleanup integration: CleanupExpiredReportsWorker.ReportsDir optional field calls audit.PruneExpiredCSVExports"

requirements-completed: [AUDIT-02, AUDIT-03]

# Metrics
duration: 37min
completed: 2026-05-12
---

# Phase 06 Plan 07: Audit Browse + Export Summary

**Cursor-paginated audit log browse with inline/async CSV export (50k split), admin-only /audit React route with URL-state filter chips, virtualized TanStack Table, and side-by-side JsonTree diff highlighting.**

## Performance

- **Duration:** 37 min
- **Started:** 2026-05-12T10:09:14Z
- **Completed:** 2026-05-12T10:46:43Z
- **Tasks:** 3 (each TDD: RED + GREEN)
- **Files modified:** 20

## Accomplishments

- Cursor-paginated audit log browse with full filter support (from/to, entity_type[], action[], user_id, request_id LIKE); server enforces 7-day default (D-33) and vocabulary validation (T-06-07-02)
- CSV export pipeline: UTF-8 BOM, timezone comment header, ISO-8601 in install_tz, apostrophe-prefix CSV injection mitigation; inline path for <=50k rows, River AuditExportWorker for >50k with 24h mtime-based filesystem TTL
- /audit React route: admin-only guard (D-31), URL-state zod schema with .catch() fallbacks, virtualized TanStack Table with row expand + side-by-side JsonTree diff (JsonTree highlightKeys prop D-34), Export CSV button with inline/async split

## Task Commits

Each task was committed atomically with TDD pattern (RED then GREEN):

1. **Task 1 GREEN: cursor query + browse store + HTTP handlers** — `a289a5b` (feat)
2. **Task 2 RED: export + worker tests** — `d774758` (test)
3. **Task 2 GREEN: CSV export inline <=50k + AuditExportWorker for >50k** — `1eafb34` (feat)
4. **Task 3 RED: /audit route unit tests** — `9d65322` (test)
5. **Task 3 GREEN: /audit React route — filter chips + table + diff** — `b213eac` (feat)

_Note: Task 1 RED commit occurred in prior session (not shown above)._

## Files Created/Modified

- `internal/audit/browse_store.go` — ListCursor + CountAudit + DistinctValues; nullable column scan via *string intermediaries
- `internal/audit/browse_store_test.go` — TestListCursor_DefaultWindow, TestListCursor_Pagination, TestCountAudit, TestDistinctValues
- `internal/audit/export.go` — StreamCSVExportToWriter (UTF-8 BOM, REPT-03), ExportHandler (inline), ExportAsyncHandler (River enqueue), PruneExpiredCSVExports
- `internal/audit/export_test.go` — 11 tests covering BOM, headers, timezone, column order, ISO timestamps, CSV injection, filters, 50k cap, async enqueue, cleanup
- `internal/audit/export_worker.go` — AuditExportWorker (river.Worker), AuditExportArgs (river.JobArgs), writes audit-export.csv to ReportsDir/{job_id}/
- `internal/audit/export_worker_test.go` — 6 tests covering Kind/InsertOpts, file write, path glob matching for cleanup, prune age logic
- `internal/audit/handler.go` — Deps struct with optional RiverClient; browse/count/distincts/export/export-async handlers; vocabulary validation middleware
- `internal/cli/serve.go` — AuditExportWorker registered; CleanupExpiredReportsWorker.ReportsDir wired; AuditDeps.RiverClient set
- `internal/report/cleanup.go` — ReportsDir field; calls audit.PruneExpiredCSVExports at end of Work()
- `internal/report/pdf_worker_test.go` — TestCleanupExpiredReportsWorker_PrunesCSV added
- `web/src/lib/auditParams.ts` — zod URL-state schema; useAuditParams hook (merging setter)
- `web/src/hooks/useAudit.ts` — useAuditList (infinite), useAuditCount, useAuditDistincts, useExportAuditAsync
- `web/src/components/metering-point/JsonTree.tsx` — highlightKeys prop; bg-warning/20 + data-highlight on matching keys; recursive propagation
- `web/src/routes/audit/index.tsx` — AuditPage with admin guard → Navigate; AuditPageContent with layout
- `web/src/routes/audit/AuditFilterChips.tsx` — datetime-local inputs, multi-select entity_type/action, "Last 7 days" button, Clear all
- `web/src/routes/audit/AuditTable.tsx` — TanStack Table + useVirtualizer; columns: time/user/action/entity/request_id/expand; expandedRows Set; AuditRowExpand
- `web/src/routes/audit/AuditRowExpand.tsx` — diffKeys(); side-by-side Before/After JsonTree panels; "(created)"/"(removed)" for null
- `web/src/routes/audit/AuditExportButton.tsx` — INLINE_CAP=50k; window.location.href for inline; asyncExport.mutate for >50k; data-inline attr
- `web/src/routes/audit/index.test.tsx` — 12 passing tests; auditParams, JsonTree highlightKeys, AuditRowExpand, AuditPage viewer redirect, AuditExportButton
- `web/playwright/specs/audit-export.spec.ts` — admin: /audit heading + last-7-days chip + CSV BOM bytes; viewer: redirect
- `web/src/App.tsx` — lazy AuditPage import + /audit route registered

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] NULL scan for nullable columns in browse_store.go**
- **Found during:** Task 1 GREEN
- **Issue:** `audit_log.notes` and `request_id` are nullable TEXT; scanning into plain `string` fails on NULL
- **Fix:** Scan into `*string` intermediaries (`var notesStr, requestIDStr, userIDStr *string`); dereference if non-nil
- **Files modified:** `internal/audit/browse_store.go`
- **Commit:** `a289a5b`

**2. [Rule 1 - Bug] NULL scan for user_id in export SQL**
- **Found during:** Task 2 GREEN — `TestExport_RespectsFilters` failed with "cannot scan NULL into *string for user_id"
- **Issue:** `a.user_id::text` returns NULL when user_id is NULL in export query
- **Fix:** Changed to `COALESCE(a.user_id::text, '') AS user_id` in the exportSQL constant
- **Files modified:** `internal/audit/export.go`
- **Commit:** `1eafb34`

**3. [Rule 3 - Blocking] river.JobInsertResult not exported from river package**
- **Found during:** Task 2 GREEN — compiler error `river.JobInsertResult undefined`
- **Issue:** `river` re-exports result types via `rivertype` sub-package; `river.JobInsertResult` does not exist
- **Fix:** Added import `github.com/riverqueue/river/rivertype`; changed interface return to `*rivertype.JobInsertResult`
- **Files modified:** `internal/audit/export.go`
- **Commit:** `1eafb34`

**4. [Rule 1 - Bug] `audit_log_action_valid` check constraint violation in test**
- **Found during:** Task 2 GREEN — `TestExport_RespectsFilters` failed with check constraint
- **Issue:** Test used `"alert.ack"` which is not a valid action enum value
- **Fix:** Changed to `"alert.acknowledged"` (the correct action value per 06-01 schema)
- **Files modified:** `internal/audit/export_test.go`
- **Commit:** `1eafb34`

**5. [Rule 1 - Bug] vi.mock factory hoisting — adminUser not yet initialized**
- **Found during:** Task 3 GREEN — test suite failed with "Cannot access 'adminUser' before initialization"
- **Issue:** `vi.mock` factories are hoisted before top-level variable initializations; referencing `adminUser` inside the factory throws
- **Fix:** Inlined the default user object literal directly in the factory; individual tests override via `vi.mocked()`
- **Files modified:** `web/src/routes/audit/index.test.tsx`
- **Commit:** `b213eac`

**6. [Rule 3 - Blocking] reportsRoot variable used before assignment in serve.go**
- **Found during:** Task 2 GREEN — compiler error: `reportsRoot` local variable undefined at worker registration
- **Issue:** `reportsRoot` was assigned below the worker registration block in serve.go
- **Fix:** Used `cfg.ReportsRoot` directly instead of local variable
- **Files modified:** `internal/cli/serve.go`
- **Commit:** `1eafb34`

## Known Stubs

None — all plan goals achieved; no placeholder data flowing to UI.

## Threat Flags

None found beyond the plan's threat model (T-06-07-01 auth guard, T-06-07-02 vocabulary validation).

## Self-Check: PASSED

Files verified:
- `internal/audit/browse_store.go` — FOUND
- `internal/audit/export.go` — FOUND
- `internal/audit/export_worker.go` — FOUND
- `web/src/routes/audit/index.tsx` — FOUND
- `web/src/routes/audit/AuditTable.tsx` — FOUND
- `web/src/routes/audit/AuditExportButton.tsx` — FOUND
- `web/src/lib/auditParams.ts` — FOUND
- `web/src/hooks/useAudit.ts` — FOUND

Commits verified:
- `a289a5b` — FOUND (feat: cursor query + browse store + handlers)
- `1eafb34` — FOUND (feat: CSV export + AuditExportWorker)
- `b213eac` — FOUND (feat: /audit React route)
