---
phase: quick
plan: 260513-nqo
subsystem: [backend/http, frontend/routes, frontend/components]
tags: [bugfix, uat-regression, mobile, routing]
dependency_graph:
  requires: []
  provides: [GET /api/settings/chirpstack 404, /account route, /metering-points list, mobile-safe dashboard]
  affects: [settings page, account page, metering-points list, dashboard mobile layout]
tech_stack:
  added: []
  patterns: [pgx.ErrNoRows sentinel check, React Router lazy route, TanStack Table list page, Tailwind responsive flex]
key_files:
  created:
    - web/src/routes/account.tsx
    - web/src/routes/metering-points/index.tsx
  modified:
    - internal/http/testconn.go
    - internal/http/testconn_test.go
    - web/src/App.tsx
    - web/src/components/dashboard/DateRangePicker.tsx
    - web/src/routes/dashboard.tsx
    - web/playwright/specs/dashboard-mobile.spec.ts
decisions:
  - Used listMPs (actual export name) instead of listMeteringPoints (plan used incorrect name)
  - Docker image rebuild required for Playwright verification since container embeds Go binary with SPA
metrics:
  duration: ~35min
  completed: 2026-05-13
  tasks_completed: 4
  files_modified: 8
---

# Quick Plan 260513-nqo: Fix 4 Major UAT Issues Summary

Fix 4 UAT regressions: chirpstack 500→404 on empty table, /account ErrorBoundary, /metering-points missing route, and dashboard 375px horizontal overflow.

## Tasks Completed

| Task | Description | Commit |
|------|-------------|--------|
| 1 | Fix GET /api/settings/chirpstack 500→404 on empty chirpstack_connection | d8fbb31 |
| 2 | Add /account route with user info and change-password dialog | 7d8b9f2 |
| 3 | Add /metering-points list route (TanStack Table, 4 columns) | c4abad8 |
| 4 | Fix dashboard horizontal overflow at 375px viewport | 05de8eb |

## Deviations from Plan

**1. [Rule 1 - Bug] Import name mismatch: `listMeteringPoints` vs `listMPs`**
- Found during: Task 3
- Issue: Plan referenced `listMeteringPoints()` but the actual export in `@/lib/metering-points` is `listMPs`
- Fix: Used `listMPs` in the new index.tsx
- Files modified: web/src/routes/metering-points/index.tsx
- Commit: c4abad8

**2. [Rule 3 - Blocking] Docker container rebuild required for Playwright verification**
- Found during: Tasks 2, 3, 4
- Issue: The shifter container serves a pre-built image (`shifter:0.1.0`) with embedded SPA; Playwright tests run against localhost:8080. After frontend changes, the Go binary must be rebuilt with `go:embed` to pick up the new `web/dist`. The `--build` flag on `docker compose up` was a no-op because the service uses a pre-built image tag, not a build context.
- Fix: Used `docker build -t shifter:0.1.0 .` to rebuild the full image (web-builder → go-builder → runtime stages), then `docker compose up -d shifter` to restart
- No code changes required

## Verification Results

- `go test ./internal/http/... -race -count=1`: 32 passed (includes new 404 test)
- `pnpm --dir web build`: clean, no TypeScript errors
- `playwright test dashboard-mobile.spec.ts --project=chromium`: 2/2 pass, scrollWidth <= clientWidth at 375px
- `playwright test metering-point-detail.spec.ts --project=chromium`: 2/2 pass
- `playwright test audit-smoke-uncovered-routes.spec.ts --project=chromium`: 9/9 pass

## Known Stubs

None — all new pages render real data from existing API endpoints.

## Self-Check: PASSED

- `internal/http/testconn.go`: modified, pgx.ErrNoRows check present
- `internal/http/testconn_test.go`: new test TestSettings_GetChirpStack_EmptyTable_Returns404 present
- `web/src/routes/account.tsx`: created
- `web/src/routes/metering-points/index.tsx`: created
- `web/src/App.tsx`: account + metering-points routes added
- `web/src/components/dashboard/DateRangePicker.tsx`: flex-wrap added
- `web/src/routes/dashboard.tsx`: flex-col responsive classes added
- `web/playwright/specs/dashboard-mobile.spec.ts`: seedFixture removed
