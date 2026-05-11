---
phase: 03-provisioning-gateways-devices-bulk-import
plan: 02
subsystem: database
tags: [postgres, timescaledb, migrations, authz, audit, rbac, schema, sqlc]

# Dependency graph
requires:
  - phase: 02
    provides: ["audit_log table + CHECK vocab (0016)", "touch_updated_at function (0002)", "device table for FK targets (0012)", "user table for FK targets (0002)", "Phase 2 authz Action constants + roleBundles"]
provides:
  - "gateway table with operator metadata, soft-delete snapshot, stats cache columns (0018)"
  - "import_job + import_job_row tables with status enums and 1h preview TTL (0019)"
  - "audit_log CHECK vocab extended for Phase 3: gateway.{create,update,archive,restore}, device.bulk_import, device.reveal_secrets actions; gateway + import_job entity types (0020)"
  - "internal/audit constants: ActionGateway{Create,Update,Archive,Restore}, ActionBulkImport, ActionRevealSecrets, EntityTypeGateway, EntityTypeImportJob"
  - "internal/auth/authz.go: ActionGateway{Create,Update,Archive,Restore,Read}, ActionDeviceBulkImport, ActionDeviceRevealSecrets wired into roleBundles"
affects: [03-04, 03-05, 03-06, 03-07, 03-08, 03-09, 03-10, all Phase 3 plans depending on gateway/import_job/audit vocab]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "CHECK-vocab DROP+re-ADD extension pattern (preserve constraint names so dependent diagnostics/observability remain valid)"
    - "Two-phase import job (import_job + import_job_row with ON DELETE CASCADE + UNIQUE row_index)"
    - "Stats cache co-located with entity row (gateway.stats_* columns refreshed by single-flight async)"
    - "Soft-delete + archived_snapshot JSONB (preserve external proto for faithful restore)"
    - "Partial indexes gated on archived_at IS NULL for hot-path list queries"
    - "Phase-3 RBAC: every mutating action admin-only; single .read action mirrors Phase 2 read patterns; viewer fail-closed"

key-files:
  created:
    - "internal/db/migrations/0018_gateway.up.sql"
    - "internal/db/migrations/0018_gateway.down.sql"
    - "internal/db/migrations/0019_import_job.up.sql"
    - "internal/db/migrations/0019_import_job.down.sql"
    - "internal/db/migrations/0020_audit_log_vocabulary.up.sql"
    - "internal/db/migrations/0020_audit_log_vocabulary.down.sql"
    - "internal/db/migrations_steps_test.go"
    - ".planning/phases/03-provisioning-gateways-devices-bulk-import/03-02-SUMMARY.md"
  modified:
    - "internal/audit/log.go"
    - "internal/auth/authz.go"
    - "internal/auth/can_phase3_test.go"
    - "internal/db/migrations_test.go"
    - "internal/db/roundtrip_test.go"

key-decisions:
  - "Constraint-name preservation: audit_log_action_valid + audit_log_entity_type_valid stay named the same after DROP+re-ADD in 0020 so dependent diagnostics/observability + the existing Phase 2 regression assertions remain valid."
  - "import_job owner_id ON DELETE SET NULL (not CASCADE) — preserves audit-style observability of imports even if the owner user is hard-deleted in a Phase 6 admin emergency."
  - "gateway soft-delete uses archived_snapshot JSONB to capture the ChirpStack Gateway proto at archive-time, so RestoreGateway can re-create the CS row faithfully (D-30 verbatim)."
  - "Stats cache columns live on the gateway row (not a sidecar table) — single-row UPDATE refresh is cheap and avoids join overhead on the hot list query (D-02)."
  - "RBAC: ActionGatewayRead is the only Phase 3 action viewers retain. Matches Phase 2 pattern (site.read / device.read viewer-allowed) so the dashboard's read-only role works for gateway pages."

patterns-established:
  - "CHECK vocabulary extension: ALTER TABLE ... DROP CONSTRAINT ... + ADD CONSTRAINT ... with the same name, preserving the Phase 2 baseline set verbatim and adding new literals. Down migration reverts identically. Future phases adding new audit vocabulary follow this exact shape."
  - "Test helper runMigrateSteps in migrations_steps_test.go for partial up/down migration tests (not exposed in production migrations.go; lives in _test.go so production never depends on Steps())."
  - "RBAC namespacing: dotted action strings (gateway.create, device.reveal_secrets) over flat verbs — keeps audit-log scans groupable by namespace."

