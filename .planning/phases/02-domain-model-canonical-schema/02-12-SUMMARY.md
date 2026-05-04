---
phase: 02-domain-model-canonical-schema
plan: 12
subsystem: cmd-serve
tags: [composition-root, chirpstack-grpc, resolver, ingest-binding, profile-seed-sync, devicedeps, swapdeps, profiledeps, full-boot-integration]

# Dependency graph
requires:
  - phase: 02-domain-model-canonical-schema
    provides: [chirpstack.NewClient + EnsureTenantAndApplication (Plan 02-05), resolver.New + Run (Plan 02-07), ingest.UplinkHandler + SQLCMappingStore (Plan 02-09), profile.RunSeedSync (Plan 02-08), profile.HTTPDeps + swap.HTTPDeps (Plan 02-11), device.Deps + CSBootstrapper interface (Plan 02-10), sqlc.SetChirpStackTenantApp + GetChirpStackTenantApp (Plan 02-06)]
provides:
  - "internal/cli/connection_store.go — sqlc-backed ConnectionStore satisfying BOTH chirpstack.ConnectionStore (read+write) and profile.ConnectionStore (read only). One struct, two interfaces — composition root constructs ONE instance and threads it everywhere."
  - "internal/ingest/numeric.go — exported BigFloatFromNumeric + BigFloatFromNumericNullable (W5: extracted from handler.go's package-private bigFloatFromNumeric so cmd/serve resolver loader reuses instead of duplicating). Generic 0 default; mapping-scale 'default to 1' lives at the SQLCMappingStore call site."
  - "Full Phase 2 wiring in serve.go RunE: ChirpStack gRPC Client construction, EnsureTenantAndApplication boot call, *resolver.Resolver + listener goroutine, mqttSub.SetUplinkHandler(ingest.UplinkHandler(deps)), profile.RunSeedSync first-boot codec push, DeviceDeps + SwapDeps + ProfileDeps construction. Production binary now mounts every Phase 2 route surface."
  - "sqlcResolverLoader (file-private helper at end of serve.go) — wraps sqlc.GetActiveBindingByDevEUI; reuses ingest.BigFloatFromNumeric + BigFloatFromNumericNullable (W5 — single source of truth)."
  - "bootstrapperFunc adapter — function-to-CSBootstrapper-interface so EnsureTenantAndApplication threads cleanly into device.Deps."
  - "internal/cli/serve_fullboot_test.go — 4 end-to-end integration tests (testcontainer Postgres + Mosquitto + bufconn ChirpStack v4 mock) proving every wiring claim."
affects: [no downstream plans within Phase 2 — this plan IS the missing wiring step. Phase 3 ingest-realtime + Phase 4 dashboards inherit a fully-wired serve.go that already routes MQTT uplinks through the ingest pipeline.]

# Tech tracking
tech-stack:
  added: []  # No new dependencies. argon2 helper uses existing golang.org/x/crypto/argon2 already in go.mod.
  patterns:
    - "Composition-root ConnectionStore: one sqlc-backed struct satisfies multiple narrow interfaces (chirpstack.ConnectionStore + profile.ConnectionStore) — no duplication, no fanout of sqlc imports across business-logic packages."
    - "Generic 'parse pgtype.Numeric → *big.Float' helper with sentinel default at the helper layer; mapping-scale 'default to 1' moved to the SQLCMappingStore call site where the semantic actually applies."
    - "sqlcResolverLoader wraps the production sqlc query; loader functions stay in cmd/serve (composition root) so the resolver package remains business-logic-only."
    - "bootstrapperFunc(func(...) (...)) adapter pattern — adapt free-function with extra captured args into a 1-method interface without per-call object construction."
    - "End-to-end full-boot integration test pattern: testcontainer Postgres + Mosquitto + bufconn CS mock → mirror serve.go's wiring blocks 1-by-1 in test code → assert real MQTT publish lands a measurement row + HTTP routes return 200. Doubles as a wiring contract test."

