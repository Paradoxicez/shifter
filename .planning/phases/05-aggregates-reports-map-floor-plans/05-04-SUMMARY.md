---
phase: 05-aggregates-reports-map-floor-plans
plan: 04
subsystem: api
tags: [go, sqlc, timescaledb, leaflet, map, postgis-free, capability-gating]

# Dependency graph
requires:
  - phase: 05-aggregates-reports-map-floor-plans
    provides: "measurement_hourly CAGG, install_identity with capabilities, site/gateway/device schema"
  - phase: 04
    provides: "D-07 online rule (last_seen_at + expected_interval_s), D-09 capability gating, binding table for device-MP joins"
provides:
  - "GET /api/map/data endpoint returning {sites, gateways} with lat/lng and rollups"
  - "mapapi Go package (internal/map/) with Deps, DataHandler, RegisterRoutes"
  - "Three typed sqlc queries: ListSitesForMap, ListGatewaysForMap, TodaySiteConsumption"
affects: [frontend-map-view, floor-plan-picker, gateway-create-modal]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "httptest.Server + cookiejar + /seed endpoint for SCS session integration tests (mirrors rbac_test.go)"
    - "measurement_hourly CAGG with materialized_only=false for real-time last-2h window — always insert at now() not past timestamps in tests"
    - "capability gating via switch on install_identity.capabilities string (water/electricity/both)"

key-files:
  created:
    - internal/db/queries/map.sql
    - internal/db/sqlc/map.sql.go
    - internal/map/doc.go
    - internal/map/handler.go
    - internal/map/routes.go
    - internal/map/handler_test.go
  modified:
    - internal/db/sqlc/querier.go
    - internal/db/migrations/0030_report.up.sql

key-decisions:
  - "Gateway online detection uses stats_refreshed_at > now() - 5min (gateway table has no last_seen_at column)"
  - "Device-MP join goes through binding table (device has no metering_point_id column)"
  - "TodaySiteConsumption uses midnight-in-install-tz as lower bound, fetched server-side from GetInstallIdentity — never from request"
  - "CAGG real-time zone covers last 2 hours; test measurements inserted at now()-2min and now()-1min"

patterns-established:
  - "mapapi package name (map is a Go keyword); directory is internal/map/ for consistency"
  - "nil lat/lng dereferencing pattern: query WHERE lat IS NOT NULL ensures safe dereference in Go"

requirements-completed: [MAP-01, MAP-04]

# Metrics
duration: 90min
completed: 2026-05-12
---

# Phase 05 Plan 04: Map Backend Summary

**Read-only GET /api/map/data serving site/gateway map markers with D-07 online counts and capability-gated today_consumption from the measurement_hourly CAGG**

## Performance

- **Duration:** ~90 min
- **Started:** 2026-05-12T00:00:00Z
- **Completed:** 2026-05-12T00:39:10Z
- **Tasks:** 1 (single implementation task with integrated test)
- **Files modified:** 8

## Accomplishments

- Built `internal/map/` package (`mapapi`) with `DataHandler`, `RegisterRoutes`, and typed `Deps` struct
- Three sqlc queries cover all data needs: site rollups with D-07 online/offline counts via binding table, gateway online from `stats_refreshed_at`, today's consumption from `measurement_hourly` CAGG within install-timezone midnight window
- Capability gating (D-09): `install_identity.capabilities ∈ {water, electricity, both}` filters `today_consumption` keys — single-capability installs return only the matching key
- MAP-04 invariant: no tile URL or API key in any response field
- 4 integration tests pass with real Postgres: full data, water-only cap gate, 401 unauthenticated, OSM invariant check
- Fixed pre-existing migration bug in 0030_report.up.sql (REFERENCES users → REFERENCES "user")

## Task Commits

1. **Task 1: sqlc queries + handler + capability-gated assembly** - `3124ff9` (feat)

**Plan metadata:** _(created after this SUMMARY)_

## Files Created/Modified

- `internal/db/queries/map.sql` - Three sqlc-annotated queries: ListSitesForMap, ListGatewaysForMap, TodaySiteConsumption
- `internal/db/sqlc/map.sql.go` - Generated typed Go from map.sql
- `internal/db/sqlc/querier.go` - Updated interface with the three new methods
- `internal/map/doc.go` - Package godoc: endpoint contract, D-09 gating, MAP-04 invariant, GW-04 reuse note
- `internal/map/handler.go` - DataHandler, SiteMarker, GatewayMarker, Response, capabilityIncludes, writeJSON
- `internal/map/routes.go` - RegisterRoutes mounting GET /api/map/data behind RequireAction(ActionSiteRead)
- `internal/map/handler_test.go` - 4 integration tests with httptest.Server + cookiejar + /seed pattern
- `internal/db/migrations/0030_report.up.sql` - Bugfix: REFERENCES "user"(id) not REFERENCES users(id)

