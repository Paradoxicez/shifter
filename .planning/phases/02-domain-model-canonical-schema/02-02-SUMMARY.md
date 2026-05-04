---
phase: 02-domain-model-canonical-schema
plan: 02
subsystem: schema
tags: [wave-1, migrations, timescaledb, postgres, golang-migrate, soft-delete, seeds]

requires:
  - phase: 01-foundation
    plan: 03
    provides: golang-migrate library mode + iofs embed + touch_updated_at function + integer-prefix migration naming + chirpstack_connection singleton row
  - phase: 02-domain-model-canonical-schema
    plan: 01
    provides: Wave 0 stubs + 02-VALIDATION.md sampling contract (DATA-01, DATA-02, DATA-08, DATA-10 rows reference these migrations)
provides:
  - 5 forward + 5 reverse migrations (0007..0011), all reversible, all using Phase 1 patterns
  - site, metering_point, device_profile tables ready for sqlc generation in plan 02-03
  - 3 seeded device_profile rows (axioma_w1, acrel_adl200, acrel_adw300) — codec_js empty until plan 02-08 //go:embed boot fill
  - chirpstack_connection.cs_tenant_id + cs_application_id columns ready for plan 02-05 D-28 bootstrap
  - TestRunMigrations_RoundTrip — forward → full down → forward harness so future Phase 2 plans can't land non-idempotent downs
affects: [02-03, 02-04, 02-05, 02-06, 02-07, 02-08, 02-09, 02-10]

tech-stack:
  added: []
  patterns:
    - "Domain table shape: UUID PK with gen_random_uuid() default, archived_at TIMESTAMPTZ NULL for soft-delete, partial index `WHERE archived_at IS NULL` to keep live-list queries fast, BEFORE UPDATE trigger reusing Phase 1's touch_updated_at()"
    - "Capability vocabulary enforced at DB layer via TEXT[] + CHECK using SQL `<@` (subset) operator instead of an ENUM type — adding/removing a capability is a single ALTER TABLE swap of the CHECK, no enum migration dance"
    - "GIN index on TEXT[] columns when the access pattern is `column @> ARRAY['x']` (find profiles supporting capability X)"
    - "Self-referential FKs use ON DELETE RESTRICT so a parent can never be removed while children reference it; archive the children first"
    - "Seeds via dedicated migration (0010), data-only — codec source stays in Go //go:embed rather than SQL string literals (Open Q#5)"

key-files:
  created:
    - internal/db/migrations/0007_site.up.sql
    - internal/db/migrations/0007_site.down.sql
    - internal/db/migrations/0008_metering_point.up.sql
    - internal/db/migrations/0008_metering_point.down.sql
    - internal/db/migrations/0009_device_profile.up.sql
    - internal/db/migrations/0009_device_profile.down.sql
    - internal/db/migrations/0010_seed_profiles.up.sql
    - internal/db/migrations/0010_seed_profiles.down.sql
    - internal/db/migrations/0011_chirpstack_connection_cs_ids.up.sql
    - internal/db/migrations/0011_chirpstack_connection_cs_ids.down.sql
    - internal/db/roundtrip_test.go
  modified:
    - internal/db/migrations_test.go

key-decisions:
  - "cs_tenant_id and cs_application_id stored as TEXT, not UUID, because ChirpStack v4's gRPC API returns IDs as strings. Storing as TEXT avoids parsing UUIDs on every boot just to write them back as strings to gRPC requests. The 36-char string format is validated at the application layer in plan 02-05's bootstrap. This is a deliberate departure from the otherwise-uniform UUID PK pattern in Phase 2."
  - "device_profile.capabilities is TEXT[] + CHECK using SQL `<@` (subset) operator, not a Postgres ENUM. ENUMs require ALTER TYPE ADD VALUE migrations that are not transactional in older Postgres versions and don't compose well with array column types. The TEXT[] + CHECK shape gives identical enforcement at the DB layer (T-02-02-01 mitigation) plus a one-line ALTER TABLE if D-04 ever expands."
  - "Seed migration 0010 inserts ALL three D-07 profiles in a single statement rather than splitting per profile. Single statement is atomic by default; on rollback (0010.down.sql) the matching DELETE is symmetric and idempotent."
  - "Created TestRunMigrations_RoundTrip in internal/db/roundtrip_test.go (forward → full down → forward → assert v=11 + seeds present). The plan only required a forward verification; round-trip is added as Rule 2 critical functionality because down migrations 0007..0011 will accumulate to 0012..0019 across the rest of Phase 2, and a non-idempotent down landing without notice would silently break operator rollback paths. Future plans inherit a guardrail at zero per-plan cost."
  - "TestRunMigrations expected schema_migrations.version is bumped per task (8 after Task 1, 9 after Task 2, 11 after Task 3) so each task commit lands with green tests. Single 6→11 jump would have meant all three task commits had a failing test in between."

