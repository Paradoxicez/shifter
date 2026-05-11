---
phase: 04-realtime-dashboard
plan: 04
subsystem: dashboard

tags: [dashboard, kpi, timeseries, rest-api, sqlc, timescaledb, go]

# Dependency graph
requires:
  - phase: 04-realtime-dashboard
    plan: 01
    provides: "install_identity.capabilities (D-09), device_profile.expected_interval_s (D-07), measurement_inserted trigger"

provides:
  - "GET /api/dashboard/scope — capabilities + onboarding tuple (D-09, D-21)"
  - "GET /api/dashboard/snapshot — KPI tiles (D-05/06/07/08) + per-MP latest readings"
  - "GET /api/dashboard/timeseries — D-12 time-bucket aggregated cumulative series"
  - "internal/dashboard package with BuildSnapshot + BucketIntervalForRange"
  - "sqlc queries: TodayConsumptionByUtility, CurrentInstantSumByUtility, PeriodDeltaByUtility, DeviceOnlineCount, DashboardLatestReadings, OnboardingCounts, GetCapabilities, DashboardTimeseries, MeteringPointTimeseries"

affects:
  - "04-07-PLAN (Settings UI reads capabilities via same GetCapabilities query)"
  - "Frontend Plan 07 (dashboard route shell) — consumes all 3 endpoints"
  - "Frontend Plan 08 (consumption chart) — consumes GET /api/dashboard/timeseries"

# Tech tracking
tech-stack:
  added: []  # zero new deps; pure sqlc + stdlib
  patterns:
    - "D-12 bucket schedule: today/24h=5min, 7d=1h, 30d=4h, custom≤30d=1h, custom>30d=1day"
    - "D-09 capability gating: kpis map key absent (not null) when utility inactive"
    - "LAG window + SUM aggregate split into CTE (deltas) + outer GROUP BY to avoid SQLSTATE 42803"
    - "sqlc.arg() named params on multi-parameter queries for clean Params struct field names"
    - "Parallel goroutines for GetCapabilities + OnboardingCounts in scope handler (sync.WaitGroup)"
    - "T-04-04-04: timezone always from install_identity.timezone server-side, never request param"

key-files:
  created:
    - "internal/db/queries/dashboard.sql"
    - "internal/db/queries/timeseries.sql"
    - "internal/db/sqlc/dashboard.sql.go (generated)"
    - "internal/db/sqlc/timeseries.sql.go (generated)"
    - "internal/dashboard/doc.go"
    - "internal/dashboard/kpi.go"
    - "internal/dashboard/kpi_test.go"
    - "internal/dashboard/snapshot_handler.go"
    - "internal/dashboard/snapshot_handler_test.go"
    - "internal/dashboard/timeseries_handler.go"
    - "internal/dashboard/timeseries_handler_test.go"
    - "internal/dashboard/install_scope_handler.go"
    - "internal/dashboard/install_scope_handler_test.go"
    - "internal/dashboard/routes.go"
  modified:
    - "internal/http/router.go (DashboardDeps field + nil-guard RegisterRoutes call)"
    - "internal/cli/serve.go (dashboard import + DashboardDeps construction)"
    - "internal/db/sqlc/querier.go (regenerated interface)"

key-decisions:
  - "CTE pattern for DashboardTimeseries: LAG window function computes row_delta in CTE; outer query SUMs by bucket. Postgres forbids SUM(LAG(...)) directly (SQLSTATE 42803)."
  - "sqlc.arg() named parameters on timezone params prevent sqlc from inferring pgtype.Interval for AT TIME ZONE args — forces clean string field names (Timezone string, not pgtype.Interval)."
  - "D-09 absent-key gating: kpis map only includes active utility keys. Frontend uses 'water' in kpis to gate rendering — not 'water' in kpis == null check."
  - "T-04-04-03 DoS protection: custom range capped at 365 days at handler layer before any DB query. Returns 400 immediately."

requirements-completed: [DASH-01, DASH-02, DASH-04, DASH-05, DASH-06]

# Metrics
duration: 21min
completed: 2026-05-11
---

# Phase 4 Plan 04: Dashboard REST Endpoints Summary

**3 dashboard REST endpoints + sqlc KPI queries — feeds KPI tiles, consumption chart, and onboarding empty-state in the frontend.**

## Performance