key-files:
  created:
    - "internal/cli/connection_store.go (~92 LoC) — ConnectionStore + NewConnectionStore + GetCSConnection + SetCSTenantApp + compile-time interface assertions"
    - "internal/cli/connection_store_test.go (~100 LoC) — 4 tests (GetEmpty, RoundTrip, NoRow, SatisfiesBothInterfaces)"
    - "internal/ingest/numeric.go (~49 LoC) — exported BigFloatFromNumeric + BigFloatFromNumericNullable"
    - "internal/ingest/numeric_test.go (~67 LoC) — 2 tests (round-trip + nullable null-handling)"
    - "internal/cli/serve_fullboot_test.go (~611 LoC) — fullBootHarness + 4 integration tests"
  modified:
    - "internal/cli/serve.go — +208 LoC for blocks 6a-6e (ConnectionStore, CS Client + bootstrap, Resolver + listener, ingest binding, RunSeedSync) + block 8 (DeviceDeps + SwapDeps + ProfileDeps) + sqlcResolverLoader helper + bootstrapperFunc adapter at file end. Package doc comment updated to describe Plan 02-12 wiring; the 'Phase 2+ adds device handlers' line is gone."
    - "internal/ingest/handler.go — removed package-private bigFloatFromNumeric body (replaced by W5 export); SQLCMappingStore.GetMappingsByProfile retains scale=1 default via explicit r.Scale.Valid check at the call site. The other helpers (numericFromBigFloat, numericFromBigFloatNullable) untouched — different semantic concern (encode direction)."

key-decisions:
  - "BigFloatFromNumeric default for invalid pgtype.Numeric is big.NewFloat(0), NOT 1. The 'default to 1 for missing scale' lives at the mapping call site (SQLCMappingStore.GetMappingsByProfile) where the identity-scale semantic actually applies. This keeps the generic helper unbiased and makes the per-caller default explicit."
  - "Test harness mirrors serve.go's wiring blocks rather than calling serveCmd.RunE. RunE reads config from env vars + dials a real *grpc.ClientConn — exposing it for direct test invocation would require either env-var hackery or a refactor extracting RunE into a parameterized function. The mirror approach exercises the SAME exported helpers that RunE uses (NewConnectionStore, chirpstack.NewClient, EnsureTenantAndApplication, resolver.New, ingest.UplinkHandler, profile.RunSeedSync) so the test still proves the wiring contract; the cost is that a future RunE refactor adding a NEW wiring block must remember to add it to the harness too. Documented here so a future executor knows to mirror."
  - "FirstRunGate gate-pass: harness creates an admin user but does NOT seed install_state. Per migration 0004, the canonical post-install state is 'admin user exists, install_state row deleted by Finish.' Inserting an install_state row would either need the right column name (completed_at, not finished_at) or no row at all; we picked 'no row' to match production semantics."
  - "CSRF protection: auth POST handlers require X-Requested-With: shifter (RESEARCH §V13). Harness loginAdmin + authedPost helpers send it explicitly so login + swap-commit POSTs reach the handler instead of being short-circuited at csrfHeaderPresent."
  - "Degraded-mode device POST result: in TestServe_FullBoot_DegradedMode_NoCS, POST /api/devices is asserted to NOT return 200 (404/405/503 acceptable). The exact code depends on chi's route-tree resolution when DeviceDeps==nil and only the GET /api/sites + GET /api/metering-points routes are mounted — the assertion accepts the equivalence class so future router changes don't false-fail."

patterns-established:
  - "Composition-root sqlc-adapter pattern: when a struct needs to satisfy multiple narrow interfaces from separate packages (here chirpstack + profile), put the adapter in the composition root (cmd/serve / internal/cli) and let it import sqlc — DON'T push sqlc imports up into chirpstack or profile."
  - "W5-style helper extraction: when two packages need the same primitive operation, extract to the LOWER package (here ingest) as exported, and let the HIGHER package (cli) reuse. Don't fork the helper; don't import upward."
  - "Test harness as wiring contract: a harness that inlines the same composition-root steps as RunE doubles as a regression test — break the wiring in serve.go, the harness diverges, the integration tests fail."

requirements-completed: [DATA-01, DATA-03, DATA-05, DATA-07, DATA-09, CHIRP-04]

# Metrics
duration: ~17min
completed: 2026-05-04
---

# Phase 02 Plan 12: cmd/serve Wiring (Plan 02-15 placeholder fulfilled) Summary

**Production binary now constructs every Phase 2 dep at boot — ChirpStack gRPC client + bootstrap + Resolver + ingest binding + RunSeedSync + DeviceDeps + SwapDeps + ProfileDeps — and a 4-test full-boot integration suite proves MQTT → measurement row, swap HTTP → 200, profile HTTP → 200, degraded-no-CS still serves Phase 1 surface.**

## Performance

