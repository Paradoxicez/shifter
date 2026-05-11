---
phase: 04-realtime-dashboard
plan: 05
subsystem: metering-point-detail

tags: [rest-api, sqlc, timeseries, cursor-pagination, metering-point, go]

# Dependency graph
requires:
  - phase: 04-realtime-dashboard
    plan: 01
    provides: "measurement hypertable indexes (measurement_mp_time_idx, measurement_quality_flagged_idx)"
  - phase: 04-realtime-dashboard
    plan: 04
    provides: "MeteringPointTimeseries query, BucketIntervalForRange, dashboard package"

provides:
  - "GET /api/metering-points/{id} — DETL-01 composite detail (Normal + Advanced tab data)"
  - "GET /api/metering-points/{id}/uplinks — DETL-02 cursor-paginated uplink log"
  - "GET /api/metering-points/{id}/timeseries — per-MP time-bucket series (D-12)"
  - "GET /api/metering-points/{id}/signal-history — D-17 fixed 24h hourly sparklines"
  - "ListUplinksByMP sqlc query — cursor (time DESC), quality whitelist filter, ≤500 rows"
  - "QualitySummaryByMP sqlc query — last-100 uplinks grouped by quality (D-19)"
  - "GetMeteringPointDetail sqlc query — composite LEFT JOIN MP+binding+latest reading"
  - "MeteringPointOnlineStatus sqlc query — D-07 online rule as device_bound+is_online"
  - "bytesToHex helper — D-18 lowercase space-separated hex format"

affects:
  - "04-09-PLAN (MP detail frontend) — consumes all 4 endpoints directly"

# Tech tracking
tech-stack:
  added: []  # zero new deps
  patterns:
    - "D-22 empty-MP: GetMeteringPointDetail LEFT JOINs always return MP row; handler emits active_binding=null, latest_reading=null, online=null when no binding/measurements"
    - "D-07 online flag: MeteringPointOnlineStatus returns device_bound (bool) + is_online (bool); handler emits online=null when device_bound=false"
    - "COALESCE(latest.quality,'') in LATERAL join prevents NULL scan into non-nullable string field"
    - "Cursor pagination: ListUplinksByMP uses time < COALESCE($3::timestamptz, 'infinity') — stable pages under concurrent ingest unlike OFFSET"
    - "Quality whitelist validated at handler layer (T-04-05-03) before sqlc call — 5 valid values: ok, decode_fail, missing_canonical, out_of_range, duplicate_fcnt"
    - "BucketIntervalForRange imported from internal/dashboard — single source of truth for D-12 bucket schedule"
    - "bytesToHex: fmt.Sprintf(%02x) per byte, strings.Join with space — 'de ad be ef' format (D-18)"

key-files:
  created:
    - "internal/db/queries/uplinks.sql (ListUplinksByMP + QualitySummaryByMP)"
    - "internal/db/queries/mp_detail.sql (GetMeteringPointDetail + MeteringPointOnlineStatus)"
    - "internal/db/sqlc/uplinks.sql.go (generated)"
    - "internal/db/sqlc/mp_detail.sql.go (generated)"
    - "internal/meteringpoint/detail_handler.go (GET /api/metering-points/{id})"
    - "internal/meteringpoint/detail_handler_test.go"
    - "internal/meteringpoint/uplinks_handler.go (GET /api/metering-points/{id}/uplinks)"
    - "internal/meteringpoint/uplinks_handler_test.go"
    - "internal/meteringpoint/timeseries_handler.go (GET /api/metering-points/{id}/timeseries)"
    - "internal/meteringpoint/timeseries_handler_test.go"
    - "internal/meteringpoint/signal_handler.go (GET /api/metering-points/{id}/signal-history)"
    - "internal/meteringpoint/signal_handler_test.go"
  modified:
    - "internal/meteringpoint/handlers.go (RegisterRoutes — 4 new GET routes; replaced Phase 2 /{id} with Phase 4 handleDetail)"
    - "internal/meteringpoint/handlers_test.go (updated TestGetMPDetail_* to Phase 4 response shape)"
    - "internal/db/sqlc/querier.go (regenerated interface)"

