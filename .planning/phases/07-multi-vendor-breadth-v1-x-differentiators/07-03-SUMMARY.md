---
phase: 07-multi-vendor-breadth-v1-x-differentiators
plan: "03"
subsystem: codec-catalog
tags: [catalog, drift-detection, boot, embed, timescaledb, tdd]
dependency_graph:
  requires:
    - phase: 07-multi-vendor-breadth-v1-x-differentiators
      plan: "01"
      provides: "CatalogEntry struct, embed.FS, catalog/ dir"
    - phase: 07-multi-vendor-breadth-v1-x-differentiators
      plan: "02"
      provides: "4 catalog JSON files, migration 0050, sqlc queries (MarkProfileCustomerEdited)"
  provides:
    - LoadAll() ([]CatalogEntry, error) — reads 4 catalog JSON files from embed.FS
    - Get(slug string) (CatalogEntry, error) + ErrCatalogEntryNotFound sentinel
    - TestCatalogValid — build-time D-22/D-33 schema validation
    - RunCatalogDriftCheck(ctx, pool) — D-31 boot-time codec hash comparison
    - ListCatalogProfilesForDriftCheck sqlc query
    - OverwriteProfileCodec sqlc query
    - Boot wiring in internal/cli/serve.go before RunSeedSync
  affects:
    - plan 07-04 (catalog HTTP API calls LoadAll, Get)
    - plan 07-05 (vendor catalog UI reads catalog entries via HTTP API)
    - all downstream plans that assume Itron+KINMY codec_js is populated on first boot
tech_stack:
  added:
    - golang.org/x/mod v0.36.0 (semver validation in TestCatalogValid)
  patterns:
    - TDD RED→GREEN for both catalog loader and drift check
    - embed.FS ReadDir + ReadFile loop for catalog JSON loading
    - sha256 hash comparison for drift detection (normalizeLines for CRLF safety)
    - placeholderCodecMarker sentinel string for migration placeholder detection
    - Best-effort boot call pattern (errors logged, never block boot)
key_files:
  created:
    - internal/codec/catalog.go (LoadAll, Get, ErrCatalogEntryNotFound)
    - internal/codec/drift.go (RunCatalogDriftCheck, normalizeLines)
    - internal/codec/drift_test.go (3 integration tests)
  modified:
    - internal/codec/catalog_test.go (full TestCatalogValid + TestGet_NotFound replacing skeleton)
    - internal/db/queries/device_profiles.sql (2 new queries: ListCatalogProfilesForDriftCheck, OverwriteProfileCodec)
    - internal/db/sqlc/device_profiles.sql.go (sqlc regenerated)
    - internal/db/sqlc/querier.go (2 new interface methods)
    - internal/cli/serve.go (codec import + RunCatalogDriftCheck call before RunSeedSync)
    - internal/profile/seed.go (boot-order documentation comment)
    - go.mod (golang.org/x/mod v0.36.0 added as direct dep)
    - go.sum (updated)
decisions:
  - "golang.org/x/mod/semver used for version validation in TestCatalogValid — already an indirect dep, promoted to direct"
  - "drift_test.go uses package codec_test (black-box) so RunCatalogDriftCheck called with codec. prefix"
  - "RunCatalogDriftCheck placed before the csClient nil-guard in serve.go — drift check only needs the pool, not CS"
  - "placeholderCodecMarker uses prefix match (HasPrefix) so the string does not need to be exactly equal to the migration value"
metrics:
  duration: 6min
  completed_date: "2026-05-13"
  tasks_completed: 3
  files_changed: 10
---

# Phase 7 Plan 03: Catalog Loader and Drift Detection Summary

**One-liner:** Catalog Go API (LoadAll/Get) + build-time D-22 schema validation via TestCatalogValid + boot-time D-31 drift check with 3 branches (match/operator-edit/placeholder) wired before RunSeedSync.

## What Was Built

### Task 1: LoadAll, Get, ErrCatalogEntryNotFound + TestCatalogValid (TDD)

**RED commit:** `bc4ce5b` — `test(07-03): add failing TestCatalogValid + TestGet_NotFound`
**GREEN commit:** `196b69a` — `feat(07-03): implement LoadAll, Get, ErrCatalogEntryNotFound`

- `internal/codec/catalog.go` — replaced nil-nil stub with real `embed.FS` `ReadDir`+`ReadFile` loop; entries sorted vendor-then-family; `ErrCatalogEntryNotFound` sentinel error
- `internal/codec/catalog_test.go` — full `TestCatalogValid`: asserts 4 entries, slug uniqueness, lowercase slug, valid semver (via `golang.org/x/mod/semver`), enum membership for `battery_curve` + `anomaly_compatibility`, valid capabilities, and `codecs.CodecBySlug` resolves for each entry (D-22)
- `go.mod` — `golang.org/x/mod v0.36.0` promoted from indirect to direct dep
- 6 tests pass: `TestCatalogValid/axioma_w1`, `TestCatalogValid/acrel_adl200`, `TestCatalogValid/acrel_adw300`, `TestCatalogValid/itron_kinmy_lora`, overall count assertion, `TestGet_NotFound`

### Task 2: RunCatalogDriftCheck with hash comparison (TDD)

