---
phase: 07-multi-vendor-breadth-v1-x-differentiators
plan: "02"
subsystem: database
tags: [migration, sqlc, catalog, device-profile, timescaledb, postgres]
dependency_graph:
  requires:
    - phase: 07-multi-vendor-breadth-v1-x-differentiators
      plan: "01"
      provides: "CatalogEntry struct with D-33 schema, embed.FS, catalog/ dir"
  provides:
    - migration 0050_catalog_metadata (up + down) with 7 new device_profile columns
    - CHECK constraints for anomaly_compatibility and battery_curve
    - Itron+KINMY device_profile seed row inserted with ON CONFLICT DO NOTHING
    - 4 catalog JSON files at internal/codec/catalog/*.json
    - 6 new sqlc queries for catalog read/update/create (plan 07-03 and 07-04 deps)
  affects:
    - plan 07-03 (catalog loader reads JSON files + uses SetProfileCatalogSource)
    - plan 07-04 (ImportFromCatalogHandler uses CreateDeviceProfileFromCatalog)
    - plan 07-09b (profile-aware alerts read expected_uplink_interval_seconds + offline_threshold_multiplier)
tech_stack:
  added: []
  patterns:
    - "ADD COLUMN IF NOT EXISTS with DEFAULT — safe idempotent schema extension (Postgres 11+)"
    - "ON CONFLICT (slug) DO NOTHING — idempotent seed row insertion pattern"
    - "CHECK constraints added after INSERT to avoid ordering issues with seed data"
    - "codec_js placeholder in migration seed row — replaced at boot by RunCatalogSeedSync (plan 07-03)"
key_files:
  created:
    - internal/db/migrations/0050_catalog_metadata.up.sql
    - internal/db/migrations/0050_catalog_metadata.down.sql
    - internal/codec/catalog/axioma_w1.json
    - internal/codec/catalog/acrel_adl200.json
    - internal/codec/catalog/acrel_adw300.json
    - internal/codec/catalog/itron_kinmy_lora.json
  modified:
    - internal/db/queries/device_profiles.sql (6 new queries appended)
    - internal/db/sqlc/device_profiles.sql.go (sqlc regenerated)
    - internal/db/sqlc/models.go (DeviceProfile struct gains 7 fields)
    - internal/db/sqlc/querier.go (6 new interface methods)
    - internal/db/migrations_test.go (migration step counts corrected)
    - internal/db/roundtrip_test.go (version 49→50, seed count 3→4)
key_decisions:
  - "CHECK constraints added AFTER the Itron+KINMY INSERT in migration — ensures seed row passes constraints without ordering the INSERT's values relative to the constraint declaration"
  - "codec_js placeholder in Itron+KINMY seed row — avoids embedding 4KB JS in a SQL migration; plan 07-03 RunCatalogSeedSync replaces at boot via the same path as other profiles"
  - "Migration step counts in tests corrected to actually land at target versions — previous counts (since v45) were off and tests were silently failing; documented in deferred-items.md, fixed here as part of Rule 1 deviation"
requirements-completed: [V2-VEND-01, ALERT-04]
duration: 17min
completed: "2026-05-13"
---

# Phase 7 Plan 02: Migration and Catalog JSON Summary

**Migration 0050 adds 7 catalog-metadata columns to device_profile with CHECK constraints; 4 catalog JSON files embedded for the vendor catalog; 6 sqlc queries wired for plan 07-03/07-04 consumption.**

## Performance

- **Duration:** 17 min
- **Started:** 2026-05-12T17:07:45Z
- **Completed:** 2026-05-12T17:25:00Z
- **Tasks:** 3
- **Files modified:** 10

## Accomplishments

- 4 catalog JSON files (axioma_w1, acrel_adl200, acrel_adw300, itron_kinmy_lora) created at `internal/codec/catalog/` — embed.FS from plan 07-01 now has data to load
- Migration 0050 adds 7 device_profile columns (catalog_source, catalog_source_version, customer_edited, battery_curve, expected_uplink_interval_seconds, offline_threshold_multiplier, anomaly_compatibility), backfills 3 existing seeds, inserts Itron+KINMY row, and enforces CHECK constraints
- 6 new sqlc queries (ListProfilesWithCatalogMetadata, GetProfileCatalogMetadata, SetProfileCatalogSource, MarkProfileCustomerEdited, ApplyCatalogUpdate, CreateDeviceProfileFromCatalog) satisfy plan 07-03 and 07-04 dependencies
- All 32 db tests pass including full migration round-trip, CAGG drop/re-apply, River down/up cycle, and all Phase 3 migration-down assertions

## Task Commits

1. **Task 1: 4 catalog JSON files** - `7acf494` (feat)
2. **Task 2: Migration 0050 + test fixes** - `c6f3156` (feat)
3. **Task 3: 6 sqlc queries + regenerated bindings** - `7bd0572` (feat)

## Files Created/Modified

- `internal/codec/catalog/axioma_w1.json` — Axioma Qalcosonic W1 v1.0.0, linear_pct battery, full anomaly compat
- `internal/codec/catalog/acrel_adl200.json` — Acrel ADL200 v1.0.0, shared acrel_family.js, full anomaly compat
- `internal/codec/catalog/acrel_adw300.json` — Acrel ADW300 v1.0.0, shared acrel_family.js, full anomaly compat
- `internal/codec/catalog/itron_kinmy_lora.json` — Itron KINMY v1.0.0, li_socl2_3v6 battery, limited anomaly compat, vendor_has_separate_meter_serial=true
- `internal/db/migrations/0050_catalog_metadata.up.sql` — 7 ADD COLUMN IF NOT EXISTS + 3 seed backfills + Itron INSERT + 2 CHECK constraints
- `internal/db/migrations/0050_catalog_metadata.down.sql` — reverses constraints + Itron DELETE + 7 DROP COLUMN IF EXISTS
- `internal/db/queries/device_profiles.sql` — 6 new queries appended
- `internal/db/sqlc/device_profiles.sql.go` — sqlc regenerated with new queries + expanded DeviceProfile scan
- `internal/db/sqlc/models.go` — DeviceProfile struct gains 7 new fields
- `internal/db/sqlc/querier.go` — 6 new interface methods added
- `internal/db/migrations_test.go` — migration step counts corrected (Rule 1 deviation)
- `internal/db/roundtrip_test.go` — version 49→50, seed count 3→4

## Decisions Made

- CHECK constraints for `anomaly_compatibility` and `battery_curve` placed AFTER the Itron+KINMY INSERT in the migration — avoids having to declare the INSERT before the constraint while still enforcing correctness on all subsequent operations
- `codec_js` in the Itron+KINMY seed row uses a placeholder comment string — plan 07-03 RunCatalogSeedSync replaces it at boot via the same seed.go mechanism used for the other 3 profiles, keeping codec JS out of SQL
- `CreateDeviceProfileFromCatalog` uses 15 positional parameters with explicit column list — avoids ambiguity for sqlc and makes the param-to-column mapping visible to future readers

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Corrected migration test step counts (known-broken since v45)**
- **Found during:** Task 2 verification (`go test ./internal/db/...`)
- **Issue:** 6 migration tests (TestPhase3Migrations_0018/0019/0020_Down, TestRunMigrations_RiverDownUpClean, TestRunMigrations_CAGGsDropClean, TestRunMigrations_RoundTrip) were failing because their step counts and version assertions were stale. The NOTE comment in `migrations_test.go` explicitly documented the tests as "already failing on main prior to Plan 06-01." Adding 0050 exposed them further.
- **Fix:** Computed correct step counts from current migration chain (49 files, gap at 0041): 0018_Down needs -32 steps to reach v17, 0019_Down needs -31, 0020_Down needs -30, RiverDownUpClean needs -26/-26, CAGGsDropClean needs -25/-25. Updated version assertions (49→50, 45→50). Also corrected the final-version check in CAGGsDropClean (45→50) and extended roundtrip_test.go seed count assertion to include itron_kinmy_lora (3→4).
- **Files modified:** `internal/db/migrations_test.go`, `internal/db/roundtrip_test.go`
- **Verification:** All 32 db tests pass
- **Committed in:** c6f3156 (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (Rule 1 — bug fix for stale test assertions)
**Impact on plan:** Required to satisfy the `go test ./internal/db/... -count=1` acceptance criterion. No scope creep.

## Issues Encountered

None beyond the stale migration test step counts described above.

## Known Stubs

| File | Line | Stub | Resolving Plan |
|------|------|------|----------------|
| `internal/db/migrations/0050_catalog_metadata.up.sql` | Itron INSERT | `codec_js = '// placeholder — replaced at boot by RunCatalogSeedSync (plan 07-03)'` | Plan 07-03 |

The stub is intentional — the migration seed row exists so the schema is complete and plan 07-04 can import against it. Plan 07-03 RunCatalogSeedSync replaces codec_js at boot via the same path used for axioma_w1/acrel profiles.

## Threat Surface Scan

No new network endpoints, auth paths, or file access patterns introduced. The migration touches `device_profile` at deploy time (trust boundary documented in plan threat model T-07-02-01/03). The 4 catalog JSON files contain only public vendor information (T-07-02-04 accepted).

## Self-Check: PASSED

Files created/exist on disk:
- internal/db/migrations/0050_catalog_metadata.up.sql ✓
- internal/db/migrations/0050_catalog_metadata.down.sql ✓
- internal/codec/catalog/axioma_w1.json ✓
- internal/codec/catalog/acrel_adl200.json ✓
- internal/codec/catalog/acrel_adw300.json ✓
- internal/codec/catalog/itron_kinmy_lora.json ✓
- internal/db/queries/device_profiles.sql (6 queries appended) ✓
- internal/db/sqlc/device_profiles.sql.go (regenerated) ✓

Commits verified in git log:
- 7acf494: feat(07-02): add 4 embedded catalog JSON files
- c6f3156: feat(07-02): migration 0050 catalog metadata + Itron seed row + test fixes
- 7bd0572: feat(07-02): add 6 catalog sqlc queries + regenerate bindings

All 32 db tests pass. `sqlc generate` exits 0. `go build ./...` exits 0.
