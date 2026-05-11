---
phase: 04-realtime-dashboard
plan: 01
subsystem: database

tags: [timescaledb, postgres, listen-notify, sse-substrate, schema, sqlc, migrations]

# Dependency graph
requires:
  - phase: 02-domain-model-canonical-schema
    provides: measurement hypertable (0015), audit_log table (0016), device_profile schema (0009), install_identity singleton (0005), binding_changed trigger pattern (0017)
  - phase: 03-provisioning-gateways-devices-bulk-import
    provides: schema_migrations chain at version 20 (gateway, import_job, audit_log_vocabulary)

provides:
  - 0021_measurement_inserted_trigger — AFTER INSERT trigger on measurement hypertable emitting pg_notify('measurement_inserted', <7-field JSON>)
  - 0022_install_capabilities — install_identity.capabilities TEXT NOT NULL DEFAULT 'both' CHECK water|electricity|both
  - 0023_device_profile_expected_interval — device_profile.expected_interval_s INTEGER NOT NULL DEFAULT 3600 CHECK > 0, with backfill for Acrel ADL200/ADW300 to 300s
  - sqlc-generated bindings for the new columns (DeviceProfile.ExpectedIntervalS, InstallIdentity.Capabilities)
  - Integration-test proof that the AFTER INSERT trigger propagates to TimescaleDB chunks (TestRunMigrations_MeasurementInsertedNotifyPropagatesToChunks)

affects:
  - 04-02-PLAN (LISTEN/NOTIFY substrate — consumes measurement_inserted channel + 7-field payload contract)
  - 04-04-PLAN (online/offline KPI rule — consumes device_profile.expected_interval_s)
  - 04-07-PLAN (Settings UI for install scope — consumes install_identity.capabilities)
  - All Phase 4 plans (every dashboard surface assumes these 3 schema atoms exist)

# Tech tracking
tech-stack:
  added: []  # zero new deps; pure SQL + sqlc regeneration + stdlib NOTIFY assertion
  patterns:
    - "Measurement-side AFTER INSERT trigger emits compact (<200B) JSON payload via pg_notify; never raw_payload/decoded_object — 8KB cap is silent-truncation risk"
    - "Schema additions land as separate migrations even when small (one column = one migration), preserving git-bisectable schema history"
    - "Per-vendor seed backfill belongs inside the column-add migration when the seed rows are SQL-defined (0010_seed_profiles is INSERT-only; backfill in 0023 stays atomic with the column add)"

key-files:
  created:
    - "internal/db/migrations/0021_measurement_inserted_trigger.up.sql"
    - "internal/db/migrations/0021_measurement_inserted_trigger.down.sql"
    - "internal/db/migrations/0022_install_capabilities.up.sql"
    - "internal/db/migrations/0022_install_capabilities.down.sql"
    - "internal/db/migrations/0023_device_profile_expected_interval.up.sql"
    - "internal/db/migrations/0023_device_profile_expected_interval.down.sql"
  modified:
    - "internal/db/sqlc/models.go (DeviceProfile + InstallIdentity field additions)"
    - "internal/db/sqlc/install_identity.sql.go (capabilities scan)"
    - "internal/db/sqlc/device_profiles.sql.go (expected_interval_s scan)"
    - "internal/db/migrations_test.go (version assertion 20 -> 23, Phase 3 down-test step counts bumped)"
    - "internal/db/roundtrip_test.go (version assertion + new chunk-propagation NOTIFY test)"
    - "internal/profile/seed_test.go (TestSeed_ExpectedIntervalS_PerProfile)"

key-decisions:
  - "Place per-profile expected_interval_s backfill inside 0023 (the column-add migration) rather than mutating 0010_seed_profiles.up.sql — backfill stays atomic with the column add and works for both fresh installs and existing installs (idempotent UPDATE)."
  - "NOTIFY channel name pinned to literal 'measurement_inserted' (Plan 02 will LISTEN on this exact string — never abbreviate or rename)."
  - "Payload size discipline enforced by the trigger body itself (only 7 explicit json_build_object pairs; raw_payload/decoded_object DELIBERATELY excluded). Documented in trigger header comment without referencing the excluded column names verbatim, to keep the size-discipline grep gate one-line scriptable."
  - "Default expected_interval_s=3600s chosen so existing installs upgrade safely (1-hour threshold for water meters; backfill UPDATE narrows electricity profiles to 300s)."

patterns-established:
  - "Trigger ↔ chunk propagation contract: every new measurement-side trigger MUST be covered by a TestRun…_PropagatesToChunks integration test that LISTENs, INSERTs into the parent (which lands in a chunk via time=now()), and WaitForNotification within 2s. Without this assertion, a future migration that breaks chunk propagation is silent."
  - "Phase boundary down-test maintenance: when a phase adds N new migrations, every prior phase's down-test step count must be bumped by N (Rule 3 deviation surfaces; the test names already encode their target migration so the bump is mechanical)."