- **Duration:** ~21 minutes
- **Started:** 2026-05-11T14:12:36Z
- **Completed:** 2026-05-11T14:33:23Z
- **Tasks:** 2
- **Files modified:** 16 (2 new SQL, 3 sqlc-regenerated, 10 new Go, 2 modified Go)

## Accomplishments

- **GET /api/dashboard/scope** returns `{capabilities, onboarding: {gateway_count, device_count, uplink_count}}`. Runs `GetCapabilities` and `OnboardingCounts` in parallel via `sync.WaitGroup`. D-21 progressive empty-state and D-09 capability flag both served from this single endpoint.
- **GET /api/dashboard/snapshot** returns `{capabilities, generated_at, kpis: {water?, electricity?}, latest_readings: [...]}`. D-09 gating: the `kpis` map only contains keys for active utilities — if `capabilities='water'`, the `electricity` key is entirely absent (not null). Frontend uses `'water' in kpis` to decide what to render.
- **GET /api/dashboard/timeseries** parses `range`, `utility`, `start`, `end`; validates inputs (400 for invalid range/utility/custom-missing-dates/span>365d); selects bucket interval per D-12 schedule; runs `DashboardTimeseries` query; returns `{utility, bucket_interval_seconds, series: [{bucket, cumulative_delta}]}`.
- **sqlc queries** (9 total): `TodayConsumptionByUtility` (D-05 timezone boundary), `CurrentInstantSumByUtility` (D-06), `PeriodDeltaByUtility` with `today_slice`+`yesterday_slice` CTEs (D-08), `DeviceOnlineCount` with verbatim D-07 rule, `DashboardLatestReadings`, `OnboardingCounts`, `GetCapabilities`, `DashboardTimeseries` (CTE+LAG pattern), `MeteringPointTimeseries`.
- **34 tests pass under -race**: 15 unit (BucketIntervalForRange table, utilitiesFor, unitLabel) + 19 integration (testcontainer Postgres) covering all handler paths, all validation cases, D-09 capability gating, empty-install zero values.
- **Full -short suite**: 380/380 tests pass (baseline was 346).

## Endpoint Contracts (for Plans 07 and 08)

### GET /api/dashboard/scope

```json
{
  "capabilities": "water" | "electricity" | "both",
  "onboarding": {
    "gateway_count": 0,
    "device_count": 0,
    "uplink_count": 0
  }
}
```

### GET /api/dashboard/snapshot

```json
{
  "capabilities": "both",
  "generated_at": "2026-05-11T12:34:56Z",
  "kpis": {
    "water": {
      "today_consumption": 47.2,
      "today_unit": "m³",
      "instant_total": 12.4,
      "instant_unit": "L/min",
      "period_delta_abs": 3.4,
      "period_delta_pct": 8.0,
      "online_count": 10,
      "total_count": 12
    },
    "electricity": {
      "today_consumption": 120.5,
      "today_unit": "kWh",
      "instant_total": 5400.0,
      "instant_unit": "W",
      "period_delta_abs": null,
      "period_delta_pct": null,
      "online_count": 3,
      "total_count": 3
    }
  },
  "latest_readings": [
    {
      "metering_point_id": "<uuid>",
      "utility_class": "water",
      "time": "2026-05-11T12:00:00Z",
      "cumulative_value": 1234.56,
      "instant_value": 4.2,
      "quality": "ok",
      "battery_pct": 87,
      "rssi": -65
    }
  ]
}
```

**D-09 gating:** if `capabilities='water'`, the `electricity` key is entirely absent from `kpis` (not null). `period_delta_abs` / `period_delta_pct` are null when yesterday has no data (install <24 h old).

### GET /api/dashboard/timeseries

```json
{
  "utility": "water",
  "bucket_interval_seconds": 300,
  "series": [
    { "bucket": "2026-05-11T00:00:00Z", "cumulative_delta": 1.2 },
    { "bucket": "2026-05-11T00:05:00Z", "cumulative_delta": 0.8 }
  ]
}
```

## Unit Mapping (Plan 07 contract)

| utility_class | today_unit | instant_unit |
|---------------|------------|--------------|
| water         | m³         | L/min        |
| electricity   | kWh        | W            |

## D-12 Bucket Interval Schedule

| range        | bucket_interval | bucket_interval_seconds |
|--------------|-----------------|-------------------------|
| today        | 5 minutes       | 300                     |
| 24h          | 5 minutes       | 300                     |
| 7d           | 1 hour          | 3600                    |
| 30d          | 4 hours         | 14400                   |
| custom ≤ 30d | 1 hour          | 3600                    |
| custom > 30d | 1 day           | 86400                   |

