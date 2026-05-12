---
phase: 07-multi-vendor-breadth-v1-x-differentiators
plan: 13
subsystem: gateway-bulk-import
tags: [gateway, bulk-import, csv, tdd, rbac, audit]
dependency_graph:
  requires: [07-01]
  provides: [bulk-gateway-import-service, bulk-gateway-import-api, bulk-gateway-import-ui]
  affects: [gateways-list-page, audit-log, authz]
tech_stack:
  added: []
  patterns:
    - Phase 3 device-import lifecycle (Validate → Commit, idempotency on EUI)
    - xmax=0 trick for INSERT vs UPDATE detection on upsert
    - Pre-fetch existing row before upsert for "skipped" vs "updated" detection
    - Serializable tx for per-row audit atomicity
    - createMemoryRouter migration for tests using useRouteLoaderData
key_files:
  created:
    - internal/db/migrations/0055_audit_vocab_gateway_bulk.up.sql
    - internal/db/migrations/0055_audit_vocab_gateway_bulk.down.sql
    - internal/db/queries/gateway_import.sql
    - internal/db/sqlc/gateway_import.sql.go
    - internal/gateway/import.go
    - internal/gateway/import_test.go
    - internal/api/gateway_import_handler.go
    - internal/api/gateway_import_handler_test.go
    - web/src/lib/gatewayImport.ts
    - web/src/routes/gateways/BulkImportDialog.tsx
    - web/src/routes/gateways/BulkImportDialog.test.tsx
  modified:
    - internal/audit/log.go
    - internal/auth/authz.go
    - internal/db/migrations_test.go
    - internal/db/roundtrip_test.go
    - internal/http/router.go
    - web/src/routes/gateways/index.tsx
    - web/src/routes/gateways/index.test.tsx
decisions:
  - Pre-fetch existing gateway row before upsert (instead of post-upsert comparison) to correctly distinguish "skipped" vs "updated" outcomes — xmax=0 only tells INSERT vs UPDATE, not whether fields actually changed
  - index.test.tsx migrated from MemoryRouter to createMemoryRouter+RouterProvider to support useCurrentUser/useRouteLoaderData hook introduced by admin-gating the Import button
metrics:
  duration: ~90 minutes execution
  completed: 2026-05-13
  tasks_completed: 4
  tasks_total: 4
  files_created: 11
  files_modified: 7
requirements-completed: [UX-POWER, V2-VEND-03]
---

# Phase 07 Plan 13: Bulk Gateway Import Summary

**One-liner:** CSV bulk gateway import with validate-then-commit lifecycle, idempotent on EUI, per-row audit, admin-only RBAC, and 3-step dialog mirroring Phase 3 device-import pattern.

## What Was Built

### Task 1: Migration + audit vocab + authz + import service (commit: 7ae6514)

Migration `0055_audit_vocab_gateway_bulk` adds `gateway.bulk_imported` to the `audit_log_action_valid` CHECK constraint via DROP+re-ADD pattern (matching prior migrations 0051, 0054).

`internal/gateway/import.go` — `ImportService` with two methods:
- `Validate(ctx, csvBytes) (ValidateResult, error)` — parses CSV (UTF-8 BOM guard, EUI regex validation, lat/lng pair requirement, known region slug validation), returns per-row error outcomes with no DB writes
- `Commit(ctx, csvBytes, actorID) (CommitResult, error)` — pre-fetches each existing gateway row, upserts with `ON CONFLICT (gateway_id) DO UPDATE`, writes one `audit_log` row per imported gateway in a Serializable transaction, returns `{Created, Updated, Skipped, Outcomes}`

`ActionGatewayBulkImport` added to `internal/auth/authz.go`, admin-only.

5 service tests (all green): Validate 3 valid rows, Validate invalid EUI, Commit idempotent re-run, Commit partial update (changed name → "updated"), Commit per-row audit assertions.

### Task 2: HTTP handlers + router mount + CSV template (commit: 21a5dbe)

`internal/api/gateway_import_handler.go` — three handlers:
- `POST /api/gateways/bulk-import/validate` — multipart CSV → `ValidateResult` JSON
- `POST /api/gateways/bulk-import/commit` — multipart CSV → `CommitResult` JSON  
- `GET /api/gateways/bulk-import/template` — streams CSV with UTF-8 BOM + example row

All three guarded by `RequireAction(ActionGatewayBulkImport)` (admin-only).
5 MiB `MaxBytesReader` cap returns 413 before parsing.
`internal/http/router.go` mounts routes via nil-guarded `GatewayImportDeps` field.

