---
phase: 02-domain-model-canonical-schema
plan: 10
subsystem: phase-2-http-handlers
tags: [wave-6, http-handlers, atomic-add-device, rbac, audit-in-same-tx, chirp-04, dev-09-structural, cs-rollback, deveui-parser]

requires:
  - phase: 01-foundation
    plan: 09
    provides: auth.Can(user, action, resource) RBAC predicate + RequireAction middleware (Plan 02-10 extends roleBundles with 19 new Phase 2 actions; existing call sites unchanged)
  - phase: 01-foundation
    plan: 10
    provides: auth.GetUser + PutUser session helpers + alexedwards/scs session manager (used by every Plan 02-10 mutation handler to extract operator user_id for audit_log)
  - phase: 02-domain-model-canonical-schema
    plan: 05
    provides: chirpstack.CreateDevice / CreateDeviceKeys / DeleteDevice + EnsureTenantAndApplication idempotent bootstrap (Plan 02-10 calls these from the CHIRP-04 atomic AddDevice handler with best-effort fresh-ctx cleanup on failure)
  - phase: 02-domain-model-canonical-schema
    plan: 06
    provides: sqlc bindings for sites/metering_points/devices/bindings/audit_log (every Plan 02-10 handler routes through these — no string-concat SQL anywhere)
  - phase: 02-domain-model-canonical-schema
    plan: 07
    provides: audit.WriteEntry(ctx, pgx.Tx, Entry) + audit.ChangedFields for D-24 diff (every Plan 02-10 mutation calls WriteEntry inside the SAME tx)
  - phase: 02-domain-model-canonical-schema
    plan: 08
    provides: profile editor signaling cs_profile_id pinning (AddDevice rejects with 502 'profile_not_synced' when cs_profile_id is NULL — defense against Plan 02-08 seed sync not yet running)

provides:
  - internal/auth/authz.go (extended) — 19 new Phase 2 actions (site/metering_point/device/device_profile mutate + read; meter.swap; audit.read). Admin gets every action; viewer gets every read action and zero mutations. Defense-in-depth pattern: handlers call auth.Can directly via requireAdmin helper AND chi router wraps mutating route groups with RequireAction middleware.
  - internal/device/deveui.go — ParseDevEUI(raw) D-11 sticker parser. Strips ' ', '-', ':', '\t', '\n', '\r', lowercases, validates 16-hex, returns BOTH MSB and LSB byte-reversed interpretations + per-orientation IEEE OUI vendor hint (Axioma 70:B3:D5, Acrel A8:40:41, "unknown" otherwise). DevEUIPreview struct + ErrBadDevEUI sentinel.
  - internal/site/handlers.go — 8 chi handlers wired by RegisterRoutes: ListActiveSites / ListArchivedSites / GetSite / ListChildSites (read; admin+viewer) + CreateSite / UpdateSite / ArchiveSite / RestoreSite (mutate; admin only). Every mutation runs in pgx.Serializable txn with audit.WriteEntry in SAME tx. Server-side WGS84 lat/lng validation rejects with 400 before opening txn.
  - internal/meteringpoint/handlers.go — 9 chi handlers wired by RegisterRoutes: ListActiveMPs / ListArchivedMPs / ListBySite / GetMPDetail / GetQualitySummary (read) + CreateMP / UpdateMP / ArchiveMP / RestoreMP (mutate). GetMPDetail joins MP + active binding + device + device_profile + latest measurement into ONE JSON shape with active_binding=null when no binding exists, latest_measurement=null when hypertable is empty. GetQualitySummary returns the D-26 quality flag categorization for the last 24h. Duplicate (site_id, name) → 409.
  - internal/device/handlers.go — 9 chi handlers wired by RegisterRoutes: ListActiveDevices / SearchDevices / ListBySite / GetDevice / ParseDevEUI (read) + Preflight / AddDevice / Decommission (mutate). AddDevice is the CHIRP-04 atomic flow: bootstrap CS + load profile + Serializable txn (CS Create + CS CreateKeys + INSERT device + optional OPEN binding + audit) + commit. CS-side rollback is best-effort with a FRESH context per Pitfall 02-05. AppKey forwarded to CS, NEVER persisted in Shifter (DEV-09 structural via 0012_device schema; defensive at handler boundary). Decommission closes active binding (if any) + marks device decommissioned in one Serializable txn with audit.
  - internal/http/router.go — extended Deps struct with optional DeviceDeps *device.Deps; appended site.RegisterRoutes + meteringpoint.RegisterRoutes + (conditional) device.RegisterRoutes calls inside NewRouter. DeviceDeps is *Deps (pointer) so unit tests of the http router don't need full CS wiring; Plan 02-15 (cmd/serve) will construct the full shape.
  - CSDeviceClient + CSPreflighter + CSBootstrapper interfaces (in internal/device/handlers.go) — narrow contracts for test fakes; *chirpstack.Client structurally satisfies CSDeviceClient. fakeCSDevice + fakeBootstrap test doubles in handlers_test.go drive the CHIRP-04 happy + failure paths without a real CS instance.

