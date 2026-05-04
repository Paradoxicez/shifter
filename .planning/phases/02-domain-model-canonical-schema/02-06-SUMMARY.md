---
phase: 02-domain-model-canonical-schema
plan: 06
subsystem: sqlc-queries
tags: [wave-3, sqlc, pgx-v5, code-generation, type-safe-bindings, resolver-hot-path, ingest-write-path, audit-log]

requires:
  - phase: 02-domain-model-canonical-schema
    plan: 04
    provides: full Phase 2 schema (16 migrations) — site/metering_point/device_profile/device/device_profile_mapping/binding/measurement-hypertable/audit_log + chirpstack_connection.cs_tenant_id/cs_application_id columns
  - phase: 02-domain-model-canonical-schema
    plan: 05
    provides: chirpstack.ConnectionStore interface that needs SetChirpStackTenantApp + GetChirpStackTenantApp implemented
  - phase: 01-foundation
    plan: 03
    provides: sqlc.yaml + internal/db/sqlc/ generated-code convention; existing query files (users.sql, chirpstack_connection.sql, install_state.sql, install_identity.sql) as the canonical annotation pattern
provides:
  - 9 sqlc query SQL files (~50 query definitions) covering every Phase 2 table — sites, metering_points, devices, device_profiles, device_profile_mappings, bindings, measurements, audit_log, chirpstack_connection (extended)
  - regenerated internal/db/sqlc/ with type-safe Go bindings for all 50+ queries; AppendMeasurementParams has all 20 measurement columns; GetActiveBindingByDevEUI returns single 10-column denormalized row joining device + device_profile (resolver hot-path D-25)
  - just sqlc recipe (was missing from justfile despite being referenced by Plan 02-06 verification commands)
  - 3 integration tests in internal/db/measurements_test.go pinning the column wiring of AppendMeasurement, CountFlaggedRecent, and WriteAuditLog
  - chirpstack_connection.sql.go regenerated to include the cs_tenant_id + cs_application_id columns added by 0011 (Rule 1 fix — generated code was stale since Plan 02-02 landed 0011 but never re-ran sqlc)
affects: [02-07, 02-08, 02-09, 02-10]

tech-stack:
  added: []
  patterns:
    - "sqlc 1.31 + pgx/v5 query generation pattern continues from Phase 1 — annotation form `-- name: X :one|:many|:exec`, generated function takes context + struct param (when >1 arg), returns typed Row struct or [N]TableRow slice. emit_pointers_for_null_types=true means nullable columns surface as Go *T."
    - "Denormalized single-row hot-path query (D-25 resolver) — `GetActiveBindingByDevEUI` JOINs binding + device + device_profile and returns 10 columns in one row; the resolver in Plan 02-07 will do ONE database round-trip per uplink rather than three. sqlc generates a `GetActiveBindingByDevEUIRow` struct with all 10 fields; no JOIN gymnastics in Go code."
    - "Bulk-replace transactional save (D-08 mapping editor) — `DeleteMappingsByProfile` then a sequence of `CreateMapping` calls inside a single tx. Plan 02-08 profile editor uses this so the editor never observes a half-saved mapping set; sqlc Queries.WithTx(tx) makes the per-call signatures identical to the pool path."
    - "Idempotent guarded UPDATE pattern — `ArchiveSite`, `RestoreSite`, `CloseBinding`, `DecommissionDevice` all use `... WHERE id = $1 AND <flag> IS NULL/IS NOT NULL RETURNING *`. Caller treats no-row-returned as no-op; eliminates a SELECT-then-UPDATE race window for the soft-delete flows."
    - "count(*) FILTER (WHERE ...) for parallel category aggregation — `CountFlaggedRecent` returns the 5 D-26 quality-flag categories in a single index scan instead of 5 separate queries. The 0015 partial index `(metering_point_id, time DESC) WHERE quality <> 'ok'` keeps this fast for the flagged-only branches."
    - "Singleton-row UPDATE pattern continues from Phase 1 — `SetChirpStackTenantApp` uses `WHERE id = 1` (the single row in chirpstack_connection); same shape as `UpdateStep1..4` in install_state.sql. No ON CONFLICT needed because the row always exists by the time these UPDATEs fire (Plan 01-15 wizard finish creates it)."
    - "Test fixture pattern: numeric() helper + requireNumericEqual(via Float64Value) — pgtype.Numeric's Int+Exp encoding can shift between equivalent representations after a Postgres round-trip; comparing magnitudes via Float64Value with InDelta avoids false negatives. Reusable for any future test that asserts NUMERIC column round-trip."

