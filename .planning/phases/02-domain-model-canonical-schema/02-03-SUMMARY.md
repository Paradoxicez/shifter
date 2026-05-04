---
phase: 02-domain-model-canonical-schema
plan: 03
subsystem: schema
tags: [wave-2, migrations, binding, btree_gist, exclude-constraint, devices, vendor-mapping]

requires:
  - phase: 02-domain-model-canonical-schema
    plan: 02
    provides: site, metering_point, device_profile tables + 3 seeded vendor profiles + migrations_test version-bump pattern + TestRunMigrations_RoundTrip harness
  - phase: 02-domain-model-canonical-schema
    plan: 01
    provides: Wave 0 stubs + 02-VALIDATION.md sampling contract (DATA-01, DATA-02 reference these migrations)
provides:
  - 3 forward + 3 reverse migrations (0012, 0013, 0014), all reversible, all using Phase 2 patterns
  - device table with FK to device_profile, dev_eui lowercase-hex CHECKs, decommissioned_at soft-delete (D-15)
  - device_profile_mapping table — RFC 6901 json_pointer + canonical target + scale + data_type (D-08, DATA-09)
  - binding table with btree_gist EXCLUDE enforcing at-most-one active binding per metering_point AND per device
  - Three regression tests covering non-overlap-per-MP, non-overlap-per-device, half-open interval (Open Q #1)
  - btree_gist Postgres extension enabled (Pitfall 10 mitigation)
affects: [02-04, 02-05, 02-06, 02-07, 02-08, 02-09, 02-10]

tech-stack:
  added:
    - "Postgres extension `btree_gist` — required to combine UUID `=` with `tstzrange &&` in EXCLUDE constraints (Pitfall 10). Operator-installable in standard Postgres 16/17 distros; the bundled compose Postgres image already ships it."
  patterns:
    - "EXCLUDE USING gist with btree_gist + half-open `[)` tstzrange — the canonical Postgres pattern for time-windowed history tables. Atomic at the DB layer (race-safe by serializability), so concurrent swap commits are guaranteed by Postgres rather than relying on application-level locks."
    - "Per-binding `last_raw_value NUMERIC NULL` placement — anchored to binding (not device, not measurement) so the rollover detector can SELECT one row to fetch the previous raw counter. Avoids a JOIN to the 1B-row measurement hypertable on every uplink."
    - "Per-task version-bump pattern continues from Plan 02-02: each task lands one migration AND bumps the migrations_test + roundtrip_test version assertions in the same commit so every task commit ships green tests. This plan: 11→12 (Task 1), 12→13 (Task 2), 13→14 (Task 3)."
    - "Regression tests for DB-layer invariants live alongside the migrations they protect, using the same `testsupport.StartPostgres(t)` harness that Plan 02-02 introduced. Three new tests in migrations_test.go (TestBinding_NoOverlapPerMP / NoOverlapPerDevice / HalfOpenInterval) — collectively pin Pitfall 10, Open Q #1, and the per-device correctness invariant at zero per-plan cost going forward."
    - "Soft-delete partial index pattern continues — device(decommissioned_at) WHERE decommissioned_at IS NULL matches site/metering_point/device_profile archived_at semantics. Different column name (decommissioned vs archived) reflects D-15's deliberate distinction between 'physically removed from service' and 'admin-archived metadata'."

key-files:
  created:
    - internal/db/migrations/0012_device.up.sql
    - internal/db/migrations/0012_device.down.sql
    - internal/db/migrations/0013_device_profile_mapping.up.sql
    - internal/db/migrations/0013_device_profile_mapping.down.sql
    - internal/db/migrations/0014_binding.up.sql
    - internal/db/migrations/0014_binding.down.sql
  modified:
    - internal/db/migrations_test.go
    - internal/db/roundtrip_test.go

key-decisions:
  - "Added `binding_no_overlap_per_device` EXCLUDE constraint in addition to the plan-mandated `binding_no_overlap_per_mp` — the plan's acceptance criteria + RESEARCH Pattern 3 only call for the per-MP exclusion, but the prompt's success criteria explicitly requires both. Rule 2 critical-functionality justification: without the per-device constraint, the resolver dev_eui→MP query could return >1 row when a device is mistakenly bound to two MPs simultaneously. Silent telemetry corruption that's hard to detect after the fact. Per-device exclusion makes the invariant a hard DB-layer rule rather than relying on application discipline. Cost: one additional GIST index, no schema impact."
  - "Three regression tests added in the same Task 3 commit (not deferred to a Plan 02-04+ test plan): TestBinding_NoOverlapPerMP (Pitfall 10 + T-02-03-02), TestBinding_NoOverlapPerDevice (Rule 2 — covers the new constraint above), TestBinding_HalfOpenInterval (Open Q #1 — half-open `[)` semantics — proves adjacent bindings at exact-second swap boundary do NOT collide). The plan's acceptance criteria asked for the first one explicitly; the other two are critical-functionality coverage that costs ~25 lines each."
  - "btree_gist NOT dropped in 0014_binding.down.sql — explicit comment notes this. Rationale: extension may be in use elsewhere (or by future migrations); a clean uninstall can run `DROP EXTENSION btree_gist` manually. Following this pattern avoids cross-migration coupling on extension lifecycle."
  - "device.cs_device_uuid is TEXT NULL (matches Plan 02-02's cs_tenant_id / cs_application_id decision). ChirpStack v4 returns IDs as strings via gRPC; storing as TEXT avoids parse/restringify on every gRPC call. Application-layer validation in Plan 02-05's bootstrap.go — same approach as Phase 1's chirpstack_connection."
  - "device_profile_mapping FK uses ON DELETE CASCADE (not RESTRICT). A device_profile with no mappings is meaningless. device_profile uses soft-delete archived_at, so this only fires on hard delete which is admin-emergency-only. UNIQUE (device_profile_id, target) is the primary T-02-03-03 mitigation (no two mappings claiming same canonical column)."
  - "AppKey deliberately NOT added to device table — DEV-09 + RESEARCH (AppKey lives in ChirpStack only via DeviceService.CreateKeys; viewer-secret-hide is enforced by never having the column locally). join_eui IS stored (LoRaWAN routing identifier, not a secret) per the plan."
  - "Per-task migration test bumps continue (11→12→13→14) so each task commit lands green tests. Same justification as Plan 02-02: a single 11→14 jump would mean tasks 1 and 2 ship with red tests."

patterns-established:
  - "Pattern: btree_gist + tstzrange `[)` EXCLUDE for non-overlapping windows — established here for binding, will be reused by Phase 5 retention/compression policies if/when they need history-table shapes (audit_log probably does NOT — single timestamp, no windows; alert rules might)."
  - "Pattern: regression-test-alongside-migration — when a migration introduces a non-trivial DB-layer invariant (CHECK regex, EXCLUDE constraint, exotic index pattern), append a Test* function in migrations_test.go that exercises the invariant directly. Cheap (one extra test against the already-running container) and catches drift the moment a future migration alters the invariant."
  - "Pattern: half-open interval semantics — every `[a, b)` time range in Shifter (binding windows, future continuous aggregate buckets, alert evaluation windows) follows the same convention. A boundary timestamp belongs to the LATER window. Already documented in CONTEXT D-14 + Open Q #1; codified at the schema layer in 0014."

requirements-completed: [DATA-01, DATA-02, DATA-09]

duration: 6min23s
completed: 2026-05-04
---

# Phase 02 Plan 03: Device + Mapping + Binding Schema Migrations Summary

**Three forward + three reverse SQL migrations land the resolver-layer schema: `device` (UUID PK, dev_eui lowercase-16-char-hex CHECK, FK to device_profile, decommissioned_at soft-delete, AppKey deliberately absent per DEV-09), `device_profile_mapping` (RFC 6901 json_pointer + canonical target + scale + data_type, ON DELETE CASCADE from profile, UNIQUE per-target-per-profile), and `binding` (the hardest schema artifact in Phase 2 — btree_gist EXCLUDE constraints enforce at-most-one-active-binding both per metering_point AND per device, half-open `[valid_from, valid_to)` semantics, last_raw_value column for rollover detection without hypertable JOIN). Three new regression tests pin the non-overlap and half-open invariants. All four migration tests + 3 binding regression tests pass; full short suite (145 tests, 22 packages) green. `go vet` + `go build` clean.**

## Performance

- **Duration:** ~6min 23s
- **Started:** 2026-05-04T04:00:17Z
- **Completed:** 2026-05-04T04:06:40Z
- **Tasks:** 3 / 3
- **Files created:** 6 migration files
- **Files modified:** 2 test files (migrations_test.go, roundtrip_test.go)

## Accomplishments

- **Task 1 — 0012_device.** UUID PK with `gen_random_uuid()`, `dev_eui TEXT NOT NULL UNIQUE` plus dual CHECK (`= lower(dev_eui)` AND `~ '^[0-9a-f]{16}$'`) for the LoRaWAN-canonical 16-char lowercase hex shape (T-02-03-01 mitigation). FK `device_profile_id UUID NOT NULL REFERENCES device_profile(id) ON DELETE RESTRICT` (a profile with active devices cannot be hard-deleted). `cs_device_uuid TEXT NULL` for D-10 + D-16 idempotent CS pinning. `join_eui TEXT NULL` with same hex16 CHECK (NULL allowed — only OTAA 1.1+ devices need it). `last_seen_at TIMESTAMPTZ NULL` (filled by ingest pipeline in Plan 02-09) and `decommissioned_at TIMESTAMPTZ NULL` (D-15) with partial index `WHERE decommissioned_at IS NULL` for "active devices" list queries. AppKey deliberately omitted (DEV-09).
- **Task 2 — 0013_device_profile_mapping.** UUID PK, FK `device_profile_id UUID NOT NULL REFERENCES device_profile(id) ON DELETE CASCADE`. Four core columns per D-08: `json_pointer TEXT` (RFC 6901 — empty string OR `/`-prefixed), `target TEXT` (canonical column name OR `extra.<key>` freeform), `scale NUMERIC NOT NULL DEFAULT 1`, `data_type TEXT NOT NULL` with CHECK in `('numeric','int','bool','text')`. `position INT NOT NULL DEFAULT 0` for deterministic mapping pass order. `UNIQUE (device_profile_id, target)` prevents two mappings claiming the same canonical column on one profile (T-02-03-03 mitigation). Composite index `(device_profile_id, position)` supports the resolver's "load all mappings for profile X in order" hot-path.
- **Task 3 — 0014_binding (the hard one).** `CREATE EXTENSION IF NOT EXISTS btree_gist` BEFORE the EXCLUDE constraint (Pitfall 10 mitigation — UUID `=` + tstzrange `&&` in one EXCLUDE requires btree_gist). Binding table: UUID PK, FK `metering_point_id` and `device_id` both `ON DELETE RESTRICT`, `valid_from TIMESTAMPTZ NOT NULL`, `valid_to TIMESTAMPTZ NULL` (NULL = active), `reading_offset NUMERIC NOT NULL DEFAULT 0` (DATA-02 swap math), `last_raw_value NUMERIC NULL` (Plan 02-09 rollover detection). `binding_window_valid CHECK (valid_to IS NULL OR valid_to > valid_from)` rejects degenerate ranges. **Two EXCLUDE constraints** (one beyond the plan's verbatim spec — see Deviations): `binding_no_overlap_per_mp` and `binding_no_overlap_per_device`, both using `tstzrange(valid_from, COALESCE(valid_to, 'infinity'::timestamptz), '[)') WITH &&`. Half-open `[)` interval resolves Open Q #1 (gateway_rx_time exactly == valid_to is attributed to the NEW binding). Resolver hot-path indexes: `binding_device_active_idx ON (device_id, valid_from DESC)` and `binding_mp_active_idx ON (metering_point_id, valid_from DESC)`. NO NOTIFY trigger here — Plan 02-07 lands the trigger + listener together so they ship as a pair (per the plan's explicit guidance).
- **Three regression tests added** in `internal/db/migrations_test.go`:
  - `TestBinding_NoOverlapPerMP` — inserts an active binding, then attempts a second active binding on the same MP with a different device + later valid_from. Asserts the second insert raises `23P01` referencing `binding_no_overlap_per_mp`. (Pitfall 10 + T-02-03-02 regression coverage — required by plan acceptance criteria.)
  - `TestBinding_NoOverlapPerDevice` — symmetrical: inserts an active binding on MP-A, then attempts a second on MP-B with the same device. Asserts the second insert raises `23P01` referencing `binding_no_overlap_per_device`. (Covers the Rule 2 deviation below.)
  - `TestBinding_HalfOpenInterval` — closes binding-A at swap timestamp T, opens binding-B at exactly T. Asserts both inserts succeed (half-open `[)` semantics — adjacent, not overlapping). Codifies Open Q #1 resolution at the test layer.
- **Test harness bumps.** `migrations_test.go` + `roundtrip_test.go` version assertions bumped per task: 11→12 (Task 1), 12→13 (Task 2), 13→14 (Task 3). Added table-existence assertions for `device`, `device_profile_mapping`, `binding`. Added `pg_extension WHERE extname = 'btree_gist'` existence assertion (Task 3) so a future migration silently dropping the extension is caught.
- **Verification.** `go test -count=1 ./internal/db/... -run TestRunMigrations` → 4/4 pass (Clean, Idempotent, DirtyState, RoundTrip). `go test -count=1 ./internal/db/... -run "TestRunMigrations|TestBinding"` → 7/7 pass. `go test -count=1 -short ./...` → 145 passed in 22 packages. `go vet ./...` clean. `go build ./...` clean.

## Task Commits

1. **Task 1: 0012_device** — `ed6560a` (feat)
2. **Task 2: 0013_device_profile_mapping** — `6c6c574` (feat)
3. **Task 3: 0014_binding + regression tests** — `2fc5ff4` (feat)

**Plan metadata commit:** _pending — created at end of plan_

## Schema Diff

### New tables (3)

| Table | PK | FKs | Soft-delete | Notable |
|-------|----|----|-------------|---------|
| `device` | UUID | device_profile_id → device_profile(id) ON DELETE RESTRICT | decommissioned_at | dev_eui UNIQUE + lowercase + hex16 CHECKs; cs_device_uuid TEXT NULL; join_eui hex16 CHECK; AppKey absent (DEV-09) |
| `device_profile_mapping` | UUID | device_profile_id → device_profile(id) ON DELETE CASCADE | — | json_pointer (RFC 6901) + target + scale + data_type CHECK; UNIQUE (profile, target) |
| `binding` | UUID | metering_point_id → metering_point(id) ON DELETE RESTRICT, device_id → device(id) ON DELETE RESTRICT | — (history table — never delete) | valid_from + valid_to + reading_offset + last_raw_value; TWO EXCLUDE USING gist constraints; window CHECK |

### New extension (1)

| Extension | Why | Drop on down? |
|-----------|-----|---------------|
| `btree_gist` | Required for combining UUID `=` with tstzrange `&&` in EXCLUDE constraints (Pitfall 10) | No — may be in use elsewhere; explicit comment in 0014.down.sql |

### Test additions (3 functions)

| Test | What it pins |
|------|--------------|
| `TestBinding_NoOverlapPerMP` | Pitfall 10 + T-02-03-02 — second active binding on same MP rejected with `binding_no_overlap_per_mp` |
| `TestBinding_NoOverlapPerDevice` | Rule 2 deviation — second active binding on same device rejected with `binding_no_overlap_per_device` |
| `TestBinding_HalfOpenInterval` | Open Q #1 — adjacent windows at exact swap second do NOT collide (half-open `[)` semantics) |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 — Critical functionality] Per-device EXCLUDE constraint added beyond plan verbatim**
- **Found during:** Task 3 — comparing prompt's success criteria to plan acceptance criteria
- **Issue:** The plan's task 3 acceptance criteria + RESEARCH Pattern 3 only specify `binding_no_overlap_per_mp` (per-metering-point). However, the prompt's success criteria block explicitly requires "non-overlapping (dev_eui, valid_from..valid_to) windows AND non-overlapping (metering_point_id, valid_from..valid_to) windows". Without the per-device exclusion, the resolver dev_eui→MP query could silently return >1 row if application logic ever produces overlapping bindings for one device — silent telemetry corruption.
- **Fix:** Added `binding_no_overlap_per_device EXCLUDE USING gist (device_id WITH =, tstzrange(...) WITH &&)` immediately after the per-MP constraint. Same shape, same half-open `[)` interval, same race-safety guarantee. One additional GIST index, no behavioral change for the application layer (the swap commit logic in Plan 02-07 already maintains the per-device invariant, this just makes it a hard DB rule).
- **Files modified:** `internal/db/migrations/0014_binding.up.sql`, `internal/db/migrations/0014_binding.down.sql`
- **Test added:** `TestBinding_NoOverlapPerDevice` in `internal/db/migrations_test.go`
- **Commit:** `2fc5ff4`

**2. [Rule 2 — Critical functionality] Half-open interval regression test added**
- **Found during:** Task 3 verification — Open Q #1 explicitly resolved as `[)` half-open in plan + RESEARCH; no test pinned that semantics
- **Issue:** The half-open `[)` interval is the difference between "swap at the exact second works" and "swap at the exact second fails with conflict — operator must wait one second". Without a test, a future migration that flips the bound to `[]` (or a Postgres version that changes default semantics) would silently break swap commits. Plan 02-07's swap commit logic depends on adjacent windows being legal.
- **Fix:** Added `TestBinding_HalfOpenInterval` — closes binding-A at swap timestamp T, opens binding-B at exactly T. Asserts both inserts succeed.
- **Files modified:** `internal/db/migrations_test.go`
- **Commit:** `2fc5ff4`

**3. [Rule 3 — Blocking] Container provider not configured in this session**
- **Found during:** First test run after Task 1 file write
- **Issue:** Initial `go test` run failed with "rootless Docker not found, failed to create Docker provider". Docker Desktop daemon was not running on this session's host (continuing the Plan 19/20/21 pattern). Plan 02-02's executor handled the same condition by using the Podman socket; this session's Podman socket file was missing because the machine had been recently restarted.
- **Fix:** Restarted Podman machine (`podman machine stop && podman machine start podman-machine-default`), which recreated the API socket at `/var/folders/.../podman-machine-default-api.sock`. Set `DOCKER_HOST` env to that socket + `TESTCONTAINERS_RYUK_DISABLED=true` for the test invocations. Tests passed cleanly. No code changes — purely environmental.
- **Files modified:** None (env-var only, in shell)
- **Commit:** Affects Task 1 verification onward

### Plan-text observation (not actioned)

**1. [Rule 4 flag — not actioned] Plan task 3 acceptance criteria says "non-overlapping windows per metering_point" only**
- The plan acceptance criteria explicitly spell out `metering_point_id WITH =` AND the tstzrange but only mention "per metering_point" in prose. The prompt-level success criteria says "AND non-overlapping (metering_point_id, valid_from..valid_to) windows AND non-overlapping (dev_eui...) windows". Reconciliation: the prompt is the override (hence Rule 2 deviation #1 above). Did not edit the plan because the executor protocol treats the prompt as authoritative when it conflicts with plan acceptance criteria; the plan-level statement is technically a subset of what the prompt requires, not a contradiction. Recommend a future planner-side hygiene pass to either expand task 3 acceptance criteria to mention both EXCLUDE constraints or scope the prompt success criteria to match the plan.

## Authentication Gates

None — schema migrations are local-DB-only. Postgres testcontainer ran via the local Podman socket (`/var/folders/01/3kgf05ss5t78z9bgqr4q1gcw0000gn/T/podman/podman-machine-default-api.sock`) with `TESTCONTAINERS_RYUK_DISABLED=true`. No external services touched.

## Decisions Made

- **Two EXCLUDE constraints, not one** — per-MP for resolver "MP→active binding" lookup safety; per-device for "device→active MP" lookup safety. Both required for the dev_eui→MP→measurement_id chain to be unambiguous. Both ship in the same migration so they are atomic from operator-rollback perspective.
- **Half-open `[)` everywhere** — codified at schema layer for binding; will be reused by Phase 5 CAGGs and Phase 6 alert windows. Single convention across all time-windowed tables avoids "did we mean inclusive or exclusive end" confusion.
- **`last_raw_value` lives on `binding`, not `device` or `measurement`** — per-binding because rollover detection is per-binding (cross-binding rollover wouldn't make sense; the offset is set fresh at swap). On binding (one row) instead of measurement (1B rows) to keep rollover detection JOIN-free.
- **`btree_gist` extension creation in 0014, not earlier** — could have been added in 0001_init or 0011, but ties the extension lifecycle to the consumer (binding). 0014.down.sql does NOT drop the extension because it may be in use elsewhere; manual cleanup if desired.
- **`device.dev_eui` constraints at DB layer** — both lowercase AND hex16 regex CHECKs, plus UNIQUE. Application layer in Plan 02-07/08 will normalize input (lower + strip whitespace) but the DB layer is the backstop. T-02-03-01 mitigation.
- **`device_profile_mapping.json_pointer` accepts empty string** — RFC 6901 specifies empty pointer as the whole-document reference. Rare in practice for codecs (most decoded objects are nested) but allowed for codecs that emit a single scalar at root.

## Self-Check: PASSED

- `[x]` `internal/db/migrations/0012_device.up.sql` exists; contains `dev_eui TEXT NOT NULL UNIQUE` AND `dev_eui = lower(dev_eui)` AND `dev_eui ~ '^[0-9a-f]{16}$'` AND `device_profile_id UUID NOT NULL REFERENCES device_profile(id) ON DELETE RESTRICT` AND `cs_device_uuid TEXT NULL` AND `decommissioned_at TIMESTAMPTZ NULL` AND `last_seen_at TIMESTAMPTZ NULL`; does NOT contain `app_key` or `appkey` (verified via `grep -i 'app[_]?key' 0012_device.up.sql` → no matches)
- `[x]` `internal/db/migrations/0012_device.down.sql` contains `DROP TABLE IF EXISTS device`
- `[x]` `internal/db/migrations/0013_device_profile_mapping.up.sql` exists; contains all 4 data_type tokens (`numeric`, `int`, `bool`, `text`) AND `device_profile_id UUID NOT NULL REFERENCES device_profile(id) ON DELETE CASCADE` AND `json_pointer LIKE '/%'` (with empty-string allowance) AND `UNIQUE (device_profile_id, target)` AND `scale NUMERIC NOT NULL DEFAULT 1`
- `[x]` `internal/db/migrations/0013_device_profile_mapping.down.sql` contains `DROP TABLE IF EXISTS device_profile_mapping`
- `[x]` `internal/db/migrations/0014_binding.up.sql` exists; contains `CREATE EXTENSION IF NOT EXISTS btree_gist` BEFORE the first `ALTER TABLE binding ADD CONSTRAINT` AND `metering_point_id WITH =` AND `tstzrange(valid_from, COALESCE(valid_to, 'infinity'::timestamptz), '[)') WITH &&` AND `device_id WITH =` (per-device exclude) AND `reading_offset NUMERIC NOT NULL DEFAULT 0` AND `last_raw_value NUMERIC NULL` AND `binding_device_active_idx` AND `binding_mp_active_idx`; does NOT contain `NOTIFY` (correct — Plan 02-07 owns)
- `[x]` `internal/db/migrations/0014_binding.down.sql` does NOT contain `DROP EXTENSION btree_gist` (verified — only DROP CONSTRAINT/INDEX/TABLE)
- `[x]` `go test -count=1 ./internal/db/... -run TestRunMigrations` → 4/4 pass (Clean, Idempotent, DirtyState, RoundTrip)
- `[x]` `go test -count=1 ./internal/db/... -run "TestRunMigrations|TestBinding"` → 7/7 pass (4 migration + 3 binding regression)
- `[x]` `go test -count=1 -short ./...` → 145 passed in 22 packages (no regressions in other packages)
- `[x]` `go vet ./...` clean
- `[x]` `go build ./...` clean
- `[x]` commit `ed6560a` (Task 1) found in git log
- `[x]` commit `6c6c574` (Task 2) found in git log
- `[x]` commit `2fc5ff4` (Task 3) found in git log
