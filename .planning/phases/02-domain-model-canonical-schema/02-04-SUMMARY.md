---
phase: 02-domain-model-canonical-schema
plan: 04
subsystem: schema
tags: [wave-3, migrations, timescaledb, hypertable, audit-log, insert-only]

requires:
  - phase: 02-domain-model-canonical-schema
    plan: 03
    provides: device + device_profile_mapping + binding tables, btree_gist extension, migrations_test version-bump pattern, roundtrip harness
  - phase: 02-domain-model-canonical-schema
    plan: 02
    provides: site + metering_point + device_profile tables, seeded vendor profiles, partial-index pattern
  - phase: 01-foundation
    plan: 03
    provides: golang-migrate library mode + iofs embed + 0001_init enables timescaledb extension + 0002_users defines "user" table
provides:
  - 2 forward + 2 reverse migrations (0015, 0016) completing the Phase 2 schema (16 migrations total)
  - measurement TimescaleDB hypertable with chunk_time_interval=1d, full D-02 canonical column set, 3 critical indexes, MP-keyed (no device_id / dev_eui — DATA-01 invariant)
  - audit_log regular table (NOT hypertable per D-06) with 10 D-22 columns, INSERT-ONLY trigger enforcement (T-02-04-02), 4 browse/filter indexes
  - 3 audit_log regression tests pinning UPDATE / DELETE rejection + happy-path INSERT + CHECK vocabulary
  - 2 hypertable assertions in TestRunMigrations_Clean: timescaledb_information.hypertables row + chunk_time_interval=1 day
  - audit_log NOT-hypertable assertion (defense against accidental hypertable conversion in a future migration)
affects: [02-06, 02-07, 02-08, 02-09, 02-10]

tech-stack:
  added: []
  patterns:
    - "TimescaleDB hypertable creation pattern — function form `create_hypertable('measurement', 'time', chunk_time_interval => INTERVAL '1 day')` per CONTEXT D-06 + RESEARCH Pattern 2. Indexes created AFTER the hypertable conversion so they're inherited by all child chunks."
    - "Hybrid wide+JSONB telemetry schema — Layer-1 typed columns (raw_value, cumulative_value, instant_value, battery_pct, rssi, snr, temperature_c, pressure_kpa, leak_detected, tamper_detected) for the canonical fields the UI always renders + JSONB `extra` for vendor-specific extras. Row width stays bounded; new vendor onboarding never requires schema migration."
    - "Server-side ingest time as authoritative `time` column (Pitfall 4 mitigation) — gateway_rx_time + device_time persist as diagnostics only. CAGGs in Phase 5 will bucket on `time`, never on gateway_rx_time."
    - "INSERT-ONLY substrate via BEFORE UPDATE / BEFORE DELETE triggers — RAISE EXCEPTION inside a plpgsql function rejects the operation with a descriptive message ('audit_log is INSERT-ONLY'). Application code MUST only INSERT; the DB is the backstop. Tampering requires DBA-level direct DB access, which is auditable at the OS layer."
    - "Per-task version-bump pattern continues from 02-02/02-03 — 14→15 (Task 1), 15→16 (Task 2) so each task commit lands green tests. Single 14→16 jump would have meant Task 1's commit ships with a red test."
    - "Regression-test-alongside-migration — when a migration introduces a non-trivial DB-layer invariant (CHECK, EXCLUDE, or trigger-based rejection), pin it with a Test* function in migrations_test.go. 0014 added 3 binding regression tests; 0016 adds 3 audit_log tests."

key-files:
  created:
    - internal/db/migrations/0015_measurement.up.sql
    - internal/db/migrations/0015_measurement.down.sql
    - internal/db/migrations/0016_audit_log.up.sql
    - internal/db/migrations/0016_audit_log.down.sql
  modified:
    - internal/db/migrations_test.go
    - internal/db/roundtrip_test.go

