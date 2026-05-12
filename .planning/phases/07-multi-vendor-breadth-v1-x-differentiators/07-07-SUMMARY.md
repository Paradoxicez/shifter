---
phase: 07-multi-vendor-breadth-v1-x-differentiators
plan: "07"
subsystem: codec-runner
tags: [goja, sandbox, codec, security, http-handler, rbac, rate-limit]
dependency_graph:
  requires:
    - phase: 07-multi-vendor-breadth-v1-x-differentiators
      plan: "01"
      provides: "internal/codec_runner package stub + goja in go.mod"
    - phase: 07-multi-vendor-breadth-v1-x-differentiators
      plan: "02"
      provides: "device_profile schema with codec_js column + sqlc queries"
  provides:
    - internal/codec_runner/runner.go: RunCodecTest with sandbox + timeout + stack cap
    - internal/codec_runner/sandbox.go: newSandboxRuntime() with no host bindings
    - internal/api/codec_test_handler.go: POST /api/device-profiles/{id}/test-codec
    - internal/auth/authz.go: ActionCodecTestRun admin-only action
    - internal/db/sqlc: GetProfileForCodecTest query
  affects:
    - plan 07-08 (frontend CodecTestRunner panel calls this endpoint)
tech_stack:
  added: []
  patterns:
    - "goja.Runtime per-call (no shared state) — T-07-07-09 mitigation"
    - "time.AfterFunc + vm.Interrupt for timeout — T-07-07-01 mitigation"
    - "vm.SetMaxCallStackSize(500) — T-07-07-02 mitigation"
    - "No host bindings in newSandboxRuntime — T-07-07-03 mitigation"
    - "token-bucket rate limit via golang.org/x/time/rate — T-07-07-05 mitigation"
    - "nil-guard pattern on CodecTestDeps in router — matches DeviceDeps/ProfileDeps pattern"
key_files:
  created:
    - internal/codec_runner/runner.go
    - internal/codec_runner/sandbox.go
    - internal/codec_runner/sandbox_test.go
    - internal/codec_runner/testdata/axioma_w1.js
    - internal/codec_runner/testdata/itron_kinmy_lora.js
    - internal/api/codec_test_handler.go
    - internal/api/codec_test_handler_test.go
    - internal/api/testdata/axioma_w1.js
  modified:
    - internal/codec_runner/runner_test.go (replaced 4 t.Skip stubs with real tests)
    - internal/auth/authz.go (ActionCodecTestRun added)
    - internal/auth/authz_test.go (TestAuthz_CodecTestRun_AdminOnly added)
    - internal/db/queries/device_profiles.sql (GetProfileForCodecTest appended)
    - internal/db/sqlc/device_profiles.sql.go (regenerated)
    - internal/db/sqlc/querier.go (regenerated)
    - internal/http/router.go (CodecTestDeps field + RegisterCodecTestRoute mount)
key_decisions:
  - "Handler placed in internal/api package (not internal/profile) to avoid import cycle: profile imports ingest, ingest imports profile — internal/api imports both without cycle"
  - "syntaxLocationRE added to parse 'Line N:M' format for goja compile-time SyntaxErrors (goja uses different format than runtime errors)"
  - "codec_js seeded in integration tests via direct SQL UPDATE (not via seed.go bootstrap which requires live ChirpStack)"
  - "Route mounted as /api/device-profiles/{id}/test-codec (consistent with existing profile route prefix) not /api/profiles/{id}/test-codec"
  - "No audit row for test-codec (D-08 scratch-pad semantics: admin-only + rate-limit provide accountability)"
requirements-completed: [V2-VEND-02]
metrics:
  duration: 16min
  completed_date: "2026-05-13"
  tasks_completed: 2
  files_changed: 15
---

# Phase 7 Plan 07: Goja Codec Runner Summary

**One-liner:** goja sandbox with 100ms timeout + 500-frame stack cap + no host bindings; POST /api/device-profiles/{id}/test-codec admin-only behind per-user 30/min rate limit returning decoded JSON + canonical Layer1 mapping.

## What Was Built

### Task 1: sandbox + RunCodecTest (0b1c9aa)

- `internal/codec_runner/sandbox.go`: `newSandboxRuntime()` creates a fresh `*goja.Runtime` per call (T-07-07-09) with `SetMaxCallStackSize(500)` (T-07-07-02). No `require`, `os`, `fs`, `process`, `setTimeout`, `fetch`, or `WebAssembly` bindings exposed (T-07-07-03).
- `internal/codec_runner/runner.go`: `RunCodecTest(codecJS, hexBytes, fPort)` — 256-byte payload cap (T-07-07-06), `time.AfterFunc(100ms, vm.Interrupt)` (T-07-07-01), `decodeUplink({bytes:[...int...], fPort})` invocation, `gojaErrToResult` handles `*goja.Exception` and `*goja.InterruptedError`. `syntaxLocationRE` parses `"Line N:M"` format for compile-time syntax errors.
- **13 tests pass**: 4 runner (AxiomaW1, ItronKinmy, Timeout, SyntaxError) + 3 sandbox (NoRequire, NoStdlib, OversizedPayload).

### Task 2: HTTP handler + RBAC + sqlc query (7d4900e)