key-decisions:
  - "MeteringPointOnlineStatus returns two non-null booleans (device_bound, is_online) instead of a nullable bool CASE expression — sqlc cannot infer nullable return type from CASE/WHEN, so two concrete columns avoids a NULL scan error on the Go side."
  - "COALESCE(latest.quality,'') in GetMeteringPointDetail LATERAL join — the measurement.quality column is NOT NULL, but the LEFT LATERAL join produces NULL when no measurement exists (D-22). COALESCE prevents pgx from failing to scan NULL into a Go string."
  - "Route replacement: GET /{id} now points to deps.handleDetail (Phase 4 superset) instead of the Phase 2 getMPDetail. Pre-existing TestGetMPDetail_* tests updated to the new response shape."
  - "Import dashboard.BucketIntervalForRange rather than duplicating — keeps the D-12 bucket schedule in one place; the import is clean (no circular dependency)."

requirements-completed: [DETL-01, DETL-02, DETL-03]

# Metrics
duration: 19min
completed: 2026-05-11
---

# Phase 4 Plan 05: MP Detail REST Endpoints Summary

**4 metering-point detail endpoints — Normal tab, Advanced tab, uplinks log, and signal sparklines — powered by 2 new sqlc query files and 4 handler files.**

## Performance

- **Duration:** ~19 minutes
- **Started:** 2026-05-11T14:36:52Z
- **Completed:** 2026-05-11T14:56:04Z
- **Tasks:** 2
- **Files modified:** 14 (2 new SQL, 3 sqlc-regenerated, 8 new Go handler files, 2 modified Go files)

## Accomplishments

- **GET /api/metering-points/{id}** (DETL-01): Single round-trip composite query returning MP info, active binding, latest reading (with `decoded_object` + `extra` JSONB for the Advanced tab), quality summary (last-100 window per D-19), and D-07 online flag. D-22 empty-MP case returns 200 with null sub-objects — frontend can render the Normal tab and disable Advanced/Uplinks tabs.
- **GET /api/metering-points/{id}/uplinks** (DETL-02): Cursor-paginated uplink log (time DESC, `before=<iso>` cursor). Hard-capped at 500 rows (D-16). Quality filter via `quality=ok,decode_fail` with server-side whitelist validation (T-04-05-03). Response includes `has_more` + `next_before` for infinite-scroll pagination.
- **GET /api/metering-points/{id}/timeseries**: Per-MP time-bucket series, identical D-12 bucket schedule as dashboard timeseries (today/24h=5min, 7d=1h, 30d=4h, custom). Returns cumulative, instant, battery, RSSI, SNR averages per bucket.
- **GET /api/metering-points/{id}/signal-history** (D-17): Fixed 24-hour window, hourly buckets — returns battery/RSSI/SNR sparkline data without cumulative/instant. Simplifies frontend (no range params needed for sparklines).
- **27/27 meteringpoint tests pass** under `-race`; full short suite 380/380 green.

## Endpoint Contracts (for Plan 09 frontend consumption)

### GET /api/metering-points/{id}

```json
{
  "metering_point": {
    "id": "<uuid>",
    "name": "Main Water Inlet",
    "site_id": "<uuid>",
    "site_name": "Building A",
    "utility_class": "water",
    "location_description": "Ground floor riser"
  },
  "active_binding": {
    "device_id": "<uuid>",
    "dev_eui": "aabbccdd11223344",
    "device_profile_name": "Axioma W1",
    "valid_from": "2026-05-01T08:00:00Z"
  },
  "latest_reading": {
    "time": "2026-05-11T14:30:00Z",
    "cumulative_value": "1234.56",
    "instant_value": "4.2",
    "quality": "ok",
    "battery_pct": 87,
    "rssi": -65,
    "snr": 8.5,
    "fcnt": 428,
    "decoded_object": {"cumulative_l": 1234560, "battery_pct": 87},
    "extra": {},
    "raw_payload_hex": "0a 1f 3c 4d"
  },
  "quality_summary": {
    "window_size": 100,
    "flagged_count": 3,
    "by_quality": {"ok": 97, "decode_fail": 2, "out_of_range": 1}
  },
  "online": true
}
```