key-decisions:
  - "Added `binding_id UUID` column to measurement (NULLABLE, no FK) — forward-compat for Plan 02-09 ingest pipeline. Resolver will capture the active binding at insert time so future Phase 4/5 queries can JOIN through historical binding without time-range gymnastics. NULL allowed for edge cases (no binding covers timestamp) which Plan 02-09 will tag with quality='missing_canonical' per D-26 + Open Q#3. The plan's interfaces block listed binding_id; this is implemented exactly as specified."
  - "NO foreign key constraints on measurement table — TimescaleDB hypertable child chunks don't propagate FKs cleanly across all versions. Application layer (ingest pipeline in Plan 02-09) writes only valid metering_point_id by construction (resolver returns valid MPs only); the binding_no_overlap_per_mp + per_device EXCLUDE constraints on `binding` already enforce the consistency we need. This matches the plan note + RESEARCH guidance."
  - "Three audit_log regression tests added in Task 2 (not deferred to Plan 02-08): TestAuditLog_RejectsUpdate (T-02-04-02 — UPDATE raises 'INSERT-ONLY'), TestAuditLog_RejectsDelete (T-02-04-02 — DELETE raises), TestAuditLog_AcceptsInsertAndPersistsDiff (D-22 + D-24 happy path + CHECK rejects unknown action / entity_type). Plan acceptance criteria asks for the first two; the third is critical-functionality coverage that costs ~30 lines (Rule 2 — pins the controlled vocabulary CHECKs at the DB layer so a future migration that loosens them is caught by CI)."
  - "audit_log.user_id uses ON DELETE SET NULL (not RESTRICT) — defensive against admin-emergency hard delete of a user. Phase 6 adds USER-01 disable-instead-of-delete; until then, an admin who hard-deletes a user via direct DB access keeps the audit trail intact (with user_id NULL) instead of cascading the trail away. Plan note explicitly justifies this choice."
  - "Two hypertable assertions in TestRunMigrations_Clean — `is_hypertable=true` AND `chunk_time_interval = 1 day`. The latter catches a future migration that creates the hypertable with the wrong interval (e.g. forgetting `chunk_time_interval => INTERVAL '1 day'` falls back to TimescaleDB's 7-day default — silent perf regression). Cost: one extra SELECT, no per-test fixture impact."
  - "audit_log NOT-hypertable assertion is a defense-in-depth complement to the measurement hypertable assertion — if a future plan accidentally calls `create_hypertable('audit_log', ...)` (perhaps by copy-paste from 0015), the test fails immediately. CONTEXT D-06 explicitly chose regular table for audit_log; codifying that choice at the test layer is cheap insurance."

patterns-established:
  - "Pattern: TimescaleDB hypertable creation — function form `SELECT create_hypertable('table', 'time', chunk_time_interval => INTERVAL '1 day')` after the CREATE TABLE, before indexes. Phase 5 CAGGs (Plan 02-NA / Phase 5) will follow the same shape. Hybrid wide+JSONB schema (canonical columns + extra JSONB) extends to any future hypertable that ingests vendor-shaped data."
  - "Pattern: INSERT-ONLY table via BEFORE UPDATE / BEFORE DELETE triggers raising RAISE EXCEPTION — reusable for any audit / immutable-log table. The trigger function is `audit_log_reject_modification` (specific to audit_log naming); a future immutable table (e.g. compliance log, tamper-evident event ledger) can copy the pattern with its own function name."
  - "Pattern: hypertable assertion via timescaledb_information.hypertables — `SELECT EXISTS(SELECT 1 FROM timescaledb_information.hypertables WHERE hypertable_name = 'X')` is the canonical Postgres-side assertion. Use the same query negated to assert a table is NOT a hypertable when D-06-style decisions need codification."

requirements-completed: [DATA-01, DATA-03, DATA-07, DATA-08, AUDIT-01]

duration: 5min37s
completed: 2026-05-04
---

# Phase 02 Plan 04: Measurement Hypertable + Audit Log Migrations Summary

