---
phase: 07-multi-vendor-breadth-v1-x-differentiators
plan: 07-15-wire-phase-7-deps
subsystem: cli/serve + http/router
tags: [gap-closure, wiring, phase-7, catalog, codec-test, backtest, report-templates, compare, gateway-import]
dependency_graph:
  requires: [07-04, 07-07, 07-10, 07-11a, 07-12, 07-13]
  provides: [live-phase-7-endpoints]
  affects: [internal/cli/serve.go]
tech_stack:
  added: []
  patterns: [deps-wiring, nil-guard-pattern]
key_files:
  created: []
  modified:
    - internal/cli/serve.go
decisions:
  - "Used apipkg alias (matching router.go convention) for internal/api import"
  - "BacktestDeps only needs Pool — no SessionMgr required (RBAC at router layer)"
  - "GatewayImportDeps.ImportSvc populated with gateway.NewImportService(pool) inline"
metrics:
  duration: "~15min"
  completed: "2026-05-13"
  tasks_completed: 3
  files_modified: 1
---

# Phase 07 Plan 15: Wire Phase 7 Deps into serve.go — Summary

**One-liner:** Added `apipkg` import + 6 deps fields to `httpapi.NewRouter(httpapi.Deps{})` in `serve.go`, restoring all Phase 7 HTTP endpoints that had been silently falling through to SPA fallback.

## What Was Done

All 3 tasks completed successfully:

**Task 1 — Code change (commit 320122a):**
- Added `apipkg "github.com/shifter-io/shifter/internal/api"` import to `internal/cli/serve.go`
- Wired 6 deps fields into the `httpapi.NewRouter(httpapi.Deps{...})` literal after the `AlertTestFireDeps` block:
  - `CatalogDeps`: `{Pool, SessionMgr}`
  - `CodecTestDeps`: `{Pool, SessionMgr, Log("codec_test")}`
  - `BacktestDeps`: `{Pool}`
  - `ReportTemplateDeps`: `{Pool, SessionMgr}`
  - `CompareDeps`: `{Pool, SessionMgr}`
  - `GatewayImportDeps`: `{Pool, SessionMgr, Log("gateway_import"), ImportSvc: gateway.NewImportService(pool)}`
- `go build ./...` — exit 0
- `go test ./internal/cli/... ./internal/api/... -short -count=1` — 25 passed

**Task 2 — Docker rebuild + restart (no commit):**
- `just _compose-prep-secrets` + `just _compose-build-image` rebuilt `shifter:0.1.0` with commit `320122a` baked in
- `docker compose -f bundled.yml up -d --force-recreate --no-deps shifter` restarted container
- `/health` confirmed healthy: `{"status":"ok","commit":"320122a",...}`

**Task 3 — Endpoint verification (no commit):**
All 6 Phase 7 endpoints confirmed returning `401 application/json` (not `200 text/html`):

| Endpoint | Method | Before | After |
|----------|--------|--------|-------|
| `/api/catalog` | GET | 200 text/html | 401 application/json |
| `/api/device-profiles/{id}/test-codec` | POST | 200 text/html | 401 application/json |
| `/api/alerts/backtest` | POST | 200 text/html | 401 application/json |
| `/api/reports/templates` | GET | 200 text/html | 401 application/json |
| `/api/reports/compare` | POST | 200 text/html | 401 application/json |
| `/api/gateways/bulk-import/validate` | POST | 200 text/html | 401 application/json |

`07-HUMAN-UAT.md` gap entry updated: `status: verified`, `verified: 2026-05-13`, `fix_commit: 320122a`.

## Root Cause Recap

The `Deps` struct in `internal/http/router.go` (lines 197–236) declares 6 Phase 7 fields with nil-guards. When nil, `RegisterCatalogRoutes` / `RegisterCodecTestRoute` / etc. are never called, so those routes are never mounted. Requests fall through to the SPA fallback handler (`httpapi.SPAHandler()`) which returns `200 text/html` for any unmatched path.

`internal/cli/serve.go` never populated those 6 fields — they were added in Plans 07-04 through 07-13 but the corresponding serve.go wiring was omitted in each plan's scope. Unit tests didn't catch it because plan-level handler tests construct `Deps` directly without calling `NewRouter`.

## Deviations from Plan

None — plan executed exactly as written. The plan's code blocks were accurate; the struct field shapes matched exactly.

## Commits

| Task | Commit | Description |
|------|--------|-------------|
| 1 | 320122a | fix(07-15): wire 6 Phase 7 deps into serve.go |
| 2 | — | Docker rebuild + restart (no code change) |
| 3 | — | Endpoint probe verification (no code change) |

## Known Stubs

None — this plan wires existing handlers to production; no new stub surfaces.

## Threat Flags

None — no new network endpoints introduced; existing endpoints now correctly authenticate (returning 401 instead of leaking HTML via SPA fallback is a security improvement, not a new surface).

## Self-Check

- serve.go: FOUND
- 07-15-SUMMARY.md: FOUND
- commit 320122a: FOUND
- CatalogDeps in serve.go: FOUND
- CodecTestDeps in serve.go: FOUND
- BacktestDeps in serve.go: FOUND
- ReportTemplateDeps in serve.go: FOUND
- CompareDeps in serve.go: FOUND
- GatewayImportDeps in serve.go: FOUND
- gateway.NewImportService in serve.go: FOUND

## Self-Check: PASSED
