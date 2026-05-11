---
phase: 05-aggregates-reports-map-floor-plans
plan: "02"
subsystem: timescaledb-cagg-hierarchy
tags: [timescaledb, cagg, retention, migrations, install]
dependency_graph:
  requires: [05-01]
  provides: [measurement_hourly, measurement_daily, measurement_monthly, measurement_yearly, retention_config]
  affects: [05-03, 05-06, 05-09, 05-11]
tech_stack:
  added: []
  patterns:
    - "CAGG-over-CAGG hierarchy (measurement → hourly → daily → monthly → yearly)"
    - "WITH NO DATA in every CAGG migration to satisfy golang-migrate transaction constraint"
    - "first()/last() per bucket for cumulative_delta (LAG() inside CAGG is SQLSTATE 42803)"
    - "Serializable transaction seeding retention_config at install finish (D-23 atomicity)"
key_files:
  created:
    - internal/db/migrations/0025_cagg_hourly.up.sql
    - internal/db/migrations/0025_cagg_hourly.down.sql
    - internal/db/migrations/0026_cagg_daily.up.sql
    - internal/db/migrations/0026_cagg_daily.down.sql
    - internal/db/migrations/0027_cagg_monthly.up.sql
    - internal/db/migrations/0027_cagg_monthly.down.sql
    - internal/db/migrations/0028_cagg_yearly.up.sql
    - internal/db/migrations/0028_cagg_yearly.down.sql
    - internal/db/migrations/0029_retention_config.up.sql
    - internal/db/migrations/0029_retention_config.down.sql
    - internal/aggregate/doc.go
    - internal/install/finish_test.go
  modified:
    - internal/aggregate/aggregate_test.go
    - internal/install/finish.go
    - internal/db/migrations_test.go
    - internal/db/roundtrip_test.go
decisions:
  - "cumulative_delta = last(cumulative_value,time) - first(cumulative_value,time) per bucket — not LAG(); LAG() inside CAGG is forbidden (SQLSTATE 42803)"
  - "Refresh policy window minimum: start_offset=3x bucket, end_offset=2x max(expected_interval_s); satisfies 2-bucket minimum AND DATA-12"
  - "materialized_only=false for hourly+daily (real-time ON), true for monthly+yearly (real-time OFF) — D-11"
  - "yearly CAGG has NO retention policy — kept forever per D-09"
  - "retention_config seeded inside FinishSetup Serializable txn — same txn as admin user creation (D-23)"
metrics:
  duration: "multi-session (context limit)"
  completed: "2026-05-11"
  tasks_completed: 2
  files_changed: 16
---

# Phase 05 Plan 02: CAGG Hierarchy + Retention Config Summary

Four-level TimescaleDB CAGG hierarchy (hourly→daily→monthly→yearly) over the `measurement` hypertable, with per-level refresh and retention policies, plus the `retention_config` singleton table seeded atomically at install finish.

## Tasks Completed

| Task | Description | Commit |
|------|-------------|--------|
| 1 | CAGG migrations 0025–0028 + aggregate tests | fcd35bf |
| 2 | retention_config migration 0029 + FinishSetup seeding + tests | 3997dfe |

## What Was Built

### Task 1 — CAGG Hierarchy (0025–0028)

