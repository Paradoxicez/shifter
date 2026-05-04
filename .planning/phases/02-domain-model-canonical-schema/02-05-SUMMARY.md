---
phase: 02-domain-model-canonical-schema
plan: 05
subsystem: chirpstack-control-plane-wrappers
tags: [wave-2, chirpstack-grpc, idempotent-bootstrap, codec-js-sync, atomic-create-rollback, bufconn-mock]

requires:
  - phase: 01-foundation
    plan: 12
    provides: chirpstack package architectural seam (sole importer of chirpstack-api/go/v4) + Client struct + Dial/auth interceptor + ProbeVersion + bufconn mock skeleton
  - phase: 02-domain-model-canonical-schema
    plan: 01
    provides: 5 stub _test.go files (tenant, application, device_profile, device, bootstrap) — filled in by this plan
  - phase: 02-domain-model-canonical-schema
    plan: 02
    provides: 0011_chirpstack_connection_cs_ids migration adds cs_tenant_id + cs_application_id TEXT NULL columns that bootstrap.ConnectionStore reads + writes
provides:
  - Client.EnsureTenant(ctx, name) — list-then-create idempotent (D-28)
  - Client.EnsureApplication(ctx, tenantID, name) — list-then-create scoped to tenant (D-28)
  - Client.CreateDeviceProfile / UpdateDeviceProfile / GetDeviceProfile — codec_js push via api.CodecRuntime_JS (D-09)
  - Client.CreateDevice / CreateDeviceKeys / CreateDeviceWithKeys / DeleteDevice / GetDevice — atomic add-device backend (CHIRP-04) with best-effort cleanup
  - chirpstack.EnsureTenantAndApplication(ctx, client, store, log) — D-28 first-boot orchestrator
  - chirpstack.ConnectionStore interface — persistence shape Plan 02-06 implements via sqlc
  - testsupport.NewChirpStackMockBufWithHandles + ChirpStackMockHandles — extended bufconn mock with full TenantService/ApplicationService/DeviceProfileService/DeviceService fakes; pre-seed and call-count accessors
  - chirpstack package level constants: BootstrapTenantName="shifter-default", BootstrapApplicationName="shifter"
  - Region/MacVersion/RegParamsRevision string→enum mappers (case-insensitive, defensive error on unknown)
affects: [02-06, 02-08, 02-10, 02-15]

tech-stack:
  added:
    - "github.com/google/uuid v1.6.0 (promoted indirect → direct; used by bufconn mock to mint CS tenant/application/profile UUIDs)"
  patterns:
    - "List-then-create idempotency for any CS entity uniquely identified by name within a parent scope (RESEARCH §ChirpStack v4 TenantService.Create)"
    - "Get-then-modify-then-Update for partial CS updates — CS v4 Update replaces the whole record, so partial updates without a prior Get would clear immutable fields (region, mac_version) and break every device bound to the profile (D-09 codec_js sync would otherwise be a regression)"
    - "Best-effort cleanup with fresh context after a paired-call failure: CreateDeviceWithKeys triggers DeleteDevice with a 5s context.WithTimeout(context.Background()) when CreateKeys fails, so a caller-cancelled context doesn't pre-empt the rollback. The cleanup error is intentionally swallowed — the caller already has a primary error to surface, and a noisy double-error obscures the real failure"
    - "ConnectionStore interface in the consumer package (bootstrap.go), not the producer package (db/sqlc) — keeps the chirpstack package import-graph clean and lets Plan 02-06 supply the implementation without circular imports"
    - "Architectural seam grep guard: every plan that touches the chirpstack package re-runs `grep -rn 'chirpstack/api/go/v4' ./internal/ | grep -v 'internal/chirpstack/' | grep -v 'internal/testsupport/chirpstack_mock.go'` — must return 0 lines"