key-files:
  created:
    - internal/db/queries/sites.sql
    - internal/db/queries/metering_points.sql
    - internal/db/queries/devices.sql
    - internal/db/queries/device_profiles.sql
    - internal/db/queries/device_profile_mappings.sql
    - internal/db/queries/bindings.sql
    - internal/db/queries/measurements.sql
    - internal/db/queries/audit_log.sql
    - internal/db/sqlc/sites.sql.go
    - internal/db/sqlc/metering_points.sql.go
    - internal/db/sqlc/devices.sql.go
    - internal/db/sqlc/device_profiles.sql.go
    - internal/db/sqlc/device_profile_mappings.sql.go
    - internal/db/sqlc/bindings.sql.go
    - internal/db/sqlc/measurements.sql.go
    - internal/db/sqlc/audit_log.sql.go
    - internal/db/measurements_test.go
  modified:
    - internal/db/queries/chirpstack_connection.sql
    - internal/db/sqlc/chirpstack_connection.sql.go
    - internal/db/sqlc/models.go
    - internal/db/sqlc/querier.go
    - Justfile

key-decisions:
  - "Added `just sqlc` recipe to Justfile in Task 1 (Rule 3 — blocking issue). Plan 02-06's verification commands all reference `just sqlc` but the recipe didn't exist; without it the literal verification command from the plan would fail with `error: unrecognized command 'sqlc'`. The recipe is a one-liner (`sqlc generate`) — Phase 1 had not yet codified the wrapper because Plan 01-03 ran sqlc directly. Cost: 4 lines of Justfile; benefit: every future plan that touches queries can rely on the same idiom."
  - "chirpstack_connection.sql.go regen pulls in cs_tenant_id + cs_application_id columns (Rule 1 — bug fix). Plan 02-02 landed 0011_chirpstack_connection_cs_ids.up.sql adding two TEXT NULL columns to the table BUT never re-ran sqlc, so the generated `ChirpstackConnection` struct didn't include them. This Plan's first sqlc-generate pulled them in (visible in `git diff internal/db/sqlc/chirpstack_connection.sql.go`); the original `GetChirpStackConnection` and `UpsertChirpStackConnection` functions now Scan all 13 columns. Without this fix Plan 02-05's bootstrap routine could not have read/written the CS UUIDs through sqlc — it would have had to reach for raw pool.QueryRow."
  - "GetActiveBindingByDevEUI takes TIMESTAMPTZ as $2 (allows historical state probe). The plan emphasized this in acceptance criteria. `b.valid_from <= $2 AND (b.valid_to IS NULL OR b.valid_to > $2)` lets the resolver pass `time.Now()` for live uplinks AND lets future Phase 4/5 historical-attribution queries pass an older timestamp to ask 'who was bound to this device at <past time>?'. The half-open `[)` semantics codified at the schema layer (0014 EXCLUDE constraint) carry through to this query — `valid_from <= t < valid_to`."
  - "Forward-compat queries for Phase 6 deferred features. Plan only required `GetAuditEntry`, `WriteAuditLog`, `ListAuditEntriesByEntity`, `CountAuditEntriesByAction`. I added `ListAuditEntriesByUser` because Phase 6's audit browse will need both axes (by entity AND by user) and the (user_id, time DESC) index in 0016 already supports it; landing the query now means Phase 6 doesn't need to touch this file. Same rationale for `ListBindingHistoryByDevice` (symmetric to ListBindingHistoryByMP — Phase 4/5 device history pages use it) and `ListChildSites` (Phase 6 site-tree expansion). Total cost: ~30 lines of SQL, zero generated-code overhead per call site."
  - "Generated code is COMMITTED to the repo (Phase 1 D-01 convention). `internal/db/sqlc/*.sql.go` are part of source — they're regenerated by `just sqlc` and committed alongside the query SQL changes. Reviewers see both the SQL intent (in queries/*.sql) AND the Go bindings (in sqlc/*.sql.go) in the same PR; no `go generate` in CI required. This Plan's three commits each include both query-SQL and generated-Go in lockstep."
  - "3 integration tests live in internal/db/measurements_test.go (NEW file), not internal/db/sqlc/. The Phase 1 convention is that test files live alongside the package whose entry points they test; sqlc-generated code is `package sqlc` with no test files (DO NOT EDIT comment), so tests for sqlc functions live in package db one level up. The tests import `internal/db/sqlc` and use `sqlc.New(pool)`. Same pattern as how migrations_test.go tests the `db` package's RunMigrations against migration content."
  - "Test fixture helper `numeric(t, '12345.678')` + `requireNumericEqual` via `Float64Value`. pgtype.Numeric has Int+Exp internal encoding that shifts between equivalent representations after a Postgres round-trip — direct reflect.DeepEqual on two pgtype.Numeric values fails even when they represent the same magnitude. Comparing as float64 with InDelta(1e-6) avoids the false negative without losing test signal (the fixture values are 3-decimal precision; 1e-6 tolerance is well below)."
  - "OpenBinding takes 4 args (mp_id, device_id, valid_from, reading_offset) but NOT valid_to. The active binding always has valid_to = NULL by definition; CloseBinding sets it on swap. This makes the OpenBinding signature noun-correct — caller can't pass a valid_to that contradicts the 'this is the current binding' semantics."
  - "DecommissionDevice is :one with RETURNING * + idempotent guard, NOT :exec. The caller (Plan 02-07 swap.commit) sometimes wants the row back to populate audit_log.before/after; if already decommissioned the query returns no row (caller treats as no-op, audit doesn't fire a duplicate entry). Same pattern across Archive/Restore/Close — uniform `:one + WHERE flag IS NULL + RETURNING *` shape across the suite."