## Decisions Made

- **Gateway online via stats_refreshed_at**: Gateway table has no `last_seen_at` column; `stats_refreshed_at > now() - INTERVAL '5 minutes'` is the correct online proxy (updated by cache_refresher goroutine every ~60s)
- **Binding table for device-MP join**: `device` has no `metering_point_id` column; must join `metering_point → binding → device → device_profile` for online/offline counts
- **CAGG real-time window**: `measurement_hourly` uses `materialized_only=false` with `end_offset=2h`; test measurements inserted at `now()-2min`/`now()-1min` to be within the real-time zone without requiring explicit `CALL refresh_continuous_aggregate`
- **lat/lng column names**: site and gateway tables use `lat`/`lng` (not `latitude`/`longitude`); plan SQL had wrong names

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed migration 0030_report.up.sql: REFERENCES users → REFERENCES "user"**
- **Found during:** Task 1 (integration test setup — all migration tests failed)
- **Issue:** `0030_report.up.sql` referenced `users(id)` but the table created by `0002_users.up.sql` is `"user"` (singular, quoted). This broke ALL project migration tests.
- **Fix:** Changed `REFERENCES users(id)` → `REFERENCES "user"(id)` in the migration file
- **Files modified:** `internal/db/migrations/0030_report.up.sql`
- **Verification:** All 4 map integration tests pass; migration chain is valid
- **Committed in:** `3124ff9` (Task 1 commit)

**2. [Rule 1 - Bug] Fixed SQL column names and join path from plan**
- **Found during:** Task 1 (sqlc generate failures + query errors)
- **Issue:** Plan's SQL used `s.latitude`/`s.longitude` (actual: `s.lat`/`s.lng`), `g.last_seen_at` (actual: `g.stats_refreshed_at`), and direct `device.metering_point_id` join (actual: must go through `binding` table)
- **Fix:** Corrected all column names and join path in `map.sql`
- **Files modified:** `internal/db/queries/map.sql`
- **Committed in:** `3124ff9`

**3. [Rule 1 - Bug] Fixed CAGG test data: single measurement gives zero delta**
- **Found during:** Task 1 (TestMapData showed today_consumption as empty)
- **Issue:** A single measurement per MP gives delta = last - first = 0 (no range); CAGG requires two measurements to compute a positive cumulative_delta
- **Fix:** Insert 2 measurements per MP at `now()-2min` and `now()-1min`; also fixed timestamp to stay within CAGG real-time zone (last 2 hours)
- **Committed in:** `3124ff9`

**4. [Rule 2 - Missing Critical] Added device_profile.capabilities correct constraint values**
- **Found during:** Task 1 (test seeding violated check constraint)
- **Issue:** Test used `ARRAY['water']`/`ARRAY['electricity']` but constraint allows only `'cumulative'`, `'flow_rate'`, `'instant_power'`, `'battery'`, etc.
- **Fix:** Changed to `ARRAY['cumulative']` and `ARRAY['instant_power']` (valid values)
- **Committed in:** `3124ff9`

---

**Total deviations:** 4 auto-fixed (3 bugs, 1 missing correctness)
**Impact on plan:** All fixes were required for correct operation. Bug #1 (migration) was pre-existing from plan 05-03; fixes were in scope as they blocked test execution.

## Threat Surface Scan

No new network endpoints, auth paths, or schema changes beyond what the plan specified. The endpoint is gated by `auth.RequireAction(ActionSiteRead)` — matches T-05-04-01. Response contains no tile URLs or API keys — matches T-05-04-02 (MAP-04 invariant).

## Known Stubs

None. The endpoint queries live data from real tables; `today_consumption` returns empty map `{}` when no measurements exist (correct behavior, not a stub).

## Self-Check: PASSED

- `internal/db/queries/map.sql` — EXISTS
- `internal/db/sqlc/map.sql.go` — EXISTS
- `internal/map/doc.go` — EXISTS
- `internal/map/handler.go` — EXISTS
- `internal/map/routes.go` — EXISTS
- `internal/map/handler_test.go` — EXISTS
- Commit `3124ff9` — EXISTS (`git log --oneline | grep 3124ff9`)