- **Duration:** ~17 min cumulative across 4 commits (RED → GREEN per task)
- **Started:** 2026-05-04T10:41:08Z
- **Completed:** 2026-05-04T10:58:38Z
- **Tasks:** 3 of 3 (no checkpoints)
- **Files created:** 5 (connection_store.go + connection_store_test.go + numeric.go + numeric_test.go + serve_fullboot_test.go)
- **Files modified:** 2 (serve.go + handler.go)
- **Net LoC:** +1,127 / -32

## Accomplishments

- **Task 1: ConnectionStore + W5 BigFloatFromNumeric extraction** — ConnectionStore satisfies BOTH chirpstack.ConnectionStore + profile.ConnectionStore via compile-time `_` assertions; pgx.ErrNoRows + NULL columns degrade to ("","",nil); SetCSTenantApp persists via the existing sqlc.SetChirpStackTenantApp query (TEXT columns, *string params). 4/4 ConnectionStore tests pass against testcontainer Postgres.
- **Task 1 W5: BigFloatFromNumeric** + **BigFloatFromNumericNullable** exported from internal/ingest/numeric.go; handler.go's SQLCMappingStore call site retains scale=1 default via explicit `r.Scale.Valid` check. 2/2 BigFloat tests pass; the existing 40 ingest tests remain green (Plan 02-09 regression check).
- **Task 2: serve.go full Phase 2 wiring** — 5 new wiring blocks (6a-6e) inserted between the existing Install (block 6) and TestConn (block 7) blocks; block 8 conditionally constructs DeviceDeps + SwapDeps + ProfileDeps when csClient != nil; sqlcResolverLoader + bootstrapperFunc helpers at file end. Reuses ingest.BigFloatFromNumeric/BigFloatFromNumericNullable directly — no inline duplication. The "Phase 2+ adds device handlers" placeholder comment is gone.
- **Task 3: 4 end-to-end TestServe_FullBoot_* integration tests** — full harness (Postgres + Mosquitto + bufconn CS mock) mirrors serve.go's wiring blocks; tests prove (a) MQTT publish → measurement row with cumulative_value populated, (b) swap HTTP returns 200 + binding_id, (c) profile HTTP returns 200 with seeded profiles, (d) degraded-no-CS still serves Phase 1 surface but POST /api/devices is NOT mounted.
- **267 short tests pass** project-wide (matches the pre-Task-3 baseline; new integration tests honored -short skip).
- **All existing TestServe_RefusesV3 / AcceptsV4 / DegradedOnUnreachable / NoConfigSkipsProbe / AutoMigrate** still pass — INST-05 boot gate untouched.

## Task Commits

1. **Task 1 RED: failing tests for ConnectionStore + BigFloatFromNumeric** — `e13dbbe` (test)
2. **Task 1 GREEN: ConnectionStore + W5 numeric helpers** — `c322273` (feat)
3. **Task 2: cmd/serve full Phase 2 wiring** — `16a2692` (feat)
4. **Task 3: end-to-end TestServe_FullBoot_* integration tests** — `cd2e3ab` (test)

## Files Created/Modified

### Files created

- **`internal/cli/connection_store.go`** (92 LoC) — ConnectionStore struct, NewConnectionStore, GetCSConnection (pgx.ErrNoRows → empty, NULL → empty), SetCSTenantApp (refuses empty inputs), compile-time `_ chirpstack.ConnectionStore = (*ConnectionStore)(nil)` + `_ profile.ConnectionStore = (*ConnectionStore)(nil)` assertions.
- **`internal/cli/connection_store_test.go`** (100 LoC) — 4 tests (TestConnectionStore_GetEmpty, _RoundTrip, _NoRow, _SatisfiesBothInterfaces).
- **`internal/ingest/numeric.go`** (49 LoC) — exported `BigFloatFromNumeric(n pgtype.Numeric) *big.Float` (default 0 for invalid) + `BigFloatFromNumericNullable(n pgtype.Numeric) *big.Float` (returns nil for invalid).
- **`internal/ingest/numeric_test.go`** (67 LoC) — 2 tests (TestBigFloatFromNumeric_Roundtrip, TestBigFloatFromNumericNullable_HandlesNull).
- **`internal/cli/serve_fullboot_test.go`** (611 LoC) — fullBootHarness type + startFullBootHarness + 4 tests + helper functions (argon2HashForTest, base64Encode for hash format, seedFixture, seedAxiomaMappings, loginAdmin, authedPost, publishUplinkBlocking, waitForMeasurementCount, readBodyForTest).

