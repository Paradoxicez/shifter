---
phase: 07-multi-vendor-breadth-v1-x-differentiators
plan: "01"
subsystem: scaffolding
tags: [goja, codec, catalog, skeleton-tests, wave-0]
dependency_graph:
  requires: []
  provides:
    - github.com/dop251/goja in go.mod (Wave 2+ plans 07-07 depend on it)
    - internal/codec package with CatalogEntry struct (D-33 schema)
    - internal/codec_runner package with RunCodecTest stub
    - 9 skeleton test files for Wave 2+ plans to fill in
  affects:
    - go.mod / go.sum (new goja dependency)
    - internal/codec (new package)
    - internal/codec_runner (new package)
    - web/package.json (typecheck script added)
tech_stack:
  added:
    - github.com/dop251/goja v0.0.0-20260311135729-065cd970411c (JS runtime for codec execution)
    - github.com/dlclark/regexp2 v1.11.4 (goja transitive dep)
    - github.com/go-sourcemap/sourcemap v2.1.3 (goja transitive dep)
    - github.com/google/pprof v0.0.0-20230207041349-798e818bf904 (goja transitive dep)
  patterns:
    - //go:embed all:catalog — tolerates empty dirs (T-07-01-02 mitigation)
    - Wave 0 skeleton tests — t.Skip with plan reference for each future implementation
key_files:
  created:
    - go.mod (goja added)
    - go.sum (goja checksum)
    - internal/codec/catalog.go
    - internal/codec/catalog/.gitkeep
    - internal/codec_runner/runner.go
    - internal/codec/catalog_test.go
    - internal/codec_runner/runner_test.go
    - internal/profile/codecs/itron_kinmy_lora_test.go
    - web/src/components/codec-test-runner/CodecTestRunner.test.tsx
    - web/src/routes/settings/VendorCatalogCard.test.tsx
    - web/src/routes/reports/CompareView.test.tsx
    - web/playwright/specs/phase-07-vendor-catalog.spec.ts
  modified:
    - web/package.json (typecheck script added)
    - web/src/components/settings/BackupStatusCard.tsx (zod v4 coerce fix)
    - web/src/routes/audit/index.test.tsx (react-query v5 mock cast fix)
decisions:
  - "Use //go:embed all:catalog (not catalog/*.json) so the empty dir is buildable in Wave 0 before any JSON files land"
  - "goja pinned at pseudo-version v0.0.0-20260311135729-065cd970411c via go get @latest; checksum DB verifies integrity (T-07-01-01)"
metrics:
  duration: 6min
  completed_date: "2026-05-13"
  tasks_completed: 3
  files_changed: 13
---

# Phase 7 Plan 01: Wave 0 Scaffolding Summary

**One-liner:** goja JS runtime pinned at v0.0.0-20260311135729-065cd970411c; 9 skeleton test files created covering catalog, codec runner, Itron codec, vendor catalog UI, codec test runner, compare view, and Phase 7 E2E flows.

## What Was Built

### Task 1: goja dependency + catalog/codec_runner stubs (f4bcba7)

- `go get github.com/dop251/goja@latest` added goja at `v0.0.0-20260311135729-065cd970411c` plus 3 transitive deps
- `internal/codec/catalog.go`: `package codec` with `CatalogEntry` struct implementing the full D-33 schema (16 fields: Slug, Name, Vendor, Family, Capabilities, Version, CodecJSPath, CounterModulus, MACVersion, Region, ExpectedUplinkIntervalSeconds, OfflineThresholdMultiplier, AnomalyCompatibility, BatteryCurve, VendorHasSeparateMeterSerial, FPort) + `LoadAll()` stub returning `nil, nil`
- `internal/codec/catalog/.gitkeep`: empty file so `//go:embed all:catalog` is valid with an empty directory
- `internal/codec_runner/runner.go`: `package codec_runner` with `TestResult` struct (6 fields for decode output + error details) and `RunCodecTest(codecJS string, hexBytes []byte, fPort int) TestResult` stub returning error message "not yet implemented (plan 07-07)"
- `go build ./...` exits 0

