---
phase: 05-aggregates-reports-map-floor-plans
plan: "05-05"
subsystem: floor-plans
tags: [floor-plan, image-upload, migrations, audit, compose]
dependency_graph:
  requires: [05-01]
  provides: [floor_plan-table, device_floor_plan_placement-table, floorplan-package, audit-vocab-floor-plan]
  affects: [05-07, 05-10]
tech_stack:
  added: []
  patterns:
    - MIME sniff via http.DetectContentType (server-side, ignores client Content-Type)
    - Dimension probe via image.DecodeConfig (header-only, BEFORE disk write)
    - Disk+tx atomicity (write file, BEGIN tx, INSERT, audit.WriteEntry, COMMIT; rollback defers unlink)
    - Replay reader pattern (io.MultiReader + io.TeeReader) to allow multi-pass stream reading
key_files:
  created:
    - internal/db/migrations/0032_floor_plan.up.sql
    - internal/db/migrations/0032_floor_plan.down.sql
    - internal/db/migrations/0033_device_floor_plan_placement.up.sql
    - internal/db/migrations/0033_device_floor_plan_placement.down.sql
    - internal/db/migrations/0034_audit_vocab_floor_plan.up.sql
    - internal/db/migrations/0034_audit_vocab_floor_plan.down.sql
    - internal/db/queries/floor_plan.sql
    - internal/db/sqlc/floor_plan.sql.go
    - internal/floorplan/doc.go
    - internal/floorplan/handlers.go
    - internal/floorplan/image.go
    - internal/floorplan/routes.go
  modified:
    - internal/floorplan/handlers_test.go
    - internal/audit/log.go
    - internal/db/migrations_test.go
    - internal/db/roundtrip_test.go
    - compose/bundled.yml
    - compose/external.yml
decisions:
  - "Renumbered floor_plan migration 0031→0032 because plan 05-03 already created 0031_audit_vocab_phase5; audit vocab extension for floor plans became 0034 (sequential after 0033 device_floor_plan_placement)"
  - "Replay reader pattern (io.MultiReader + bytes.Buffer + io.TeeReader) used in ValidateImageHeader and ProbeDimensions to allow two-pass reads without seeking"
  - "uuid.Parse helper userUUID() translates auth.User.ID (string) to uuid.UUID for audit.Entry.UserID; returns uuid.Nil on failure (session middleware guarantees well-formed UUID for authenticated calls)"
  - "Handler tests use external test package (floorplan_test) with real Postgres container + full migration stack to exercise DB constraints"
metrics:
  duration_minutes: 120
  completed_at: "2026-05-12T01:02:09Z"
  tasks_completed: 2
  files_changed: 18
---

# Phase 05 Plan 05: Floor Plan Schema + Upload Summary

Floor plan CRUD backend: `floor_plan` + `device_floor_plan_placement` DB tables (D-16/D-20/D-25), sqlc query layer, Go `internal/floorplan/` package with MIME-sniff + dimension-probe image validation (10 MiB cap, 8192 px cap, PDF rejected server-side per D-17), disk+tx atomicity (D-23), and `floor_plans` named volume in both compose flavors.

## Tasks Completed

| Task | Description | Commit |
|------|-------------|--------|
| 1 | floor_plan + device_floor_plan_placement migrations (0032/0033) + sqlc query layer | 68fccda |
| 2 | floorplan package (image validation, handlers, routes), audit vocab migration (0034), compose volume mounts, 8 integration tests | ceffcc4 |

## What Was Built

### Migrations

- **0032_floor_plan**: `floor_plan` table with `UNIQUE(site_id, sort_order)`, `image_w/image_h CHECK <= 8192`, `ON DELETE CASCADE` from `site`. Index on `(site_id, sort_order)`.
- **0033_device_floor_plan_placement**: `device_floor_plan_placement` with `device_id PK`, `floor_plan_id FK`, `x_frac/y_frac REAL CHECK [0,1]`, `ON DELETE CASCADE` on both FKs. Fractional coordinates survive floor plan image swaps (D-24).
- **0034_audit_vocab_floor_plan**: Extends `audit_log` CHECK constraints to add `floor_plan.upload`, `floor_plan.replace_image`, `floor_plan.rename`, `floor_plan.delete` actions and `floor_plan` entity type. Follows the DROP+re-ADD pattern from prior vocab migrations.

Latest DB version: **34**.

### sqlc Query Layer

Seven queries in `internal/db/queries/floor_plan.sql`: `CreateFloorPlan`, `ListFloorPlansBySite` (ordered by `sort_order, uploaded_at`), `GetFloorPlan`, `UpdateFloorPlanImage`, `UpdateFloorPlanLabel`, `DeleteFloorPlan`, `CountPinsOnFloorPlan`.

### internal/floorplan/ Package

**`image.go`**: Image validation primitives:
- `ValidateImageHeader`: sniffs first 512 bytes via `http.DetectContentType`. Accepts `image/png` and `image/jpeg`; explicitly rejects `application/pdf` with T-05-05-02-compliant message. Returns replay reader.
- `ProbeDimensions`: reads image header via `image.DecodeConfig` (no full decode). Rejects dimensions > 8192 px (D-19). Returns replay reader.
- Constants: `MaxUploadBytes = 10 << 20`, `MaxDimension = 8192`.