Custom range max: 365 days. Requests exceeding this return 400 (T-04-04-03).

## D-07 Integration (expected_interval_s)

The `DeviceOnlineCount` query uses the verbatim D-07 rule from the plan:

```sql
COUNT(*) FILTER (
    WHERE d.last_seen_at > now() - (2 * dp.expected_interval_s * INTERVAL '1 second')
) AS online_count
```

Per-profile thresholds (from migration 0023): water=3600s, electricity=300s. Online count returned in each utility's KPI block.

## Task Commits

1. **Task 1: sqlc queries — dashboard KPIs + timeseries** — `cc0a042` (feat)
2. **Task 2: KPI assembler + 3 handlers + routes wiring** — `8259eb6` (feat)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] DashboardTimeseries: SUM(LAG()) forbidden by Postgres (SQLSTATE 42803)**
- **Found during:** Task 2 (integration tests returned 500)
- **Issue:** The plan-verbatim SQL had `SUM(m.cumulative_value - LAG(...) OVER (...))` directly in a GROUP BY query. Postgres forbids aggregate functions that contain window function calls.
- **Fix:** Restructured as a two-step CTE: `deltas` CTE computes `LAG(cumulative_value)` per-row as `row_delta`; outer query does `SUM(row_delta) GROUP BY bucket`. Semantically identical result, valid SQL.
- **Files modified:** `internal/db/queries/timeseries.sql`, `internal/db/sqlc/timeseries.sql.go`
- **Committed in:** `8259eb6`

**2. [Rule 1 - Bug] sqlc inferred pgtype.Interval for AT TIME ZONE params**
- **Found during:** Task 1 (inspecting generated dashboard.sql.go)
- **Issue:** sqlc inferred `Timezone pgtype.Interval` instead of `Timezone string` for parameters used in `AT TIME ZONE $1` expressions — breaking the Go handler which passes plain strings.
- **Fix:** Added `sqlc.arg(timezone)::text` named parameter annotations with explicit `::text` casts to force string inference. Generated `TodayConsumptionByUtilityParams.Timezone` and `PeriodDeltaByUtilityParams.Timezone` are now `string`.
- **Files modified:** `internal/db/queries/dashboard.sql`, `internal/db/sqlc/dashboard.sql.go`
- **Committed in:** `cc0a042`

**3. [Rule 1 - Bug] nil io.Writer panic in test logger**
- **Found during:** Task 2 (first integration test run panicked)
- **Issue:** Test files used `slog.NewTextHandler(nil, nil)` — a nil `io.Writer` causes panic when slog tries to write log output during `db.RunMigrations`.
- **Fix:** Changed to `slog.NewTextHandler(io.Discard, nil)` in `timeseries_handler_test.go` and `install_scope_handler_test.go`.
- **Files modified:** `internal/dashboard/timeseries_handler_test.go`, `internal/dashboard/install_scope_handler_test.go`
- **Committed in:** `8259eb6`

## Threat Flags

None — no new network surface beyond the three endpoints documented in the plan's threat model (T-04-04-01 through T-04-04-05 all mitigated as planned: range enum validation, custom range cap, timezone server-side only, authenticated group mount).

## Known Stubs

None — all three endpoints return real data from the database. Empty installs return zero values cleanly (no panics, no null crashes).

## Self-Check: PASSED

- `internal/db/queries/dashboard.sql` — FOUND
- `internal/db/queries/timeseries.sql` — FOUND
- `internal/dashboard/kpi.go` — FOUND
- `internal/dashboard/snapshot_handler.go` — FOUND
- `internal/dashboard/timeseries_handler.go` — FOUND
- `internal/dashboard/install_scope_handler.go` — FOUND
- `internal/dashboard/routes.go` — FOUND
- Commit `cc0a042` (Task 1) — FOUND in `git log`
- Commit `8259eb6` (Task 2) — FOUND in `git log`
- `go build ./...` — exits 0
- `sqlc generate` — exits 0
- `go test ./internal/dashboard/ -count=1 -timeout 180s -race` — 34/34 PASS
- `go test ./... -short -count=1 -race` — 380/380 PASS

---
*Phase: 04-realtime-dashboard*
*Plan: 04*
*Completed: 2026-05-11*