**D-22 empty-MP variant** (no binding, no uplinks):
```json
{
  "metering_point": { ... },
  "active_binding": null,
  "latest_reading": null,
  "quality_summary": { "window_size": 0, "flagged_count": 0, "by_quality": {} },
  "online": null
}
```

### GET /api/metering-points/{id}/uplinks

Query params: `limit` (1–500, default 100), `before` (RFC3339 cursor), `quality` (comma-separated whitelist).

```json
{
  "uplinks": [
    {
      "time": "2026-05-11T14:30:00Z",
      "cumulative_value": "1234.56",
      "instant_value": "4.2",
      "battery_pct": 87,
      "rssi": -65,
      "snr": 8.5,
      "fcnt": 428,
      "quality": "ok",
      "raw_payload_hex": "0a 1f 3c 4d",
      "decoded_object": {"cumulative_l": 1234560}
    }
  ],
  "has_more": true,
  "next_before": "2026-05-11T13:30:00Z"
}
```

### GET /api/metering-points/{id}/timeseries

Query params: `range` (today|24h|7d|30d|custom), `start`/`end` (RFC3339 for custom).

```json
{
  "bucket_interval_seconds": 300,
  "series": [
    {
      "bucket": "2026-05-11T00:00:00Z",
      "cumulative_avg": 1234.5,
      "instant_avg": 4.2,
      "battery_avg": 87.0,
      "rssi_avg": -65.0,
      "snr_avg": 8.5
    }
  ]
}
```

### GET /api/metering-points/{id}/signal-history

No query params. Always last 24h, 1-hour buckets (D-17).

```json
{
  "window_start": "2026-05-10T14:56:04Z",
  "window_end": "2026-05-11T14:56:04Z",
  "bucket_interval_seconds": 3600,
  "series": [
    { "bucket": "2026-05-11T14:00:00Z", "battery_pct": 87.0, "rssi": -65.0, "snr": 8.5 }
  ]
}
```

## raw_payload_hex Format (D-18)

Lowercase, space-separated bytes: `"0a 1f 3c 4d"`. Implemented by `bytesToHex()` in `detail_handler.go`:

```go
func bytesToHex(b []byte) string {
    parts := make([]string, len(b))
    for i, x := range b { parts[i] = fmt.Sprintf("%02x", x) }
    return strings.Join(parts, " ")
}
```

## D-22 Empty-MP Convention

When a metering point exists but has no active binding and no measurements:
- `active_binding: null`
- `latest_reading: null`
- `quality_summary: {window_size: 0, flagged_count: 0, by_quality: {}}`
- `online: null`

Frontend uses `latest_reading == null` to disable the Advanced tab and Uplinks tab. This is the only state where `online` is null (a bound device that has never been seen returns `online: false`, not null).

## Cursor Pagination Shape

```
GET /api/metering-points/{id}/uplinks?limit=100
→ { uplinks: [...100 rows...], has_more: true, next_before: "2026-05-11T13:30:00Z" }

GET /api/metering-points/{id}/uplinks?limit=100&before=2026-05-11T13:30:00Z
→ { uplinks: [...remaining rows...], has_more: false, next_before: null }
```

The `before` cursor is the ISO timestamp of the oldest row from the previous page. The query uses `time < COALESCE($3::timestamptz, 'infinity'::timestamptz)` — no OFFSET, stable under concurrent ingest.

## Task Commits

1. **Task 1: sqlc queries** — `8c7ac2f` (feat)
2. **Task 2: 4 handlers + routes + hex helper + tests** — `daa213a` (feat)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] MeteringPointOnlineStatus: nullable bool CASE expression fails sqlc type inference**
- **Found during:** Task 1 (inspecting generated mp_detail.sql.go)
- **Issue:** The CASE expression returning NULL for no-binding case caused sqlc to infer `bool` (non-nullable) rather than `*bool`. pgx would fail scanning NULL into `bool` at runtime.
- **Fix:** Rewrote query to return two non-null booleans: `(b.device_id IS NOT NULL) AS device_bound` and `COALESCE(..., FALSE) AS is_online`. Handler converts to `*bool` JSON field: `null` when `device_bound=false`, `true`/`false` otherwise.
- **Files modified:** `internal/db/queries/mp_detail.sql`, `internal/db/sqlc/mp_detail.sql.go`
- **Committed in:** `8c7ac2f`