patterns-established:
  - "Pattern: Phase 2 domain tables — UUID PK, archived_at TIMESTAMPTZ NULL, partial index on archived_at WHERE NULL, touch_updated_at trigger. site, metering_point, device_profile all conform; later plans (02-03 device, 02-03 binding, 02-08 audit_log) should match."
  - "Pattern: bumping the version assertion — when a Phase 2 plan adds N migrations, update internal/db/migrations_test.go's `require.Equal(t, X, version)` (and the matching idempotent test) inside the same task that lands the migration. The comment on the assertion should cite the plan + task number that made the bump."
  - "Pattern: enforcing controlled vocabularies — TEXT + CHECK ('val1','val2',...) for closed sets that change rarely (utility_class), TEXT[] + CHECK with `<@` subset for open-but-bounded sets that grow rarely (capabilities)."

requirements-completed: [SITE-01, DATA-09, DATA-10]

duration: 5min
completed: 2026-05-04
---

# Phase 02 Plan 02: Schema Migrations 0007..0011 Summary

**5 forward + 5 reverse SQL migrations land the Phase 2 foundation: `site` (with self-ref parent_id, lat/lng range CHECKs, soft-delete), `metering_point` (with site FK, utility_class CHECK, name-unique-per-site), `device_profile` (with TEXT[]+CHECK enforced D-04 capability vocabulary, counter_modulus default 2^32, codec_js placeholder, region inheritance, GIN index on capabilities), 3 seeded vendor profiles (Axioma W1, Acrel ADL200, Acrel ADW300), and `chirpstack_connection.cs_tenant_id` + `cs_application_id` TEXT columns for D-28 bootstrap pinning. All four migration tests (Clean, Idempotent, DirtyState, RoundTrip) pass under `-race`.**

## Performance

- **Duration:** ~5 min
- **Started:** 2026-05-04T03:45:49Z
- **Completed:** 2026-05-04T03:50:55Z
- **Tasks:** 3 / 3
- **Files created:** 11 (10 migration files + 1 round-trip test)
- **Files modified:** 1 (migrations_test.go)

## Accomplishments