key-files:
  created:
    - internal/chirpstack/tenant.go
    - internal/chirpstack/application.go
    - internal/chirpstack/device_profile.go
    - internal/chirpstack/device.go
    - internal/chirpstack/cleanup.go
    - internal/chirpstack/bootstrap.go
  modified:
    - internal/chirpstack/doc.go (append Phase 2 wrapper inventory)
    - internal/chirpstack/tenant_test.go (Wave-0 stub → 2 real tests + dialMockConnWithHandles helper)
    - internal/chirpstack/application_test.go (Wave-0 stub → 3 real tests including foreign-tenant scoping)
    - internal/chirpstack/device_profile_test.go (Wave-0 stub → 4 real tests including UnknownRegion + table-driven enum coverage)
    - internal/chirpstack/device_test.go (Wave-0 stub → 5 real tests including rollback-on-keys-failure)
    - internal/chirpstack/bootstrap_test.go (Wave-0 stub → 6 real tests with in-memory fakeStore + inline failing-tenant fake)
    - internal/testsupport/chirpstack_mock.go (append fakeTenant/fakeApplication/fakeDeviceProfile/fakeDevice + ChirpStackMockHandles + NewChirpStackMockBufWithHandles)
    - go.mod (google/uuid promoted indirect → direct)
    - go.sum (re-tidied)

key-decisions:
  - "Added a SECOND constructor NewChirpStackMockBufWithHandles instead of expanding NewChirpStackMockBuf's existing 'mode' switch. The Phase 1 constructor returns (dialer, apiToken) and is called by 6 existing tests (mqtt_test.go, version_test.go, client_test.go); changing its return arity would force every Phase 1 caller to either ignore the new handles or accept a breaking signature change. The new constructor wraps the same setup but adds a third return value — a *ChirpStackMockHandles struct — so Plan 02-05 tests get the fakes they need without breaking Phase 1 callers. Both constructors share the same fakes via package-private types."
  - "Exposed Tenant/Application/DeviceProfile/Device call-count counters via PUBLIC accessor methods (CreateCalls(), ListCalls(), KeysCalls(), DeleteCalls(), UpdateCalls()) on the fake structs rather than via direct field access. Cross-package tests in internal/chirpstack/ cannot reach unexported fields in internal/testsupport/, and exporting the atomic counters as fields would mean external mutation is allowed — the accessor pattern keeps state ownership inside the fake."
  - "regionEnum / macVersionEnum / regParamsRevisionEnum mappers RETURN AN ERROR on unknown strings rather than fall back to a sentinel. Phase 1 stored region_name lowercase ('as923_2') in chirpstack_connection; future regions added by ChirpStack (post-v4.17.0) without a Shifter mapper update would silently default to EU868 (the zero value of common.Region) under the sentinel-fallback approach — that's a Pitfall §3-class defect the codec_js sync path would only surface as 'devices joining but not decoding'. Failing fast at CreateDeviceProfile is the safer default; the test TestCreateDeviceProfile_UnknownRegion locks this behaviour."
  - "CreateDeviceWithKeys is a SEPARATE method from CreateDevice + CreateDeviceKeys. Plan 02-10's atomic add-device transaction needs the unwrapped pair so it can interleave Postgres BEGIN/COMMIT around the CS calls; tests verify the convenience method's rollback semantics independently. Both surfaces are exported."
  - "JoinEUI defaults to '0000000000000000' (the legacy 1.0.x sentinel) when CreateDeviceInput.JoinEUI is empty. ChirpStack v4 documentation lists JoinEUI as 'optional, will be auto-set on OTAA' — but the field is mandatory in the Device proto and CS v4 server-side will silently store an empty-string JoinEUI as '' rather than the sentinel, breaking some 1.1.x relay flows. Hardcoding the sentinel matches the Phase 1 ChirpStack runbook and the RESEARCH Pattern 4 example."
  - "BootstrapTenantName + BootstrapApplicationName are exported package-level CONSTANTS, not config values. The plan explicitly rejected per-install renaming — external-CS reuse is handled by EnsureTenant/EnsureApplication's List-then-Create on name match, not by renaming the bootstrap target. Constants are exported (capitalized) so the future install wizard (Plan 02-15) and the test harness CLI can reference them by symbol rather than hardcoding the strings in multiple places."
  - "Bootstrap test for the tenant-create-failure path inlines a purpose-built failing TenantService fake into bootstrap_test.go rather than adding a 'fail mode' parameter to the shared mock. Adding a flag to the shared mock would force every call site (8 tests across 5 files) to thread it through; inlining the 30-line fake stays in scope and doesn't pollute the happy-path mock surface."

