---
phase: 05-aggregates-reports-map-floor-plans
plan: 02
type: execute
wave: 1
depends_on: [01]
files_modified:
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
  - internal/db/migrations_test.go
  - internal/aggregate/aggregate_test.go
  - internal/aggregate/doc.go
  - internal/install/finish.go
  - internal/install/finish_test.go
autonomous: true
requirements: [DATA-11, DATA-12, DATA-13]
threat_refs: [T-05-02-01, T-05-02-02]

must_haves:
  truths:
    - "measurement_hourly CAGG exists and is queryable as a regular SELECT target"
    - "measurement_daily CAGG sources from measurement_hourly (not raw) — verified via continuous_aggregates catalog"
    - "measurement_monthly CAGG sources from measurement_daily — strict hierarchy"
    - "measurement_yearly CAGG sources from measurement_monthly"
    - "All four CAGG migrations use WITH NO DATA so golang-migrate's transaction wrapping does NOT cause Pitfall #1"
    - "Refresh policy `end_offset >= 2 × max(device_profile.expected_interval_s)` for every CAGG (D-11 / DATA-12)"
    - "Refresh policy `start_offset <= raw_retention` (D-11 / DATA-12) — bounds material loss footgun"
    - "Real-time mode ON for hourly + daily (materialized_only = false) per D-11"
    - "Real-time mode OFF for monthly + yearly (materialized_only = true) per D-11"
    - "Retention policy attached: hourly 1y, daily 5y, monthly 20y, yearly NO retention policy (kept forever per D-09)"
    - "retention_config table seeded at install finish with D-09 defaults (raw 90d / hourly 1y / daily 5y / monthly 20y / yearly forever)"
    - "Raw measurement retention 90d applied via add_retention_policy('measurement', INTERVAL '90 days')"
  artifacts:
    - path: "internal/db/migrations/0025_cagg_hourly.up.sql"
      provides: "Hourly CAGG (over measurement) + refresh policy + retention policy"
      contains: "WITH (timescaledb.continuous, timescaledb.materialized_only = false)"
    - path: "internal/db/migrations/0026_cagg_daily.up.sql"
      provides: "Daily CAGG (over measurement_hourly) + policies"
      contains: "FROM measurement_hourly"
    - path: "internal/db/migrations/0027_cagg_monthly.up.sql"
      provides: "Monthly CAGG (over measurement_daily) + policies (real-time OFF)"
      contains: "materialized_only = true"
    - path: "internal/db/migrations/0028_cagg_yearly.up.sql"
      provides: "Yearly CAGG (over measurement_monthly) + refresh policy ONLY (no retention)"
      contains: "FROM measurement_monthly"
    - path: "internal/db/migrations/0029_retention_config.up.sql"
      provides: "retention_config singleton table (raw_days / hourly_days / daily_days / monthly_days / yearly_days NULL = forever)"
      contains: "CREATE TABLE retention_config"
    - path: "internal/aggregate/aggregate_test.go"
      provides: "Integration tests pinning hierarchy + refresh + retention + materialized_only flags"
      contains: "TestCAGGHierarchy"
    - path: "internal/aggregate/doc.go"
      provides: "Package-level documentation of CAGG hierarchy + LAG() footgun + retention policy invariants"
      contains: "package aggregate"
    - path: "internal/install/finish.go"
      provides: "FinishSetup extension that seeds retention_config with D-09 defaults inside the existing Serializable txn"
      contains: "retention_config"
  key_links:
    - from: "internal/db/migrations/0026_cagg_daily.up.sql"
      to: "measurement_hourly"
      via: "FROM clause"
      pattern: "FROM measurement_hourly"
    - from: "internal/db/migrations/0027_cagg_monthly.up.sql"
      to: "measurement_daily"
      via: "FROM clause"
      pattern: "FROM measurement_daily"
    - from: "internal/db/migrations/0028_cagg_yearly.up.sql"
      to: "measurement_monthly"
      via: "FROM clause"
      pattern: "FROM measurement_monthly"
    - from: "internal/install/finish.go"
      to: "retention_config row"
      via: "INSERT inside Serializable txn alongside install_identity UPSERT"
      pattern: "INSERT INTO retention_config"
---