- **Task 1 — site + metering_point.** UUID PK with `gen_random_uuid()`, lat/lng range CHECKs (T-02-02-04 mitigation), self-referential `parent_id` with `ON DELETE RESTRICT` (campus → building → floor), `archived_at` soft-delete with partial index keeping live-list queries fast, name uniqueness per site for metering_point, `utility_class IN ('water','electricity')` CHECK (T-02-02-03), trigger reuses Phase 1's `touch_updated_at()`.
- **Task 2 — device_profile.** UUID PK, lowercase-slug invariant CHECK, `counter_modulus BIGINT NOT NULL DEFAULT 4294967296` (D-05, T-02-02-02), capability vocabulary via TEXT[] + CHECK with SQL `<@` subset against the 10-token D-04 set (T-02-02-01), `codec_js TEXT NOT NULL DEFAULT ''` (filled at boot from Go `//go:embed` per Open Q#5), `cs_profile_id UUID NULL` and `codec_js_synced_at TIMESTAMPTZ NULL` for the codec-sync state machine, `region TEXT NULL` (NULL = inherit install region from `chirpstack_connection.region_name` per Open Q#2), `mac_version TEXT NOT NULL DEFAULT 'LORAWAN_1_0_3'`, GIN index on `capabilities` for `@>` lookups.
- **Task 3 — seed profiles + chirpstack_connection ALTER.** 0010 inserts the D-07 trio: Axioma Qalcosonic W1 with `cumulative + flow_rate + battery + temperature + leak_detection + tamper_detection` capabilities and 2^32 counter modulus; Acrel ADL200 with `cumulative + instant_power + battery` and 10^7 modulus matching its 7-digit kWh display; Acrel ADW300 with `cumulative + instant_power + battery + temperature + multi_phase + power_quality` and 10^7 modulus. 0011 ALTERs `chirpstack_connection` to add `cs_tenant_id TEXT NULL` and `cs_application_id TEXT NULL` for D-28 idempotent bootstrap pinning (filled by plan 02-05's `bootstrap.go EnsureTenantAndApplication`).
- **Test harness.** Bumped `TestRunMigrations_Clean`'s expected version 6 → 11 across the three tasks (8 after Task 1, 9 after Task 2, 11 after Task 3 — each task commits with green tests). Added two new table-existence assertions (`site`, `metering_point`, `device_profile`), a seed-row count assertion (`SELECT count(*) … WHERE slug IN (axioma_w1,acrel_adl200,acrel_adw300) → 3`), and a column-existence assertion for `chirpstack_connection.cs_tenant_id` + `cs_application_id`. Created `internal/db/roundtrip_test.go` exercising forward → full down → forward → assert v=11 + seeds present — catches non-idempotent down migrations the moment a future plan lands one.
- **Verification.** `go test -count=1 -race ./internal/db/... -run TestRunMigrations` → 4 passed (Clean, Idempotent, DirtyState, RoundTrip). `go test -count=1 -short ./...` → 145 passed in 22 packages. `go vet ./...` clean. `go build ./...` clean.

## Task Commits

1. **Task 1: 0007_site + 0008_metering_point** — `a3751d0` (feat)
2. **Task 2: 0009_device_profile** — `c6e858c` (feat)
3. **Task 3: 0010_seed_profiles + 0011_chirpstack_connection_cs_ids** — `ba18db5` (feat)

**Plan metadata commit:** _pending — created at end of plan_

## Schema Diff

### New tables (3)

| Table | PK | FKs | Soft-delete | Notable |
|-------|----|----|-------------|---------|
| `site` | UUID | self → site(id) ON DELETE RESTRICT | archived_at | lat/lng range CHECK, parent_id partial idx |
| `metering_point` | UUID | site_id → site(id) ON DELETE RESTRICT | archived_at | utility_class CHECK, UNIQUE (site_id, name) |
| `device_profile` | UUID | — | archived_at | capabilities TEXT[]+CHECK+GIN, counter_modulus BIGINT default 2^32, codec_js TEXT default '' |

### Altered table (1)

| Table | Change | Why |
|-------|--------|-----|
| `chirpstack_connection` | +cs_tenant_id TEXT NULL, +cs_application_id TEXT NULL | D-28 bootstrap pin — filled by plan 02-05 first-boot orchestrator |

### Seeds (1 migration, 3 rows)

| Slug | Vendor | Family | Capabilities | counter_modulus |
|------|--------|--------|--------------|-----------------|
| `axioma_w1` | Axioma | Qalcosonic | cumulative, flow_rate, battery, temperature, leak_detection, tamper_detection | 4294967296 (2^32) |
| `acrel_adl200` | Acrel | ADL | cumulative, instant_power, battery | 10000000 (10^7) |
| `acrel_adw300` | Acrel | ADW | cumulative, instant_power, battery, temperature, multi_phase, power_quality | 10000000 (10^7) |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 — Blocking] Pre-existing migration test asserted version=6, would fail after every task**
- **Found during:** Task 1 first test run
- **Issue:** `internal/db/migrations_test.go::TestRunMigrations_Clean` had `require.Equal(t, 6, version, …)` and `TestRunMigrations_Idempotent` had `require.Equal(t, 6, version)` — both written for Phase 1's six migrations. Adding migration 0007 immediately broke them, blocking the per-task commit policy.
- **Fix:** Bumped version per task (8 after Task 1, 9 after Task 2, 11 after Task 3) so every task commits green. Added new table-existence assertions (`site`, `metering_point`, `device_profile`), seed-row count assertion, and `cs_tenant_id` + `cs_application_id` column-existence assertions in the same edit.
- **Files modified:** `internal/db/migrations_test.go`
- **Commits:** `a3751d0`, `c6e858c`, `ba18db5`

**2. [Rule 2 — Critical functionality] No round-trip test for down migrations**
- **Found during:** Task 3 verification, before commit
- **Issue:** The plan asks for "Each migration applies cleanly forward (`go run ./cmd/shifter migrate up`) and reverse (`migrate down`)" plus "Down migrations are idempotent (re-running is safe)" in the success criteria. The existing `TestRunMigrations_Clean` only exercises Up. Plan 02-02 lands 5 down files; plans 02-03..02-10 will land at least 5 more. Without a round-trip harness, the first non-idempotent or syntactically-broken down migration to land will not be caught until a manual operator-rollback fails in production. That's a Rule 2 critical-functionality gap.
- **Fix:** Added `internal/db/roundtrip_test.go::TestRunMigrations_RoundTrip` that opens a Postgres testcontainer, runs migrations forward to v=11, runs `m.Down()` to v=0, runs `m.Up()` back to v=11, then asserts seeds are present. Costs ~3 seconds; future plans inherit it for free.
- **Files added:** `internal/db/roundtrip_test.go`
- **Commit:** `ba18db5`

### Plan-text inconsistency flagged (no auto-fix)

**1. [Rule 4 — Architectural — flagged, NOT actioned]** Plan 02-02 verify command names a test that does not exist
- The plan's verify block reads `go test -count=1 ./internal/db/... -run TestRunMigrations` (singular). The actual test names in `internal/db/migrations_test.go` are `TestRunMigrations_Clean`, `TestRunMigrations_Idempotent`, `TestRunMigrations_DirtyState`. The shorter `-run TestRunMigrations` regex matches all three (plus the new `TestRunMigrations_RoundTrip`) by prefix, so the command is functionally correct. Did not edit the plan because it does not block execution and the regex resolves to the right set of tests.
- The plan's wave_context section references `TestMigrations`, also a prefix mismatch with the actual `TestRunMigrations_*` set; same story — `-run TestMigrations` would match zero tests as written. Used `-run TestRunMigrations` per the plan's verify block (which is correct).

**2. [Rule 4 — Traceability flag, NOT actioned]** Plan frontmatter `requirements: [SITE-01, DATA-09, DATA-10]` overstates this plan's vertical-completion claim
- Plan 02-02 lands schema bones for these requirements: site/metering_point tables (SITE-01 schema layer), device_profile table (DATA-09 schema layer), 3 seeded vendor profiles (DATA-10 "system comes with at least one fully wired vendor profile" — schema part).
- BUT none of these requirements is END-TO-END complete after 02-02:
  - SITE-01 ("Admin can create / edit / delete sites via dialogs") — needs handlers + dialogs in 02-10.
  - DATA-09 ("Admin can map decoded fields to canonical columns through a UI") — needs profile editor + hot-reload in 02-06.
  - DATA-10 ("at least one fully wired vendor profile") — seeds present but codec_js + ChirpStack push happens in 02-08 (boot-time //go:embed) and end-to-end uplink test in 02-09.
- Per the GSD executor protocol ("Extract the `requirements` array from the plan's frontmatter, then mark each complete"), all three were marked. The 02-VALIDATION.md per-task verification map clearly shows the late-plan ownership; the verifier should treat these check-marks as "schema layer landed" and not as "user-facing requirement delivered." Recommend a future planner-side hygiene pass to either (a) split each requirement into sub-IDs (SITE-01-schema, SITE-01-ui) or (b) only list a requirement in `requirements:` of the plan that delivers its final vertical slice. Did NOT edit REQUIREMENTS.md or the plan because (a) the frontmatter is the protocol's source of truth and (b) the audit trail in this SUMMARY plus the VALIDATION map per-row ownership preserves traceability.

## Authentication Gates

None — schema migrations are local-DB-only. The Postgres testcontainer ran via the local Podman socket (`unix:///var/folders/.../podman-machine-default-api.sock`) with `TESTCONTAINERS_RYUK_DISABLED=true`. No external services touched.

## Decisions Made

- **`cs_tenant_id`/`cs_application_id` as TEXT, not UUID** — ChirpStack v4 returns IDs as strings via gRPC. Storing as UUID forces parsing-then-restringifying on every boot just to feed gRPC requests. Format is validated at the application layer in plan 02-05.
- **Capabilities as TEXT[] + CHECK with `<@`, not ENUM** — ENUM ALTER TYPE ADD VALUE has historical Postgres edge cases (non-transactional in older versions, ordering quirks) and arrays of enums are awkward. TEXT[] + CHECK gives identical safety, and growing the vocabulary in v2 is one ALTER TABLE.
- **3 seeds in a single INSERT, not three** — Single statement is atomic by default; on rollback the symmetric `DELETE … WHERE slug IN (…)` is idempotent. Per-row inserts would have been fine too; single statement is just less code.
- **`mac_version TEXT NOT NULL DEFAULT 'LORAWAN_1_0_3'`** — Both Axioma W1 and Acrel ADL200/ADW300 ship LoRaWAN 1.0.3 firmware. Storing this on the profile (not the device) matches D-01 layer 2 (profile owns vendor-firmware-stable attributes); a v2 vendor that bumps to 1.0.4 will get its own profile row anyway.
- **Counter modulus per profile (D-05) baked into seeds, not the device row** — Profile-level placement means swapping a meter to a same-profile replacement reuses the modulus automatically; only a profile change requires re-deriving the rollover detection threshold.
- **`region TEXT NULL` (inherit from install)** — 99% of installs run a single ChirpStack region (Open Q#2 resolution). Per-profile override left NULL by default; a v2 multi-region install can populate it without a schema change.

## Self-Check: PASSED

- `[x]` `internal/db/migrations/0007_site.up.sql` exists and contains `CREATE TABLE site` AND `gen_random_uuid()` AND `archived_at TIMESTAMPTZ NULL` AND `parent_id UUID NULL REFERENCES site(id)` AND `CREATE TRIGGER site_touch` AND `CONSTRAINT site_lat_range`
- `[x]` `internal/db/migrations/0007_site.down.sql` contains `DROP TABLE IF EXISTS site`
- `[x]` `internal/db/migrations/0008_metering_point.up.sql` contains `REFERENCES site(id)` AND `utility_class IN ('water','electricity')` AND `UNIQUE (site_id, name)`
- `[x]` `internal/db/migrations/0008_metering_point.down.sql` contains `DROP TABLE IF EXISTS metering_point`
- `[x]` `internal/db/migrations/0009_device_profile.up.sql` contains `counter_modulus BIGINT NOT NULL DEFAULT 4294967296` AND all 10 D-04 capability tokens AND `cs_profile_id UUID NULL` AND `codec_js_synced_at TIMESTAMPTZ NULL` AND `codec_js TEXT NOT NULL DEFAULT ''` AND `slug TEXT NOT NULL UNIQUE` AND CHECK `slug = lower(slug)` AND GIN index on capabilities
- `[x]` `internal/db/migrations/0009_device_profile.down.sql` cleanly drops the table
- `[x]` `internal/db/migrations/0010_seed_profiles.up.sql` contains 3 INSERT rows (`'axioma_w1'`, `'acrel_adl200'`, `'acrel_adw300'`) AND `4294967296` for Axioma row AND `10000000` for both Acrel rows AND `'leak_detection'` + `'tamper_detection'` for Axioma W1 AND `'multi_phase'` + `'power_quality'` for ADW300
- `[x]` `internal/db/migrations/0010_seed_profiles.down.sql` deletes all 3 by slug
- `[x]` `internal/db/migrations/0011_chirpstack_connection_cs_ids.up.sql` contains `ADD COLUMN cs_tenant_id` AND `ADD COLUMN cs_application_id`
- `[x]` `internal/db/migrations/0011_chirpstack_connection_cs_ids.down.sql` drops both columns
- `[x]` `go test -count=1 ./internal/db/... -run TestRunMigrations` exits 0 (4/4: Clean, Idempotent, DirtyState, RoundTrip)
- `[x]` `go test -count=1 -short ./...` exits 0 (145 passed, 22 packages — no regressions in other Phase 1 / Wave 0 stub packages)
- `[x]` `go vet ./...` clean
- `[x]` `go build ./...` clean
- `[x]` commit `a3751d0` (Task 1) found in git log
- `[x]` commit `c6e858c` (Task 2) found in git log
- `[x]` commit `ba18db5` (Task 3) found in git log