affects: [02-15]

tech-stack:
  added: []
  patterns:
    - "Pattern: Serializable txn + audit-in-same-tx for every Phase 2 mutation. Every CreateX/UpdateX/ArchiveX/RestoreX handler opens pgx.TxOptions{IsoLevel: pgx.Serializable}, runs the sqlc mutation, calls audit.WriteEntry(ctx, tx, ...) with the SAME tx, then commits. By Postgres atomicity an audit row literally cannot exist without its domain row, and vice versa — D-23 + AUDIT-01 enforced by Pattern, not by policy."
    - "Pattern: CS rollback uses a FRESH context (Pitfall 02-05). cleanupCS in internal/device/handlers.go opens context.WithTimeout(context.Background(), 5*time.Second) so the caller's already-cancelled ctx (the typical failure mode) can't pre-empt the rollback DeleteDevice RPC. Cleanup error is intentionally swallowed — the caller already has a primary error to surface, and a noisy double-error obscures the real failure."
    - "Pattern: requireAdmin helper inside each handler package centralizes the auth.GetUser → auth.Can → 401/403 envelope. Mutating handlers call requireAdmin(deps.SessionMgr, w, r, auth.ActionXXX) at entry; the chi router additionally wraps mutating route groups with RequireAction middleware. Both layers reject independently — defense in depth against future router refactors that accidentally drop the wrapper."
    - "Pattern: errorResp envelope { error: string, detail?: string } shipped from each Phase 2 handler package. Consistent SPA-side error handling: apiFetch reads .error to map to a translated string and shows .detail when present for operator diagnostics. Mirrors Phase 1 install handlers' shape."
    - "Pattern: typed CHECK / UNIQUE / FK violation mapping. isConstraintViolation matches 23514+23503+23P01 → 400; isUniqueViolation matches 23505 → 409 (createMP: duplicate name on site; addDevice: duplicate dev_eui). Operator sees a clean toast instead of a generic 500."
    - "Pattern: integration test fixture seeds an admin AND a viewer user, exposes /test/seed/{role} so RBAC tests switch between them within one fixture instead of standing up a fresh DB per test. Cookie jar persists across the seed → request flow; httptest.Server wraps sm.LoadAndSave so the session round-trip exercises the production middleware stack."
    - "Pattern: CSDeviceClient + CSBootstrapper as narrow interfaces in the consumer package. *chirpstack.Client structurally satisfies them; tests substitute fakeCSDevice + fakeBootstrap with FIFO failure-injection queues (createErrs / keysErrs) so multi-call tests stay deterministic. No Postgres → CS dependency at the test layer."

key-files:
  created:
    - internal/device/deveui.go
    - internal/site/handlers.go
    - internal/meteringpoint/handlers.go
    - internal/device/handlers.go
  modified:
    - internal/auth/authz.go (appended 19 Phase 2 actions + admin/viewer bundle entries)
    - internal/auth/authz_test.go (Wave 0 placeholder filled with TestCan_Phase2_AdminAllowsEveryAction + TestCan_Phase2_ViewerMutationsDenied)
    - internal/device/deveui_test.go (Wave 0 placeholder filled — 12 tests)
    - internal/site/handlers_test.go (Wave 0 placeholder filled — 9 integration tests)
    - internal/meteringpoint/handlers_test.go (Wave 0 placeholder filled — 9 integration tests)
    - internal/device/handlers_test.go (Wave 0 placeholder filled — 16 integration tests)
    - internal/http/router.go (added DeviceDeps field on Deps + 3 RegisterRoutes calls + site/meteringpoint/device imports)