requirements-completed: []

# Metrics
duration: 31min
completed: 2026-05-11
---

# Phase 3 Plan 02: Schema + Authz Foundation Summary

**Three migrations (0018 gateway, 0019 import_job, 0020 audit_log vocab extension) + 6 new authz actions + 6 new audit constants land the entire schema + RBAC foundation every downstream Phase 3 plan depends on.**

## Performance

- **Duration:** ~31 min
- **Started:** 2026-05-11T05:54:00Z
- **Completed:** 2026-05-11T06:25:34Z
- **Tasks:** 4 (3 implementation tasks TDD + 1 verification gate)
- **Files modified:** 11 (6 new SQL files, 1 new test helper, 4 modified Go files)

## Accomplishments

- **Migration 0020** extends `audit_log_action_valid` + `audit_log_entity_type_valid` CHECK constraints with Phase 3 vocab (6 new actions, 2 new entity types) using a DROP+re-ADD with name preservation.
- **Migration 0018** creates the `gateway` table with 20 columns: identity (`id`, `gateway_id`, `name`), metadata (`description`, `region`, `lat`, `lng`, `altitude`, `tags`, `cs_tenant_id`), 5 stats-cache columns (D-02), 3 soft-delete columns including `archived_snapshot` JSONB (D-30), plus standard timestamps. CHECK invariants: lowercase hex16 EUI, lat/lng range, name non-empty. Partial indexes on `archived_at IS NULL` and `region WHERE archived_at IS NULL`.
- **Migration 0019** creates `import_job_status` + `import_job_row_status` enums and the `import_job` + `import_job_row` tables. `import_job` carries 5 counters (valid/invalid/already_exists/created/failed) + `expires_at` for the 1h preview TTL. `import_job_row` carries `raw_payload` JSONB for errors.xlsx round-trip and `created_device_id` FK to `device`. ON DELETE CASCADE removes child rows when the job is deleted.
- **Audit constants:** 6 new `Action*` + 2 new `EntityType*` constants in `internal/audit/log.go` exactly mirror the CHECK vocabulary strings (any drift = 23514 at write).
- **Authz:** 7 new `Action*` constants in `internal/auth/authz.go` (6 mutating admin-only + `ActionGatewayRead` viewer-allowed). RoleAdmin bundle grants all 7; RoleViewer bundle grants only `ActionGatewayRead`. Fail-closed for anonymous/zero-User across every Phase 3 action.

## Task Commits

Each task committed atomically following TDD where flagged:

1. **Task 1: Migration 0020 + audit constants + tests** — `cd74d04` (feat, includes TDD RED→GREEN for 0020 Apply/AcceptsNew/Down)
2. **Task 2: Migrations 0018 + 0019 + tests** — `c6d1baf` (feat, TDD RED→GREEN for 4 new Phase 3 migration tests; also bumped existing version assertions 17 → 20)
3. **Task 3: Authz extension + Phase 3 RBAC tests** — `31bc40d` (feat, RED→GREEN for can_phase3_test.go 7 new tests)
4. **Task 4: BLOCKING verification gate** — no commit (verification-only; `go test ./internal/db -run TestPhase3Migrations` passes 7/7 against fresh testcontainer)

**Plan metadata commit:** *(SUMMARY.md + ROADMAP commit follows this section)*

## Files Created/Modified

### Created

- `internal/db/migrations/0018_gateway.up.sql` — gateway table + CHECKs + indexes + trigger
- `internal/db/migrations/0018_gateway.down.sql` — drops trigger, indexes, table CASCADE
- `internal/db/migrations/0019_import_job.up.sql` — 2 enums + 2 tables + indexes + trigger
- `internal/db/migrations/0019_import_job.down.sql` — drops in reverse-dependency order
- `internal/db/migrations/0020_audit_log_vocabulary.up.sql` — DROP+re-ADD vocab CHECKs
- `internal/db/migrations/0020_audit_log_vocabulary.down.sql` — reverts to Phase 2 vocab
- `internal/db/migrations_steps_test.go` — test-only `runMigrateSteps` helper (calls `migrate.Steps()` for partial up/down)
- `.planning/phases/03-provisioning-gateways-devices-bulk-import/03-02-SUMMARY.md` — this file

### Modified