### Files modified

- **`internal/cli/serve.go`** — package doc comment rewritten (Plan 18 → Plan 18 + Plan 02-12); imports extended (math/big, uuid, pgx, pgtype, pgxpool, sqlc, device, ingest, profile, resolver, swap); blocks 6a-6e inserted (ConnectionStore + CS Client + bootstrap + Resolver + ingest-binding + RunSeedSync); block 8 constructs DeviceDeps + SwapDeps + ProfileDeps conditionally; block 9 (Router) extended with the three new fields. Helpers at file end: sqlcResolverLoader (resolver.Loader implementation calling sqlc.GetActiveBindingByDevEUI + reusing ingest.BigFloatFromNumeric/Nullable) + bootstrapperFunc adapter.
- **`internal/ingest/handler.go`** — package-private `bigFloatFromNumeric` body removed; SQLCMappingStore.GetMappingsByProfile retains scale=1 default via `if r.Scale.Valid { scale = BigFloatFromNumeric(r.Scale) }` instead of the helper-internal default-to-1.

### W5 line ranges moved

- **From:** `internal/ingest/handler.go` lines 247-267 (the original `bigFloatFromNumeric` body, semantically "default to 1 for invalid").
- **To:** `internal/ingest/numeric.go` lines 18-39 (`BigFloatFromNumeric`, semantically "default to 0 for invalid"). The semantic shift is intentional: the generic helper has a generic default; the call site preserves the original "default to 1" via an explicit r.Scale.Valid check.

### Verification snippets

```bash
# Plan 02-15 reference removed
$ grep -F "Phase 2+ adds device handlers" internal/cli/serve.go
(no output — string is gone)

# Wiring acceptance criteria
$ for tok in "ingest.UplinkHandler" "mqttSub.SetUplinkHandler" "resolver.New" "go res.Run" "profile.RunSeedSync" "EnsureTenantAndApplication" "DeviceDeps:" "SwapDeps:" "ProfileDeps:" "ingest.BigFloatFromNumeric"; do echo -n "$tok: "; grep -cF "$tok" internal/cli/serve.go; done
ingest.UplinkHandler: 2
mqttSub.SetUplinkHandler: 1
resolver.New: 1
go res.Run: 1
profile.RunSeedSync: 3
EnsureTenantAndApplication: 5
DeviceDeps:: 1
SwapDeps:: 1
ProfileDeps:: 1
ingest.BigFloatFromNumeric: 3

# W5 fix landed
$ grep -F "func BigFloatFromNumeric(" internal/ingest/numeric.go
func BigFloatFromNumeric(n pgtype.Numeric) *big.Float {
$ grep -F "func bigFloatFromNumeric(" internal/ingest/handler.go
(no output — lowercase form removed)
$ grep -F "ingest.BigFloatFromNumeric" internal/cli/serve.go
(2 matches — loader uses both helpers, no inline duplication)

# 4 integration tests + 4 ConnectionStore tests + 2 BigFloat tests pass
$ go test ./internal/cli/... -run "TestServe_FullBoot|TestConnectionStore" -count=1 -timeout 240s
ok  github.com/shifter-io/shifter/internal/cli  ...

$ go test ./internal/ingest/... -run "TestBigFloat" -race -count=1
ok  github.com/shifter-io/shifter/internal/ingest  ...

# Project-wide regression
$ go test -short ./... -count=1
ok ... 267 tests passed in 23 packages
```

## Decisions Made