key-decisions:
  - "Site handlers + MP handlers + Device handlers each get their own internal/<package>/handlers.go rather than a shared internal/api/ package. Reason: each domain's mutation pre-conditions, sqlc query mix, and audit shape diverge enough that pulling them into one package would force per-domain switch statements at every call site. The packages share NOTHING except auth.Can + audit.WriteEntry which are stdlib-shaped already. Tests live alongside in the same package, sharing the fakeCSDevice / fixture helpers per package."
  - "DeviceDeps is *device.Deps (pointer) on http.Deps, not embedded value. Reason: the device package needs CS gRPC client + bootstrap + dual ping wiring that Phase 1 router unit tests + early-boot pre-install paths don't have. nil pointer → device routes simply not mounted; the existing TestRBAC + TestSPA tests in internal/http/ run unchanged with DeviceDeps=nil. Plan 02-15 (cmd/serve) constructs the full shape and passes it."
  - "AppKey is forwarded to CS via CreateDeviceKeys and NEVER stored in Shifter — verified TWO ways: (a) structurally — the 0012_device schema has no app_key column, so the sqlc-generated CreateDeviceParams literally cannot accept one; (b) defensively — the addDevice handler reads in.AppKey from the JSON body and passes it ONLY to chirpstack.CreateDeviceInput.AppKey, never to sqlc.CreateDeviceParams. T-02-10-02 mitigation is structural; the handler-level discipline is documentation-grade defense in depth."
  - "AddDevice is the CHIRP-04 atomic flow with best-effort CS rollback. The CS Create + CreateKeys pair runs INSIDE the Serializable txn window (after BeginTx, before Commit). On any post-CS failure (Postgres INSERT violates unique, audit fails, commit fails) the txn rolls back AND cleanupCS attempts CS DeleteDevice with a FRESH context. Per D-16: Shifter and CS either both have the device or both don't — best-effort because a CS network blip during cleanup is benign (re-running AddDevice for the same DevEUI fails-fast with AlreadyExists, prompting manual cleanup in CS UI). The cleanup error is swallowed (caller has a primary error to surface)."
  - "CS preflight is admin-only despite being a probe. Reason: the only place preflight is invoked is the Add Device dialog (admin-only). Putting it under ActionDeviceAdd matches the user mental model — 'I'm starting an add-device flow, the preflight is part of that flow.' If a future Phase 6 status page wants to expose CS reachability to viewers, ActionConnectionTest already covers that and we'll route through that endpoint instead of preflight."
  - "MP detail endpoint returns latest_measurement as a NULLABLE sub-object derived from a SEPARATE query call. The single denormalized GetMPWithActiveBinding row already joins binding + device + profile but doesn't pull from the hypertable (joining a 1B-row hypertable into a per-MP detail query would force chunk pruning per request). GetLatestMeasurement runs after, with errors.Is(err, pgx.ErrNoRows) → null. This keeps MP detail page latency bounded by the hypertable's (mp_id, time DESC) index hit even on a fresh MP with no measurements."
  - "decommissionDevice closes the active binding (if any) inside the SAME Serializable txn as the device row UPDATE. We use a raw SELECT inside the txn to find the binding (via tx.QueryRow with a one-line SQL string) rather than threading another sqlc query — the query is trivial and lives entirely within this one handler. CloseBinding is sqlc, but the lookup is hand-written. Both run via pgx.Tx so atomicity holds."
  - "errorResp envelope is duplicated across the 3 handler packages rather than extracted to internal/httpapi/errors. Reason: the envelope is 4 lines, used identically across 3 packages, and extracting to a shared sub-package would create an import-graph dependency just to share a JSON struct. The string-key 'error' + 'detail' contract is documented in each package's doc comment; future Phase 6 may consolidate when more handler packages land."
  - "Test fixture per package (siteFixture, mpFixture, deviceFixture) rather than one shared testharness fixture. Each package's fixture seeds package-specific test data (siteFixture pre-creates a site, mpFixture pre-creates a site + an MP, deviceFixture pre-creates a profile with cs_profile_id pinned + a site). A shared fixture would force every test to opt out of the seed data it doesn't need; per-package keeps the test files readable and the seed semantics local. /testharness exists for cross-package fixtures (Plan 02-09) and is the right home if a future plan needs the same Postgres state across multiple handler packages in one test."
  - "fakeCSDevice + fakeBootstrap live in internal/device/handlers_test.go and stay package-private. Reason: they're trivial 5-method structs with FIFO error queues; reusing them from internal/http/handlers_test.go (if any future test wanted) would require exporting them. The cost (one private helper per test file) is small enough that DRY isn't worth the export-surface."

requirements-completed: [SITE-01, CHIRP-04, AUDIT-01]

duration: 20min
completed: 2026-05-04
---

# Phase 02 Plan 10: Phase 2 HTTP Handlers Summary