- `internal/audit/log.go` — added 6 `Action*` + 2 `EntityType*` Phase 3 constants
- `internal/auth/authz.go` — added 7 `Action*` Phase 3 const + 7 admin bundle entries + 1 viewer bundle entry
- `internal/auth/can_phase3_test.go` — replaced 6 `t.Skip` stubs with real RBAC assertions + 2 new tests (gateway.read viewer-allowed; anonymous-denied for every Phase 3 action)
- `internal/db/migrations_test.go` — replaced 6 `t.Skip` stubs with real assertions for 0018/0019/0020 apply + down; bumped `TestRunMigrations_Clean` + `TestRunMigrations_Idempotent` expected version 17 → 20
- `internal/db/roundtrip_test.go` — bumped expected version 17 → 20

## Schema Invariants Documented

### 0020 audit_log CHECK vocabulary (canonical SQL)

```sql
ALTER TABLE audit_log DROP CONSTRAINT audit_log_action_valid;
ALTER TABLE audit_log ADD CONSTRAINT audit_log_action_valid CHECK (action IN (
    'create','update','archive','restore','swap','decommission',
    'profile_create','profile_update','binding_open','binding_close',
    'rollover_detected',
    'gateway.create','gateway.update','gateway.archive','gateway.restore',
    'device.bulk_import','device.reveal_secrets'
));

ALTER TABLE audit_log DROP CONSTRAINT audit_log_entity_type_valid;
ALTER TABLE audit_log ADD CONSTRAINT audit_log_entity_type_valid CHECK (entity_type IN (
    'site','metering_point','device','device_profile','binding',
    'gateway','import_job'
));
```

Future Phase 4+ planners extending the vocabulary copy this exact shape (DROP+re-ADD, preserve constraint names, list ALL prior values).

### 0018 gateway invariants enforced at DB layer