**`handlers.go`**: Six HTTP handlers with correct validation order (MaxBytesReader → ParseMultipartForm → MIME sniff → dim probe → disk write → DB tx + audit):
- `UploadImageHandler`: server-generates UUID filename (T-05-05-03 path traversal defense). Rollback defer unlinks new file on any error.
- `ReplaceImageHandler`: writes new file, UPDATE in tx, unlinks old file only after successful commit. Keeps all `device_floor_plan_placement` rows intact (D-24).
- `RenameHandler`, `ListBySiteHandler`, `GetHandler`, `DeleteHandler`.

**`routes.go`**: Chi router with per-action auth guards (`site.create` for upload, `site.update` for PATCH, `site.archive` for DELETE, `site.read` for GET).

### Compose Changes

Both `compose/bundled.yml` and `compose/external.yml` updated:
- `SHIFTER_FLOOR_PLANS_DIR: /var/lib/shifter/floor-plans` env var on the `shifter` service
- `- floor_plans:/var/lib/shifter/floor-plans` volume mount on the `shifter` service
- Top-level `floor_plans:` named volume declaration

### Audit Constants

`internal/audit/log.go` gains:
- `ActionFloorPlanUpload = "floor_plan.upload"`
- `ActionFloorPlanReplace = "floor_plan.replace_image"`
- `ActionFloorPlanRename = "floor_plan.rename"`
- `ActionFloorPlanDelete = "floor_plan.delete"`
- `EntityTypeFloorPlan = "floor_plan"`

## Test Results

All 8 handler integration tests pass:
- `TestImageUpload_PNG_HappyPath` — 201, DB row, file on disk
- `TestImageUpload_RejectsPDF` — 415, no file written (D-17)
- `TestImageUpload_RejectsOversizedFile` — 413 (MaxBytesReader cap)
- `TestImageUpload_RejectsOversizedDimensions` — 422 (image.DecodeConfig header-only check)
- `TestImageUpload_MimeSniffOverridesHeader` — JPEG bytes with .png filename stores as .jpg (T-05-05-02)
- `TestImageUpload_AuditAndDBRollback` — UNIQUE conflict cleans up disk file
- `TestMultiFloor` — list returns plans ordered by sort_order
- `TestReplaceKeepsPins` — PATCH replaces image, pin count unchanged, old file unlinked (D-24)

Migration tests also pass (version 34): `TestRunMigrations_Clean`, `TestRunMigrations_Idempotent`, `TestRunMigrations_FloorPlanCascadeFromSite`, `TestRunMigrations_PlacementFractionalBounds_Rejected`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Migration number conflict] Renumbered floor_plan migration 0031→0032**
- **Found during:** Task 1
- **Issue:** Plan specified migration 0031 for `floor_plan`, but plan 05-03 already created `0031_audit_vocab_phase5`. Sequential numbering was violated.
- **Fix:** Renumbered `floor_plan` → 0032, `device_floor_plan_placement` → 0033. Added new 0034 for floor plan audit vocab extension. Updated all migration test version assertions and step counts (+3 steps each for down-migration tests).
- **Files modified:** All migration files, `migrations_test.go`, `roundtrip_test.go`
- **Commits:** 68fccda, ceffcc4

**2. [Rule 1 - Bug] auth.User.ID is string, not uuid.UUID**
- **Found during:** Task 2
- **Issue:** `audit.Entry.UserID` requires `uuid.UUID` but `auth.User.ID` is `string`. Direct assignment would not compile.
- **Fix:** Added `userUUID(id string) uuid.UUID` helper that calls `uuid.Parse()` and returns `uuid.Nil` on failure.
- **Files modified:** `internal/floorplan/handlers.go`
- **Commits:** ceffcc4

**3. [Rule 2 - Missing critical] Added 0034 audit vocab migration for floor_plan actions**
- **Found during:** Task 2
- **Issue:** The `audit_log` table enforces CHECK constraints on `action` and `entity_type` columns. Without extending the vocabulary, `audit.WriteEntry` for floor plan operations would fail with a CHECK violation at runtime.
- **Fix:** Created `0034_audit_vocab_floor_plan.up.sql` using the DROP+re-ADD constraint pattern from prior migrations.
- **Files modified:** New migration files, `migrations_test.go`
- **Commits:** ceffcc4

## Known Stubs

None — all handlers wire to real DB queries; no placeholder data flows to any response.

## Threat Flags

No new threat surface beyond what the plan's threat model covered (T-05-05-01, T-05-05-02, T-05-05-03 all mitigated).

## Self-Check: PASSED

Files verified present:
- `/Users/suraboonsung/Documents/Programming/shifter/internal/floorplan/handlers.go` — FOUND
- `/Users/suraboonsung/Documents/Programming/shifter/internal/floorplan/image.go` — FOUND
- `/Users/suraboonsung/Documents/Programming/shifter/internal/db/migrations/0032_floor_plan.up.sql` — FOUND
- `/Users/suraboonsung/Documents/Programming/shifter/internal/db/migrations/0033_device_floor_plan_placement.up.sql` — FOUND
- `/Users/suraboonsung/Documents/Programming/shifter/internal/db/migrations/0034_audit_vocab_floor_plan.up.sql` — FOUND

Commits verified:
- `68fccda` — floor_plan + device_floor_plan_placement migrations + sqlc
- `ceffcc4` — floor plan package, audit vocab, compose volumes, 8 integration tests