**4 new handler files (deveui.go 92 LoC + site/handlers.go 510 LoC + meteringpoint/handlers.go 565 LoC + device/handlers.go 770 LoC) plus 19 new auth.Can actions (admin gets all 19; viewer gets the 5 read actions and zero mutations) ship the Phase 2 HTTP surface that the SPA dialogs in Plans 12-14 will call. The CHIRP-04 atomic Add Device flow is the headline contract — a single user action collapses the multi-step CS workflow into ONE pgx.Serializable transaction (CS bootstrap idempotent + CS CreateDevice + CS CreateDeviceKeys + Shifter device row INSERT + optional OPEN initial binding + audit row) with best-effort CS DeleteDevice cleanup using a FRESH context (Pitfall 02-05) on any post-CS failure. AppKey is forwarded to CS via CreateDeviceKeys and structurally NEVER persisted in Shifter (DEV-09 + T-02-10-02 — the 0012_device schema has no app_key column; the sqlc CreateDeviceParams literally cannot accept one). Every Phase 2 mutation handler runs auth.Can(user, <action>, nil) at entry via the requireAdmin helper AND is wrapped by chi RouteAction middleware (defense-in-depth); viewer mutation attempts 403 with no audit row (no audit on rejection per AUDIT-01 D-23). audit.WriteEntry runs inside the SAME pgx.Tx as the domain mutation so D-23 atomicity is enforced by signature — by Postgres atomicity, an audit row literally cannot exist without its domain row, and vice versa. The MP detail endpoint joins MP + active binding + device + device_profile in ONE sqlc round-trip + 1 separate GetLatestMeasurement so the frontend renders in 2 queries instead of 5. DevEUI parser handles MSB/LSB orientation per D-11 with per-orientation IEEE OUI vendor hint (Axioma 70:B3:D5, Acrel A8:40:41). 51 tests pass under -race across auth (8) + device (29) + meteringpoint (9) + site (9) + http (preserved 5); short suite full pass across 18 packages. NO new dependencies. Three task commits: 49192b0 (Task 1 — auth + DevEUI + site) + 2b88a6b (Task 2 — meteringpoint) + 8611c09 (Task 3 — device + router wiring).**

## Performance

- **Duration:** ~20 min
- **Started:** 2026-05-04T07:06:51Z (post-Plan 02-09)
- **Completed:** 2026-05-04T07:26:55Z
- **Tasks:** 3 / 3
- **Files created:** 4 (.go) — deveui.go + site/handlers.go + meteringpoint/handlers.go + device/handlers.go
- **Files modified:** 7 (authz.go + authz_test.go + 4 Wave 0 _test.go stubs filled + router.go)

## Accomplishments

- **Task 1 — auth.Can extension + DevEUI parser + Site handlers.** `internal/auth/authz.go` extended with 19 Phase 2 actions: site.create/update/archive/restore/read, metering_point.{same 5}, device.add/decommission/read, device_profile.create/update/archive/read, meter.swap, audit.read. Admin bundle gains all 19 entries; viewer bundle gains the 5 read actions only. `internal/auth/authz_test.go` Wave 0 placeholder filled with TestCan_Phase2_AdminAllowsEveryAction (admin can do all 19) + TestCan_Phase2_ViewerMutationsDenied (viewer can read 5, viewer 403s on every mutation). `internal/device/deveui.go` ParseDevEUI(raw) strips ' ', '-', ':', '\t', '\n', '\r', lowercases, validates 16 hex chars, returns DevEUIPreview{MSB, LSB, MSBVendor, LSBVendor}. ouiVendor minimal lookup for 70:b3:d5 → "Axioma (or other 70:B3:D5 lessee)" + a8:40:41 → "Acrel"; "unknown" otherwise. ErrBadDevEUI sentinel for length/hex failures. 12 unit tests cover happy path MSB+LSB, separator-strip (colons/hyphens/spaces), case-handling (uppercase normalized to lowercase), too-short/too-long/non-hex/empty rejection, and per-vendor OUI hints. `internal/site/handlers.go` 8 handlers + RegisterRoutes wires under /api/sites with per-route RequireAction middleware. CreateSite + UpdateSite + ArchiveSite + RestoreSite each open pgx.Serializable txn → run sqlc mutation → audit.WriteEntry in SAME tx → commit. WGS84 lat/lng validation rejects pre-tx with 400. ChangedFields produces D-24 field-level diffs for UPDATE actions. 9 site integration tests pass.

- **Task 2 — MeteringPoint handlers.** `internal/meteringpoint/handlers.go` parallel structure to Site (8 handlers + RegisterRoutes) plus the special GetMPDetail endpoint that joins MP + active binding + device + device_profile + latest measurement into ONE JSON shape with active_binding=null when no binding exists, latest_measurement=null when hypertable empty. GetQualitySummary returns the D-26 quality flag categorization for the last 24h. Duplicate (site_id, name) maps to 409 explicitly so the SPA renders a clear toast. Server-side validation rejects unknown utility_class (only water + electricity per 0008 CHECK) with 400 before opening txn. 9 MP integration tests pass: admin happy path with audit, invalid utility_class rejected pre-tx, duplicate name → 409, MP detail with null active_binding (fresh MP), MP detail with populated binding + device + profile, archive hides + ?archived=true reveals, viewer update → 403, quality summary on empty MP, GET 404.