**Two forward + two reverse SQL migrations complete the Phase 2 schema: `0015_measurement` is the lone TimescaleDB hypertable in the project (chunk_time_interval=1 day per CONTEXT D-06; full D-02 canonical column set + JSONB `extra` for vendor-specific data; deliberately MP-keyed with NO device_id / dev_eui per the DATA-01 invariant; server-side `time` is authoritative per DATA-03 + Pitfall 4; gateway_rx_time + device_time persist as diagnostics; quality TEXT CHECK enumerates the 5 D-26 values). `0016_audit_log` is a regular Postgres table (NOT a hypertable per D-06) with 10 D-22 columns and BEFORE UPDATE / BEFORE DELETE trigger enforcement that raises 'audit_log is INSERT-ONLY' (T-02-04-02 mitigation). Three new audit_log regression tests pin the rejection invariants and the controlled-vocabulary CHECKs. Plan 06 (sqlc generation) can now proceed against the complete 16-migration schema. All 10 db tests pass (4 migration + 3 binding + 3 audit_log); full short suite 176/176 green; `go vet` and `go build` clean.**

## Performance

- **Duration:** ~5min 37s
- **Started:** 2026-05-04T04:43:56Z
- **Completed:** 2026-05-04T04:49:33Z
- **Tasks:** 2 / 2
- **Files created:** 4 migration files
- **Files modified:** 2 test files (migrations_test.go, roundtrip_test.go)

## Accomplishments

- **Task 1 — 0015_measurement.** TimescaleDB hypertable with the verbatim D-02 canonical column set: `time TIMESTAMPTZ NOT NULL` (server-side ingest, Pitfall 4 mitigation), `metering_point_id UUID NOT NULL` (DATA-01 — NEVER device_id), then the full Layer-1 set (raw_value, cumulative_value, instant_value, battery_pct, rssi, snr, temperature_c, pressure_kpa, leak_detected, tamper_detected), plus `extra JSONB NOT NULL DEFAULT '{}'::jsonb` for vendor-specific fields, `raw_payload BYTEA NOT NULL`, `decoded_object JSONB NOT NULL`, `quality TEXT NOT NULL DEFAULT 'ok'` with 5-value CHECK (T-02-04-01 mitigation), plus `fcnt`, `gateway_rx_time`, `device_time`, `binding_id` for the ingest pipeline. `SELECT create_hypertable('measurement', 'time', chunk_time_interval => INTERVAL '1 day')` per RESEARCH Pattern 2 + CONTEXT D-06. Three indexes: `(metering_point_id, time DESC)` hot-path, `GIN (extra jsonb_path_ops)` for vendor advanced view, partial `(metering_point_id, time DESC) WHERE quality <> 'ok'` for the flagged-uplinks badge. NO LISTEN/NOTIFY trigger (Phase 4 SSE owns that). NO FK constraints (TimescaleDB FK-on-hypertable caveats; binding EXCLUDEs already enforce consistency).
- **Task 2 — 0016_audit_log.** 10-column D-22 schema: `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`, `time TIMESTAMPTZ NOT NULL DEFAULT now()`, `user_id UUID NULL REFERENCES "user"(id) ON DELETE SET NULL`, `action TEXT NOT NULL`, `entity_type TEXT NOT NULL`, `entity_id UUID NOT NULL`, `before JSONB`, `after JSONB`, `notes TEXT`, `request_id TEXT`. CHECK constraints enumerate the D-22 + D-05 action vocabulary (create, update, archive, restore, swap, decommission, profile_create, profile_update, binding_open, binding_close, rollover_detected) and the 5 D-22 entity types (site, metering_point, device, device_profile, binding). Four browse/filter indexes for the Phase 6 audit viewer: time DESC, (user_id, time DESC), (entity_type, entity_id, time DESC), partial on request_id. **`audit_log_reject_modification()` plpgsql function + BEFORE UPDATE + BEFORE DELETE triggers** raise `RAISE EXCEPTION 'audit_log is INSERT-ONLY ...'` — T-02-04-02 mitigation. Application code MUST only INSERT.
- **Task 2 regression tests (3 new).** `TestAuditLog_RejectsUpdate` — inserts a row, attempts UPDATE, asserts error contains `INSERT-ONLY` and the original row's `notes` is still NULL. `TestAuditLog_RejectsDelete` — same shape for DELETE; asserts the row is still present after the rejected DELETE. `TestAuditLog_AcceptsInsertAndPersistsDiff` — inserts a `update`-action row with before/after JSONB diff, round-trips the JSONB, then probes both CHECK constraints (unknown action `frobnicate` and unknown entity_type `gizmo` both rejected with the named CHECK constraint in the error message).
- **Test harness bumps.** Per the established Phase 2 pattern, version assertions advance per task: `migrations_test` 14→15 (Task 1), 15→16 (Task 2); `roundtrip_test` lockstep. Added `measurement` to the table-existence loop (Task 1), `audit_log` to the same loop (Task 2). Added two new hypertable assertions to `TestRunMigrations_Clean`: `is_hypertable` for `measurement` (must be true) and `is_hypertable` for `audit_log` (must be false — D-06 codification). Added a `chunk_time_interval = 1 day` assertion against `timescaledb_information.dimensions` to catch a future migration that forgets the explicit interval and falls back to the 7-day default.
- **Verification.** `go test -count=1 ./internal/db/... -run "TestRunMigrations|TestBinding|TestAuditLog"` → 10/10 pass (4 migration + 3 binding + 3 audit_log). `go test -count=1 -short ./...` → 176 passed in 22 packages (no regressions in any other package). `go vet ./...` clean. `go build ./...` clean.

