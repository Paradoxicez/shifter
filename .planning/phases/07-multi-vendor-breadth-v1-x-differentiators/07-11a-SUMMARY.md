---
phase: 07-multi-vendor-breadth-v1-x-differentiators
plan: "11a"
subsystem: report-templates-backend
tags: [report-templates, rbac, audit, migration, sqlc, chi, http]
dependency_graph:
  requires: [07-01]
  provides:
    - "GET /api/reports/templates"
    - "GET /api/reports/templates/{id}"
    - "POST /api/reports/templates"
    - "PATCH /api/reports/templates/{id}"
    - "DELETE /api/reports/templates/{id}"
  affects: [07-11b, 07-14]
tech_stack:
  added: []
  patterns:
    - "audit-in-tx: every mutation wraps sqlc query + audit.WriteEntry in same pgx.Tx (D-23)"
    - "23505 unique_violation → 409 Conflict via errors.As(*pgconn.PgError)"
    - "pgx.ErrNoRows chain unwrap via errors.Is for 404 detection"
    - "nil-guarded route mount in router.go (PITFALL #4 preserved)"
key_files:
  created:
    - internal/db/migrations/0053_report_template.up.sql
    - internal/db/migrations/0053_report_template.down.sql
    - internal/db/migrations/0054_audit_vocab_report_template.up.sql
    - internal/db/migrations/0054_audit_vocab_report_template.down.sql
    - internal/db/queries/report_templates.sql
    - internal/db/sqlc/report_templates.sql.go
    - internal/report/template_store.go
    - internal/report/template_store_test.go
    - internal/api/report_templates_handler.go
    - internal/api/report_templates_handler_test.go
  modified:
    - internal/db/sqlc/models.go
    - internal/db/sqlc/querier.go
    - internal/audit/log.go
    - internal/auth/authz.go
    - internal/auth/authz_test.go
    - internal/http/router.go
    - internal/db/migrations_test.go
    - internal/db/roundtrip_test.go
decisions:
  - "audit_log.user_id FK requires seeding a real user row in store integration tests — uuid.New() fails with 23503 (FK violation)"
  - "TemplateStore.Update does a pre-flight GetReportTemplate to surface ErrNoRows before UpdateReportTemplate exec (which returns nil for 0 rows affected)"
  - "writeAPIError/writeAPIJSON helpers are local to report_templates_handler.go; no shared api-package helper exists for JSON error responses"
  - "Migration version bumped 51 → 54 (0052 was 09b, 0053/0054 are 11a) — migration_test.go and roundtrip_test.go assertions updated"
metrics:
  duration_minutes: 25
  completed_date: "2026-05-12"
  tasks_completed: 2
  tasks_total: 2
  files_changed: 18
---

# Phase 07 Plan 11a: Saved Report Templates Backend Summary

**Report template CRUD backend — 2 migrations (0053/0054) + 5 sqlc queries + 3 audit constants + 4 authz actions + TemplateStore + 5 HTTP handlers + router mount, with 7 integration tests covering happy paths, 409 collisions, viewer RBAC enforcement, and audit-row guarantees.**

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Migrations + sqlc + audit vocab + authz | 34240fd | 0053*.sql, 0054*.sql, report_templates.sql, models.go, querier.go, log.go, authz.go, authz_test.go, migrations_test.go, roundtrip_test.go |
| 2 RED | Failing tests for store + handlers | 6b7d969 | template_store_test.go, report_templates_handler_test.go |
| 2 GREEN | Template store + 5 HTTP handlers + router | abcf200 | template_store.go, report_templates_handler.go, router.go |
| 2 FIX | Seed real user in store test (Rule 1) | 5f0c30e | template_store_test.go |

## What Was Built

### Schema (Migration 0053)

```sql
CREATE TABLE report_template (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    state       JSONB NOT NULL,
    created_by  UUID NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX report_template_name_lower_idx ON report_template (lower(name));
```

### Audit Vocab (Migration 0054)

Extends `audit_log` CHECK constraints with:
- `report_template.created` — POST /api/reports/templates
- `report_template.updated` — PATCH /api/reports/templates/{id}
- `report_template.deleted` — DELETE /api/reports/templates/{id}
- `report_template` entity type

### sqlc Queries (5)

| Query | Kind | Purpose |
|-------|------|---------|
| ListReportTemplates | :many | ORDER BY lower(name) ASC |
| GetReportTemplate | :one | by UUID primary key |
| CreateReportTemplate | :one | INSERT RETURNING * |
| UpdateReportTemplate | :exec | name/description/state/updated_at |
| DeleteReportTemplate | :exec | hard DELETE by id |