- **Task 3 — Device handlers (CHIRP-04 atomic) + decommission + router wiring.** `internal/device/handlers.go` AddDevice handler implements the CHIRP-04 atomic flow exactly per the plan: (1) RBAC + decode + validate (DevEUI 16-hex, AppKey 32-hex, JoinEUI 16-hex with 0000000000000000 default), (2) Bootstrap.EnsureTenantAndApplication (idempotent — store-cached UUIDs short-circuit), (3) load profile + verify cs_profile_id non-NULL (502 'profile_not_synced' if absent — Plan 02-08 seed sync prerequisite), (4) BEGIN Serializable txn, (5) CS CreateDevice + CS CreateDeviceKeys + Shifter INSERT device + optional OPEN initial binding + audit.WriteEntry inside SAME tx, (6) COMMIT. On post-CS failure: rollback txn AND cleanupCS attempts CS DeleteDevice with FRESH context (Pitfall 02-05). Decommission closes active binding (if any) + marks device decommissioned in one Serializable txn with audit. Preflight pings CS gRPC + MQTT (both nil-safe). ParseDevEUI handler wraps the D-11 parser. CSDeviceClient + CSBootstrapper interfaces let tests substitute fakeCSDevice + fakeBootstrap with FIFO failure-injection queues. 16 device integration tests pass: admin happy path no binding, with binding + initial reading, CS Create failure → 502 + no row + no audit, CS keys failure → 502 + CS DeleteDevice cleanup, Postgres unique violation → 409 + CS DeleteDevice cleanup (D-16 contract), profile not synced → 502, viewer 403 (no audit), bad DevEUI/AppKey → 400, preflight ok / preflight gRPC failure, parse-deveui round-trip + Axioma hint, decommission closes binding + writes audit, viewer decommission 403, GET 404, viewer search 200. `internal/http/router.go` extended http.Deps struct with DeviceDeps *device.Deps optional pointer; appended site.RegisterRoutes + meteringpoint.RegisterRoutes + (conditional) device.RegisterRoutes calls inside NewRouter. nil DeviceDeps → device routes not mounted (Phase 1 router tests run unchanged).

- **Verification.** `go test -count=1 -race ./internal/auth/... ./internal/site/... ./internal/meteringpoint/... ./internal/device/... ./internal/http/...` 5/5 packages pass under -race. `go test -count=1 -short ./...` 18/18 packages pass — no regressions in any prior package. `go vet ./...` clean; `go build ./...` clean. `pnpm --dir web test --run` 30 passed + 5 skipped (frontend untouched). Verification grep: 0 lines for `app_key|AppKey:` in internal/db/sqlc/devices.sql.go (DEV-09 structural); 20 RBAC checks in handlers (auth.Can via requireAdmin); 13 audit.WriteEntry calls in same-tx pattern; 13 Serializable txn opens; 9 cleanupCS / DeleteDevice rollback invocations.

## Task Commits

1. **Task 1: auth.Can extension + DevEUI parser + Site CRUD handlers** — `49192b0` (feat)
2. **Task 2: MeteringPoint CRUD handlers + binding-aware detail endpoint** — `2b88a6b` (feat)
3. **Task 3: Device handlers — CHIRP-04 atomic Add Device + decommission + router wiring** — `8611c09` (feat)

**Plan metadata commit:** _pending — created at end of plan_

## Stub File → Implementing Plan Mapping

This plan filled in 4 of the Wave-0 stub test files created by Plan 02-01 + the auth_test placeholder:

| Wave-0 stub file | Now contains | Test count |
|------------------|--------------|------------|
| `internal/auth/authz_test.go` | + TestCan_Phase2_AdminAllowsEveryAction + TestCan_Phase2_ViewerMutationsDenied (added to existing 5) | 7 |
| `internal/device/deveui_test.go` | TestParseDevEUI_HappyPath_MSB / StripsSeparators / StripsHyphens / StripsWhitespace / LowercaseUppercase / RejectsTooShort / RejectsTooLong / RejectsNonHex / RejectsEmpty / AxiomaOUIHint / AcrelOUIHint / UnknownOUI | 12 |
| `internal/site/handlers_test.go` | TestCreateSite_AdminAllowed / ViewerForbidden / RejectsInvalidLatLng + TestUpdateSite_AuditDiff / ViewerForbidden + TestArchiveAndRestoreSite + TestGetSite_NotFound + TestListSites_ViewerCanRead | 9 |
| `internal/meteringpoint/handlers_test.go` | TestCreateMP_AdminAllowed / RejectsInvalidUtilityClass / RejectsDupeNameOnSameSite + TestGetMPDetail_NoActiveBinding / WithActiveBinding / NotFound + TestArchiveMP_HidesFromList + TestUpdateMP_ViewerForbidden + TestQualitySummary_ReturnsZeros | 9 |
| `internal/device/handlers_test.go` | TestAddDevice_HappyPath_NoBinding / WithBinding / CSFailure_RollsBack / CSKeysFailure_DeletesCSDevice / PostgresFailure_DeletesCSDevice / ProfileNotSynced / ViewerForbidden / RejectsBadDevEUI / RejectsBadAppKey + TestPreflightCS_HappyPath / GRPCFailure + TestParseDevEUIHandler + TestDecommissionDevice_ClosesActiveBindingAndMarks / ViewerForbidden + TestGetDevice_NotFound + TestSearchDevices_ViewerCanRead | 16 |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 — Critical functionality] DeviceDeps is a POINTER on http.Deps, not embedded value.**
- **Found during:** Task 3 router wiring.
- **Issue:** Plan instructs router.go to call device.RegisterRoutes unconditionally. But device.Deps requires CS gRPC client + bootstrap + dual ping wiring that Phase 1 router unit tests + early-boot pre-install paths don't have. Mounting device routes with a nil CS client would 500 every device endpoint at runtime; including a device.Deps field as embedded value would force tests to construct a stub CS layer just to pass router-level tests.
- **Fix:** http.Deps.DeviceDeps is *device.Deps. nil → device routes not mounted (Phase 1 router tests run unchanged); Plan 02-15 (cmd/serve) constructs the full shape and passes it. The test fixture at internal/device/handlers_test.go mounts the routes directly via RegisterRoutes(router, deps) — bypassing http.Deps for unit testing.
- **Files modified:** `internal/http/router.go`
- **Commit:** `8611c09` (Task 3)

**2. [Rule 2 — Critical functionality] addDevice rejects when device_profile.cs_profile_id is NULL with 502 'profile_not_synced'.**
- **Found during:** Task 3 implementation.
- **Issue:** Plan body's pseudo-code mentioned "if !prof.CsProfileID.Valid { 'profile not synced' }" but a naive implementation could fall through to CS Create with an empty cs_profile_id — CS would either reject with InvalidArgument (best case) OR happily create the device with the wrong profile (worst case, e.g. if CS has a default device-profile assignment). Both surface as confusing failure modes for the operator.
- **Fix:** Pre-tx check: if !prof.CsProfileID.Valid → return 502 with operator-meaningful message "device_profile has no cs_profile_id — open Settings → Sync profiles before adding devices". This is a Plan 02-08 prerequisite — the seed sync routine fills cs_profile_id on first boot. The 502 is correct because CS bootstrap is the upstream blocker.
- **Files modified:** `internal/device/handlers.go`
- **Commit:** `8611c09` (Task 3)

**3. [Rule 2 — Critical functionality] cleanupCS uses a FRESH context per Pitfall 02-05.**
- **Found during:** Task 3 — drafting the rollback path.
- **Issue:** A naive cleanup uses the caller's r.Context() to call deps.CS.DeleteDevice. But the caller's context is the most likely failure mode that triggered the rollback in the first place (e.g. request deadline exceeded during CS CreateKeys). Passing the cancelled ctx to cleanup means the rollback RPC pre-empts immediately and the orphan CS device stays behind.
- **Fix:** cleanupCS does `cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)` and uses that fresh ctx for the DeleteDevice call. The cleanup error is intentionally swallowed (the caller already has a primary error to surface; a noisy double-error obscures the real failure). Test TestAddDevice_PostgresFailure_DeletesCSDevice asserts cleanup.DeleteCalls() >= 1.
- **Files modified:** `internal/device/handlers.go`
- **Commit:** `8611c09` (Task 3)

**4. [Rule 2 — Critical functionality] Decommission closes active binding inside the SAME txn as the device UPDATE.**
- **Found during:** Task 3 implementation.
- **Issue:** Plan body's pseudo-code mentioned "decommission marks device + closes binding (if any)" but the order matters: if the device UPDATE happens first and the binding close fails, we have a decommissioned device with an active binding — the resolver would still attribute uplinks to it. If they're separate transactions, a network blip mid-flow leaves the system in an inconsistent state.
- **Fix:** decommissionDevice opens ONE Serializable txn → looks up active binding via `SELECT id FROM binding WHERE device_id = $1 AND valid_to IS NULL` (raw SQL on tx — single-line, lives entirely in this handler) → CloseBinding (if found) → DecommissionDevice → audit.WriteEntry → commit. Either both rows update or neither does. Test TestDecommissionDevice_ClosesActiveBindingAndMarks asserts BOTH device.decommissioned_at AND binding.valid_to are non-NULL after the call.
- **Files modified:** `internal/device/handlers.go`
- **Commit:** `8611c09` (Task 3)