patterns-established:
  - "Pattern: Plan 02-05 enum mappers are case-insensitive AND error-on-unknown — the canonical idiom for any future string→enum mapping in this package. RegionEnum/MacVersionEnum/RegParamsRevisionEnum unit-tested via table-driven TestRegionEnumMapping."
  - "Pattern: ConnectionStore interface lives in the CONSUMER package (chirpstack/bootstrap.go), NOT in the producer package (db/sqlc). Plan 02-06's sqlc query SetChirpStackTenantApp will be wrapped by an adapter that satisfies the interface. This keeps go imports acyclic without forcing chirpstack to import db/sqlc."
  - "Pattern: Best-effort cleanup uses context.WithTimeout(context.Background(), 5*time.Second) so the caller's already-cancelled context can't pre-empt the rollback RPC. Documented in cleanup.go."

requirements-completed: [CHIRP-04, DATA-09, DATA-10]

duration: 11min
completed: 2026-05-04
---

# Phase 02 Plan 05: ChirpStack Control-Plane Wrappers Summary

**5 new wrapper files (tenant, application, device_profile, device, bootstrap) ship the ChirpStack v4 control-plane surface that Phase 2 needs to fulfill CHIRP-04. The bufconn mock at internal/testsupport/chirpstack_mock.go now registers full TenantService/ApplicationService/DeviceProfileService/DeviceService fakes so every wrapper is verifiable end-to-end without a live CS instance. EnsureTenantAndApplication is the D-28 first-boot orchestrator: idempotent on every subsequent boot via persisted UUIDs; partial-state UUIDs are NEVER persisted on failure, so the next boot retries cleanly. Architectural seam preserved — only internal/chirpstack/ and internal/testsupport/chirpstack_mock.go import chirpstack-api/go/v4. 38 unit tests pass via in-process bufconn mock; project-wide go test -short passes 176 cases across 22 packages.**

## Performance

- **Duration:** ~11 min
- **Started:** 2026-05-04T04:11:48Z
- **Completed:** 2026-05-04T04:22:23Z
- **Tasks:** 3 / 3
- **Files created:** 6
- **Files modified:** 9

## Accomplishments