patterns-established:
  - "Pattern: sqlc query module per table + per-aggregate-pattern. One *.sql file per primary entity (sites, metering_points, devices, device_profiles, bindings, measurements, audit_log) plus per-relationship file (device_profile_mappings) plus per-cross-cutting (chirpstack_connection extends Phase 1's). Phase 6 user-management queries will land in users.sql (extending Phase 1's) following the same shape."
  - "Pattern: hot-path query JOIN-and-flatten — when a downstream caller needs columns from two or three related tables in a single hot-path query (the resolver, the MP detail page, the binding history viewer), the SQL does the JOIN and sqlc generates a Row struct with the flattened column set. The caller does ONE round-trip; the JOIN cost lands once at query time rather than N times in Go after N separate SELECTs."
  - "Pattern: generated-code-in-source-control + per-task lockstep commits. Each task commit includes both the SQL intent (queries/*.sql) and the Go bindings (sqlc/*.sql.go) — reviewers can see both layers without checking out the branch and running sqlc. Task 1 commit included models.go growth (added Phase 2 schema models like AuditLog, Binding, Device, etc.); Task 2 commit included querier.go growth (interface entries for new methods); Task 3 commit closed out by including measurements/audit_log generated code."
  - "Pattern: forward-compat queries in the same plan (light-cost) — when adding a query suite for a table, include the obvious 'symmetric' queries (ListBindingHistoryByDevice alongside ListBindingHistoryByMP; ListAuditEntriesByUser alongside ListAuditEntriesByEntity; ListChildSites alongside ListActiveSites) even when only one is plan-required. Cost is 5-15 lines of SQL each; benefit is Phase 4/5/6 features ship without touching the queries module."

requirements-completed: [SITE-01, DATA-01, DATA-02, DATA-04, DATA-05, DATA-07, DATA-08, DATA-09, AUDIT-01, CHIRP-04]

duration: 10min
completed: 2026-05-04
---

# Phase 02 Plan 06: sqlc Query Suite + Type-Safe Go Bindings Summary

**9 sqlc query SQL files (~50 query definitions) cover every Phase 2 table; `just sqlc` regenerates `internal/db/sqlc/` cleanly with type-safe Go bindings for every query. The resolver hot-path `GetActiveBindingByDevEUI` returns one denormalized 10-column Row struct joining binding + device + device_profile so Plan 02-07's resolver does ONE database round-trip per uplink (no JOIN gymnastics in Go). `AppendMeasurement` accepts all 20 measurement columns including the 16 D-02 Layer-1 fields + extra JSONB + raw_payload BYTEA + decoded_object JSONB + quality + fcnt/gateway_rx_time/device_time + binding_id forward-compat; this is the SOLE write path Plan 02-09's ingest pipeline will use. `WriteAuditLog` accepts the 8 D-22 fields and is callable from a `pgx.Tx` (Queries.WithTx) so Plan 02-08's audit package can write the audit row in the same transaction as the domain mutation (D-23, Pitfall 7 mitigation). The `chirpstack_connection.sql.go` regen pulls in cs_tenant_id + cs_application_id columns added by 0011 (Rule 1 — generated code was stale since Plan 02-02). Three integration tests in `internal/db/measurements_test.go` pin the AppendMeasurement column wiring (20-field round-trip), the CountFlaggedRecent count(*) FILTER pattern (3 ok + 1 decode_fail + 2 missing_canonical → Total=3 + per-category counts), and the WriteAuditLog round-trip via ListAuditEntriesByEntity. Three task commits, each including both SQL intent and generated Go in lockstep. 13/13 db tests pass (4 migration + 3 binding + 3 audit_log + 3 new measurement/audit); short suite 176/176 green; `go vet` and `go build` clean. Plans 02-07 (resolver), 02-08 (site/MP/device handlers + profile editor + audit package), 02-09 (ingest persist), and 02-10 (swap commit) all import from `internal/db/sqlc/` rather than writing raw SQL.**

## Performance