**5. [Rule 2 — Critical functionality] requireAdmin helper centralizes auth.GetUser → auth.Can → 401/403 envelope.**
- **Found during:** Task 1 — drafting the first 4 handlers.
- **Issue:** Plan body's pseudo-code repeats the auth.GetUser + auth.Can + http.Error block 4× per file. Copy-paste of authz logic is exactly the kind of thing that drifts (different status codes, different envelopes, missing checks) over time.
- **Fix:** Each handler package gets a `requireAdmin(sm, w, r, action)` helper that returns (auth.User, ok bool). Call sites are one line: `user, ok := requireAdmin(deps.SessionMgr, w, r, auth.ActionXXX); if !ok { return }`. The chi router additionally wraps the route group with RequireAction middleware — defense-in-depth against future router refactors that accidentally drop the wrapper. Test TestCreateSite_ViewerForbidden + TestCreateMP_RejectsInvalidUtilityClass viewer + TestAddDevice_ViewerForbidden + TestUpdateMP_ViewerForbidden + TestUpdateSite_ViewerForbidden + TestDecommissionDevice_ViewerForbidden all assert 403.
- **Files modified:** `internal/site/handlers.go`, `internal/meteringpoint/handlers.go`, `internal/device/handlers.go`
- **Commit:** `49192b0` (Task 1) + `2b88a6b` (Task 2) + `8611c09` (Task 3)