1. **BigFloatFromNumeric default for invalid → 0, not 1.** The "default to 1 for missing scale" semantic is mapping-specific; the generic helper doesn't carry it. The single existing caller (SQLCMappingStore) handles the 1-default explicitly at the call site. This makes future callers see the unbiased generic primitive.
2. **Test harness mirrors serve.go's wiring blocks (does NOT call RunE).** Direct RunE invocation requires env-var setup + a Dial-injection seam. The mirror approach uses the SAME exported helpers RunE calls — proves the wiring contract without the env-var dance. Documented as an explicit decision so a future RunE refactor adds the wiring block to BOTH places.
3. **No install_state seed in harness; admin user existence drives FirstRunGate.** Per migration 0004, post-Finish state is "row deleted." Harness creates an admin user, FirstRunGate's adminExists check passes, and /api/* requests reach the route handlers.
4. **CSRF X-Requested-With: shifter sent explicitly by harness POST helpers.** Auth handlers reject state-changing POST without it (RESEARCH §V13). The login + swap + degraded-device-POST tests all need it.
5. **Degraded device POST: assert NOT 200 (404/405/503 acceptable).** Exact code depends on chi's tree resolution when DeviceDeps == nil; the assertion accepts the equivalence class.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] BigFloatFromNumeric semantic mismatch with original**
- **Found during:** Task 1 GREEN (handler.go SQLCMappingStore call-site update)
- **Issue:** The original `bigFloatFromNumeric` returns `big.NewFloat(1)` for invalid (mapping-scale identity default). The plan's Test 5 specifies the new exported helper returns `big.NewFloat(0)` for invalid (generic). Naively replacing all in-package call sites with `BigFloatFromNumeric` would change the SQLCMappingStore.GetMappingsByProfile contract — every NULL scale would silently become 0 instead of identity (1), breaking every existing axioma_w1 + acrel_adw300 mapping.
- **Fix:** Updated SQLCMappingStore.GetMappingsByProfile to do an explicit `r.Scale.Valid` check; falls back to `big.NewFloat(1)` (identity) explicitly at the call site. The exported helper carries the generic "0 for invalid" default the plan asks for.
- **Files modified:** `internal/ingest/handler.go` line 230-241
- **Commit:** `c322273` (Task 1 GREEN)

**2. [Rule 1 - Bug] install_state column name mismatch**
- **Found during:** Task 3 first run of TestServe_FullBoot_*
- **Issue:** Plan psuedocode + my initial harness used `INSERT INTO install_state ... finished_at = now()`. The actual migration 0004 column is `completed_at`, and per the migration's contract "After 'Finish setup', the row is DELETED" — so the canonical post-install state is NO row at all. Inserting any install_state row was unnecessary AND used the wrong column.
- **Fix:** Removed the install_state INSERT entirely. FirstRunGate's adminExists query passes once the admin user is seeded; no install_state row needed.
- **Files modified:** `internal/cli/serve_fullboot_test.go` step 4 of harness
- **Commit:** `cd2e3ab` (Task 3)

**3. [Rule 1 - Bug] CSRF rejection on harness POSTs**
- **Found during:** Task 3 first run of TestServe_FullBoot_SwapEndpointMounted, _ProfileEndpointMounted, _DegradedMode_NoCS
- **Issue:** Plan psuedocode used `client.Post(...)` for login + swap. Auth handlers reject every state-changing POST without `X-Requested-With: shifter` (RESEARCH §V13 csrfHeaderPresent check); the response is `400 missing_csrf_header`.
- **Fix:** Added `loginAdmin` + `authedPost` harness helpers that build `http.Request` + `req.Header.Set("X-Requested-With", "shifter")`. Updated all POST call sites in the integration tests.
- **Files modified:** `internal/cli/serve_fullboot_test.go`
- **Commit:** `cd2e3ab` (Task 3)

**4. [Rule 1 - Bug] readBodyForTest body double-consumption**
- **Found during:** Task 3 first run of TestServe_FullBoot_ProfileEndpointMounted
- **Issue:** `require.Equal(t, 200, resp.StatusCode, "...; body=%s", readBodyForTest(resp))` evaluates the format args even when the assertion passes, so `readBodyForTest` is invoked AND drains resp.Body. The subsequent `body := readBodyForTest(resp)` then reads from a drained body and gets "". The test failed with empty body assertion even though the endpoint returned the seeded profiles.
- **Fix:** Read the body ONCE into a variable, then use it for both the status-message format and the substring assertion.
- **Files modified:** `internal/cli/serve_fullboot_test.go` TestServe_FullBoot_ProfileEndpointMounted
- **Commit:** `cd2e3ab` (Task 3)

### Notes on plan-text references

- Plan psuedocode for connection_store.go used `pgtype.UUID` for the cs_tenant_id / cs_application_id columns. The actual sqlc-generated types are `*string` (per migration 0011 column type TEXT). The implementation uses `*string` to match the generated SetChirpStackTenantAppParams shape.
- Plan referenced `csConn.Conn()` for a wrapped grpc connection — the actual `chirpstack.Dial` returns `*grpc.ClientConn` directly (no wrapper); the code passes the raw conn to `chirpstack.NewClient(grpcConn)`.
- Plan referenced existing `TestProbeChirpStackOrRefuse_*` tests; the actual existing tests are named `TestServe_RefusesV3 / _AcceptsV4 / _DegradedOnUnreachable / _NoConfigSkipsProbe / _AutoMigrate`. All 5 still present and passing.