requirements-completed: [DASH-01, DASH-02]

# Metrics
duration: 18min
completed: 2026-05-11
---

# Phase 4 Plan 01: Schema Foundation Summary

**3 SQL migrations + sqlc regeneration + chunk-propagated NOTIFY proof — unblocks every downstream Phase 4 plan at the schema layer.**

## Performance

- **Duration:** ~18 minutes
- **Started:** 2026-05-11T13:07:45Z
- **Completed:** 2026-05-11T13:25:32Z
- **Tasks:** 5
- **Files modified:** 11 (6 new SQL files, 3 sqlc-regenerated, 2 modified Go test files, 1 modified test for new assertion)

## Accomplishments

- **Migration 0021** lands the SSE substrate: AFTER INSERT trigger on the `measurement` hypertable emits `pg_notify('measurement_inserted', <7-field JSON>)` for every uplink — Plan 04-02 (LISTEN substrate) can build directly on this without further schema work.
- **Migration 0022** lands `install_identity.capabilities` (TEXT NOT NULL DEFAULT 'both' CHECK water|electricity|both) — DASH-01 adaptive-scope flag, ready for Plan 04-07 Settings UI.
- **Migration 0023** lands `device_profile.expected_interval_s` (INTEGER NOT NULL DEFAULT 3600 CHECK > 0) + per-vendor backfill (Acrel ADL200/ADW300 narrowed to 300s) — DASH-02 online/offline KPI rule (`device.last_seen_at > now() - 2 * expected_interval_s`) gets a real per-profile threshold.
- **Chunk-propagation proof:** `TestRunMigrations_MeasurementInsertedNotifyPropagatesToChunks` LISTENs on `measurement_inserted`, inserts a row that lands in a TimescaleDB chunk, and asserts the NOTIFY fires within 2s with all 7 D-02 keys present in the JSON payload. This pins the load-bearing TimescaleDB claim that the Phase 4 SSE substrate depends on.
- **sqlc bindings regenerated** cleanly: `DeviceProfile.ExpectedIntervalS int32` + `InstallIdentity.Capabilities string` are now first-class Go fields. `go build ./...` exits 0; full short suite passes 346 tests.

## Task Commits

Each task was committed atomically:

1. **Task 1: Create migration 0021 — measurement_inserted trigger** — `4a2a090` (feat)
2. **Task 2: Create migration 0022 — install_identity.capabilities** — `7a69321` (feat)
3. **Task 3: Create migration 0023 — device_profile.expected_interval_s** — `b2419c3` (feat)
4. **Task 4: Backfill expected_interval_s on seeded profiles + per-profile assertion** — `184522a` (test)
5. **Task 5: sqlc generate + chunk-propagation NOTIFY assertion** — `e364f78` (feat)

**Plan metadata:** (final docs commit, see footer)

## Files Created/Modified

### Created (migrations)

- `internal/db/migrations/0021_measurement_inserted_trigger.up.sql` — `measurement_notify()` plpgsql function + AFTER INSERT trigger on measurement parent, fires `pg_notify('measurement_inserted', json_build_object(7 D-02 fields))`.
- `internal/db/migrations/0021_measurement_inserted_trigger.down.sql` — drops trigger then function.
- `internal/db/migrations/0022_install_capabilities.up.sql` — adds `install_identity.capabilities TEXT NOT NULL DEFAULT 'both'` with CHECK water|electricity|both.
- `internal/db/migrations/0022_install_capabilities.down.sql` — drops the column.
- `internal/db/migrations/0023_device_profile_expected_interval.up.sql` — adds `device_profile.expected_interval_s INTEGER NOT NULL DEFAULT 3600` + CHECK > 0; backfills `acrel_adl200`/`acrel_adw300` to 300s.
- `internal/db/migrations/0023_device_profile_expected_interval.down.sql` — drops the column.

### Modified (tests + regen)

- `internal/db/sqlc/models.go` — sqlc regen added `ExpectedIntervalS int32` to `DeviceProfile` and `Capabilities string` to `InstallIdentity`.
- `internal/db/sqlc/install_identity.sql.go` — `GetInstallIdentity` / `UpsertInstallIdentity` now scan capabilities.
- `internal/db/sqlc/device_profiles.sql.go` — every device-profile query now scans expected_interval_s.
- `internal/db/migrations_test.go` — bumped `schema_migrations.version` assertion 20 → 23 (TestRunMigrations_Clean + TestRunMigrations_Idempotent); bumped Phase 3 down-test step counts (`-3 → -6`, `-2 → -5`, `-1 → -4`) so each test still rolls back to its intended pre-migration state.
- `internal/db/roundtrip_test.go` — added `TestRunMigrations_MeasurementInsertedNotifyPropagatesToChunks` (LISTEN + INSERT + WaitForNotification with 2s deadline + 7-key payload assertion); bumped roundtrip version assertion 20 → 23.
- `internal/profile/seed_test.go` — added `TestSeed_ExpectedIntervalS_PerProfile` asserting axioma_w1=3600, acrel_adl200=300, acrel_adw300=300 + min(expected_interval_s) > 0.