- `ActionCodecTestRun Action = "codec.test_run"` added to `internal/auth/authz.go` (admin bundle only — T-07-07-08).
- `GetProfileForCodecTest :one` sqlc query returns `id, slug, codec_js` for the handler. Regenerated bindings in `device_profiles.sql.go` and `querier.go`.
- `internal/api/codec_test_handler.go`: hex cleaning (whitespace/colon tolerant), 256B cap, `GetProfileForCodecTest` load, `RunCodecTest` sandbox call, `ingest.NormalizeMeasurement` for canonical mapping (non-fatal on failure), no audit row (D-08). Per-user 30/min token-bucket via `golang.org/x/time/rate`.
- `internal/http/router.go`: `CodecTestDeps *apipkg.CodecTestDeps` field + nil-guard mount pattern matching DeviceDeps/ProfileDeps.
- **7 handler tests pass**: AxiomaHappyPath (200+decoded_json), OversizedPayload (400), UnknownProfile (404), ViewerForbidden (403), TimeoutCodec (200+error_message), InvalidHex (400), NoAuditWritten (audit count unchanged).
- **1 authz test added**: `TestAuthz_CodecTestRun_AdminOnly` — admin true, viewer false.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] syntaxLocationRE for goja compile-time SyntaxErrors**
- **Found during:** Task 1 TestRunCodecTest_SyntaxError (RED → first GREEN attempt)
- **Issue:** goja reports syntax errors as `"SyntaxError: ...: Line 1:41 Unexpected end of input"` (not the `"at ...:line:col"` runtime stack format). The plan's `stackLocationRE` regex didn't match, leaving ErrorLine=0 and ErrorCol=0.
- **Fix:** Added `syntaxLocationRE = regexp.MustCompile("Line (\\d+):(\\d+)")` and a fallback parse path in `gojaErrToResult`. Tests now confirm ErrorLine > 0 or ErrorCol > 0 for syntax errors.
- **Files modified:** `internal/codec_runner/runner.go`
- **Commit:** 0b1c9aa

**2. [Rule 1 - Bug] Handler placed in internal/api (not internal/profile)**
- **Found during:** Task 2 initial handler placement attempt
- **Issue:** Placing the handler in `internal/profile` created an import cycle: `profile` → `ingest` → `profile`. The plan suggested `internal/api/codec_test_handler.go` which is exactly the right package to import both without cycle.
- **Fix:** Followed the plan's file path spec (`internal/api/codec_test_handler.go`). Created production Go file in `internal/api` (previously test-only package).
- **Files modified:** `internal/api/codec_test_handler.go`
- **Commit:** 7d4900e

**3. [Rule 2 - Missing Critical] codec_js seeded in integration tests via SQL UPDATE**
- **Found during:** Task 2 TestCodecTestHandler_AxiomaHappyPath
- **Issue:** Migration `0010_seed_profiles.up.sql` intentionally leaves `codec_js = ''` for all seeded profiles (filled at runtime boot by `seed.go` → ChirpStack push). In tests without a live ChirpStack, `codec_js` is empty → `decodeUplink is not a function`.
- **Fix:** Added `pool.Exec(...UPDATE device_profile SET codec_js = $1 WHERE slug = 'axioma_w1'...)` using the embedded `axiomaCodecJS` in tests that need happy-path decoding.
- **Files modified:** `internal/api/codec_test_handler_test.go`
- **Commit:** 7d4900e

**4. [Rule 1 - Bug] counter_modulus=0 violated CHECK constraint**
- **Found during:** Task 2 TestCodecTestHandler_TimeoutCodec
- **Issue:** Test INSERT used `counter_modulus = 0` but the schema has `CHECK (counter_modulus > 0)`.
- **Fix:** Changed to `counter_modulus = 1`.
- **Files modified:** `internal/api/codec_test_handler_test.go`
- **Commit:** 7d4900e

**5. [Rule 1 - Deviation] Route prefix /api/device-profiles/{id}/test-codec (not /api/profiles/{id}/test-codec)**
- **Found during:** Task 2 route mounting
- **Issue:** Plan frontmatter says `/api/profiles/{id}/test-codec` but plan body says `POST /api/device-profiles/{id}/test-codec` and all existing profile routes use `/api/device-profiles/`. Using `/api/profiles/` would diverge from established conventions.
- **Fix:** Used `/api/device-profiles/{id}/test-codec` (consistent with existing profile surface).
- **Impact:** Frontend (plan 07-08) must use `/api/device-profiles/{id}/test-codec`.

## Known Stubs

None — all stubs from plan 07-01 in `internal/codec_runner/runner.go` are replaced with real implementations.

## Threat Surface Scan

| Flag | File | Description |
|------|------|-------------|
| threat_flag: operator-js-execution | `internal/api/codec_test_handler.go` | POST endpoint executes operator-supplied JS from device_profile.codec_js via goja. All T-07-07-01..09 mitigations applied: timeout, stack cap, no host bindings, payload cap, per-user rate limit, fresh runtime per call, admin-only RBAC. |

## Self-Check: PASSED

Files verified on disk:
- internal/codec_runner/runner.go ✓
- internal/codec_runner/sandbox.go ✓
- internal/codec_runner/runner_test.go ✓
- internal/codec_runner/sandbox_test.go ✓
- internal/api/codec_test_handler.go ✓
- internal/api/codec_test_handler_test.go ✓
- internal/auth/authz.go (ActionCodecTestRun) ✓
- internal/db/sqlc/device_profiles.sql.go (GetProfileForCodecTest) ✓
- internal/http/router.go (RegisterCodecTestRoute) ✓

Commits verified:
- 0b1c9aa: feat(07-07): implement goja sandbox + RunCodecTest with timeout and stack cap
- 7d4900e: feat(07-07): POST /api/device-profiles/{id}/test-codec handler + RBAC + sqlc query

Test results:
- `go test ./internal/codec_runner/... -race -count=1`: 13 passed
- `go test ./internal/auth/... -count=1`: 85 passed
- `go test ./internal/api/... -run TestCodecTestHandler -count=1`: 7 passed
- `go build ./...`: success