## Task Commits

1. **Task 1: 0015_measurement** — `48bd892` (feat)
2. **Task 2: 0016_audit_log + INSERT-ONLY enforcement** — `fc6192f` (feat)

**Plan metadata commit:** _pending — created at end of plan_

## Schema Diff

### New tables (2)

| Table | Type | PK | FKs | Notable |
|-------|------|----|----|---------|
| `measurement` | TimescaleDB hypertable (1d chunks) | (time, metering_point_id) implicit | — (no FK on hypertable per TS caveats) | Full D-02 column set + extra JSONB + raw_payload BYTEA + decoded_object JSONB; quality CHECK (5 values); 3 indexes incl. GIN; binding_id UUID NULL forward-compat |
| `audit_log` | Regular Postgres (NOT hypertable per D-06) | id UUID | user_id → "user"(id) ON DELETE SET NULL | INSERT-ONLY (UPDATE / DELETE rejected by trigger); 4 browse indexes; D-22 action + entity_type CHECKs |

### New triggers / functions (3)

| Object | Type | Purpose |
|--------|------|---------|
| `audit_log_reject_modification()` | plpgsql function | RAISE EXCEPTION 'audit_log is INSERT-ONLY' |
| `audit_log_no_update` | BEFORE UPDATE trigger on audit_log | Calls reject function |
| `audit_log_no_delete` | BEFORE DELETE trigger on audit_log | Calls reject function |

### Test additions (3 functions + 3 hypertable assertions)

| Test / Assertion | What it pins |
|------------------|--------------|
| `TestAuditLog_RejectsUpdate` | T-02-04-02 — UPDATE on audit_log raises 'INSERT-ONLY' |
| `TestAuditLog_RejectsDelete` | T-02-04-02 — DELETE on audit_log raises 'INSERT-ONLY' |
| `TestAuditLog_AcceptsInsertAndPersistsDiff` | D-22 + D-24 happy path + CHECK rejects unknown action / entity_type |
| `is_hypertable=true` for measurement | DATA-01 / D-06 — hypertable conversion succeeded |
| `chunk_time_interval=1 day` for measurement | D-06 — explicit interval, not the 7d TS default |
| `is_hypertable=false` for audit_log | D-06 codification — defense against accidental hypertable conversion |

### Phase 2 schema status (after this plan)