**2. [Rule 1 - Bug] GetMeteringPointDetail: NULL scan into non-nullable quality string (D-22 empty case)**
- **Found during:** Task 2 (TestDetailHandler_EmptyMP failed with 500)
- **Issue:** When no measurement exists, the LEFT LATERAL join produces NULL for `latest.quality`. sqlc inferred `Quality string` (non-nullable from the `NOT NULL DEFAULT 'ok'` column), but the LATERAL join returns NULL. pgx fails to scan NULL into string.
- **Fix:** Added `COALESCE(latest.quality, '') AS quality` in the LATERAL join projection. Handler gates on `LatestTime.Valid` to ignore the empty-string quality when no reading exists.
- **Files modified:** `internal/db/queries/mp_detail.sql`, `internal/db/sqlc/mp_detail.sql.go`
- **Committed in:** `daa213a`

**3. [Rule 1 - Bug] Pre-existing TestGetMPDetail_* tests expected Phase 2 response shape**
- **Found during:** Task 2 (full meteringpoint test suite run after route replacement)
- **Issue:** Replacing `getMPDetail` with `deps.handleDetail` on the `GET /{id}` route changed the response shape from Phase 2 (nested `site`, `latest_measurement`, `active_binding.device.dev_eui`) to Phase 4 (flat `metering_point.site_name`, `latest_reading`, `active_binding.dev_eui`). Two pre-existing tests panicked with interface conversion errors.
- **Fix:** Updated `TestGetMPDetail_NoActiveBinding` and `TestGetMPDetail_WithActiveBinding` to decode into `DetailResponse` and assert on the Phase 4 field names.
- **Files modified:** `internal/meteringpoint/handlers_test.go`
- **Committed in:** `daa213a`

**4. [Rule 1 - Bug] seedNMeasurements: `'{"i":$4}'` is invalid SQL parameter syntax**
- **Found during:** Task 2 (TestUplinksHandler_HappyPath + CursorPagination failed with SQLSTATE 22P02)
- **Issue:** The test helper used `'{"i":$4}'::jsonb` — Postgres does not expand `$4` inside string literals; it's literal text, causing JSON parse error.
- **Fix:** Simplified to `'{}'::jsonb` (content not needed for pagination tests).
- **Files modified:** `internal/meteringpoint/uplinks_handler_test.go`
- **Committed in:** `daa213a`

## Known Stubs

None — all 4 endpoints return real data from the database. Empty-MP case returns zero/null values cleanly per D-22 contract.

## Threat Flags

None beyond the plan's threat register (T-04-05-01 through T-04-05-05 all mitigated as planned):
- T-04-05-01: `uuid.Parse` at handler entry; 400 on parse error.
- T-04-05-02: Hard-cap limit ≤ 500 enforced at parse stage; explicit `n > 500` → 400.
- T-04-05-03: Quality whitelist validated against `validQualityValues` map before sqlc call.

## Self-Check: PASSED

- `internal/db/queries/uplinks.sql` — FOUND
- `internal/db/queries/mp_detail.sql` — FOUND
- `internal/db/sqlc/uplinks.sql.go` — FOUND
- `internal/db/sqlc/mp_detail.sql.go` — FOUND
- `internal/meteringpoint/detail_handler.go` — FOUND
- `internal/meteringpoint/uplinks_handler.go` — FOUND
- `internal/meteringpoint/timeseries_handler.go` — FOUND
- `internal/meteringpoint/signal_handler.go` — FOUND
- Commit `8c7ac2f` (Task 1) — FOUND in `git log`
- Commit `daa213a` (Task 2) — FOUND in `git log`
- `go build ./...` — exits 0
- `go test ./internal/meteringpoint/ -count=1 -timeout 180s -race` — 27/27 PASS
- `go test ./... -short -count=1 -race` — 380/380 PASS
- `grep -q "2 \* dp.expected_interval_s \* INTERVAL"` — PASS
- `grep -q "time < COALESCE"` — PASS
- Routes mounted for all 4 endpoints — PASS

---
*Phase: 04-realtime-dashboard*
*Plan: 05*
*Completed: 2026-05-11*
