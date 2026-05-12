---
phase: 07-multi-vendor-breadth-v1-x-differentiators
plan: "04"
subsystem: catalog-http-api
tags: [catalog, http, rbac, audit, migration]
dependency_graph:
  requires: [07-02, 07-03]
  provides: [GET /api/catalog, GET /api/catalog/{slug}, POST /api/catalog/import, POST /api/catalog/{profile_id}/update]
  affects: [07-05, 07-06]
tech_stack:
  added: []
  patterns:
    - "nil-guarded route mount in router.go (PITFALL #4 preserved)"
    - "D-34 allowlist for catalog field update (8 fields)"
    - "golang.org/x/mod/semver for downgrade rejection"
    - "atomic tx: CreateDeviceProfileFromCatalog + audit.WriteEntry in same pgx.Tx"
key_files:
  created:
    - internal/api/catalog_handler.go
    - internal/api/catalog_handler_test.go
    - internal/db/migrations/0051_audit_vocab_catalog.up.sql
    - internal/db/migrations/0051_audit_vocab_catalog.down.sql
  modified:
    - internal/auth/authz.go
    - internal/auth/authz_test.go
    - internal/audit/log.go
    - internal/http/router.go
    - internal/db/migrations_test.go
    - internal/db/roundtrip_test.go
decisions:
  - "Use GetDeviceProfile (not GetProfileCatalogMetadataRow) to fetch MacVersion/CounterModulus/Region for update params — the catalog metadata row intentionally omits those fields"
  - "Viewer gets ActionCatalogRead only; Import/Update are admin-only matching the plan's RBAC matrix"
  - "D-34 allowlist hard-coded as a package-level map[string]bool — 8 fields: display_name, manufacturer, model, codec_js, mac_version, reg_params_revision, counter_modulus, region"
  - "Downgrade rejection uses golang.org/x/mod/semver.Compare; empty/non-semver versions treated as no-semver-guard (pass through)"
metrics:
  duration_minutes: 20
  completed_date: "2026-05-13"
  tasks_completed: 3
  tasks_total: 3
  files_changed: 10
---

# Phase 07 Plan 04: Catalog HTTP API Summary

**One-liner:** Four RBAC-gated catalog endpoints (list/get/import/update) with atomic audit writes, D-34 allowlist validation, and semver downgrade rejection, wired into the chi router via nil-guard mount.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Authz actions + audit vocab + migration 0051 | a1bbe5e | authz.go, log.go, 0051*.sql, migrations_test.go, roundtrip_test.go |
| 2 RED | Failing catalog handler tests | 58c50f0 | catalog_handler_test.go |
| 2 GREEN | Implement catalog handlers | a424096 | catalog_handler.go |
| 3 GREEN | Wire catalog routes into router | 26612c2 | router.go |

## What Was Built

### Endpoints

- `GET /api/catalog` — joins `codec.LoadAll()` with `ListProfilesWithCatalogMetadata` to compute per-row status (`available` / `installed` / `update_available`). Returns JSON array.
- `GET /api/catalog/{slug}` — returns single catalog entry or 404.
- `POST /api/catalog/import` — 409 on duplicate slug; atomic tx: `CreateDeviceProfileFromCatalog` + `audit.WriteEntry(catalog.profile.imported)`.
- `POST /api/catalog/{profile_id}/update` — D-34 field allowlist (8 fields); rejects unknown fields HTTP 400; rejects semver downgrade HTTP 422; atomic tx: `ApplyCatalogUpdate` + `audit.WriteEntry(catalog.profile.updated)`.

### Auth Matrix

| Action | Admin | Viewer |
|--------|-------|--------|
| ActionCatalogRead | true | true |
| ActionCatalogImport | true | false |
| ActionCatalogUpdate | true | false |

### Migration 0051

Extends `audit_log_action_valid` CHECK constraint with three new literals:
- `catalog.profile.imported`
- `catalog.profile.updated`
- `catalog.profile.codec_resynced`

Migration chain is now at version 51.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] GetProfileCatalogMetadataRow missing MacVersion/CounterModulus/Region**
- **Found during:** Task 2 GREEN (implementing ApplyCatalogUpdateHandler)
- **Issue:** `GetProfileCatalogMetadataRow` only has 13 fields; MacVersion, CounterModulus, Region are needed to build `ApplyCatalogUpdateParams` but are absent from the row type.
- **Fix:** Added secondary `q.GetDeviceProfile(r.Context(), pgID)` call to retrieve the full profile and source those three fields from `fullProfile` when building update params.
- **Files modified:** internal/api/catalog_handler.go
- **Commit:** a424096

## Verification Results

- `go test ./internal/api/... ./internal/auth/... ./internal/http/...` — 109 tests pass (short)
- `go test -run 'TestListCatalog|TestImportFrom|TestApplyCatalog|TestGetCatalog|TestViewerCatalog' ./internal/api/` — 14 integration tests pass (testcontainers)
- `go build ./...` — clean

## Known Stubs

None — all 4 endpoints return live data from the database or the embedded catalog JSON.

## Self-Check: PASSED

- internal/api/catalog_handler.go: FOUND
- internal/api/catalog_handler_test.go: FOUND
- internal/db/migrations/0051_audit_vocab_catalog.up.sql: FOUND
- internal/db/migrations/0051_audit_vocab_catalog.down.sql: FOUND
- Commits a1bbe5e, 58c50f0, a424096, 26612c2: all present in git log