All 10 Phase 2 schema migrations land. Tables: `site`, `metering_point`, `device_profile`, `device`, `device_profile_mapping`, `binding`, `measurement`, `audit_log` (8 new tables) + `chirpstack_connection.cs_tenant_id/cs_application_id` columns added by 0011. Phase 2 schema is complete; Plan 02-06 (sqlc generation) can now run.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 — Critical functionality] Three audit_log regression tests in Task 2, not just two**
- **Found during:** Task 2 acceptance-criteria review
- **Issue:** Plan acceptance criteria asks for "Regression test confirms UPDATE on audit_log raises exception with message containing 'INSERT-ONLY'" + "Regression test confirms DELETE on audit_log raises exception" — the obvious two. But the D-22 action / entity_type CHECK constraints are equally critical (a future migration that loosens them silently changes the audit-trail vocabulary; a future bug that inserts a non-canonical action string would corrupt the audit viewer). Without a happy-path test that probes the CHECKs, the migration could silently regress.
- **Fix:** Added `TestAuditLog_AcceptsInsertAndPersistsDiff` covering: (a) happy-path INSERT works with user_id FK, (b) before/after JSONB round-trips, (c) unknown action `frobnicate` is rejected with `audit_log_action_valid` in the error, (d) unknown entity_type `gizmo` is rejected with `audit_log_entity_type_valid`. ~30 lines, single testcontainer reuse, no perf cost.
- **Files modified:** `internal/db/migrations_test.go`
- **Commit:** `fc6192f`

**2. [Rule 2 — Critical functionality] audit_log NOT-hypertable assertion**
- **Found during:** Task 2 — adding the measurement is_hypertable assertion in Task 1, realized the symmetric assertion was missing
- **Issue:** D-06 explicitly chose regular Postgres for audit_log because hundreds of rows/day per single-tenant install is a B-tree workload, not a hypertable workload. If a future planner copy-pastes the create_hypertable line from 0015 into a new audit-related migration, the silent conversion has no obvious downstream symptom (queries still work, just differently). The plan acceptance criteria says "File does NOT use SELECT create_hypertable(...) for audit_log" but only checks the file content; a future migration could still convert it.
- **Fix:** Added `is_hypertable=false` assertion for `audit_log` in `TestRunMigrations_Clean`. Costs one SELECT against `timescaledb_information.hypertables`; catches accidental hypertable conversion at the test layer for the lifetime of the project.
- **Files modified:** `internal/db/migrations_test.go`
- **Commit:** `fc6192f`

**3. [Rule 2 — Critical functionality] chunk_time_interval = 1 day assertion**
- **Found during:** Task 1 — adding the is_hypertable assertion, realized the interval itself wasn't asserted
- **Issue:** D-06 specifies `chunk_time_interval = INTERVAL '1 day'`. If a future migration calls `create_hypertable` without the explicit interval, TimescaleDB falls back to its 7-day default — silent perf regression for high-cardinality writes (chunk-pruning loses precision). The plan acceptance criteria checks the file contains the literal string but not the resulting state.
- **Fix:** Added `chunk_time_interval / INTERVAL '1 day'` assertion against `timescaledb_information.dimensions` in `TestRunMigrations_Clean`, using `EXTRACT(EPOCH FROM time_interval) / 86400.0` and `require.InDelta(1.0, chunkDays, 0.0001, ...)`. The InDelta tolerance handles potential float rounding without false positives.
- **Files modified:** `internal/db/migrations_test.go`
- **Commit:** `48bd892`

### Plan-text observations (not actioned)