**Total deviations:** 4 auto-fixed bugs (all surfaced during test execution; none changed the plan's intent).

## Authentication Gates Encountered

None during execution.

## Issues Encountered

- None beyond the auto-fixed deviations above.

## Self-Check: PASSED

Files claimed exist:

```
✓ internal/cli/connection_store.go            (92 LoC)
✓ internal/cli/connection_store_test.go       (100 LoC)
✓ internal/ingest/numeric.go                  (49 LoC)
✓ internal/ingest/numeric_test.go             (67 LoC)
✓ internal/cli/serve_fullboot_test.go         (611 LoC)
```

Modified files contain claimed changes:

```
✓ grep -F "ingest.UplinkHandler" internal/cli/serve.go        → 2 lines
✓ grep -F "mqttSub.SetUplinkHandler" internal/cli/serve.go    → 1 line
✓ grep -F "resolver.New" internal/cli/serve.go                → 1 line
✓ grep -F "go res.Run" internal/cli/serve.go                  → 1 line
✓ grep -F "profile.RunSeedSync" internal/cli/serve.go         → 3 lines (1 call + 2 doc lines)
✓ grep -F "EnsureTenantAndApplication" internal/cli/serve.go  → 5 lines
✓ grep -F "DeviceDeps:" internal/cli/serve.go                 → 1 line
✓ grep -F "SwapDeps:" internal/cli/serve.go                   → 1 line
✓ grep -F "ProfileDeps:" internal/cli/serve.go                → 1 line
✓ grep -F "ingest.BigFloatFromNumeric" internal/cli/serve.go  → 3 lines
✓ grep -F "Phase 2+ adds device handlers" internal/cli/serve.go → 0 lines (removed)
✓ grep -F "func bigFloatFromNumeric(" internal/ingest/handler.go → 0 lines (extracted)
```

Commits reachable:

```
✓ e13dbbe test(02-12): add failing tests for ConnectionStore + BigFloatFromNumeric
✓ c322273 feat(02-12): ConnectionStore adapter + W5 BigFloatFromNumeric extraction
✓ 16a2692 feat(02-12): cmd/serve full Phase 2 wiring (CS Client + Resolver + ingest + DeviceDeps + SwapDeps + ProfileDeps)
✓ cd2e3ab test(02-12): end-to-end full-boot integration tests for Phase 2 wiring
```

Build + vet + tests:

```
✓ go build ./...                                    → exit 0
✓ go vet ./...                                      → clean
✓ go test ./internal/cli/... -run TestServe_FullBoot -count=1   → 4/4 passed
✓ go test ./internal/cli/... -run TestConnectionStore -count=1  → 4/4 passed
✓ go test ./internal/ingest/... -run TestBigFloat -count=1      → 2/2 passed
✓ go test ./internal/ingest/... -count=1 (regression)           → 40/40 passed
✓ go test ./internal/cli/... -count=1 (full)                    → 25/25 passed (incl. 4 integration)
✓ go test -short ./... -count=1                                 → 267/267 passed in 23 packages
```

## Next Phase Readiness

- **VERIFICATION.md Truths 2 + 3 + 4 are now wired** in production. Truth 1 (operator-reachable frontend dialogs) was already shipped by Plan 02-14; the loop is closed: a deployed binary boot → admin opens the SPA → uses dialogs → POST /api/* → real handlers run → MQTT uplinks land in measurement → the dashboard chart in Phase 3 will render real data.
- **Plan 02-15** (next/last in Phase 2) covers reconciliation + sign-off documentation; no further wiring work expected.
- **Phase 3** (realtime ingest + SSE fan-out) inherits a fully-wired serve.go: the resolver + ingest + Postgres binding are live; SSE will subscribe to TimescaleDB notifications and push to the browser. No new composition root work needed.
- **W5 single-source-of-truth**: any future caller that needs `pgtype.Numeric → *big.Float` decoding imports `internal/ingest` and uses `BigFloatFromNumeric` / `BigFloatFromNumericNullable`. (Note: `internal/testharness/scenarios.go` also has a private `bigFloatFromNumeric` — separate package, separate concern, plan 02-12 W5 scope was ingest + cli only; testharness can be unified in a future cleanup.)

---
*Phase: 02-domain-model-canonical-schema*
*Completed: 2026-05-04*