## Decisions Made

- **Per-profile backfill placement.** Plan task 4 asked to modify `internal/profile/seed.go`, but that file is the CS-side codec syncer (`RunSeedSync`), not a row inserter — the actual seed inserts live in `0010_seed_profiles.up.sql` which cannot reference `expected_interval_s` (the column doesn't exist at 0010 time). Cleanest approach: backfill UPDATE inside 0023 itself; assertion in `seed_test.go` pins the per-profile values post-migration. This stays atomic with the column add and works on both fresh installs and existing installs.
- **Profile slug list correction.** The plan's task 4 cited "Diehl HYDRUS" as one of the seeded profiles — but the actual 3 seeded slugs are `axioma_w1`, `acrel_adl200`, `acrel_adw300` (no Diehl). Both Acrel slugs are electricity profiles with 5-minute uplink cadence (300s); Axioma stays at the 3600s default.
- **Trigger header comment rewritten to satisfy size-discipline grep gate.** The plan's automated_verify gate `! grep -qE "raw_payload|decoded_object"` would have failed against a comment that mentioned those column names verbatim. Rewrote the comment to "NEVER include the bulky columns" — same intent, gate passes one-line scriptable.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Trigger header comment rewritten to satisfy automated_verify gate**
- **Found during:** Task 1
- **Issue:** Plan-verbatim trigger header included `-- NEVER include raw_payload or decoded_object` comment, but the plan's own automated_verify gate `! grep -qE "raw_payload|decoded_object"` rejected those exact strings appearing anywhere in the file.
- **Fix:** Rewrote header line to `NEVER include the bulky columns` — same size-discipline intent without the tripwire strings.
- **Files modified:** `internal/db/migrations/0021_measurement_inserted_trigger.up.sql`
- **Verification:** All 3 grep gates now pass (`pg_notify('measurement_inserted'`, `AFTER INSERT ON measurement`, no `raw_payload|decoded_object` matches).
- **Committed in:** `4a2a090`

**2. [Rule 3 - Blocking] Phase 3 migration-down test step counts bumped**
- **Found during:** Task 5 (full-suite run after sqlc regen)
- **Issue:** `TestPhase3Migrations_0018_Down` rolled back `-3` (was 17→14, but with 0021/0022/0023 added the chain is now 23→20, leaving the gateway table still in place); `_0019_Down` similarly under-rolled (`-2`); `_0020_Down` (`-1`).
- **Fix:** Bumped step counts mechanically by the +3 new migrations: 0018_Down `-3 → -6`, 0019_Down `-2 → -5`, 0020_Down `-1 → -4`. Updated comments naming the rolled-back migrations.
- **Files modified:** `internal/db/migrations_test.go`
- **Verification:** All 7 Phase 3 migration tests pass (`go test ./internal/db/... -run "TestPhase3Migrations" -count=1`).
- **Committed in:** `e364f78`

**3. [Rule 3 - Blocking] Per-profile backfill location & seed.go non-modification**
- **Found during:** Task 4 (reading internal/profile/seed.go)
- **Issue:** Plan task 4 explicitly said modify `internal/profile/seed.go` to add `expected_interval_s` column + per-profile values. But seed.go is the CS-side codec syncer (`RunSeedSync`); it has no INSERT logic. The actual seed rows are in `0010_seed_profiles.up.sql`, which cannot reference `expected_interval_s` (column doesn't exist at 0010 time).
- **Fix:** Per-profile UPDATE backfill placed inside the 0023 column-add migration itself (atomic with the column add). New `TestSeed_ExpectedIntervalS_PerProfile` test in `seed_test.go` pins the per-profile values post-migration (asserts axioma_w1=3600, acrel_adl200=300, acrel_adw300=300, plus a defensive min>0 over all active profiles).
- **Files modified:** `internal/db/migrations/0023_device_profile_expected_interval.up.sql` (UPDATE block), `internal/profile/seed_test.go` (new test).
- **Verification:** `go test ./internal/profile/... -run "TestSeed_ExpectedIntervalS_PerProfile" -count=1` passes; full short suite passes 346 tests across 28 packages.
- **Committed in:** `184522a`

**4. [Rule 3 - Blocking] Plan's "Diehl HYDRUS" profile slug does not exist**
- **Found during:** Task 4 (cross-checking 0010_seed_profiles.up.sql)
- **Issue:** Plan task 4 cited "Diehl HYDRUS (water, slug likely contains diehl or hydrus)" as one of three seeded profiles. No such slug exists; the actual three seeded slugs are `axioma_w1`, `acrel_adl200`, `acrel_adw300`. Both Acrel slugs are electricity profiles.
- **Fix:** Backfill targets the actual seeded slugs (`acrel_adl200`, `acrel_adw300` → 300s) and `axioma_w1` keeps the 3600s default. Test pins the same set.
- **Files modified:** `internal/db/migrations/0023_device_profile_expected_interval.up.sql`, `internal/profile/seed_test.go`.
- **Verification:** Test asserts 3 specific slugs by name; `go test ./internal/profile/... -run "TestSeed_ExpectedIntervalS_PerProfile"` passes.
- **Committed in:** `b2419c3` + `184522a`

---

**Total deviations:** 4 auto-fixed (all Rule 3 blocking). All four were preconditions for the plan to compile/pass; none added scope. The plan's own automated_verify gate (Task 1) and adjacent test maintenance (Task 5 sqlc regen surfacing pre-existing Phase 3 down tests with stale step counts) were the primary causes; the seed.go misroute and Diehl HYDRUS slug error were planning artifacts the executor reconciled against the actual codebase.

**Impact on plan:** All deviations preserve the plan's success criteria 1-for-1. The schema atoms land at the exact column names + types the plan specified; the chunk-propagation NOTIFY assertion goes further than the plan's "include an assertion" wording (also asserts all 7 D-02 keys present + payload size <1KB).

## Issues Encountered

- **None functionally.** One environmental note: RTK proxy rewrites bare `grep` to ripgrep, which interprets `'` as regex syntax. Worked around by piping through `rtk proxy grep -F` for fixed-string matches; this is a tooling quirk, not a code issue.

## User Setup Required

None — pure schema additions; migrations apply on next `shifter serve` boot.

## Next Phase Readiness

- **Plan 04-02 (LISTEN/NOTIFY substrate)** is now unblocked: the channel name `measurement_inserted` is live and the 7-field payload contract is asserted by `TestRunMigrations_MeasurementInsertedNotifyPropagatesToChunks`. Plan 02 can simply `LISTEN measurement_inserted` and decode the JSON into a typed Go struct.
- **Plan 04-04 (online/offline KPI rule)** is now unblocked: `device_profile.expected_interval_s` exists with realistic per-vendor values (300s for electricity, 3600s for water).
- **Plan 04-07 (Settings UI)** is now unblocked: `install_identity.capabilities` exists with the canonical `water|electricity|both` value domain enforced at the DB layer; the frontend can read it via the existing `GetInstallIdentity` query (sqlc binding regenerated).
- **All Phase 4 plans** can reference the migration numbers + column names verbatim from this SUMMARY's frontmatter `provides:` block.

## Threat Flags

None — this plan only adds schema atoms with no new network surface, no new auth paths, no new file access, no new trust boundaries. The threat register entries (T-04-01-01 through T-04-01-04) are all mitigated as planned: payload whitelist enforced by trigger body composition; trigger overhead bounded (literal column reads + one pg_notify call, no subqueries); `capabilities` CHECK constraint enforces value domain at the DB layer; `expected_interval_s > 0` CHECK prevents degenerate KPI thresholds.

## Self-Check: PASSED

Verified the following claims against the working tree and git log:

- `internal/db/migrations/0021_measurement_inserted_trigger.up.sql` — FOUND
- `internal/db/migrations/0021_measurement_inserted_trigger.down.sql` — FOUND
- `internal/db/migrations/0022_install_capabilities.up.sql` — FOUND
- `internal/db/migrations/0022_install_capabilities.down.sql` — FOUND
- `internal/db/migrations/0023_device_profile_expected_interval.up.sql` — FOUND
- `internal/db/migrations/0023_device_profile_expected_interval.down.sql` — FOUND
- Commit `4a2a090` (Task 1) — FOUND in `git log`
- Commit `7a69321` (Task 2) — FOUND in `git log`
- Commit `b2419c3` (Task 3) — FOUND in `git log`
- Commit `184522a` (Task 4) — FOUND in `git log`
- Commit `e364f78` (Task 5) — FOUND in `git log`
- `go build ./...` — exits 0
- `sqlc generate` — exits 0
- `go test ./internal/db/... -run TestRunMigrations_RoundTrip -count=1` — PASS
- `go test ./internal/profile/... -run "TestSeed_ExpectedIntervalS_PerProfile" -count=1` — PASS
- Full `-short` suite (346 tests across 28 packages) — PASS

---
*Phase: 04-realtime-dashboard*
*Plan: 01*
*Completed: 2026-05-11*