**1. [Rule 4 flag — not actioned] Plan task 1 acceptance criteria mentions "16 columns" but the plan body shows 20**
- The plan body's `CREATE TABLE measurement` block defines 20 columns: time, metering_point_id, raw_value, cumulative_value, instant_value, battery_pct, rssi, snr, temperature_c, pressure_kpa, leak_detected, tamper_detected, extra, raw_payload, decoded_object, quality, fcnt, gateway_rx_time, device_time, binding_id. The acceptance criteria says "all 16 columns from D-02" and lists 16 names (the D-02 frozen Layer-1 set). The plan body includes 4 additional fields (fcnt, gateway_rx_time, device_time, binding_id) that aren't D-02 Layer-1 — they're auxiliary diagnostic / forward-compat columns. Implemented exactly as the plan body shows (20 columns); did not actually verify "16 names from D-02" because the broader 20-column set is what the plan body literally specifies. Recommend a future planner-side hygiene pass to either tighten the acceptance criteria to "all 20 columns" or split the assertion into "16 D-02 + 4 auxiliary."

**2. [Rule 4 flag — not actioned] Plan frontmatter `requirements: [DATA-01, DATA-03, DATA-07, DATA-08, AUDIT-01]` overstates this plan's vertical-completion claim**
- This plan lands schema bones for the requirements: measurement table (DATA-01 schema layer with MP-keying + DATA-03 server-side time + DATA-07 raw/decoded/canonical persistence + DATA-08 hybrid wide+JSONB), audit_log table (AUDIT-01 substrate). BUT none of these requirements is END-TO-END complete after 02-04:
  - DATA-01 ("Telemetry is keyed by metering_point_id") — schema enforces; runtime resolver / ingest in 02-09 actually populates the rows.
  - DATA-03 ("`time` is server-side, never gateway_rx_time") — schema column type; ingest pipeline in 02-09 sets `time = now()`.
  - DATA-07 / DATA-08 — schema columns exist; ingest normalizer in 02-04 (named in VALIDATION map) / 02-09 actually writes the canonical fields.
  - AUDIT-01 — schema substrate exists with INSERT-ONLY enforcement; audit package in 02-08 actually writes rows; same-txn assertion in 02-08 test.
- Per the GSD executor protocol ("Extract the `requirements` array from the plan's frontmatter, then mark each complete"), all five were marked. The 02-VALIDATION.md per-task verification map clearly shows the late-plan ownership; the verifier should treat these check-marks as "schema layer landed" and not as "user-facing requirement delivered." Same recommendation as Plan 02-02 — split each requirement into sub-IDs (e.g. DATA-01-schema, DATA-01-runtime) or only list a requirement in the plan that delivers its final vertical slice. Did NOT edit REQUIREMENTS.md or the plan because the frontmatter is the protocol's source of truth.

## Authentication Gates

None — schema migrations are local-DB-only. Postgres testcontainer ran via the local Podman socket (`unix:///var/folders/01/3kgf05ss5t78z9bgqr4q1gcw0000gn/T/podman/podman-machine-default-api.sock`) with `TESTCONTAINERS_RYUK_DISABLED=true`. Podman machine had to be stopped + restarted at the start of execution to recreate the API socket file (continuing the Plan 02-02 / 02-03 pattern); no code changes — purely environmental.

## Decisions Made