**RED commit:** `f3278b0` — `test(07-03): add failing TestDriftCheck_* tests + 2 new sqlc queries`
**GREEN commit:** `cc8497c` — `feat(07-03): implement RunCatalogDriftCheck with 3-branch logic`

SQL queries added to `internal/db/queries/device_profiles.sql`:
- `ListCatalogProfilesForDriftCheck :many` — `WHERE catalog_source IS NOT NULL AND customer_edited = FALSE`
- `OverwriteProfileCodec :exec` — replaces `codec_js`, clears `codec_js_synced_at = NULL`

`internal/codec/drift.go`:
- `RunCatalogDriftCheck(ctx, pool)` — iterates rows, computes `sha256` of normalized codec strings
- Branch 1: hashes match → no-op
- Branch 2: `codec_js` starts with `placeholderCodecMarker` → `OverwriteProfileCodec` (replaces with embedded source, clears synced_at)
- Branch 3: hashes differ (no placeholder) → `MarkProfileCustomerEdited` (operator edited)
- `normalizeLines()` normalizes CRLF → LF before hashing for platform safety

`internal/codec/drift_test.go` — 3 integration tests against TimescaleDB testcontainer:
- `TestDriftCheck_HashesMatch_NoChange` — matching hashes leave `customer_edited = FALSE`
- `TestDriftCheck_HashesDiffer_MarksCustomerEdited` — operator edit flips flag, codec_js preserved
- `TestDriftCheck_PlaceholderMarker_OverwritesCodec` — placeholder replaced with embedded source, `customer_edited` stays FALSE, `codec_js_synced_at` is NULL

### Task 3: Wire RunCatalogDriftCheck into boot sequence

**Commit:** `5c3f7e3` — `feat(07-03): wire RunCatalogDriftCheck into boot sequence before RunSeedSync`

- `internal/cli/serve.go` step 6e: `codec.RunCatalogDriftCheck(ctx, pool)` inserted before `profile.RunSeedSync` (line 221 < line 236)
- `codec` import added to serve.go
- Errors logged via `slog.Error` but never block boot — best-effort per D-31
- `internal/profile/seed.go` — boot-order comment added explaining why drift check must run first (prevents placeholder being pushed to ChirpStack)

## Task Commits

1. **Task 1 RED** — `bc4ce5b` (test)
2. **Task 1 GREEN** — `196b69a` (feat)
3. **Task 2 RED** — `f3278b0` (test)
4. **Task 2 GREEN** — `cc8497c` (feat)
5. **Task 3** — `5c3f7e3` (feat)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] drift_test.go called RunCatalogDriftCheck without package qualifier**
- **Found during:** Task 2 GREEN verification
- **Issue:** Test file used `package codec_test` (black-box) but called `RunCatalogDriftCheck` without the `codec.` prefix, causing build failure
- **Fix:** Added `"github.com/shifter-io/shifter/internal/codec"` import and prefixed all 3 calls with `codec.`
- **Files modified:** `internal/codec/drift_test.go`
- **Commit:** `cc8497c`

**2. [Rule 3 - Blocking Issue] Plan references cmd/serve/main.go but actual boot file is internal/cli/serve.go**
- **Found during:** Task 3 read-first phase
- **Issue:** The plan says to edit `cmd/serve/main.go` but the project has `cmd/shifter/main.go` (2 lines, delegates to `cli.Execute()`). The actual boot sequence lives in `internal/cli/serve.go`
- **Fix:** Applied wiring to `internal/cli/serve.go` — the correct file
- **Files modified:** `internal/cli/serve.go`
- **Commit:** `5c3f7e3`

## Known Stubs

None — all stubs from plans 07-01/07-02 that this plan was responsible for resolving are now wired:
- `LoadAll()` stub replaced with real implementation
- `GetProfileForCodecTest` sqlc query existed (plan 07-07 pre-wired it)

## Threat Surface Scan

No new network endpoints or auth paths introduced. `RunCatalogDriftCheck` writes to `device_profile` at boot time (trust boundary documented in plan threat model T-07-03-01/02). T-07-03-01 is mitigated: the overwrite branch only fires when `codec_js` starts with the specific placeholder marker; `TestDriftCheck_HashesDiffer_MarksCustomerEdited` proves operator edits flip the flag instead of being overwritten.

## Self-Check: PASSED

Files exist on disk:
- internal/codec/catalog.go ✓
- internal/codec/drift.go ✓
- internal/codec/catalog_test.go ✓
- internal/codec/drift_test.go ✓
- internal/db/queries/device_profiles.sql (2 queries appended) ✓
- internal/db/sqlc/device_profiles.sql.go (regenerated) ✓

Commits verified in git log:
- bc4ce5b: test(07-03): add failing TestCatalogValid + TestGet_NotFound (RED)
- 196b69a: feat(07-03): implement LoadAll, Get, ErrCatalogEntryNotFound (GREEN)
- f3278b0: test(07-03): add failing TestDriftCheck_* tests + 2 new sqlc queries (RED)
- cc8497c: feat(07-03): implement RunCatalogDriftCheck with 3-branch logic (GREEN)
- 5c3f7e3: feat(07-03): wire RunCatalogDriftCheck into boot sequence before RunSeedSync

All 9 tests in `./internal/codec/...` pass. `go build ./...` exits 0.
