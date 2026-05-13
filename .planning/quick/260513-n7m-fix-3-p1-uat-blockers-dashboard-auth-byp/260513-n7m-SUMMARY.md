---
phase: quick
plan: 260513-n7m
subsystem: auth, router, frontend
tags: [bugfix, security, auth, rate-limit, swap-meter, playwright]
requirements: [AUTH-04]

dependency_graph:
  requires: []
  provides: [dashboard-auth-gate, login-rate-limit-fix, swap-meter-cta]
  affects: [internal/http/router.go, internal/auth/handlers.go, web/src/routes/metering-points/$id.tsx]

tech_stack:
  added: []
  patterns:
    - RequireAction group wrapping for dashboard route auth gate
    - Admin-only CTA pattern via useCurrentUser + isAdmin gate

key_files:
  created:
    - web/playwright/specs/swap-meter-cta.spec.ts
  modified:
    - internal/http/router.go
    - internal/http/rbac_test.go
    - internal/auth/handlers.go
    - internal/auth/handlers_test.go
    - web/src/routes/metering-points/$id.tsx

decisions:
  - Used ActionSiteRead (held by both admin and viewer) as the RequireAction guard for dashboard routes — satisfies "any authenticated user" without a weaker no-op
  - SwapMeterDialog mounted after TooltipProvider to avoid z-index conflicts with tooltip overlay

metrics:
  duration: ~25 minutes
  completed: 2026-05-13
  tasks_completed: 3
  files_changed: 5
---

# Quick Task 260513-n7m: Fix 3 P1 UAT Blockers Summary

**One-liner:** Dashboard routes gated with RequireAction(ActionSiteRead), login rate-limit double-decrement removed (5-attempt burst restored), and SwapMeterDialog CTA wired admin-only into MP detail page header.

## Tasks Completed

| # | Task | Commit | Files |
|---|------|--------|-------|
| 1 | Wrap dashboard routes in RequireAction auth group | `5835452` | internal/http/router.go, internal/http/rbac_test.go |
| 2 | Remove duplicate LoginLimiter.Allow() — fix double-decrement | `fe68a1b` | internal/auth/handlers.go, internal/auth/handlers_test.go |
| 3 | Wire SwapMeterDialog CTA into MP detail page header | `c7c6f97` | web/src/routes/metering-points/$id.tsx, web/playwright/specs/swap-meter-cta.spec.ts |

## What Was Done

### Task 1 — Dashboard Auth Bypass (T-04-04-05)

`dashboard.RegisterRoutes(r, ...)` was called on the root chi router `r`, bypassing all session middleware. Wrapped it in `r.Group(func(rt chi.Router) { rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionSiteRead)); dashboard.RegisterRoutes(rt, ...) })`. Added `TestRouter_DashboardRequiresAuth` covering all three dashboard endpoints (`/scope`, `/snapshot`, `/timeseries`) — each must return 401 for anonymous requests.

Live verification: `curl http://localhost:8080/api/dashboard/scope` now returns `401` (required Docker image rebuild + `--force-recreate` to pick up the new binary).

### Task 2 — Login Rate-Limit Double-Decrement (AUTH-04)

The unknown-email branch called `deps.LoginLimiter.Allow(ip, email)` a second time at line ~155, after the first call at line ~109 already consumed a token. The comment incorrectly described it as a "no-op counter bump path" — `Allow()` always consumes a token. Deleted the duplicate call. Added `TestLogin_RateLimit_UnknownEmail` asserting exactly 5 unknown-email attempts return 401 and the 6th returns 429.

### Task 3 — SwapMeterDialog Not Mounted

Added `Button`, `useCurrentUser`, and `SwapMeterDialog` imports to `$id.tsx`. Added `swapOpen` state and `isAdmin` gate. Updated the `<header>` to a flex row with the "Swap Meter" button on the right (admin-only). Mounted `<SwapMeterDialog>` after `</TooltipProvider>` with proper props. Created Playwright smoke spec that discovers the first MP via API, navigates to its detail page, asserts the button is visible, clicks it, and asserts `role="dialog"` appears.

## Deviations from Plan

None — plan executed exactly as written. The Docker image rebuild step was an implicit requirement not stated in the plan (the live `curl` verify required the new binary), handled transparently.

## Verification Results

| Check | Result |
|-------|--------|
| `go test ./internal/http/... -race -count=1 -run TestRouter_DashboardRequiresAuth` | PASS |
| `go test ./internal/auth/... -race -count=1 -run TestLogin_RateLimit` | PASS (both tests) |
| `pnpm exec playwright test swap-meter-cta.spec.ts --project=chromium` | PASS (1/1) |
| `curl http://localhost:8080/api/dashboard/scope` | 401 |
| `go test ./internal/http/... -race -count=1 -short` | 25 PASS |
| `go test ./internal/auth/... -race -count=1 -short` | PASS |

## Known Stubs

None.

## Threat Flags

None — no new network endpoints or auth paths introduced. The dashboard auth gate is a security fix (closing an existing bypass), not new surface.

## Self-Check: PASSED

- `5835452` exists: confirmed via `git log`
- `fe68a1b` exists: confirmed via `git log`
- `c7c6f97` exists: confirmed via `git log`
- `web/playwright/specs/swap-meter-cta.spec.ts` exists: created by Write tool
- All three verification commands passed