- **Hypertable function form, not declarative `CREATE TABLE WITH (tsdb.hypertable, ...)`** — RESEARCH Pattern 2 prefers the function form because every TigerData example, the docs, and existing tooling (golang-migrate) treat it as the canonical path. The declarative form (TimescaleDB 2.23+) is a newer alternative with less ecosystem coverage. Function form keeps us in well-trodden territory.
- **NO FK constraints on measurement** — TimescaleDB hypertable child chunks have historical FK propagation issues. Application correctness is enforced by: (a) ingest resolver only returns valid metering_point_id by construction, (b) binding_no_overlap_per_mp + per_device EXCLUDEs on `binding` already make the resolver result unambiguous. A FK on a 1B-row table would also be a write-amp source.
- **`binding_id` on measurement is forward-compat, NULLABLE** — captured at insert time so future Phase 4/5 queries can JOIN through historical binding without time-range gymnastics. NULL allowed for edge cases (no binding covers timestamp) which Plan 02-09 will tag with `quality='missing_canonical'` per D-26 + Open Q#3.
- **NO LISTEN/NOTIFY trigger on measurement in Phase 2** — emitting NOTIFY on every uplink with no Phase 2 subscribers is wasted work. Phase 4 SSE owns that trigger; the comment in 0015 explicitly notes the deferral.
- **GIN opclass `jsonb_path_ops`, not default `jsonb_ops`** — the access pattern for `extra` is containment (`extra @> '{...}'`) not generic JSON path queries. `jsonb_path_ops` is smaller and faster for the containment pattern; standard recipe per Postgres GIN-on-jsonb documentation.
- **Partial index on `quality <> 'ok'`** — most rows are 'ok'; a partial index keeps the flagged-only listing fast (Phase 4 D-26 badge) without bloating the index for the common case. Costs nearly nothing on insert (only flagged rows index) and trades constant query speedup.
- **`audit_log_reject_modification` is plpgsql, not internal-only** — the function is reused by both UPDATE and DELETE triggers. Single function, two triggers; cheaper to maintain than two near-identical inlined trigger bodies. Function name is intentionally descriptive so the EXPLAIN output is self-documenting.
- **`ON DELETE SET NULL` for audit_log.user_id** — defensive against admin-emergency hard delete. Phase 6 will add disable-instead-of-delete (USER-01); SET NULL guarantees the audit trail survives even if someone bypasses that policy via direct DB access.
- **D-22 action vocabulary CHECK explicit, no enum** — same rationale as Plan 02-02's capabilities decision: ENUM ALTER TYPE ADD VALUE has historical Postgres edge cases. Plan 02-08 audit package will use these exact 11 strings; growing the vocabulary in v2 is a one-line `ALTER TABLE audit_log DROP CONSTRAINT audit_log_action_valid; ALTER TABLE audit_log ADD CONSTRAINT ... CHECK ( ... )` migration.

## Self-Check: PASSED

- `[x]` `internal/db/migrations/0015_measurement.up.sql` exists; contains literal `create_hypertable('measurement', 'time', chunk_time_interval => INTERVAL '1 day')`, all 20 columns including the 16 D-02 Layer-1 names (raw_value, cumulative_value, instant_value, battery_pct, rssi, snr, temperature_c, pressure_kpa, leak_detected, tamper_detected, extra, raw_payload, decoded_object, quality + time + metering_point_id) plus 4 auxiliary (fcnt, gateway_rx_time, device_time, binding_id), all 5 quality CHECK values, `metering_point_id UUID NOT NULL`, `time TIMESTAMPTZ NOT NULL` first in column order; does NOT contain `device_id` or `dev_eui`; contains the 3 indexes (mp_time, extra_gin with jsonb_path_ops, quality_flagged partial)
- `[x]` `internal/db/migrations/0015_measurement.down.sql` drops the 3 indexes + the table (which cascades to chunks)
- `[x]` `internal/db/migrations/0016_audit_log.up.sql` exists; contains all 10 columns (id, time, user_id, action, entity_type, entity_id, before, after, notes, request_id), `REFERENCES "user"(id) ON DELETE SET NULL`, all 11 D-22 + D-05 actions in CHECK, all 5 D-22 entity types in CHECK, `audit_log_reject_modification` function + `audit_log_no_update` + `audit_log_no_delete` triggers, 4 indexes (time, user, entity, request_id partial); does NOT use `create_hypertable`
- `[x]` `internal/db/migrations/0016_audit_log.down.sql` drops triggers + function + indexes + table cleanly
- `[x]` `go test -count=1 ./internal/db/... -run "TestRunMigrations|TestBinding|TestAuditLog"` exits 0 (10/10: 4 migration + 3 binding + 3 audit_log)
- `[x]` `go test -count=1 -short ./...` exits 0 (176 passed in 22 packages — no regressions)
- `[x]` `go vet ./...` clean
- `[x]` `go build ./...` clean
- `[x]` commit `48bd892` (Task 1) found in git log
- `[x]` commit `fc6192f` (Task 2) found in git log