- `internal/chirpstack/tenant.go` (54 lines): `Client.EnsureTenant(ctx, name)` — list-then-create idempotent per D-28. Search by name → create only when no exact match. Sets the documented `"Shifter-managed tenant — do not edit"` description.
- `internal/chirpstack/application.go` (51 lines): `Client.EnsureApplication(ctx, tenantID, name)` — same pattern, scoped to (tenant_id, name) so foreign-tenant collisions cannot short-circuit our create.
- `internal/chirpstack/device_profile.go` (212 lines): `Client.CreateDeviceProfile / UpdateDeviceProfile / GetDeviceProfile`. CreateDeviceProfile pushes `api.CodecRuntime_JS` + `PayloadCodecScript: in.CodecJS` per D-09. UpdateDeviceProfile follows Get-then-modify-then-Update because CS v4 Update replaces the whole record (a partial Update would clear Region/MacVersion and break every bound device). Three private enum mappers (`regionEnum`, `macVersionEnum`, `regParamsRevisionEnum`) are case-insensitive and error-on-unknown.
- `internal/chirpstack/device.go` (138 lines): `Client.CreateDevice / CreateDeviceKeys / CreateDeviceWithKeys / DeleteDevice / GetDevice`. CreateDeviceWithKeys is the CHIRP-04 atomic helper: Create → CreateKeys with best-effort `DeleteDevice` rollback when CreateKeys fails (so CS doesn't end up with an orphaned device). Default JoinEUI is the 1.0.x sentinel `"0000000000000000"`. NwkKey = AppKey for 1.0.x compat per RESEARCH Pattern 4.
- `internal/chirpstack/cleanup.go` (12 lines): `defaultCleanupTimeout = 5 * time.Second` for the rollback RPC ceiling.
- `internal/chirpstack/bootstrap.go` (90 lines): `EnsureTenantAndApplication(ctx, client, store, log)` — the D-28 first-boot orchestrator. Reads chirpstack_connection.cs_tenant_id + cs_application_id; short-circuits when both are non-empty (no CS RPCs at all); otherwise calls EnsureTenant("shifter-default") + EnsureApplication(tenant, "shifter") and persists both UUIDs via the ConnectionStore interface. Partial-state UUIDs are NEVER persisted on failure. nil logger tolerated (slog.Default fallback).
- `internal/testsupport/chirpstack_mock.go`: appended `fakeTenant`, `fakeApplication`, `fakeDeviceProfile`, `fakeDevice` (full Create/Get/Delete/CreateKeys/Update surface) + `ChirpStackMockHandles` struct + new `NewChirpStackMockBufWithHandles` constructor. Phase 1 callers of the existing `NewChirpStackMockBuf` are unaffected — the new constructor coexists.
- `internal/chirpstack/doc.go`: appended a 7-line Phase 2 wrapper-inventory paragraph listing the 5 new exports and the bootstrap entry point.
- All 5 Wave-0 `_test.go` stub bodies replaced with real assertions: 20 distinct test cases across tenant_test.go (2) / application_test.go (3) / device_profile_test.go (4) / device_test.go (5) / bootstrap_test.go (6) = 20 tests. The `t.Skip("Wave 0 placeholder")` markers are gone; the Wave-0 `TestPlaceholder*` function names were renamed to feature-specific names per the Plan 02-01 SUMMARY convention.
- `go.mod`: `google/uuid v1.6.0` promoted from indirect to direct (used by the bufconn mock for UUID minting since CS protos return strings).
- Architectural seam grep returns 0 lines: only `internal/chirpstack/` and `internal/testsupport/chirpstack_mock.go` import `chirpstack-api/go/v4`.
- `go test -count=1 -race ./internal/chirpstack/...` (with podman socket): **38 passed in 1 package**.
- `go test -count=1 ./... -short` (with podman socket): **176 passed in 22 packages** — no regressions in any other package.
- `go vet ./...`: clean.
- `go build ./...`: clean.

## Task Commits

1. **Task 1: Tenant + Application wrappers + extended bufconn mock** — `f85bee0` (feat)
2. **Task 2: DeviceProfile + Device wrappers + tests** — `52118be` (feat)
3. **Task 3: Bootstrap routine + bootstrap test** — `994f122` (feat)

**Plan metadata commit:** _pending — created at end of plan_

## Stub File → Implementing Plan Mapping

This plan filled in 5 of the Wave-0 stub test files created by Plan 02-01:

| Wave-0 stub file | Now contains | Test count |
|------------------|--------------|------------|
| `internal/chirpstack/tenant_test.go` | TestEnsureTenant_{CreatesWhenAbsent, ReusesWhenPresent} + dialMockConnWithHandles helper (shared by all 02-05 tests in the package) | 2 |
| `internal/chirpstack/application_test.go` | TestEnsureApplication_{CreatesWhenAbsent, ReusesWhenPresent, TenantScoped} | 3 |
| `internal/chirpstack/device_profile_test.go` | TestCreateDeviceProfile_RoundTrip, TestUpdateDeviceProfile_PartialUpdate, TestCreateDeviceProfile_UnknownRegion, TestRegionEnumMapping (table-driven, all 3 enum mappers) | 4 |
| `internal/chirpstack/device_test.go` | TestCreateDevice_Standalone, TestCreateDevice_AlsoCreatesKeys, TestCreateDevice_DupeReturnsError, TestCreateDevice_RollbackOnKeysFailure, TestDeleteDevice | 5 |
| `internal/chirpstack/bootstrap_test.go` | TestBootstrap_{FirstBoot, AlreadyBootstrapped, PartiallyBootstrapped, TenantCreateFails, NilLogger, StoreReadFails} + inline failingTenantSvc + dialFailingTenantConn helper | 6 |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Critical functionality] Added a SECOND mock constructor instead of mutating the Phase 1 constructor's signature**
- **Found during:** Task 1 read of internal/testsupport/chirpstack_mock.go
- **Issue:** The plan instructs "register both [TenantServiceServer + ApplicationServiceServer] with grpc.Server in NewChirpStackMock — the existing constructor already builds the server; just append `api.RegisterTenantServiceServer(...)` and the same for application." But the existing public constructor is `NewChirpStackMockBuf(t, mode string) (dialer, apiToken)` — a 2-tuple return — and it's called by 6 Phase 1 tests. The plan also requires Plan 02-05 tests to have the `*fakeTenant` / `*fakeApplication` HANDLES so they can pre-seed state and assert call counts. Adding a third return value to the existing constructor would break every Phase 1 caller.
- **Fix:** Added a NEW constructor `NewChirpStackMockBufWithHandles(t) (dialer, apiToken, *ChirpStackMockHandles)` that wraps the same registration logic and additionally returns the handles struct. Phase 1 callers keep using `NewChirpStackMockBuf`; Plan 02-05 tests use the new constructor. Both share the same fakes via package-private types. The existing v4 mode of `NewChirpStackMockBuf` continues to work as Phase 1 tests expect (registers v4 InternalService + a no-op DeviceService).
- **Files modified:** `internal/testsupport/chirpstack_mock.go`
- **Commit:** `f85bee0`