<objective>
Land the four-level continuous-aggregate hierarchy (hourly → daily → monthly → yearly) on the existing `measurement` hypertable, attach refresh + retention policies per D-11 / D-09, and seed the `retention_config` table at install finish so plan 05-11's Settings UI has a row to read. Every CAGG migration uses `WITH NO DATA` to avoid golang-migrate's transaction-block constraint (Pitfall #1).

Purpose: This is the data substrate every report query in plan 05-03 will read from. Without it: 7d/30d/yearly reports run against raw measurement and time out. With it: yearly reports hit ~12 rows/MP/year and return in milliseconds.

Output: 5 new SQL migrations (0025–0029), one `internal/aggregate/` package with doc + integration tests, FinishSetup extended to seed retention_config row with D-09 defaults.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-VALIDATION.md
@internal/db/migrations/0015_measurement.up.sql
@internal/db/migrations/0023_device_profile_expected_interval.up.sql
@internal/db/migrations/0024_river_tables.up.sql
@internal/db/migrations_test.go
@internal/aggregate/CUMULATIVE_DELTA.md
@internal/install/finish.go
@internal/install/finish_test.go

<interfaces>
<!-- measurement (existing, 0015) -->
```
time TIMESTAMPTZ NOT NULL,
metering_point_id UUID NOT NULL,
cumulative_value NUMERIC,
instant_value NUMERIC,
battery_pct SMALLINT,
rssi SMALLINT,
snr REAL,
quality TEXT NOT NULL DEFAULT 'ok',
... (no cumulative_delta — see internal/aggregate/CUMULATIVE_DELTA.md)
```

<!-- device_profile (existing, 0023) -->
```
expected_interval_s INTEGER NOT NULL DEFAULT 3600 CHECK (expected_interval_s > 0)
```

<!-- New table this plan introduces -->
```
retention_config (
  id              INTEGER PRIMARY KEY CHECK (id = 1),
  raw_days        INTEGER NOT NULL,
  hourly_days     INTEGER NOT NULL,
  daily_days      INTEGER NOT NULL,
  monthly_days    INTEGER NOT NULL,
  yearly_days     INTEGER,  -- NULL = forever (D-09 yearly default)
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
)
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: CAGG hierarchy migrations 0025–0028 (WITH NO DATA)</name>
  <read_first>
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md §CAGG Hierarchy + §CAGG Retention Footgun + §golang-migrate CAGG Transaction Constraint + §Common Pitfalls #1 #2 #7
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md §D-08..D-11
    - internal/aggregate/CUMULATIVE_DELTA.md (plan 05-01 verdict — must compute via LAG())
    - internal/db/migrations/0015_measurement.up.sql (canonical measurement columns)
    - internal/db/migrations_test.go (existing round-trip test patterns)
  </read_first>
  <behavior>
    - Test 1: All four CAGG views exist after migrate-up — query `timescaledb_information.continuous_aggregates` returns rows for measurement_{hourly,daily,monthly,yearly}
    - Test 2: Hierarchy chain verified — measurement_daily's `materialization_hypertable` source is measurement_hourly, etc.
    - Test 3: `materialized_only` flag: false for hourly + daily; true for monthly + yearly (D-11)
    - Test 4: All four CAGGs round-trip: migrate-down drops them, migrate-up recreates
    - Test 5: Insert N raw measurement rows → CALL refresh_continuous_aggregate manually → query measurement_hourly returns expected `cumulative_delta` sum (per-MP LAG correctness verified)
  </behavior>
  <action>
**Step A — Write 0025_cagg_hourly.up.sql (verbatim, except for header comments which can adapt):**

```sql
-- 0025_cagg_hourly.up.sql
-- Hourly continuous aggregate over measurement hypertable (DATA-11, D-08, D-10, D-11).
--
-- Pitfall #1 (RESEARCH §golang-migrate CAGG Transaction Constraint): CREATE
-- MATERIALIZED VIEW ... WITH (timescaledb.continuous) ... WITH DATA cannot run
-- inside a transaction block. golang-migrate wraps every file in BEGIN/COMMIT.
-- We use WITH NO DATA — the refresh policy below backfills automatically on
-- the first scheduled run (or call refresh_continuous_aggregate manually for
-- backfill).
--
-- Pitfall #2 (RESEARCH §CAGG Retention Footgun): start_offset MUST be smaller
-- than the raw retention window (90d default per D-09). We use INTERVAL '2 hours'
-- which is well inside the 90d window.
--
-- cumulative_delta computation: the source `measurement.cumulative_value` is
-- the absolute meter reading. Consumption inside a bucket = Σ(per-uplink delta)
-- = Σ(cumulative_value − LAG(cumulative_value)). The LAG window partitions by
-- metering_point_id and orders by time so deltas cross hourly bucket boundaries
-- correctly (the first uplink of a bucket subtracts the last uplink of the
-- previous bucket — NOT bucket-internal).
--
-- D-11: materialized_only = false enables real-time mode so freshly-ingested
-- rows are visible in 7d/30d charts within the refresh interval (5 min).

CREATE MATERIALIZED VIEW measurement_hourly
WITH (timescaledb.continuous, timescaledb.materialized_only = false)
AS
SELECT
  time_bucket('1 hour', time)            AS bucket,
  metering_point_id,
  sum(
    cumulative_value - LAG(cumulative_value) OVER (
      PARTITION BY metering_point_id ORDER BY time
    )
  )                                       AS cumulative_delta,
  avg(instant_value)                      AS avg_instant,
  max(instant_value)                      AS max_instant,
  min(battery_pct)                        AS min_battery,
  avg(battery_pct)                        AS avg_battery,
  avg(rssi)                               AS avg_rssi,
  avg(snr)                                AS avg_snr,
  count(*)                                AS uplink_count,
  count(*) FILTER (WHERE quality <> 'ok') AS flagged_count
FROM measurement
GROUP BY 1, 2
WITH NO DATA;

-- DATA-12 refresh policy (D-11):
--   end_offset    = 1 hour     — excludes the in-progress hour AND absorbs late
--                                 uplinks; ≥ 2 × max(expected_interval_s) is
--                                 satisfied for all profiles (≤ 1800s).
--   start_offset  = 2 hours    — well inside raw 90d retention (Pitfall #2).
--   schedule_interval = 5 min  — D-11 / Discretion.
SELECT add_continuous_aggregate_policy('measurement_hourly',
  start_offset      => INTERVAL '2 hours',
  end_offset        => INTERVAL '1 hour',
  schedule_interval => INTERVAL '5 minutes');

-- D-09 retention: hourly = 1 year.
SELECT add_retention_policy('measurement_hourly', INTERVAL '1 year');

-- D-09 raw retention: 90 days (apply HERE — must land somewhere before any
-- CAGG refreshes attempt to read raw data older than this window).
SELECT add_retention_policy('measurement', INTERVAL '90 days');
```

**Step B — Write 0025_cagg_hourly.down.sql (verbatim):**

```sql
-- 0025_cagg_hourly.down.sql
-- Idempotent reverse of 0025.

SELECT remove_retention_policy('measurement', if_exists => true);
SELECT remove_retention_policy('measurement_hourly', if_exists => true);
SELECT remove_continuous_aggregate_policy('measurement_hourly', if_exists => true);

DROP MATERIALIZED VIEW IF EXISTS measurement_hourly;
```

**Step C — Write 0026_cagg_daily.up.sql:**

```sql
-- 0026_cagg_daily.up.sql
-- Daily CAGG over measurement_hourly (CAGG-over-CAGG, D-08).
--
-- Pitfall #7: time_bucket on a fixed-width bucket (1h) → fixed-width (1d) is
-- valid. Do NOT skip the daily level and go directly hourly → monthly (1mo is
-- variable-width).

CREATE MATERIALIZED VIEW measurement_daily
WITH (timescaledb.continuous, timescaledb.materialized_only = false)
AS
SELECT
  time_bucket('1 day', bucket)         AS bucket,
  metering_point_id,
  sum(cumulative_delta)                AS cumulative_delta,
  avg(avg_instant)                     AS avg_instant,
  max(max_instant)                     AS max_instant,
  min(min_battery)                     AS min_battery,
  avg(avg_battery)                     AS avg_battery,
  avg(avg_rssi)                        AS avg_rssi,
  avg(avg_snr)                         AS avg_snr,
  sum(uplink_count)                    AS uplink_count,
  sum(flagged_count)                   AS flagged_count
FROM measurement_hourly
GROUP BY 1, 2
WITH NO DATA;

SELECT add_continuous_aggregate_policy('measurement_daily',
  start_offset      => INTERVAL '2 days',
  end_offset        => INTERVAL '1 hour',
  schedule_interval => INTERVAL '30 minutes');

SELECT add_retention_policy('measurement_daily', INTERVAL '5 years');
```

**Step D — Write 0026_cagg_daily.down.sql:**

```sql
SELECT remove_retention_policy('measurement_daily', if_exists => true);
SELECT remove_continuous_aggregate_policy('measurement_daily', if_exists => true);
DROP MATERIALIZED VIEW IF EXISTS measurement_daily;
```

**Step E — Write 0027_cagg_monthly.up.sql + .down.sql:**

```sql
-- 0027_cagg_monthly.up.sql
-- Monthly CAGG over measurement_daily.
-- D-11: real-time mode OFF (materialized_only = true) — monthly reports are
-- rarely-queried; real-time would scan-on-read combining materialized data with
-- all daily rows since last refresh, which is wasteful.

CREATE MATERIALIZED VIEW measurement_monthly
WITH (timescaledb.continuous, timescaledb.materialized_only = true)
AS
SELECT
  time_bucket('1 month', bucket)       AS bucket,
  metering_point_id,
  sum(cumulative_delta)                AS cumulative_delta,
  avg(avg_instant)                     AS avg_instant,
  max(max_instant)                     AS max_instant,
  min(min_battery)                     AS min_battery,
  avg(avg_battery)                     AS avg_battery,
  avg(avg_rssi)                        AS avg_rssi,
  avg(avg_snr)                         AS avg_snr,
  sum(uplink_count)                    AS uplink_count,
  sum(flagged_count)                   AS flagged_count
FROM measurement_daily
GROUP BY 1, 2
WITH NO DATA;

SELECT add_continuous_aggregate_policy('measurement_monthly',
  start_offset      => INTERVAL '2 months',
  end_offset        => INTERVAL '1 day',
  schedule_interval => INTERVAL '6 hours');

SELECT add_retention_policy('measurement_monthly', INTERVAL '20 years');
```

```sql
-- 0027_cagg_monthly.down.sql
SELECT remove_retention_policy('measurement_monthly', if_exists => true);
SELECT remove_continuous_aggregate_policy('measurement_monthly', if_exists => true);
DROP MATERIALIZED VIEW IF EXISTS measurement_monthly;
```

**Step F — Write 0028_cagg_yearly.up.sql + .down.sql:**

```sql
-- 0028_cagg_yearly.up.sql
-- Yearly CAGG over measurement_monthly.
-- D-09 yearly retention: NEVER drops (essentially free at ~1 row/MP/year).
-- We do NOT call add_retention_policy for this CAGG.

CREATE MATERIALIZED VIEW measurement_yearly
WITH (timescaledb.continuous, timescaledb.materialized_only = true)
AS
SELECT
  time_bucket('1 year', bucket)        AS bucket,
  metering_point_id,
  sum(cumulative_delta)                AS cumulative_delta,
  avg(avg_instant)                     AS avg_instant,
  max(max_instant)                     AS max_instant,
  min(min_battery)                     AS min_battery,
  avg(avg_battery)                     AS avg_battery,
  avg(avg_rssi)                        AS avg_rssi,
  avg(avg_snr)                         AS avg_snr,
  sum(uplink_count)                    AS uplink_count,
  sum(flagged_count)                   AS flagged_count
FROM measurement_monthly
GROUP BY 1, 2
WITH NO DATA;

SELECT add_continuous_aggregate_policy('measurement_yearly',
  start_offset      => INTERVAL '2 years',
  end_offset        => INTERVAL '7 days',
  schedule_interval => INTERVAL '1 day');

-- NO add_retention_policy — yearly CAGG kept forever (D-09).
```

```sql
-- 0028_cagg_yearly.down.sql
SELECT remove_continuous_aggregate_policy('measurement_yearly', if_exists => true);
DROP MATERIALIZED VIEW IF EXISTS measurement_yearly;
```

**Step G — Create `internal/aggregate/doc.go` (package documentation):**

```go
// Package aggregate documents the four-level continuous-aggregate hierarchy
// landed by migrations 0025–0028 (Phase 5 DATA-11..13).
//
// # Hierarchy
//
//   measurement (hypertable, raw, 90d retention)
//     ↓ time_bucket('1 hour') + Σ LAG-delta + agg
//   measurement_hourly (real-time ON, 1y retention)
//     ↓ time_bucket('1 day') + Σ + agg
//   measurement_daily  (real-time ON, 5y retention)
//     ↓ time_bucket('1 month') + Σ + agg
//   measurement_monthly (real-time OFF, 20y retention)
//     ↓ time_bucket('1 year') + Σ + agg
//   measurement_yearly (real-time OFF, no retention — forever)
//
// # cumulative_delta semantics
//
// The raw measurement table stores cumulative_value (absolute meter reading,
// monotonic per metering_point modulo rollover). Phase 5's hourly CAGG computes
// the per-bucket consumption as Σ (cumulative_value − LAG(cumulative_value) OVER
// (PARTITION BY metering_point_id ORDER BY time)). The LAG window is NOT
// bucket-scoped — it crosses bucket boundaries so the first uplink of a bucket
// correctly subtracts the last uplink of the previous bucket.
//
// Higher CAGGs (daily/monthly/yearly) sum the lower CAGG's cumulative_delta
// directly — no further LAG math needed because the delta is already a
// non-cumulative consumption value.
//
// # Retention policy footgun (Pitfall #2 / RESEARCH §CAGG Retention Footgun)
//
// add_retention_policy('measurement', INTERVAL '90 days') drops chunks older
// than 90d. CAGG refresh policies MUST keep start_offset within the 90d window
// or the CAGG silently overwrites already-materialized buckets with NULLs when
// it tries to re-read dropped raw data. Our hourly start_offset is 2 hours —
// safely inside 90 days. The daily/monthly/yearly CAGGs source from lower CAGGs
// (not raw), so their start_offset can exceed 90d without footgun risk.
//
// # Real-time mode (D-11)
//
// hourly + daily: materialized_only=false. Freshly-ingested rows appear in
// 7d/30d charts within the next refresh interval.
//
// monthly + yearly: materialized_only=true. Real-time on rarely-queried
// upper CAGGs causes expensive scan-on-read combining materialized data with
// all raw rows since last refresh. Off is the right default.
//
// # Reference
//
//   - 05-CONTEXT.md D-08 through D-11
//   - 05-RESEARCH.md §CAGG Hierarchy, §CAGG Retention Footgun, §Common Pitfalls
//   - internal/aggregate/CUMULATIVE_DELTA.md (Plan 05-01 pre-check)
package aggregate
```

**Step H — Wire integration tests in `internal/aggregate/aggregate_test.go` (replace skeleton):**

Replace each `t.Skip` with bodies that:

1. `TestCAGGHierarchy`: connect to the testcontainer Postgres, query `SELECT view_name, materialized_only FROM timescaledb_information.continuous_aggregates ORDER BY view_name` and assert exact 4 rows with the right `materialized_only` flags (true for monthly/yearly, false for hourly/daily).

2. `TestRefreshPolicyParams`: query `SELECT view_name, config FROM timescaledb_information.jobs WHERE proc_name = 'policy_refresh_continuous_aggregate'` and parse each `config` JSON. Assert per-CAGG `end_offset` is parseable as ≥ 2 × the largest `expected_interval_s` in `device_profile` (use `SELECT max(expected_interval_s) FROM device_profile WHERE archived_at IS NULL`). Assert `start_offset <= raw retention` for the hourly CAGG (parse the raw retention from `timescaledb_information.jobs WHERE proc_name = 'policy_retention'` with the `hypertable_name = 'measurement'` config).

3. `TestRetentionPolicy`: query `timescaledb_information.jobs WHERE proc_name = 'policy_retention'` → expect 4 rows: measurement (90d), measurement_hourly (1y), measurement_daily (5y), measurement_monthly (20y). Assert NO row for measurement_yearly.

4. Add `TestCAGGChain_DeltaCorrectness` (load-bearing): insert 5 measurement rows for one MP across two consecutive hours with strictly increasing cumulative_value (e.g. 100, 110, 130, 145, 200 across hours 09 and 10), then `CALL refresh_continuous_aggregate('measurement_hourly', NULL, NULL)`. Query measurement_hourly for that MP and assert hour-09 `cumulative_delta = 45` (110-100 + 130-110 + 145-130 = 45) and hour-10 `cumulative_delta = 55` (200-145 = 55, the cross-bucket LAG case). This pins the LAG-across-bucket-boundary behavior.

**Step I — Update `internal/db/migrations_test.go`:**

- Bump migration count assertions by +4 (0024 added by 05-01, 0025–0028 here = +5 total since the last-checked baseline; verify by reading the existing test first to know the absolute number).
- Add assertion inside `TestRunMigrations_Clean`: `SELECT count(*) FROM timescaledb_information.continuous_aggregates` returns 4.
- Add a separate `TestRunMigrations_CAGGsDropClean` that runs migrate-down by exactly 4 from head and asserts `count(*)` from continuous_aggregates returns 0.
  </action>
  <verify>
    <automated>go test ./internal/aggregate/... -race -count=1 -short=false -run "TestCAGGHierarchy|TestRefreshPolicyParams|TestRetentionPolicy|TestCAGGChain_DeltaCorrectness" &amp;&amp; go test ./internal/db/... -race -count=1 -short=false -run "TestRunMigrations" &amp;&amp; grep -q "WITH NO DATA" internal/db/migrations/0025_cagg_hourly.up.sql &amp;&amp; grep -q "WITH NO DATA" internal/db/migrations/0026_cagg_daily.up.sql &amp;&amp; grep -q "WITH NO DATA" internal/db/migrations/0027_cagg_monthly.up.sql &amp;&amp; grep -q "WITH NO DATA" internal/db/migrations/0028_cagg_yearly.up.sql</automated>
  </verify>
  <acceptance_criteria>
    - `internal/db/migrations/0025_cagg_hourly.up.sql` contains literal `WITH (timescaledb.continuous, timescaledb.materialized_only = false)` and `WITH NO DATA` and `LAG(cumulative_value) OVER` and `add_retention_policy('measurement', INTERVAL '90 days')`
    - `internal/db/migrations/0026_cagg_daily.up.sql` contains literal `FROM measurement_hourly` and `materialized_only = false` and `WITH NO DATA`
    - `internal/db/migrations/0027_cagg_monthly.up.sql` contains literal `FROM measurement_daily` and `materialized_only = true` and `WITH NO DATA`
    - `internal/db/migrations/0028_cagg_yearly.up.sql` contains literal `FROM measurement_monthly` and `materialized_only = true` and `WITH NO DATA` and does NOT contain `add_retention_policy('measurement_yearly'`
    - All 4 `.down.sql` files contain `DROP MATERIALIZED VIEW IF EXISTS`
    - `internal/aggregate/doc.go` contains literal `package aggregate` and `LAG(cumulative_value)`
    - `TestCAGGHierarchy` body asserts 4 rows in `continuous_aggregates` with correct materialized_only flags
    - `TestCAGGChain_DeltaCorrectness` body inserts measurements and asserts hour-9 delta = 45, hour-10 delta = 55
    - No `t.Skip` remains in `internal/aggregate/aggregate_test.go`
    - `go test ./internal/aggregate/... -short=false` exits 0
    - `go test ./internal/db/... -short=false -run TestRunMigrations` exits 0
  </acceptance_criteria>
  <done>4 CAGGs live, real-time flags per D-11, retention per D-09, LAG-across-bucket math correct, no Pitfall #1.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: retention_config table (0029) + install seed of D-09 defaults</name>
  <read_first>
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md §D-09 (retention defaults)
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-UI-SPEC.md §Settings — Data Retention Card (configurable ranges)
    - internal/install/finish.go (FinishSetup transaction; existing Serializable txn pattern)
    - internal/install/finish_test.go (existing test patterns for the txn)
    - internal/db/migrations/0005_install_identity.up.sql (singleton pattern reference)
  </read_first>
  <behavior>
    - Test 1: `retention_config` table exists after migrate-up with id=1 row absent (seeded only at install finish, NOT in the migration)
    - Test 2: FinishSetup inserts retention_config (1, 90, 365, 1825, 7300, NULL) inside the same Serializable txn that creates the admin + install_identity rows; rollback rolls all back
    - Test 3: Re-running FinishSetup (ErrAlreadyCompleted path) does NOT insert a duplicate retention_config row
    - Test 4: A query helper `LoadRetentionConfig(ctx, q)` returns the seeded values
  </behavior>
  <action>
**Step A — `internal/db/migrations/0029_retention_config.up.sql`:**

```sql
-- 0029_retention_config.up.sql
-- Singleton retention configuration (DATA-13 + D-09).
--
-- One row, id always 1, columns store the desired retention windows for raw
-- measurement and each CAGG level. NULL = forever (D-09 yearly default).
--
-- Plan 05-02 seeds this row inside the install FinishSetup transaction with
-- the D-09 defaults. Plan 05-11 exposes a Settings UI that PATCH-updates the
-- row and (in plan 05-11 Task 2) reconciles the actual TimescaleDB retention
-- policies to match.

CREATE TABLE retention_config (
  id           INTEGER PRIMARY KEY CHECK (id = 1),
  raw_days     INTEGER NOT NULL CHECK (raw_days BETWEEN 30 AND 365),
  hourly_days  INTEGER NOT NULL CHECK (hourly_days BETWEEN 180 AND 1825),
  daily_days   INTEGER NOT NULL CHECK (daily_days BETWEEN 365 AND 7300),
  monthly_days INTEGER NOT NULL CHECK (monthly_days BETWEEN 1825 AND 18250),
  yearly_days  INTEGER,
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE retention_config IS 'Singleton retention windows for measurement + CAGGs (D-09 / DATA-13). NULL yearly_days = forever.';
COMMENT ON COLUMN retention_config.raw_days IS 'Raw measurement retention in days. UI-SPEC range 30..365. Default 90 (D-09).';
COMMENT ON COLUMN retention_config.hourly_days IS 'measurement_hourly retention. UI-SPEC range 180..1825 (6mo..5y). Default 365 (D-09).';
COMMENT ON COLUMN retention_config.daily_days IS 'measurement_daily retention. UI-SPEC range 365..7300 (1y..20y). Default 1825 (D-09).';
COMMENT ON COLUMN retention_config.monthly_days IS 'measurement_monthly retention. UI-SPEC range 1825..18250 (5y..50y). Default 7300 (D-09).';
COMMENT ON COLUMN retention_config.yearly_days IS 'measurement_yearly retention. NULL = forever (D-09).';
```

**Step B — `internal/db/migrations/0029_retention_config.down.sql`:**

```sql
DROP TABLE IF EXISTS retention_config;
```

**Step C — Extend `internal/install/finish.go`:**

Inside the existing `FinishSetup` body, AFTER the install_identity UPSERT and BEFORE the install_state DELETE, add:

```go
// Seed retention_config with D-09 defaults (Phase 5 DATA-13). NULL yearly_days
// = forever per D-09. ON CONFLICT DO NOTHING preserves operator changes made
// before a re-run (which is itself blocked above by ErrAlreadyCompleted, but
// belt-and-suspenders).
if _, err := tx.Exec(ctx, `
  INSERT INTO retention_config (id, raw_days, hourly_days, daily_days, monthly_days, yearly_days)
  VALUES (1, 90, 365, 1825, 7300, NULL)
  ON CONFLICT (id) DO NOTHING
`); err != nil {
    return fmt.Errorf("seed retention_config: %w", err)
}
```

Document in the file-level comment that the seed is part of the atomic install txn (D-23 audit/atomicity invariant).

**Step D — Extend `internal/install/finish_test.go`:**

Add `TestFinishSetup_SeedsRetentionConfig` that:

1. Runs the existing FinishSetup happy path
2. Queries `SELECT raw_days, hourly_days, daily_days, monthly_days, yearly_days FROM retention_config WHERE id = 1`
3. Asserts (90, 365, 1825, 7300, NULL)

Add `TestFinishSetup_RetentionRollsBackOnFailure` that forces an artificial post-retention failure (e.g. by injecting an admin row that already exists so the admin INSERT fails — same fixture used in the existing rollback test, just verifying retention_config remains empty after rollback).
  </action>
  <verify>
    <automated>go test ./internal/install/... -race -count=1 -short=false -run "TestFinishSetup" &amp;&amp; go test ./internal/db/... -race -count=1 -short=false -run "TestRunMigrations" &amp;&amp; grep -q "INSERT INTO retention_config" internal/install/finish.go</automated>
  </verify>
  <acceptance_criteria>
    - `internal/db/migrations/0029_retention_config.up.sql` contains literal `CREATE TABLE retention_config` and `CHECK (id = 1)` and `CHECK (raw_days BETWEEN 30 AND 365)` and `yearly_days INTEGER,` (with no NOT NULL)
    - `internal/install/finish.go` contains literal `INSERT INTO retention_config (id, raw_days, hourly_days, daily_days, monthly_days, yearly_days)` AND literal `VALUES (1, 90, 365, 1825, 7300, NULL)` AND literal `ON CONFLICT (id) DO NOTHING`
    - `internal/install/finish.go` retention INSERT is positioned AFTER install_identity UPSERT and BEFORE install_state DELETE (verify by reading the file order)
    - `TestFinishSetup_SeedsRetentionConfig` body asserts exact tuple (90, 365, 1825, 7300, NULL)
    - `TestFinishSetup_RetentionRollsBackOnFailure` body asserts retention_config remains empty when txn rolls back
    - `go test ./internal/install/...` exits 0
  </acceptance_criteria>
  <done>retention_config seeded with D-09 defaults inside the existing FinishSetup Serializable txn.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| migrate-up runtime → TimescaleDB | golang-migrate executes 5 new files inside transactions (CAGG `WITH NO DATA` workaround keeps this safe) |
| FinishSetup txn → retention_config | Install wizard caller seeds retention; same Serializable txn as admin user creation — atomic |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-05-02-01 | Tampering | CAGG retention footgun (Pitfall #2) | high | mitigate | All `start_offset` values verified ≤ 90d raw retention; explicit doc.go + 0025 header comment naming the constraint; `TestRefreshPolicyParams` asserts the bound programmatically. CAGG-over-CAGG sources (daily/monthly/yearly) bypass the footgun because their source is the lower CAGG (multi-year retention), not raw. |
| T-05-02-02 | Denial of Service | Real-time mode on monthly/yearly causes scan-on-read explosion (Pitfall #2 / D-11) | medium | mitigate | Monthly + yearly explicitly set `materialized_only = true`; `TestCAGGHierarchy` asserts the flag values; doc.go names the failure mode |
</threat_model>

<verification>
1. `go test ./internal/aggregate/... -race -count=1 -short=false` exits 0
2. `go test ./internal/install/... -race -count=1 -short=false` exits 0
3. `go test ./internal/db/... -race -count=1 -short=false -run TestRunMigrations` exits 0
4. `grep -l "WITH NO DATA" internal/db/migrations/002[5-8]_cagg_*.up.sql | wc -l` returns 4
5. `grep -l "materialized_only = false" internal/db/migrations/002[5-6]_cagg_*.up.sql | wc -l` returns 2
6. `grep -l "materialized_only = true" internal/db/migrations/002[7-8]_cagg_*.up.sql | wc -l` returns 2
7. NO Pitfall #1: `grep "WITH DATA" internal/db/migrations/002[5-8]*.sql` returns empty (only WITH NO DATA)
</verification>

<success_criteria>
- 4 CAGG views created with the correct hierarchy chain (hourly→daily→monthly→yearly)
- Refresh + retention policies attached per D-09/D-11 spec
- `cumulative_delta` computed via LAG() in hourly CAGG (per CUMULATIVE_DELTA.md verdict)
- `retention_config` table seeded with D-09 defaults at install finish, inside the existing FinishSetup Serializable txn
- All migrations round-trip clean (up → down → up via testcontainer)
- LAG-across-bucket math verified by `TestCAGGChain_DeltaCorrectness`
</success_criteria>

<output>
After completion, create `.planning/phases/05-aggregates-reports-map-floor-plans/05-02-SUMMARY.md` recording:
- Total CAGGs created (4) and their `materialized_only` settings
- Retention policies attached and their durations
- The exact `start_offset / end_offset` values per CAGG and the math that proves the DATA-12 invariant
- Whether `TestCAGGChain_DeltaCorrectness` cross-bucket LAG case passed on first attempt
- retention_config seed verified inside FinishSetup rollback test
</output>