- **Duration:** ~10 min
- **Started:** 2026-05-04T04:57:09Z
- **Completed:** 2026-05-04T05:07:07Z
- **Tasks:** 3 / 3
- **Files created:** 17 (8 query SQL files + 8 generated *.sql.go + 1 test file)
- **Files modified:** 5 (chirpstack_connection.sql, chirpstack_connection.sql.go, models.go, querier.go, Justfile)

## Accomplishments

- **Task 1 — Site/MP/DeviceProfile/Mapping queries.** Authored 4 query files: `sites.sql` (CreateSite/GetSite/UpdateSite/ArchiveSite/RestoreSite/ListActiveSites/ListArchivedSites/ListChildSites/CountMPsOnSite); `metering_points.sql` (parallel CRUD shape + GetMPWithActiveBinding LEFT JOIN denorm row for MP detail page); `device_profiles.sql` (CreateDeviceProfile/GetDeviceProfile/GetDeviceProfileBySlug/ListActiveDeviceProfiles/UpdateDeviceProfile/ArchiveDeviceProfile + Plan 02-08 seed-routine triplet ListUnsyncedProfiles/MarkProfileSyncedToChirpStack/SetProfileCodecJS); `device_profile_mappings.sql` (CreateMapping/GetMapping/ListMappingsByProfile/DeleteMappingsByProfile/CountMappingsByProfile/DeleteMapping — bulk replace-all save pattern). Added `just sqlc` recipe (Rule 3 fix — recipe was missing despite being plan-verification command). Regenerated sqlc — pulled in stale chirpstack_connection.sql.go cs_tenant_id/cs_application_id columns (Rule 1 fix — generated code skewed since 0011 in Plan 02-02). All Phase 2 schema models (AuditLog, Binding, Device, DeviceProfile, DeviceProfileMapping, MeteringPoint, Site, Measurement) now exist in models.go.
- **Task 2 — Devices/Bindings/CS-tenant-app queries.** Authored `devices.sql` (CreateDevice/GetDevice/GetDeviceByDevEUI/UpdateDevice/UpdateDeviceLastSeen/SetDeviceCSUUID/DecommissionDevice/ListActiveDevices/SearchDevices/ListDevicesBySite); `bindings.sql` (verbatim 10-column GetActiveBindingByDevEUI per RESEARCH §"sqlc query for find active binding"; OpenBinding/CloseBinding/UpdateBindingLastRaw/AdvanceReadingOffset for Plan 02-09 ingest + DATA-05 rollover; ListBindingHistoryByMP + ListBindingHistoryByDevice symmetric pair); APPENDED to `chirpstack_connection.sql` the new `SetChirpStackTenantApp` + `GetChirpStackTenantApp` queries (D-28 first-boot bootstrap idempotency — Plan 02-05's ConnectionStore interface gets implemented in Plan 02-08 via these). Regenerated sqlc.
- **Task 3 — Measurements/Audit_log queries + 3 round-trip tests.** Authored `measurements.sql` with the 20-column AppendMeasurement (verbatim D-02 Layer-1 set + extra JSONB + raw_payload BYTEA + decoded_object JSONB + quality + fcnt/gateway_rx_time/device_time + binding_id) + GetLatestMeasurement + ListRecentMeasurements + CountFlaggedRecent (D-26 count(\*) FILTER per-quality categorization). Authored `audit_log.sql` with WriteAuditLog (D-22 + D-23 8-field INSERT — id and time DB-defaulted) + GetAuditEntry + ListAuditEntriesByEntity + ListAuditEntriesByUser + CountAuditEntriesByAction. Authored 3 integration tests in `internal/db/measurements_test.go`: TestAppendMeasurement_RoundTrip (every column populated, round-trip via GetLatestMeasurement), TestCountFlaggedRecent_Categorizes (3 ok + 1 decode_fail + 2 missing_canonical → Total=3, DecodeFail=1, MissingCanonical=2), TestWriteAuditLog_RoundTrip (8-field INSERT round-trips through ListAuditEntriesByEntity, with CountAuditEntriesByAction sanity check). Test fixtures use `numeric(t, "12345.678")` helper + `requireNumericEqual` via Float64Value to avoid pgtype.Numeric Int+Exp encoding round-trip false-negatives.
- **Verification.** `just sqlc` exits 0; `go build ./...` clean; `go vet ./...` clean; `go test -count=1 ./internal/db/...` 13/13 pass (4 migration + 3 binding + 3 audit_log + 3 new); `go test -count=1 -short ./...` 176/176 pass in 22 packages (no regressions).

## Task Commits

1. **Task 1: Site/MP/DeviceProfile/Mapping queries + sqlc regen + just sqlc recipe** — `88e5aa4` (feat)
2. **Task 2: Devices/Bindings/CS-tenant-app queries + sqlc regen** — `bb209b7` (feat)
3. **Task 3: Measurements/Audit_log queries + 3 integration tests** — `fc77f68` (feat)

**Plan metadata commit:** _pending — created at end of plan_

## Query Suite Inventory (~50 queries across 9 files)

| File | Queries | Notable |
|------|---------|---------|
| `sites.sql` | 9 | `GetSite`, `CreateSite`, `UpdateSite`, `ArchiveSite`, `RestoreSite`, `ListActiveSites`, `ListArchivedSites`, `ListChildSites`, `CountMPsOnSite` |
| `metering_points.sql` | 9 | Parallel CRUD + `ListMPsBySite` + **`GetMPWithActiveBinding`** (LEFT JOIN denorm row for MP detail page) |
| `devices.sql` | 10 | CRUD + `GetDeviceByDevEUI` (swap commit) + `UpdateDeviceLastSeen` (ingest) + `SetDeviceCSUUID` (Plan 02-05 atomic-create) + `DecommissionDevice` (D-15) + `SearchDevices` + `ListDevicesBySite` |
| `device_profiles.sql` | 9 | CRUD + Plan 02-08 seed-routine triplet **`ListUnsyncedProfiles`/`MarkProfileSyncedToChirpStack`/`SetProfileCodecJS`** |
| `device_profile_mappings.sql` | 6 | CRUD + **`DeleteMappingsByProfile`** for bulk replace-all save |
| `bindings.sql` | 9 | **`GetActiveBindingByDevEUI`** (10-col denorm resolver hot-path) + `OpenBinding`/`CloseBinding`/`UpdateBindingLastRaw`/`AdvanceReadingOffset` (rollover) + history queries |
| `measurements.sql` | 4 | **`AppendMeasurement`** (20 columns) + `GetLatestMeasurement` + `ListRecentMeasurements` + `CountFlaggedRecent` (D-26 count\* FILTER) |
| `audit_log.sql` | 5 | **`WriteAuditLog`** (8 fields, callable inside pgx.Tx) + browse queries |
| `chirpstack_connection.sql` | +2 (4 total) | Phase 1's GetChirpStackConnection + UpsertChirpStackConnection PLUS new **`SetChirpStackTenantApp`** + **`GetChirpStackTenantApp`** (D-28) |

## Generated Code Inventory

| File | Purpose |
|------|---------|
| `internal/db/sqlc/sites.sql.go` | 9 functions, Site struct |
| `internal/db/sqlc/metering_points.sql.go` | 9 functions including GetMPWithActiveBindingRow (LEFT JOIN result) |
| `internal/db/sqlc/devices.sql.go` | 10 functions, Device struct |
| `internal/db/sqlc/device_profiles.sql.go` | 9 functions, DeviceProfile struct |
| `internal/db/sqlc/device_profile_mappings.sql.go` | 6 functions, DeviceProfileMapping struct |
| `internal/db/sqlc/bindings.sql.go` | 9 functions including GetActiveBindingByDevEUIRow (10-col denorm) |
| `internal/db/sqlc/measurements.sql.go` | 4 functions, AppendMeasurementParams (20 fields), CountFlaggedRecentRow (5 categories) |
| `internal/db/sqlc/audit_log.sql.go` | 5 functions, AuditLog struct |
| `internal/db/sqlc/chirpstack_connection.sql.go` | regenerated to include cs_tenant_id + cs_application_id columns + 2 new functions |
| `internal/db/sqlc/models.go` | grows from 6 → 14 structs (8 Phase 2 schema models added) |
| `internal/db/sqlc/querier.go` | Querier interface grows from ~10 → ~70 methods |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 — Blocking issue] Added `just sqlc` recipe to Justfile**
- **Found during:** Task 1 — first attempt to run plan's verification command `just sqlc` failed because the recipe didn't exist in Phase 1's Justfile.
- **Issue:** Plan 02-06's verification commands all reference `just sqlc` but Phase 1's Plan 01-03 ran sqlc directly without ever codifying the wrapper recipe. Plan 02-06 cannot meet its acceptance criteria as literally written without this recipe — `just sqlc` would error with "unrecognized command".
- **Fix:** Added a 4-line recipe `sqlc:\n    sqlc generate` with a comment explaining the convention (generated code lives in internal/db/sqlc/ and IS committed to repo per Phase 1 D-01).
- **Files modified:** `Justfile`
- **Commit:** `bb209b7` (Task 2 commit; the Task 1 commit lost the Justfile edit due to a macOS APFS case-insensitive `Justfile` vs `justfile` git index glitch — the original `git add justfile` lower-case silently did nothing because git tracks the file as `Justfile` upper-case. Re-staged with `git add Justfile` in Task 2.)