Four migration files create the CAGG chain `WITH NO DATA` (golang-migrate Pitfall #1 mitigation):

- **0025_cagg_hourly**: `measurement_hourly` over `measurement`. `materialized_only=false`. Refresh: start=4h end=2h schedule=5min. Retention: raw=90d, hourly=1y.
- **0026_cagg_daily**: `measurement_daily` over `measurement_hourly`. `materialized_only=false`. Refresh: start=3d end=2h schedule=30min. Retention: daily=5y.
- **0027_cagg_monthly**: `measurement_monthly` over `measurement_daily`. `materialized_only=true`. Refresh: start=3 months end=1 day schedule=6h. Retention: monthly=20y.
- **0028_cagg_yearly**: `measurement_yearly` over `measurement_monthly`. `materialized_only=true`. Refresh: start=3 years end=7 days schedule=1 day. No retention policy (forever per D-09).

`cumulative_delta` in every CAGG: `last(cumulative_value, time) - first(cumulative_value, time)`. Window functions (LAG) inside CAGG aggregates are forbidden by TimescaleDB (SQLSTATE 42803).

`internal/aggregate/doc.go` documents the design rationale, Pitfall #2 (retention footgun), and why first/last is the correct CAGG-compatible equivalent of LAG.

`internal/aggregate/aggregate_test.go` has four integration tests:
- `TestCAGGHierarchy`: verifies source view chain in `timescaledb_information.continuous_aggregates`
- `TestRefreshPolicyParams`: checks end_offset >= 2×max(expected_interval_s) for each CAGG (DATA-12)
- `TestRetentionPolicy`: checks raw=90d, hourly=1y, daily=5y, monthly=20y, yearly=no policy (D-09)
- `TestCAGGChain_DeltaCorrectness`: inserts 6 raw rows across two hours, manually refreshes, asserts delta=45 and delta=55

### Task 2 — retention_config + FinishSetup Seeding (0029)

`0029_retention_config.up.sql` creates the singleton table:
```sql
CREATE TABLE retention_config (
  id           INTEGER PRIMARY KEY CHECK (id = 1),
  raw_days     INTEGER NOT NULL CHECK (raw_days BETWEEN 30 AND 365),
  hourly_days  INTEGER NOT NULL CHECK (hourly_days BETWEEN 180 AND 1825),
  daily_days   INTEGER NOT NULL CHECK (daily_days BETWEEN 365 AND 7300),
  monthly_days INTEGER NOT NULL CHECK (monthly_days BETWEEN 1825 AND 18250),
  yearly_days  INTEGER,
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

`internal/install/finish.go` extended with step 4 inside the Serializable transaction:
```sql
INSERT INTO retention_config (id, raw_days, hourly_days, daily_days, monthly_days, yearly_days)
VALUES (1, 90, 365, 1825, 7300, NULL)
ON CONFLICT (id) DO NOTHING
```

`internal/install/finish_test.go` adds three tests verifying seeding, rollback-on-failure, and idempotency.

`internal/db/migrations_test.go` and `roundtrip_test.go` updated: version assertions 24→29, phase-3 down-migration step counts corrected for the 29-deep chain.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed SQLSTATE 42803: LAG() inside CAGG aggregate**
- **Found during:** Task 1 — writing aggregate_test.go and CAGG SQL
- **Issue:** `sum(cumulative_value - LAG(cumulative_value) OVER (PARTITION BY metering_point_id ORDER BY time))` is invalid inside a TimescaleDB CAGG (window functions cannot appear inside aggregate functions — SQLSTATE 42803)
- **Fix:** Replaced with `last(cumulative_value, time) - first(cumulative_value, time)` per bucket, which is semantically equivalent for monotonically increasing cumulative meters
- **Files modified:** all four CAGG up migrations, aggregate_test.go, doc.go

**2. [Rule 1 - Bug] Fixed SQLSTATE 22023: refresh policy window too narrow**
- **Found during:** Task 1 — test execution
- **Issue:** TimescaleDB requires `start_offset - end_offset >= 2 × bucket_size`. Original offsets (start=2x, end=1x) gave a window of exactly 1 bucket — below the 2-bucket minimum.
- **Fix:** Increased start_offset to 3x bucket for all four CAGGs; verified window = start - end >= 2x bucket everywhere
- **Files modified:** 0025–0028 up migrations

**3. [Rule 1 - Bug] Fixed DATA-12 end_offset violation**
- **Found during:** Task 1 — TestRefreshPolicyParams
- **Issue:** hourly end_offset=1h=3600s < 2×3600=7200s (maximum expected device interval). Daily end_offset=1h also needed increase.
- **Fix:** Set hourly end=2h, daily end=2h — both >= 2×max(expected_interval_s)=7200s
- **Files modified:** 0025_cagg_hourly.up.sql, 0026_cagg_daily.up.sql

**4. [Rule 1 - Bug] Fixed TestCAGGChain_DeltaCorrectness wrong expected delta**
- **Found during:** Task 1 — test execution
- **Issue:** Test data had only 3 rows in hour-09 ([100, 110, 130]) giving last=130-first=100=30, not 45 as expected
- **Fix:** Added 4th reading at 09:55 with value=145 in hour-09, giving last(145)-first(100)=45; hour-10 starts at 10:10 with value=145 giving last(200)-first(145)=55
- **Files modified:** internal/aggregate/aggregate_test.go

**5. [Rule 1 - Bug] Fixed interval string parsing for TimescaleDB 2.26**
- **Found during:** Task 1 — TestRefreshPolicyParams
- **Issue:** TimescaleDB 2.26 stores job config intervals as strings like "01:00:00" not `{"microseconds": N}` JSON objects
- **Fix:** Added `parseIntervalString` helper in aggregate_test.go handling HH:MM:SS, "N days", "N mons", "N years" formats
- **Files modified:** internal/aggregate/aggregate_test.go

**6. [Rule 1 - Bug] Fixed phase-3 down-migration step counts**
- **Found during:** Task 2 — test run after all migrations in place
- **Issue:** Step counts for TestPhase3Migrations_0018_Down (-11), 0019_Down (-10), 0020_Down (-9) were calibrated for chain depth 28; adding migration 0029 made each off-by-one
- **Fix:** Updated to -12, -11, -10 respectively
- **Files modified:** internal/db/migrations_test.go

## Known Stubs

None. The retention_config row is intentionally not seeded by the migration (only by FinishSetup). Plan 05-11 will expose a Settings UI to update the row and reconcile actual TimescaleDB retention policies.

## Threat Flags

None. No new network endpoints, auth paths, or trust-boundary schema changes introduced. The `retention_config` table is an internal configuration singleton with no direct API exposure in this plan.

## Self-Check: PASSED

- [x] `internal/db/migrations/0025_cagg_hourly.up.sql` exists
- [x] `internal/db/migrations/0026_cagg_daily.up.sql` exists
- [x] `internal/db/migrations/0027_cagg_monthly.up.sql` exists
- [x] `internal/db/migrations/0028_cagg_yearly.up.sql` exists
- [x] `internal/db/migrations/0029_retention_config.up.sql` exists
- [x] `internal/aggregate/doc.go` exists
- [x] `internal/install/finish_test.go` exists
- [x] Commit fcd35bf exists (Task 1)
- [x] Commit 3997dfe exists (Task 2)
- [x] All 57 tests pass: `go test ./internal/aggregate/... ./internal/install/... ./internal/db/... -race -count=1`