5 handler integration tests (all green): happy-path validate, commit creates+audits, idempotent commit, oversized file 413, viewer 403.

### Task 3: BulkImportDialog frontend + Gateways list integration (commit: 8b3e33d)

`web/src/lib/gatewayImport.ts` — typed API client with `validateGatewayCSV`, `commitGatewayCSV`, `gatewayImportTemplateURL`.

`web/src/routes/gateways/BulkImportDialog.tsx` — 3-step dialog:
- Step 1 (Upload): "Import Gateways" heading, description, "Download CSV template" link, "Discard import" / "Validate" footer
- Step 2 (Validation Results): valid count summary (`N gateways ready to import.`), error badge, error table, "Import N Gateways" / Retry footer
- Commit: "Importing gateways…" spinner state
- Done: "N gateways imported successfully." + "Show details" expand + "Close" footer
- Success toast: `Import complete: N gateways added.`
- Error toast: `Import failed. Check your connection and try again.`

`web/src/routes/gateways/index.tsx` — "Import gateways" button (admin-only via `useCurrentUser`) + `<BulkImportDialog>` mount.

5 frontend tests (all green): step 1→2 flow, error badge, Import N Gateways label, success toast, error toast.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] "skipped" vs "updated" detection required pre-fetch before upsert**
- **Found during:** Task 1 — `TestGatewayImport_Commit_PartialUpdate` failing (expected 2 updated, got 0)
- **Issue:** `UpsertGatewayForBulkImport` always UPDATEs the row. After the upsert, comparing `upserted.Name` against CSV input always matches because the upsert already applied the new name. No way to detect "was this actually different?" post-upsert.
- **Fix:** Pre-fetch existing gateway row with `GetGatewayByGatewayID` inside the same Serializable transaction BEFORE the upsert, then compare pre-existing DB values against CSV input via `isExistingGatewayUnchanged`.
- **Files modified:** `internal/gateway/import.go`
- **Commit:** 7ae6514

**2. [Rule 1 - Bug] index.test.tsx broken by useCurrentUser router requirement**
- **Found during:** Task 3 — 5 existing gateway list tests failed with "useRouteLoaderData must be used within a data router"
- **Issue:** Adding `useCurrentUser()` to `index.tsx` introduced `useRouteLoaderData` which requires a data router. Existing tests wrapped `GatewaysPage` in `MemoryRouter` (non-data router).
- **Fix:** Migrated `renderPage()` in `index.test.tsx` from `MemoryRouter` to `createMemoryRouter + RouterProvider` with a root route providing a viewer session user.
- **Files modified:** `web/src/routes/gateways/index.test.tsx`
- **Commit:** 8b3e33d

## Threat Mitigations Applied

| Threat ID | Mitigation | Verified By |
|-----------|------------|-------------|
| T-07-13-01 | 5 MiB `MaxBytesReader` cap → 413 | `TestBulkImportValidate_OversizedFile_413` |
| T-07-13-02 | UTF-8 BOM guard + trim in `parseGatewayCSV` | `TestGatewayImport_Validate_3RowsAllValid` |
| T-07-13-03 | Row cap 5000 in `parseGatewayCSV` | Runtime enforcement |
| T-07-13-04 | `ActionGatewayBulkImport` admin-only | `TestBulkImportValidate_ViewerForbidden_403` |
| T-07-13-05 | Per-row audit in Serializable tx | `TestGatewayImport_Commit_AuditPerRow` |

## Known Stubs

None — all data paths are wired. The dialog reads from real API endpoints (mocked in tests, real in production).

## Threat Flags

None — no new network surface beyond the three endpoints already in the plan's threat model.

## Task 4: Operator Verification (APPROVED)

**Task 4 (checkpoint:human-verify):** Operator approved the end-to-end bulk gateway import flow.

Verification steps confirmed:
1. `/gateways` — "Import gateways" button visible to admin
2. Step 1 dialog with "Download CSV template" link
3. Upload valid CSV → "Validate" → "3 gateways ready to import."
4. "Import 3 Gateways" → success toast + "3 gateways imported successfully."
5. Re-upload same CSV → all 3 "skipped" (idempotency confirmed)
6. Change one row name → 1 "updated"
7. Invalid EUI → error badge + error row in table

## Self-Check: PASSED

All key files found: import.go, handler.go, BulkImportDialog.tsx, gatewayImport.ts, migration 0055.
All 4 task commits confirmed: 7ae6514, 21a5dbe, 8b3e33d (+ operator approval Task 4).
15 tests total (5 service + 5 handler + 5 frontend) — all green.
Go build: clean. TypeScript typecheck: clean. Frontend build: clean.