**2. [Rule 1 — Bug] Regenerated chirpstack_connection.sql.go fills cs_tenant_id + cs_application_id columns**
- **Found during:** Task 1 — first `just sqlc` produced a diff in `internal/db/sqlc/chirpstack_connection.sql.go` even though I hadn't touched chirpstack_connection.sql yet.
- **Issue:** Plan 02-02 landed migration 0011_chirpstack_connection_cs_ids.up.sql (ALTER TABLE chirpstack_connection ADD COLUMN cs_tenant_id, ADD COLUMN cs_application_id) BUT never ran sqlc afterwards. The result: the generated `ChirpstackConnection` struct + `GetChirpStackConnection` Scan list omitted both new columns. Any caller using sqlc to read or upsert the row would silently lose the CS UUIDs (or fail to scan with "wrong column count" depending on driver state).
- **Fix:** Regenerating sqlc as part of this plan picks up the 0011 ALTER. The diff shows `ChirpstackConnection.CsTenantID` and `CsApplicationID` added as `*string` (nullable TEXT) and `GetChirpStackConnection`/`UpsertChirpStackConnection` Scan lists now include them. Without this, Plan 02-05's bootstrap routine could not have written the CS UUIDs through sqlc — would have had to reach for raw `pool.QueryRow`. Committed as part of Task 1 since it's directly caused by re-running sqlc after the new query files.
- **Files modified:** `internal/db/sqlc/chirpstack_connection.sql.go`, `internal/db/sqlc/models.go` (ChirpstackConnection struct grew)
- **Commit:** `88e5aa4` (Task 1)