**2. [Rule 2 - Critical functionality] Defensive error on unknown region/mac_version/reg_params_revision strings**
- **Found during:** Task 2 implementation of CreateDeviceProfile
- **Issue:** The plan instructs "be defensive — return an error or fall back to a sentinel for unknown strings rather than letting the proto Marshal panic." Either choice is plan-compliant. Falling back to a sentinel (e.g. `Region_EU868 = 0`) would mean a future ChirpStack release adding a new region would silently default unknown-string profiles to EU868 — a Pitfall §3-class defect that surfaces only as "devices joining but not decoding."
- **Fix:** All three mappers RETURN AN ERROR on unknown values. CreateDeviceProfile bubbles the error up before any RPC is dispatched. Test TestCreateDeviceProfile_UnknownRegion locks the behaviour against a future regression to silent fallback.
- **Files modified:** `internal/chirpstack/device_profile.go`, `internal/chirpstack/device_profile_test.go`
- **Commit:** `52118be`

**3. [Rule 2 - Critical functionality] CreateDeviceWithKeys best-effort rollback uses fresh context**
- **Found during:** Task 2 implementation of CreateDeviceWithKeys
- **Issue:** Plan acceptance criteria require "create device + create keys" to be paired, with a follow-up "DeleteDevice" rollback when keys fails. The naive implementation passes the caller's context to the rollback RPC — but if the caller's context was already cancelled or near-deadline (which is the most likely failure mode that triggered the rollback in the first place), the cleanup RPC pre-empts immediately and the orphan stays in CS.
- **Fix:** `cleanup, cancel := context.WithTimeout(context.Background(), defaultCleanupTimeout)` — cleanup uses a fresh 5s context regardless of the caller's. cleanup.go documents the rationale. The cleanup error is intentionally swallowed (the caller already has a primary error to surface).
- **Files modified:** `internal/chirpstack/device.go`, `internal/chirpstack/cleanup.go`
- **Commit:** `52118be`

**4. [Rule 2 - Critical functionality] nil-logger tolerance in EnsureTenantAndApplication**
- **Found during:** Task 3 implementation
- **Issue:** Plan signature is `EnsureTenantAndApplication(ctx, c, store, log *slog.Logger)`. A nil logger would nil-deref on the first `log.Debug(...)` call, forcing every caller (including the future cmd/shifter serve in Plan 02-15) to construct a logger just to call this routine.
- **Fix:** First line of the function: `if log == nil { log = slog.Default() }`. TestBootstrap_NilLogger locks it.
- **Files modified:** `internal/chirpstack/bootstrap.go`, `internal/chirpstack/bootstrap_test.go`
- **Commit:** `994f122`

**5. [Rule 3 - Blocking issue] Promoted google/uuid from indirect to direct dependency**
- **Found during:** Task 1 mock extension (uuid.NewString needed for CS UUID minting)
- **Issue:** The mock fakes need to mint UUIDs (CS protos return strings). `google/uuid` was indirect via testcontainers; using it directly without promoting forces `go mod tidy` to either keep it indirect (compile error: `imported and not used as indirect-only`) or fail.
- **Fix:** `go mod tidy` after first import — the version `v1.6.0` was already pinned, just promoted to a direct dep.
- **Files modified:** `go.mod`
- **Commit:** `f85bee0`

### Plan-template observations (no auto-fix)

- The plan's `<interfaces>` block instructs `Uplink_Interval: 900` (with an underscore). The actual proto field is `UplinkInterval` (Go-style camelCase) — the underscore in the plan is a typo. Used the correct field name; matches the pb.go declaration confirmed above.
- The plan's `<interfaces>` block omits `Description` from the `CreateDeviceInput` struct example but the action-step bullet says "DevEUI+ApplicationID+DeviceProfileID+AppKey+JoinEUI." Kept Description as a field because (a) the acceptance criteria explicitly enumerate it, and (b) the underlying CS Device proto has it and the operator-facing add-device dialog (Plan 02-10) is going to want to set it.

