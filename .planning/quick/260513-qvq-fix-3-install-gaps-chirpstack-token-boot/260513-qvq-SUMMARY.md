---
phase: 260513-qvq
plan: 01
subsystem: install
tags: [install, chirpstack, wizard, auth, sequential]
dependency_graph:
  requires: [260513-qcf]
  provides: [bootstrap-chirpstack-token, probe-version-two-step, wizard-step-order]
  affects: [internal/install, internal/chirpstack, install/bundled]
tech_stack:
  added: []
  patterns: [two-step-probe, sequential-guard]
key_files:
  created:
    - install/bundled/bootstrap-chirpstack-token.sh
  modified:
    - install/bundled/install.sh
    - internal/chirpstack/version.go
    - internal/chirpstack/errors.go
    - internal/install/handlers.go
    - internal/install/handlers_test.go
    - internal/testsupport/chirpstack_mock.go
decisions:
  - Two-step ProbeVersion separates GetVersion (unauthenticated) from TenantService.List (auth-required) to give distinct error codes for v3 vs bad-token
  - 409 step_out_of_order guard uses GREATEST-monotonic CurrentStep — equality check (==N) is correct since step N can only be re-submitted at step N
metrics:
  duration: ~20min
  completed: 2026-05-13
  tasks_completed: 3
  tasks_total: 3
  files_changed: 7
---

# Phase 260513-qvq Plan 01: Fix 3 Install Gaps (ChirpStack Token Bootstrap) Summary

Closed three install rehearsal gaps: ChirpStack API token never bootstrapped (placeholder stayed in secrets), ProbeVersion step 2 failing with Unauthenticated on valid tokens due to missing TenantService validation, and wizard accepting out-of-order step POSTs.

## Tasks Completed

| Task | Name | Commit | Key Files |
|------|------|--------|-----------|
| 1 | ChirpStack API token bootstrap script + install.sh hook | adf056f | install/bundled/bootstrap-chirpstack-token.sh, install/bundled/install.sh |
| 2 | Fix ProbeVersion auth — two-step validate+version approach | 5a4c0c5 | internal/chirpstack/version.go, internal/chirpstack/errors.go, internal/install/handlers.go, internal/testsupport/chirpstack_mock.go |
| 3 | Sequential step enforcement — 409 guard in Step2/3/4Handler + unit test | 70cb82a | internal/install/handlers.go, internal/install/handlers_test.go |

## What Was Built

**Task 1 — bootstrap-chirpstack-token.sh (bug #4)**

New script `install/bundled/bootstrap-chirpstack-token.sh` waits up to 30s for the chirpstack container CLI to be responsive, calls `create-api-key --name shifter-bootstrap`, extracts the JWT with grep -oP, writes it to `secrets/chirpstack_api_token.txt` (mode 0600), then restarts `compose-shifter-1` so the new secret is picked up. Non-fatal — exits 0 with WARNING if anything fails. Hooked into `install/bundled/install.sh` immediately after the health-poll `done`, before the final echo.

**Task 2 — two-step ProbeVersion (bug #5)**

`ProbeVersion` now performs two sequential RPCs:
1. `InternalService.GetVersion` — unauthenticated (v4 serves this without a token; v3 returns `Unimplemented` → `ErrChirpStackV3OrUnknown`)
2. `TenantService.List(limit=1)` — auth-required; `Unauthenticated` or `PermissionDenied` → new `ErrInvalidAPIToken` sentinel

`Step2Handler` maps `ErrInvalidAPIToken` → 422 `grpc_unreachable` `detail=invalid_api_token`. The testsupport v4 mock now registers `TenantServiceServer` (using the existing `fakeTenant`) so the new step 2 passes in unit tests.

**Task 3 — sequential step guard (bug #6)**

`Step2Handler`, `Step3Handler`, `Step4Handler` each load state and check `CurrentStep == N` before processing the request body. Out-of-order POST → 409 `step_out_of_order` with a detail message. Four existing tests (`TestStep3_PersistsRegionHandler`, `TestStep3_UnknownRegion`, `TestStep4_PersistsIdentityHandler`, `TestStep4_InvalidTimezone`) were updated to call prior steps in order. New test `TestStep_OutOfOrder_Returns409` verifies: step 3 at step-2 state → 409, then step 2 succeeds, then step 3 succeeds.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed 4 existing tests broken by the new sequential guard**
- **Found during:** Task 3 — after adding the 409 guard, `go test` revealed 4 tests posting to step 3/4 without satisfying preconditions
- **Issue:** `TestStep3_PersistsRegionHandler`, `TestStep3_UnknownRegion`, `TestStep4_PersistsIdentityHandler`, `TestStep4_InvalidTimezone` all POSTed directly to step 3 or 4 without running prior steps, so they received 409 instead of their expected 200/422
- **Fix:** Each test now calls prior steps in sequence before the step under test
- **Files modified:** `internal/install/handlers_test.go`
- **Commit:** 70cb82a (included in Task 3 commit)

## Known Pre-existing Failure (out of scope)

`TestState_ReturnsSingleton` in `handlers_test.go:90` checks `body["CurrentStep"]` (PascalCase) but the JSON response field is `current_step` (snake_case), so the assertion gets `nil`. This failure existed before this task and was not introduced by any change here. Logged to deferred-items rather than fixed (would be a one-line test fix: change `"CurrentStep"` to `"current_step"`).

## Verification Results

1. `bash -n install/bundled/bootstrap-chirpstack-token.sh` — PASS
2. `install/bundled/install.sh` contains `./install/bundled/bootstrap-chirpstack-token.sh || true` — PASS
3. `go build ./...` — PASS
4. `go test ./internal/chirpstack/... ./internal/install/... -count=1` — 86 passed, 1 pre-existing failure (`TestState_ReturnsSingleton`)
5. `TestStep_OutOfOrder_Returns409` — PASS

## Investigation Notes (Task 2)

ChirpStack's gRPC port (8080) is not exposed to the host in the compose stack — only accessible inside the Docker network. Direct host-side probing was not possible. The two-step design is grounded in: (a) ChirpStack v4's documented behavior that `GetVersion` is unauthenticated (used by the web UI before login), (b) the existing mock already having `fakeTenant.List` that returns empty without auth — matching the expected real behavior when a valid global API key is presented, (c) the root cause of bug #5 being the placeholder token (`"placeholder-set-after-chirpstack-boots"`) causing `Unauthenticated` on auth-required RPCs, now caught explicitly by step 2 of the probe.

## Self-Check: PASSED

Files created/modified exist:
- install/bundled/bootstrap-chirpstack-token.sh: FOUND
- install/bundled/install.sh: FOUND (contains bootstrap hook)
- internal/chirpstack/version.go: FOUND (two-step probe)
- internal/chirpstack/errors.go: FOUND (ErrInvalidAPIToken)
- internal/install/handlers.go: FOUND (409 guards + ErrInvalidAPIToken mapping)
- internal/install/handlers_test.go: FOUND (4 fixed tests + new TestStep_OutOfOrder_Returns409)
- internal/testsupport/chirpstack_mock.go: FOUND (TenantServiceServer registered for v4)

Commits:
- adf056f: Task 1 — bootstrap script + install.sh hook
- 5a4c0c5: Task 2 — two-step ProbeVersion + ErrInvalidAPIToken
- 70cb82a: Task 3 — sequential step guards + tests