### Audit Constants (3)

- `AuditActionReportTemplateCreated = "report_template.created"`
- `AuditActionReportTemplateUpdated = "report_template.updated"`
- `AuditActionReportTemplateDeleted = "report_template.deleted"`

### RBAC Actions (4)

| Action | Admin | Viewer |
|--------|-------|--------|
| ActionReportTemplateRead | true | true |
| ActionReportTemplateCreate | true | false |
| ActionReportTemplateUpdate | true | false |
| ActionReportTemplateDelete | true | false |

### API Endpoints (5)

| Method | Path | RBAC | Status codes |
|--------|------|------|-------------|
| GET | /api/reports/templates | Read (admin+viewer) | 200 |
| GET | /api/reports/templates/{id} | Read (admin+viewer) | 200, 404 |
| POST | /api/reports/templates | Create (admin) | 201, 400, 409 |
| PATCH | /api/reports/templates/{id} | Update (admin) | 204, 400, 404, 409 |
| DELETE | /api/reports/templates/{id} | Delete (admin) | 204, 404 |

### Test Coverage (7 integration tests)

| Test | What it verifies |
|------|----------------|
| TestTemplateStore_SaveListGetUpdateDelete | All 5 store ops + 3 audit rows (Save+Update+Delete) |
| TestCreateReportTemplateHandler_HappyPath | 201 + response shape |
| TestCreateReportTemplateHandler_DuplicateName409 | 409 on name collision (T-07-11a-04) |
| TestUpdateReportTemplateHandler_RenameCollision409 | 409 on rename collision |
| TestDeleteReportTemplateHandler_AuditWritten | 204 + audit row in DB (T-07-11a-03) |
| TestCreateReportTemplateHandler_ViewerForbidden | 403 for viewer POST (T-07-11a-02) |
| TestListReportTemplatesHandler_ViewerAllowed | 200 for viewer GET |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] audit_log.user_id FK violation with uuid.New() in store test**
- **Found during:** Task 2 GREEN — first integration test run
- **Issue:** `TestTemplateStore_SaveListGetUpdateDelete` passed `uuid.New()` as actorID. `audit.WriteEntry` sets `pgtype.UUID{Bytes: actorID, Valid: actorID != uuid.Nil}` — a non-nil UUID is sent as a real UUID value to the FK column, but that UUID doesn't exist in the `user` table, causing PostgreSQL error 23503 (foreign_key_violation).
- **Fix:** Seeded a real admin user row before the store operations and used its UUID as actorID. The fix matches the pattern used by all other integration tests in the project (e.g., catalog_handler_test.go, report/handlers_test.go).
- **Files modified:** `internal/report/template_store_test.go`
- **Commit:** 5f0c30e

## Verification Results

- `go build ./...` — clean
- `go test ./internal/auth/... -count=1` — 87 tests pass (including new TestAuthz_ReportTemplate_AdminCanAll)
- `go test ./internal/report/... ./internal/api/... -count=1 -run 'TestTemplateStore|ReportTemplate'` — 7 tests pass
- `go test ./internal/auth/... ./internal/http/... -count=1 -short` — 110 tests pass

## Known Stubs

None — all 5 endpoints return live database data. Plan 11b will wire the frontend dropdown and save dialog to consume these endpoints.

## Threat Flags

None — all trust boundaries from the plan's threat model were mitigated:
- T-07-11a-01: state JSONB stored via parameterized query (never interpolated)
- T-07-11a-02: ActionReportTemplateCreate/Update/Delete are admin-only + test proves viewer gets 403
- T-07-11a-03: Delete audit row written in same tx + test asserts row exists
- T-07-11a-04: UNIQUE constraint → 23505 → 409 (accepted last-write-wins)

## Self-Check: PASSED

Files exist:
- internal/db/migrations/0053_report_template.up.sql: FOUND
- internal/db/migrations/0054_audit_vocab_report_template.up.sql: FOUND
- internal/db/queries/report_templates.sql: FOUND
- internal/db/sqlc/report_templates.sql.go: FOUND
- internal/report/template_store.go: FOUND
- internal/report/template_store_test.go: FOUND
- internal/api/report_templates_handler.go: FOUND
- internal/api/report_templates_handler_test.go: FOUND

Commits exist in git log:
- 34240fd: feat(07-11a): migrations + sqlc + audit vocab + authz
- 6b7d969: test(07-11a): add failing tests (RED)
- abcf200: feat(07-11a): template store + 5 HTTP handlers + router mount
- 5f0c30e: fix(07-11a): seed real user in store test to satisfy audit_log FK