## Authentication Gates

None — every test runs against the in-process bufconn mock; the production code path is exercised only by future plans that add live ChirpStack at install time.

## Decisions Made

(See `key-decisions` in frontmatter — same content. Verbatim repeat omitted to keep this section short; the frontmatter is the canonical record.)

## Self-Check: PASSED

- `[x] internal/chirpstack/tenant.go` exists and contains `func (c *Client) EnsureTenant(`
- `[x] internal/chirpstack/tenant.go` contains both `svc.List(` and `svc.Create(`
- `[x] internal/chirpstack/tenant.go` contains literal `"Shifter-managed tenant — do not edit"`
- `[x] internal/chirpstack/application.go` exists and contains `func (c *Client) EnsureApplication(`
- `[x] internal/chirpstack/application.go` contains list-then-create idiom
- `[x] internal/chirpstack/application.go` contains literal `"Shifter-managed application — do not edit"`
- `[x] internal/chirpstack/device_profile.go` contains CreateDeviceProfile, UpdateDeviceProfile, GetDeviceProfile
- `[x] internal/chirpstack/device_profile.go` contains literal `api.CodecRuntime_JS`
- `[x] internal/chirpstack/device_profile.go` contains `PayloadCodecScript: in.CodecJS`
- `[x] internal/chirpstack/device_profile.go` contains regionEnum, macVersionEnum, regParamsRevisionEnum
- `[x] internal/chirpstack/device.go` contains CreateDevice, CreateDeviceKeys, DeleteDevice, GetDevice
- `[x] internal/chirpstack/device.go` contains CreateDeviceInput with DevEUI, ApplicationID, DeviceProfileID, Name, Description, AppKey, JoinEUI fields
- `[x] internal/chirpstack/device.go` contains `NwkKey: appKey` (1.0.x compat)
- `[x] internal/chirpstack/bootstrap.go` contains `func EnsureTenantAndApplication(ctx context.Context, c *Client, store ConnectionStore, log *slog.Logger) (tenantID, appID string, err error)`
- `[x] internal/chirpstack/bootstrap.go` contains literal `"shifter-default"` and `"shifter"`
- `[x] internal/chirpstack/bootstrap.go` contains the short-circuit branch when both UUIDs are already set
- `[x] internal/chirpstack/bootstrap.go` defines ConnectionStore interface with GetCSConnection AND SetCSTenantApp methods
- `[x] internal/testsupport/chirpstack_mock.go` contains RegisterTenantServiceServer AND RegisterApplicationServiceServer AND RegisterDeviceProfileServiceServer AND RegisterDeviceServiceServer
- `[x] internal/chirpstack/doc.go` contains "Phase 2 extends" paragraph mentioning all 5 new wrappers
- `[x] All 5 Wave-0 _test.go stubs filled in (no t.Skip remaining for these); 20 distinct test cases total`
- `[x] go test -count=1 -race ./internal/chirpstack/... -run 'TestEnsureTenant|TestEnsureApplication' → 5 passed`
- `[x] go test -count=1 -race ./internal/chirpstack/... -run 'TestCreateDeviceProfile|TestUpdateDeviceProfile|TestCreateDevice|TestRegionEnumMapping|TestDeleteDevice' → 20 passed`
- `[x] go test -count=1 -race ./internal/chirpstack/... -run TestBootstrap → 6 passed`
- `[x] go test -count=1 -race ./internal/chirpstack/... (with podman socket) → 38 passed`
- `[x] go test -count=1 ./... -short (with podman socket) → 176 passed in 22 packages`
- `[x] go vet ./... clean`
- `[x] go build ./... clean`
- `[x] grep -rn 'chirpstack/api/go/v4' ./internal/ | grep -v 'internal/chirpstack/' | grep -v 'internal/testsupport/chirpstack_mock.go' returns 0 lines (architectural seam preserved)`
- `[x] commit f85bee0 (Task 1) found in git log`
- `[x] commit 52118be (Task 2) found in git log`
- `[x] commit 994f122 (Task 3) found in git log`