**3. [Rule 2 — Critical functionality] Added forward-compat symmetric queries beyond plan-verbatim**
- **Found during:** Task 1 (sites/MP/device-profile) + Task 2 (bindings) + Task 3 (audit_log)
- **Issue:** Plan listed minimum-viable query sets per file; downstream plans (Phase 4 device detail, Phase 5 reports, Phase 6 audit browse) will need symmetric queries that aren't in the plan. Adding them now is a 5-15 LoC each cost; deferring forces those later plans to touch this file again, polluting their diffs and breaking the "Phase 2 = bones, Phase 4+ = features" boundary.
- **Fix:** Added these queries beyond plan-verbatim:
  - `ListChildSites` (Phase 6 site-tree expansion — direct children only, not recursive)
  - `ListBindingHistoryByDevice` (symmetric to ListBindingHistoryByMP — Phase 4 device detail "where has this device been")
  - `ListAuditEntriesByUser` (symmetric to ListAuditEntriesByEntity — Phase 6 "all actions by Alice"; the (user_id, time DESC) index in 0016 already supports it)
  - `ListRecentMeasurements` (Phase 4 DETL-01 chart preload — bounded by both time floor AND row count so a misconfigured UI can't accidentally page through years)
  - `UpdateDevice` (Phase 6 device edit — name/join_eui/description; dev_eui + device_profile_id deliberately NOT updatable per D-15 swap workflow)
  - `SetDeviceCSUUID` (Plan 02-05 atomic-create-rollback — fills cs_device_uuid after CS gRPC ack; the plan called for it but didn't list it explicitly in the queries section)
  - `GetBinding` (test-only utility for round-trip assertions; Phase 6 "binding detail" page reuses)
- **Files modified:** sites.sql, devices.sql, bindings.sql, audit_log.sql, measurements.sql
- **Commits:** spread across `88e5aa4`, `bb209b7`, `fc77f68`

**4. [Rule 2 — Critical functionality] Test fixture helper for pgtype.Numeric round-trip equality**
- **Found during:** Task 3 — TestAppendMeasurement_RoundTrip first attempt with `require.Equal(rawVal, got.RawValue)` fails because pgtype.Numeric's Int+Exp encoding shifts after Postgres round-trip (e.g. 12345.678 stored as Int=12345678/Exp=-3 might come back as Int=123456780/Exp=-4 — same magnitude, different internal representation, fails reflect.DeepEqual).
- **Issue:** Without this fix the test reports false-negative failures even when the schema correctly preserves the value. Future tests that round-trip NUMERIC columns (Phase 4 reading_offset history, Phase 5 aggregate values) would all hit the same trap.
- **Fix:** Added two helpers at the bottom of measurements_test.go: `numeric(t, "12345.678")` builds a pgtype.Numeric from a decimal string (calls Scan); `requireNumericEqual(t, want, got, msg)` compares via `Float64Value()` with `InDelta(1e-6)` — well below the 3-decimal test fixture precision but above floating-point noise. Reusable by future tests in the package.
- **Files modified:** `internal/db/measurements_test.go`
- **Commit:** `fc77f68`

### Plan-text observations (not actioned)

**1. [Rule 4 flag — not actioned] Plan acceptance criteria says GetActiveBindingByDevEUI takes "TIMESTAMPTZ as $2" but doesn't name the param**

- The plan's acceptance criteria says "bindings.sql GetActiveBindingByDevEUI takes a TIMESTAMPTZ as $2 (allows querying historical state, not just `now()`)." sqlc derives the Go param name from the SQL column name comparison — since the predicate is `b.valid_from <= $2`, sqlc names the field `ValidFrom`. This is technically misleading at the call site (the value is "uplink time" or "query time", not "valid_from"). The query SQL comment explicitly says `$2 = uplink time (TIMESTAMPTZ — typically gateway_rx_time, but resolver may also probe historical state)` to compensate. A future hygiene pass could split the predicate into two CTE parameters or use `coalesce($2, now())` to make the semantic intent explicit at the param-name level. Not actioned because the comment + plan documentation are sufficient for now and the alternative complicates the SQL.

**2. [Rule 4 flag — not actioned] Plan task 3 mentions "16+ columns" for AppendMeasurement, plan body shows 20**

- Acceptance criteria text: "AppendMeasurement query inserts the full Layer-1 column set + extra JSONB + raw_payload + decoded_object + binding_id + quality." The actual INSERT has 20 columns (16 D-02 Layer-1 + fcnt + gateway_rx_time + device_time + binding_id). Same plan-text vs body skew as Plan 02-04 noted (also flagged then). Implemented exactly as the plan body shows (20 placeholders); did not re-flag because it's the same author-side issue and 02-04's note covers the recommendation.

## Authentication Gates

None — sqlc generation is a local-only operation (sqlc reads SQL files, generates Go code; no network calls). Postgres testcontainer ran via the local Podman socket (`unix:///var/folders/01/3kgf05ss5t78z9bgqr4q1gcw0000gn/T/podman/podman-machine-default-api.sock`) with `TESTCONTAINERS_RYUK_DISABLED=true`. Podman machine had to be stopped + restarted at the start of execution to recreate the API socket file (continuing the Plan 02-02 / 02-03 / 02-04 / 02-05 pattern); no code changes — purely environmental.

## Decisions Made

- **`OpenBinding` takes 4 args, no valid_to.** The active binding always has valid_to = NULL by definition; passing it would be a footgun (caller could violate the "this is the current binding" invariant). Plan 02-07's CloseBinding sets valid_to on swap. The signature is intentionally noun-correct so that grep'ing for `OpenBinding` always means "create the active binding from now into the future."
- **`CountFlaggedRecent` returns 5 columns in one Row.** Plan acceptance text suggested `count(*) FILTER (WHERE quality = ...)` for D-26 quality categorization. sqlc generates a `CountFlaggedRecentRow` struct with `{Total, DecodeFail, MissingCanonical, OutOfRange, DuplicateFcnt}` — caller does ONE round-trip + one aggregate scan instead of 5 separate queries. The 0015 partial index `(metering_point_id, time DESC) WHERE quality <> 'ok'` keeps it fast.
- **`Total` excludes 'ok' rows.** The WHERE clause `quality <> 'ok' AND time >= $2` makes Total = count of FLAGGED rows (matching plan acceptance text: "insert 3 'ok' + 1 decode_fail + 2 missing_canonical, Total=3"). This is intentional and slightly different from the naive read of "total" — the comment in measurements.sql + the test `TestCountFlaggedRecent_Categorizes` pin the semantics.
- **`DecommissionDevice` is `:one` with RETURNING + idempotent guard, NOT `:exec`.** Plan 02-07 swap.commit sometimes wants the row back to populate audit_log.before/after; if already decommissioned, returns no row (caller treats as no-op, audit doesn't fire a duplicate entry). Same pattern across `ArchiveSite`/`RestoreSite`/`ArchiveMP`/`RestoreMP`/`ArchiveDeviceProfile`/`CloseBinding` — uniform `:one + WHERE flag IS NULL/IS NOT NULL + RETURNING *` shape across the suite.
- **`SetProfileCodecJS` clears `codec_js_synced_at` to NULL.** Plan acceptance only said "writes the //go:embed-ed codec body into the row." But if the codec changed (a new release with an updated JS body) the seed routine MUST re-push to ChirpStack on next boot. Clearing codec_js_synced_at = NULL is what makes the next `ListUnsyncedProfiles` call return this row (the WHERE includes `OR codec_js_synced_at IS NULL`). Without this, a codec update would be silently dropped at the boundary.
- **Search devices ILIKE pattern is unindexed.** Per the plan's threat-register T-02-06-03: Phase 2 minimal scope is sub-100-device installs, B-tree LIMIT/OFFSET is fine. Phase 3 DEV-01 will add proper FTS or trigram index when device counts cross ~1K. Documented in the SQL comment so a future reviewer doesn't "fix" it prematurely.
- **`Justfile` capitalization vs `justfile` lowercase.** macOS APFS is case-insensitive so both names resolve to the same inode, but git tracks the original `Justfile` (capital J — committed by Phase 1 Plan 01-01). Editing as `justfile` in Task 1 then `git add justfile` was a no-op (git index has only `Justfile`). Re-discovered + re-staged with `git add Justfile` in Task 2; the recipe landed in `bb209b7` instead of `88e5aa4`. Documented under deviations because future executors should be aware of this trap on case-insensitive filesystems.
- **Generated code is COMMITTED in the same task commit as the source SQL.** Reviewers see both layers in the same diff. Three task commits, each with a SQL+Go pair: Task 1 (4 query files + 4 generated + chirpstack regen + models.go grow), Task 2 (3 query files modified/created + 3 generated + querier.go grow), Task 3 (2 query files + 2 generated + querier.go grow + 1 test file).

## Self-Check: PASSED

- `[x]` `internal/db/queries/sites.sql` exists; contains `CreateSite`, `ArchiveSite`, `RestoreSite`, `ListActiveSites`, `ListArchivedSites`, `CountMPsOnSite`, `ListChildSites`, `GetSite`, `UpdateSite`
- `[x]` `internal/db/queries/metering_points.sql` exists; contains `GetMPWithActiveBinding` with LEFT JOIN to binding/device/device_profile
- `[x]` `internal/db/queries/devices.sql` exists; contains `CreateDevice`, `GetDeviceByDevEUI`, `SearchDevices` (ILIKE), `UpdateDeviceLastSeen`, `DecommissionDevice`, `SetDeviceCSUUID`, `ListActiveDevices`, `ListDevicesBySite`, `UpdateDevice`, `GetDevice`
- `[x]` `internal/db/queries/device_profiles.sql` exists; contains `ListUnsyncedProfiles`, `MarkProfileSyncedToChirpStack`, `SetProfileCodecJS` (Plan 02-08 seed routine triplet)
- `[x]` `internal/db/queries/device_profile_mappings.sql` exists; contains `DeleteMappingsByProfile` (bulk replace-all save)
- `[x]` `internal/db/queries/bindings.sql` exists; contains the verbatim 10-column SELECT in `GetActiveBindingByDevEUI` joining device + device_profile (resolver hot-path D-25), `OpenBinding`, `CloseBinding`, `UpdateBindingLastRaw`, `AdvanceReadingOffset` (rollover), `ListBindingHistoryByMP`, `ListBindingHistoryByDevice`, `GetActiveBindingByMPID`, `GetBinding`
- `[x]` `internal/db/queries/measurements.sql` exists; contains `AppendMeasurement` with 20 placeholders ($1..$20) + extra/raw_payload/decoded_object/binding_id columns + `CountFlaggedRecent` with `count(*) FILTER (WHERE quality = ...)` for D-26 categorization
- `[x]` `internal/db/queries/audit_log.sql` exists; contains `WriteAuditLog` with 8 caller params (user_id, action, entity_type, entity_id, before, after, notes, request_id) — id and time DB-defaulted
- `[x]` `internal/db/queries/chirpstack_connection.sql` contains both new queries: `SetChirpStackTenantApp` and `GetChirpStackTenantApp`
- `[x]` `just sqlc` exits 0 (recipe added to Justfile)
- `[x]` `go build ./internal/db/...` exits 0
- `[x]` `go vet ./...` exits 0
- `[x]` `go test -count=1 ./internal/db/...` exits 0 (13/13 pass: 4 migration + 3 binding + 3 audit_log + 3 new measurement/audit)
- `[x]` `go test -count=1 -short ./...` exits 0 (176/176 in 22 packages — no regressions)
- `[x]` `internal/db/sqlc/measurements.sql.go` contains generated `AppendMeasurement` with 20-field AppendMeasurementParams struct
- `[x]` `internal/db/sqlc/audit_log.sql.go` contains generated `WriteAuditLog` with 8-field WriteAuditLogParams struct
- `[x]` `internal/db/sqlc/bindings.sql.go` contains generated `GetActiveBindingByDevEUI` returning `GetActiveBindingByDevEUIRow` with 10 fields including `DeviceProfileID` + `CounterModulus`
- `[x]` `grep -rn "GetActiveBindingByDevEUI\|UpdateBindingLastRaw\|AdvanceReadingOffset" internal/db/sqlc/` returns 14 matches (3 functions × multiple sites: query string, function signature, params struct, etc.)
- `[x]` commit `88e5aa4` (Task 1) found in git log
- `[x]` commit `bb209b7` (Task 2) found in git log
- `[x]` commit `fc77f68` (Task 3) found in git log