### Task 2: Backend skeleton tests (fd9c519)

- `internal/codec/catalog_test.go`: `TestCatalogValid` skipped (plan 07-03)
- `internal/codec_runner/runner_test.go`: 4 tests skipped — `TestRunCodecTest_AxiomaW1`, `TestRunCodecTest_ItronKinmy`, `TestRunCodecTest_Timeout`, `TestRunCodecTest_SyntaxError` (plan 07-07)
- `internal/profile/codecs/itron_kinmy_lora_test.go`: `TestItronKinmyLoRa_GoldenVector` skipped with SOF=0x6F frame context (plan 07-09)
- `go test ./internal/codec/ ./internal/codec_runner/ ./internal/profile/codecs/ -count=1` exits 0

### Task 3: Frontend + Playwright skeleton tests (893b459)

- `web/src/components/codec-test-runner/CodecTestRunner.test.tsx`: 3 skipped tests for Surface 4 (plan 07-08)
- `web/src/routes/settings/VendorCatalogCard.test.tsx`: 3 skipped tests for Surface 1 (plan 07-05)
- `web/src/routes/reports/CompareView.test.tsx`: 2 skipped tests for Surface 5 (plan 07-12)
- `web/playwright/specs/phase-07-vendor-catalog.spec.ts`: 3 skipped E2E flows (plan 07-14)
- `web/package.json`: `typecheck` script added (`tsc --noEmit`)
- `pnpm --dir web typecheck` exits 0
- `pnpm --dir web test:run` exits 0 (378 pass, 8 skipped including new skeletons)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical Functionality] Added `typecheck` script to web/package.json**
- **Found during:** Task 3 verification
- **Issue:** Plan acceptance criteria references `pnpm --dir web typecheck` but the script did not exist in package.json
- **Fix:** Added `"typecheck": "tsc --noEmit"` to scripts
- **Files modified:** `web/package.json`
- **Commit:** 893b459

**2. [Rule 1 - Bug] Fixed zod v4 type error in BackupStatusCard.tsx**
- **Found during:** Task 3 — typecheck revealed pre-existing error from Plan 06-10
- **Issue:** `z.coerce.number()` in zod v4 infers `unknown` input type, breaking `zodResolver` type compatibility with `react-hook-form`
- **Fix:** Changed `z.coerce.number()` to `z.number()` and added `{ valueAsNumber: true }` to `register()` calls for numeric inputs
- **Files modified:** `web/src/components/settings/BackupStatusCard.tsx`
- **Commit:** 893b459

**3. [Rule 1 - Bug] Fixed react-query v5 mock cast in audit/index.test.tsx**
- **Found during:** Task 3 — typecheck revealed pre-existing error from Plan 06-07
- **Issue:** `UseMutationResult` in `@tanstack/react-query` v5 requires many more fields than `{ mutate, isPending }` — direct cast failed type narrowing
- **Fix:** Added `as unknown as ReturnType<typeof useExportAuditAsync>` double cast (the idiomatic pattern for partial mocks)
- **Files modified:** `web/src/routes/audit/index.test.tsx`
- **Commit:** 893b459

## Known Stubs

| File | Line | Stub | Resolving Plan |
|------|------|------|----------------|
| `internal/codec/catalog.go` | 33 | `LoadAll()` returns `nil, nil` | Plan 07-03 |
| `internal/codec_runner/runner.go` | 21 | `RunCodecTest` returns stub error message | Plan 07-07 |

These stubs are intentional Wave 0 scaffolding — they do not prevent this plan's goal (goja in module graph, skeleton test files present) from being achieved.

## Threat Surface Scan

No new network endpoints, auth paths, file access patterns, or schema changes at trust boundaries were introduced. The `internal/codec` and `internal/codec_runner` packages are pure Go library packages with no HTTP handlers, database access, or external network calls.

## Self-Check: PASSED

All 11 created/modified files verified on disk. All 3 task commits verified in git log:
- f4bcba7: feat(07-01) goja + catalog/codec_runner stubs
- fd9c519: test(07-01) backend skeleton tests
- 893b459: feat(07-01) frontend + playwright skeleton tests