- `gateway_eui_lower` — `gateway_id = lower(gateway_id)` (no uppercase letters slip through)
- `gateway_eui_hex16` — `gateway_id ~ '^[0-9a-f]{16}$'` (exact 16-char lowercase hex)
- `gateway_name_not_empty` — `length(name) > 0` (no blank names from a dialog that didn't validate)
- `gateway_lat_range` — `lat IS NULL OR (lat BETWEEN -90 AND 90)`
- `gateway_lng_range` — `lng IS NULL OR (lng BETWEEN -180 AND 180)`
- UNIQUE on `gateway_id` (no duplicate EUIs across active + archived rows)

### 0019 import_job invariants enforced at DB layer

- `file_format CHECK (file_format IN ('xlsx','csv'))` — no rogue uploaders
- `import_job_status` enum: `('preview','committed','expired','failed')`
- `import_job_row_status` enum: `('valid','invalid','already_exists','created','failed')`
- `import_job_row.import_job_id` FK with **ON DELETE CASCADE** — deleting a job removes its rows atomically
- `UNIQUE (import_job_id, row_index)` — re-uploading the same row index in the same job is a constraint violation, not a silent overwrite
- `import_job_status_idx ON import_job (status, expires_at) WHERE status = 'preview'` — partial index keeps the expiry sweeper query fast as committed/expired/failed jobs accumulate

## Phase 3 RBAC Matrix

| Action                       | Admin | Viewer | Anonymous |
| ---------------------------- | :---: | :----: | :-------: |
| `gateway.create`             |  ✓    |   ✗    |    ✗      |
| `gateway.update`             |  ✓    |   ✗    |    ✗      |
| `gateway.archive`            |  ✓    |   ✗    |    ✗      |
| `gateway.restore`            |  ✓    |   ✗    |    ✗      |
| `gateway.read`               |  ✓    |   ✓    |    ✗      |
| `device.bulk_import`         |  ✓    |   ✗    |    ✗      |
| `device.reveal_secrets`      |  ✓    |   ✗    |    ✗      |

Tests in `internal/auth/can_phase3_test.go` exercise every cell.

## Decisions Made

See `key-decisions` in frontmatter. Five decisions captured (constraint-name preservation, owner_id SET NULL vs CASCADE, archived_snapshot pattern, stats co-located on row, gateway.read viewer-allowed).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Bumped existing schema_migrations version assertions 17 → 20**
- **Found during:** Task 2 (running existing `TestRunMigrations_Clean` + `TestRunMigrations_Idempotent` + `TestRunMigrations_RoundTrip` after adding 0018/0019/0020).
- **Issue:** Three existing tests hard-coded `expected = 17`. Adding three new migrations bumped the latest version to 20, breaking the tests even though the migrations themselves applied cleanly.
- **Fix:** Updated each `require.Equal(t, 17, version)` to `require.Equal(t, 20, version)` with a comment referencing the version bump to Plan 03-02.
- **Files modified:** `internal/db/migrations_test.go` (2 sites), `internal/db/roundtrip_test.go` (1 site).
- **Verification:** Full `go test ./internal/db` now passes 20/20.
- **Committed in:** `c6d1baf` (Task 2 commit).

**2. [Rule 3 - Blocking] Relaxed gateway uppercase-EUI test assertion**
- **Found during:** Task 2 (running `TestPhase3Migrations_0018_Gateway_Apply`).
- **Issue:** The test originally asserted `err.Error()` contains `gateway_eui_lower` after INSERTing `'0123456789ABCDEF'`. Postgres reported `gateway_eui_hex16` instead — both CHECKs are violated by an uppercase string, and Postgres picks one to report (implementation-defined). The plan's test description implied a single constraint would fire.
- **Fix:** Changed the assertion to `require.True(t, strings.Contains(err.Error(), "gateway_eui_lower") || strings.Contains(err.Error(), "gateway_eui_hex16"))` with a comment explaining both CHECKs validly reject uppercase EUIs. Added `"strings"` import.
- **Files modified:** `internal/db/migrations_test.go`.
- **Verification:** `TestPhase3Migrations_0018_Gateway_Apply` passes; assertion remains semantically tight (the row MUST be rejected with one of the two EUI CHECK constraint names — no false positive escape).
- **Committed in:** `c6d1baf` (Task 2 commit).

---

**Total deviations:** 2 auto-fixed (both Rule 3 - Blocking).
**Impact on plan:** Both fixes were strictly mechanical (test-only assertion updates that did not change production schema or authz logic). No scope creep.

## Issues Encountered

- **Auth test timeout (transient):** Initial `go test ./internal/auth -timeout 60s` exceeded the 60s timeout because the auth package includes argon2id tests that take ~3s each (intentional, by design). Re-running with `-timeout 180s` passed 68/68 cleanly. No code change required; the plan's `<verify>` block specified 60s but the existing argon2id tests sometimes brush against it under `-race`.

## User Setup Required

None — no external service configuration needed for schema/RBAC changes. Migrations apply automatically on next `shifter serve` (D-13 auto-migrate).

## Next Phase Readiness

**Wave 1 (this plan) provides:**
- `gateway`, `import_job`, `import_job_row` tables ready for sqlc query generation in Wave 2.
- Extended `audit_log` CHECK vocab — every Phase 3 handler that calls `audit.WriteEntry` with the new constants will commit successfully.
- `auth.Can` admits the 7 new Phase 3 actions — every Phase 3 handler that wraps its mutation in `RequireAction(sm, auth.ActionGatewayCreate)` etc. enforces admin-only access at the router level.

**Ready for Wave 2 (gRPC wrappers, sqlc queries):**
- Plan 03-05 can generate sqlc queries against `gateway` / `import_job` / `import_job_row` immediately.
- Plan 03-06+ handlers can import `audit.ActionGatewayCreate` and `auth.ActionGatewayCreate` without modification.

**Known stubs / open items:** None — Plan 03-02 is schema + RBAC only and ships as a self-contained foundation.

## Self-Check: PASSED

Files verified to exist on disk:
- internal/db/migrations/0018_gateway.up.sql (FOUND)
- internal/db/migrations/0018_gateway.down.sql (FOUND)
- internal/db/migrations/0019_import_job.up.sql (FOUND)
- internal/db/migrations/0019_import_job.down.sql (FOUND)
- internal/db/migrations/0020_audit_log_vocabulary.up.sql (FOUND)
- internal/db/migrations/0020_audit_log_vocabulary.down.sql (FOUND)
- internal/db/migrations_steps_test.go (FOUND)

Commits verified in git log:
- cd74d04 feat(03-02): migration 0020 — extend audit_log CHECK vocab for Phase 3 (FOUND)
- c6d1baf feat(03-02): migrations 0018 (gateway) + 0019 (import_job) tables (FOUND)
- 31bc40d feat(03-02): extend auth.Can() with 7 Phase 3 actions (admin-only + 1 read) (FOUND)

Test verification:
- `go test ./internal/db -run TestPhase3Migrations -count=1` — 7/7 PASS
- `go test ./internal/db ./internal/auth ./internal/audit -count=1 -race` — 99/99 PASS

---
*Phase: 03-provisioning-gateways-devices-bulk-import*
*Completed: 2026-05-11*