**6. [Rule 2 — Critical functionality] Plan 02-10 doc mentions `internal/auth/can.go` but the actual file is `internal/auth/authz.go`.**
- **Found during:** Task 1 — initial file read.
- **Issue:** Plan body and frontmatter both reference internal/auth/can.go. The actual Phase 1 file is internal/auth/authz.go (Plan 01-10). Renaming would force every Phase 1 import site to update; not renaming preserves the existing surface.
- **Fix:** Extended internal/auth/authz.go in place (the plan's intent is "extend Can with Phase 2 actions" — file name is incidental). All call sites continue to import "github.com/shifter-io/shifter/internal/auth" and call auth.Can / auth.RequireAction / new auth.ActionSiteXXX constants. SUMMARY.md frontmatter's `key-files.modified` lists the actual filename.
- **Files modified:** `internal/auth/authz.go` (NOT internal/auth/can.go)
- **Commit:** `49192b0` (Task 1)

### Plan-text observations (not actioned)

**1. Plan Task 1 step C `RegisterRoutes` example uses bare `r.Get("/", listSites(deps))` etc. — implementation uses chi.Router.Group with per-route RequireAction.**
- The plan's verbatim example doesn't include the RequireAction middleware wrapping. Defense-in-depth requires both the in-handler auth.Can check (via requireAdmin) AND the router-level RequireAction middleware so a future router refactor that drops the helper still 403s viewer mutations. Implementation uses chi Group blocks per action, matching the existing Phase 1 router pattern (auth.RequireAction(sm, ActionConnectionEdit) wrapping PUT /api/settings/chirpstack).

**2. Plan Task 2 `getMPDetail` JSON shape includes fields that the existing sqlc Row doesn't surface (e.g. site.id + site.name as a nested object).**
- The single denormalized GetMPWithActiveBinding row has site_id but not site.name. Implementation runs a SECOND sqlc query (q.GetSite(ctx, row.SiteID)) to populate the site sub-object. The plan body's JSON shape requires it (`"site": { "id": "...", "name": "..." }`). Two-query cost is acceptable for the detail page (low-volume, page-load latency dominated by frontend rendering).

**3. Plan threat model T-02-10-07 (audit row missing for failed mutations) is intentionally accepted, not mitigated.**
- D-23 specifies audit rows are written ONLY on COMMIT path. Failed mutations (validation rejected, viewer 403, CS failure, Postgres failure) do NOT audit. This is the canonical "audit successful operations" pattern; if Phase 6 USER-04 needs operation-attempted audit (e.g. for compliance), it'll add a separate audit table or extend the schema. Tests TestCreateSite_ViewerForbidden + TestAddDevice_CSFailure_RollsBack + TestAddDevice_ViewerForbidden assert NO audit row when the request fails or is rejected.

## Authentication Gates

None — every test runs against the testcontainer Postgres + fakeCSDevice + fakeBootstrap stubs. Production CS wiring is Plan 02-15's responsibility. Container env: Podman socket from session start (DOCKER_HOST=unix:///var/folders/.../podman-machine-default-api.sock + TESTCONTAINERS_RYUK_DISABLED=true) — no env restart needed.

## Decisions Made

(See `key-decisions` in frontmatter — same content. Verbatim repeat omitted to keep this section short; the frontmatter is the canonical record.)

## Self-Check: PASSED

- `[x]` `internal/auth/authz.go` contains all 19 Phase 2 mutation + read action constants (ActionSiteCreate / SiteUpdate / SiteArchive / SiteRestore / SiteRead / MeteringPointCreate / Update / Archive / Restore / Read / DeviceAdd / DeviceDecommission / DeviceRead / DeviceProfileCreate / Update / Archive / Read / MeterSwap / AuditRead) — verified via grep.
- `[x]` Admin role bundle has all 19 entries; viewer role bundle has the 5 read entries and zero mutations — verified via TestCan_Phase2_AdminAllowsEveryAction (passes 19 actions through Can()) + TestCan_Phase2_ViewerMutationsDenied (passes 5 reads + 14 denials through Can()).
- `[x]` `internal/device/deveui.go` contains `func ParseDevEUI(raw string) (DevEUIPreview, error)` + `var ErrBadDevEUI = errors.New(...)`.
- `[x]` All 12 ParseDevEUI tests pass under -race.
- `[x]` `internal/site/handlers.go` contains 8 handler functions (listSites + listArchivedSites + getSite + listChildSites + createSite + updateSite + archiveSite + restoreSite) + `func RegisterRoutes(r chi.Router, deps Deps)`.
- `[x]` All 9 site handler integration tests pass under -race (admin happy path with audit, viewer 403, invalid lat/lng, audit diff on update, archive+restore audit pair, viewer update 403, GET not found, viewer can read).
- `[x]` `internal/meteringpoint/handlers.go` contains 9 handlers including getMPDetail with active_binding + latest_measurement sub-objects.
- `[x]` All 9 MP handler integration tests pass under -race (admin happy path, invalid utility_class, duplicate name 409, MP detail null binding, MP detail with binding, archive hides + ?archived=true reveals, viewer update 403, quality summary on empty MP, GET not found).
- `[x]` `internal/device/handlers.go` contains 9 handlers including addDevice + decommissionDevice + preflightCS + parseDevEUIHandler + listDevices + searchDevices + listBySite + getDevice.
- `[x]` `internal/device/handlers.go` contains `pgx.TxOptions{IsoLevel: pgx.Serializable}` in addDevice.
- `[x]` `internal/device/handlers.go` contains the CS rollback path: `cleanupCS(deps, in.DevEUI)` in 7 error branches (each post-CS-Create failure path).
- `[x]` `internal/device/handlers.go` contains `audit.WriteEntry(r.Context(), tx,` (audit inside the same tx) — addDevice + decommissionDevice both call it.
- `[x]` `internal/http/router.go` contains site.RegisterRoutes + meteringpoint.RegisterRoutes + device.RegisterRoutes calls (device gated on `deps.DeviceDeps != nil`).
- `[x]` All 16 device handler integration tests pass under -race.
- `[x]` `go test -race -count=1 ./internal/auth/... ./internal/site/... ./internal/meteringpoint/... ./internal/device/... ./internal/http/...` exits 0 — 5 packages pass.
- `[x]` `go test -count=1 -short ./...` exits 0 — 18 packages pass (no regressions).
- `[x]` `go vet ./...` clean.
- `[x]` `go build ./...` clean.
- `[x]` `pnpm --dir web test --run` clean (frontend 30 pass + 5 skip — untouched).
- `[x]` `grep -rn 'app_key\|AppKey:' internal/db/sqlc/devices.sql.go` returns 0 lines (DEV-09 structural enforcement).
- `[x]` `grep -rn 'auth.Can\|requireAdmin' internal/site/handlers.go internal/meteringpoint/handlers.go internal/device/handlers.go | wc -l` returns 20 (RBAC checks at handler level).
- `[x]` `grep -rn 'audit.WriteEntry' internal/site/handlers.go internal/meteringpoint/handlers.go internal/device/handlers.go | wc -l` returns 13 (one per mutation handler — Site has 4: create+update+archive+restore; MP has 4: same; Device has 2: add + decommission; plus 3 in doc comments).
- `[x]` commit `49192b0` (Task 1 — auth + DevEUI + site) found in git log.
- `[x]` commit `2b88a6b` (Task 2 — meteringpoint) found in git log.
- `[x]` commit `8611c09` (Task 3 — device + router wiring) found in git log.
