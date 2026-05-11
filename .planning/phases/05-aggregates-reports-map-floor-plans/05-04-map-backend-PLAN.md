---
phase: 05-aggregates-reports-map-floor-plans
plan: 04
type: execute
wave: 2
depends_on: [01]
files_modified:
  - internal/db/queries/map.sql
  - internal/db/sqlc/map.sql.go
  - internal/map/doc.go
  - internal/map/handler.go
  - internal/map/handler_test.go
  - internal/map/routes.go
autonomous: true
requirements: [MAP-01, MAP-04]
threat_refs: [T-05-04-01, T-05-04-02]

must_haves:
  truths:
    - "GET /api/map/data returns JSON {sites: [{id, name, lat, lng, mp_count, online_count, offline_count, today_consumption: {water?, electricity?}}], gateways: [{id, name, lat, lng, online}]}"
    - "Only sites with lat/lng are returned (sites without coordinates excluded — they cannot be plotted)"
    - "Only gateways with lat/lng are returned (matches MAP-01 contract — gateways can be plotted)"
    - "online_count / offline_count use the Phase 4 D-07 rule: `device.last_seen_at > now() − 2 × device_profile.expected_interval_s`"
    - "today_consumption respects install_identity.capabilities (Phase 4 D-09) — single-capability install returns only the matching key (water OR electricity, not both)"
    - "Today's consumption is computed via measurement_hourly bucket sum for `[install_tz today 00:00, now]`"
    - "All queries via sqlc with parameter binding — no raw SQL concat"
    - "Handler responds 200 for authenticated users (admin OR viewer — read-only data); 401 for unauthenticated"
  artifacts:
    - path: "internal/db/queries/map.sql"
      provides: "ListSitesForMap + ListGatewaysForMap + TodayConsumptionForSite sqlc queries"
      contains: "name: ListSitesForMap"
    - path: "internal/map/doc.go"
      provides: "Package documentation: data model, capability gating, OSM-only invariant"
      contains: "package mapapi"
    - path: "internal/map/handler.go"
      provides: "DataHandler — assembles sites + gateways + per-site rollups; capability-gated today_consumption"
      contains: "func DataHandler"
    - path: "internal/map/routes.go"
      provides: "RegisterRoutes(r chi.Router, deps Deps) — mounts GET /api/map/data"
      contains: "/api/map/data"
  key_links:
    - from: "internal/map/handler.go"
      to: "internal/db/sqlc.ListSitesForMap + ListGatewaysForMap + TodayConsumptionForSite"
      via: "sqlc-generated query calls"
      pattern: "q\\.(ListSitesForMap|ListGatewaysForMap|TodayConsumptionForSite)"
    - from: "internal/map/handler.go"
      to: "install_identity.capabilities + .timezone"
      via: "deps.Identity (loaded at server boot)"
      pattern: "deps\\.Identity\\.Capabilities"
---

<objective>
Build the read-only `/api/map/data` endpoint that powers the Map view (plan 05-08). Returns sites + gateways with lat/lng, per-site rollups (MP count, online/offline, today's consumption per capability), and gateway online state. All queries via sqlc against existing tables (site + gateway + metering_point + measurement_hourly) — no schema changes needed.

Purpose: The map is a "where is my fleet physically" view. Devices live on floor plans (not the map). This endpoint is also reused as the data source for the Phase 3 GW-04 "Pick on map" feature in plan 05-08 (the same `/api/map/data` payload feeds the gateway-create dialog's modal Leaflet picker).

Output: One sqlc query file, one `internal/map/` package (Go pkg name `mapapi` — `map` is a Go keyword), HTTP handler + routes registration. Frontend lands in plan 05-08.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-UI-SPEC.md
@.planning/phases/04-realtime-dashboard/04-CONTEXT.md
@internal/db/queries/dashboard.sql
@internal/db/queries/sites.sql
@internal/db/queries/gateways.sql
@internal/db/migrations/0018_gateway.up.sql
@internal/db/migrations/0023_device_profile_expected_interval.up.sql

<interfaces>
<!-- Existing schema this plan reads from -->
```
site (id, name, latitude, longitude, created_at, updated_at, archived_at)
gateway (id, name, latitude, longitude, last_seen_at, ...)
metering_point (id, site_id, name, utility_class, ...)
device (id, metering_point_id, device_profile_id, last_seen_at, decommissioned_at, ...)
device_profile (id, expected_interval_s, ...)
measurement_hourly (bucket, metering_point_id, cumulative_delta, ...)
install_identity (id=1, capabilities TEXT, timezone TEXT, ...)
```

<!-- Response JSON schema produced by this handler -->
```json
{
  "sites": [
    {
      "id": "uuid",
      "name": "string",
      "lat": 13.7563,
      "lng": 100.5018,
      "mp_count": 0,
      "online_count": 0,
      "offline_count": 0,
      "today_consumption": { "water": 1.234, "electricity": 45.6 }
    }
  ],
  "gateways": [
    {
      "id": "uuid",
      "name": "string",
      "lat": 13.7563,
      "lng": 100.5018,
      "online": true
    }
  ]
}
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: sqlc queries + handler + capability-gated assembly</name>
  <read_first>
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md §D-12 §D-15
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-UI-SPEC.md §Map-Specific Contracts
    - internal/db/queries/dashboard.sql (existing online/offline pattern — `last_seen_at > now() - 2 × expected_interval_s × INTERVAL '1 second'`)
    - internal/db/queries/sites.sql (existing site list query for shape reference)
    - internal/db/migrations/0022_install_capabilities.up.sql (capabilities column semantics)
  </read_first>
  <behavior>
    - Test 1: Site without lat/lng is omitted from the response (cannot plot)
    - Test 2: Site with 5 MPs across 3 online + 2 offline returns mp_count=5, online_count=3, offline_count=2
    - Test 3: today_consumption.water summed from measurement_hourly for MPs with utility_class='water' between install-tz midnight today and now
    - Test 4: install_identity.capabilities='water' → today_consumption has water key only, no electricity key
    - Test 5: Gateway with last_seen_at within 2 × expected_interval_s window → online=true
    - Test 6: MAP-04 invariant: response payload does NOT contain any external tile URL or API key — tile URL is frontend-only (plan 05-08)
    - Test 7: Unauthenticated request → 401
  </behavior>
  <action>
**Step A — `internal/db/queries/map.sql`:**

```sql
-- name: ListSitesForMap :many
-- Sites with both lat AND lng populated; archived sites excluded.
-- Returns per-site rollups: MP count, online vs offline device count (per
-- Phase 4 D-07 rule using device_profile.expected_interval_s).
SELECT
  s.id,
  s.name,
  s.latitude,
  s.longitude,
  count(mp.id) FILTER (WHERE mp.archived_at IS NULL)                                                   AS mp_count,
  count(d.id) FILTER (
    WHERE d.decommissioned_at IS NULL
      AND d.last_seen_at > now() - (2 * dp.expected_interval_s * INTERVAL '1 second')
  )                                                                                                    AS online_count,
  count(d.id) FILTER (
    WHERE d.decommissioned_at IS NULL
      AND (d.last_seen_at IS NULL OR d.last_seen_at <= now() - (2 * dp.expected_interval_s * INTERVAL '1 second'))
  )                                                                                                    AS offline_count
FROM site s
LEFT JOIN metering_point mp ON mp.site_id = s.id AND mp.archived_at IS NULL
LEFT JOIN device d ON d.metering_point_id = mp.id AND d.decommissioned_at IS NULL
LEFT JOIN device_profile dp ON dp.id = d.device_profile_id
WHERE s.archived_at IS NULL
  AND s.latitude IS NOT NULL
  AND s.longitude IS NOT NULL
GROUP BY s.id, s.name, s.latitude, s.longitude
ORDER BY s.name;

-- name: ListGatewaysForMap :many
-- Gateways with lat/lng populated, excluding archived.
-- online = last_seen_at within 2 × gateway-class threshold. Gateways don't have
-- a per-vendor expected_interval_s — use a fixed 5-minute health threshold
-- (matches Phase 3 GW-01 spec; can be tuned in Settings later).
SELECT
  g.id,
  g.name,
  g.latitude,
  g.longitude,
  (g.last_seen_at IS NOT NULL AND g.last_seen_at > now() - INTERVAL '5 minutes') AS online
FROM gateway g
WHERE g.archived_at IS NULL
  AND g.latitude IS NOT NULL
  AND g.longitude IS NOT NULL
ORDER BY g.name;

-- name: TodaySiteConsumption :many
-- Per-site today (install_tz 00:00 → now) consumption split by utility_class.
-- Sources measurement_hourly (CAGG) — sums bucketed deltas for MPs belonging
-- to each site, partitioned by utility_class.
SELECT
  mp.site_id,
  mp.utility_class,
  sum(mh.cumulative_delta) AS consumption
FROM measurement_hourly mh
JOIN metering_point mp ON mp.id = mh.metering_point_id
WHERE mh.bucket >= $1                       -- install_tz midnight today (computed in handler)
  AND mh.bucket <  now()
  AND mp.archived_at IS NULL
GROUP BY mp.site_id, mp.utility_class;
```

Run `just sqlc` to regen `internal/db/sqlc/map.sql.go`.

**Step B — `internal/map/doc.go`:**

```go
// Package mapapi serves the map view's data layer (Phase 5 MAP-01..04).
//
// Endpoint: GET /api/map/data
//
// Response: { sites: [...], gateways: [...] } with all coordinates as IEEE-754
// floats. Only sites/gateways with both latitude AND longitude populated are
// returned — a marker without coordinates cannot be plotted.
//
// # Capability gating (Phase 4 D-09)
//
// install_identity.capabilities ∈ {water, electricity, both}. The handler
// includes only matching keys in today_consumption. A single-capability install
// (capabilities='water') returns {"water": x} with no electricity key — the
// frontend uses key-presence as the signal to hide the missing-capability tile.
//
// # MAP-04 invariant
//
// No external tile URL or API key appears in this response. The OSM tile URL
// is hard-coded in the frontend MapView component (plan 05-08). This package
// is a pure JSON-over-HTTP source for site/gateway markers.
//
// # GW-04 reuse
//
// The Phase 3 "Pick on map" gateway-create feature (plan 05-08 frontend) reuses
// this endpoint as the data source for the modal Leaflet picker — no separate
// `/api/gateways/picker-data` endpoint needed.
//
// Go pkg name is `mapapi` because `map` is a Go keyword. Directory is
// `internal/map/` for path consistency.
package mapapi
```

**Step C — `internal/map/handler.go`:**

```go
package mapapi

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5/pgtype"

    sqlc "shifter/internal/db/sqlc"
    "shifter/internal/install"
)

type Deps struct {
    Queries  *sqlc.Queries
    Identity install.IdentityProvider  // capabilities + timezone access
}

type SiteMarker struct {
    ID               uuid.UUID          `json:"id"`
    Name             string             `json:"name"`
    Lat              float64            `json:"lat"`
    Lng              float64            `json:"lng"`
    MPCount          int                `json:"mp_count"`
    OnlineCount      int                `json:"online_count"`
    OfflineCount     int                `json:"offline_count"`
    TodayConsumption map[string]float64 `json:"today_consumption"` // keys subset of {water, electricity}
}

type GatewayMarker struct {
    ID     uuid.UUID `json:"id"`
    Name   string    `json:"name"`
    Lat    float64   `json:"lat"`
    Lng    float64   `json:"lng"`
    Online bool      `json:"online"`
}

type Response struct {
    Sites    []SiteMarker    `json:"sites"`
    Gateways []GatewayMarker `json:"gateways"`
}

func DataHandler(deps Deps) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        identity, err := deps.Identity.Load(r.Context())
        if err != nil {
            writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "identity_load_failed"})
            return
        }

        tz, err := time.LoadLocation(identity.Timezone)
        if err != nil { tz = time.UTC }

        // install_tz midnight today
        now := time.Now().In(tz)
        midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, tz)

        sites, err := deps.Queries.ListSitesForMap(r.Context())
        if err != nil { /* 500 */ return }

        gateways, err := deps.Queries.ListGatewaysForMap(r.Context())
        if err != nil { /* 500 */ return }

        consumptionRows, err := deps.Queries.TodaySiteConsumption(r.Context(), pgtype.Timestamptz{Time: midnight, Valid: true})
        if err != nil { /* 500 */ return }

        // Build today_consumption map per site, gated by capabilities.
        consumptionBySite := map[uuid.UUID]map[string]float64{}
        for _, row := range consumptionRows {
            siteID := uuid.UUID(row.SiteID.Bytes)
            if _, ok := consumptionBySite[siteID]; !ok { consumptionBySite[siteID] = map[string]float64{} }
            if !capabilityIncludes(identity.Capabilities, row.UtilityClass) { continue }
            value, _ := row.Consumption.Float64Value()
            consumptionBySite[siteID][row.UtilityClass] = value.Float64
        }

        siteMarkers := make([]SiteMarker, 0, len(sites))
        for _, s := range sites {
            lat, _ := s.Latitude.Float64Value()
            lng, _ := s.Longitude.Float64Value()
            siteMarkers = append(siteMarkers, SiteMarker{
                ID: uuid.UUID(s.ID.Bytes), Name: s.Name,
                Lat: lat.Float64, Lng: lng.Float64,
                MPCount: int(s.MpCount), OnlineCount: int(s.OnlineCount), OfflineCount: int(s.OfflineCount),
                TodayConsumption: consumptionBySite[uuid.UUID(s.ID.Bytes)],  // nil for sites with no data; JSON marshals as `null` — handler converts to `{}`
            })
        }

        gatewayMarkers := make([]GatewayMarker, 0, len(gateways))
        for _, g := range gateways {
            lat, _ := g.Latitude.Float64Value()
            lng, _ := g.Longitude.Float64Value()
            gatewayMarkers = append(gatewayMarkers, GatewayMarker{
                ID: uuid.UUID(g.ID.Bytes), Name: g.Name,
                Lat: lat.Float64, Lng: lng.Float64,
                Online: g.Online,
            })
        }

        // Normalize today_consumption to {} when nil so the frontend can iterate without null checks.
        for i := range siteMarkers {
            if siteMarkers[i].TodayConsumption == nil {
                siteMarkers[i].TodayConsumption = map[string]float64{}
            }
        }

        writeJSON(w, http.StatusOK, Response{Sites: siteMarkers, Gateways: gatewayMarkers})
    }
}

func capabilityIncludes(installCaps, utilityClass string) bool {
    switch installCaps {
    case "both": return true
    case "water": return utilityClass == "water"
    case "electricity": return utilityClass == "electricity"
    default: return false
    }
}

func writeJSON(w http.ResponseWriter, status int, body any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(body)
}
```

**Step D — `internal/map/routes.go`:**

```go
package mapapi

import "github.com/go-chi/chi/v5"

// RegisterRoutes mounts the map data endpoint under the authenticated group.
// Caller passes an already-auth-gated chi router (mirrors internal/dashboard
// and internal/events package patterns).
func RegisterRoutes(r chi.Router, deps Deps) {
    r.Get("/api/map/data", DataHandler(deps))
}
```

**Step E — Replace `t.Skip` in `internal/map/handler_test.go`:**

```go
package mapapi_test

func TestMapData(t *testing.T) {
    // Seed: 2 sites with lat/lng + 1 site without coords (excluded), 3 MPs on one
    // site (1 water online + 1 water offline + 1 electricity online), 1 gateway
    // with recent last_seen (online) + 1 gateway with stale last_seen (offline).
    // Insert measurement_hourly rows after install_tz midnight summing to
    // expected per-utility-class consumption.

    // POST seed data via test helpers, refresh measurement_hourly, then GET /api/map/data
    // and assert response shape:
    // - sites array len == 2 (no-coords site excluded)
    // - mp_count == 3, online_count == 2, offline_count == 1 for the populated site
    // - today_consumption has water + electricity keys (default capabilities='both')
    // - gateways array len == 2 with one online == true, one online == false

    require.Len(t, resp.Sites, 2)
    require.Equal(t, 3, resp.Sites[0].MPCount)
    require.Equal(t, 2, resp.Sites[0].OnlineCount)
    require.Equal(t, 1, resp.Sites[0].OfflineCount)
    require.Contains(t, resp.Sites[0].TodayConsumption, "water")
    require.Contains(t, resp.Sites[0].TodayConsumption, "electricity")
}

func TestMapData_CapabilityWaterOnly(t *testing.T) {
    // Set install_identity.capabilities = 'water'; seed water + electricity MPs.
    // GET /api/map/data
    // Assert today_consumption has 'water' key only, NO 'electricity' key.
    require.Contains(t, resp.Sites[0].TodayConsumption, "water")
    require.NotContains(t, resp.Sites[0].TodayConsumption, "electricity")
}

func TestMapData_Unauthenticated_401(t *testing.T) {
    // Issue GET without session cookie → 401.
}

func TestOSMTileURL(t *testing.T) {
    // Static / grep test: response body MUST NOT contain any tile URL or API key.
    // Read /api/map/data response body and assert it does not contain "tile.openstreetmap.org"
    // (tile URL belongs in the frontend, not the API).
    require.NotContains(t, body, "tile.openstreetmap.org")
    require.NotContains(t, body, "api_key")
    require.NotContains(t, body, "mapbox")
}
```
  </action>
  <verify>
    <automated>just sqlc &amp;&amp; go test ./internal/map/... -race -count=1 -short -run "TestMapData|TestOSMTileURL"</automated>
  </verify>
  <acceptance_criteria>
    - `internal/db/queries/map.sql` exists with `-- name: ListSitesForMap :many` and `-- name: ListGatewaysForMap :many` and `-- name: TodaySiteConsumption :many`
    - `internal/db/queries/map.sql` ListSitesForMap WHERE clause contains `s.latitude IS NOT NULL` and `s.longitude IS NOT NULL`
    - `internal/db/queries/map.sql` uses Phase 4 D-07 online rule: `last_seen_at > now() - (2 * dp.expected_interval_s * INTERVAL '1 second')`
    - `internal/db/sqlc/map.sql.go` exists (sqlc-generated)
    - `internal/map/handler.go` package declaration is `package mapapi`
    - `internal/map/handler.go` exports `DataHandler`, `Deps`, `SiteMarker`, `GatewayMarker`, `Response`
    - `internal/map/handler.go` contains literal `capabilityIncludes` function with switch on `both / water / electricity`
    - `internal/map/routes.go` mounts `/api/map/data` via `r.Get`
    - `internal/map/handler_test.go` package is `mapapi_test` or `mapapi`; contains at minimum 4 named tests: TestMapData, TestMapData_CapabilityWaterOnly, TestMapData_Unauthenticated_401, TestOSMTileURL
    - `TestOSMTileURL` body asserts the response does NOT contain `tile.openstreetmap.org`, `api_key`, or `mapbox` strings
    - `go test ./internal/map/... -short` exits 0
  </acceptance_criteria>
  <done>Map endpoint returns capability-gated site + gateway markers; OSM-only invariant verified by grep test.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Client → /api/map/data | Read-only authenticated endpoint; no input parameters; no injection surface |
| Handler → install_identity | Capability flag loaded server-side; cannot be overridden by client |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-05-04-01 | Information Disclosure | Map data exposes site lat/lng to unauthenticated callers | medium | mitigate | Handler mounted inside the authenticated chi router group (same pattern as dashboard); test `TestMapData_Unauthenticated_401` pins the 401 response |
| T-05-04-02 | Information Disclosure | Embedded tile URL or API key leaks into response | medium | mitigate | Handler response is pure JSON of site/gateway data; tile URL is frontend-only. Test `TestOSMTileURL` greps response bytes for any tile/api-key string (none allowed) |
</threat_model>

<verification>
1. `just sqlc` regenerates `internal/db/sqlc/map.sql.go` cleanly
2. `go test ./internal/map/... -race -count=1 -short` exits 0
3. Response body grep does NOT match `tile.openstreetmap.org` or `api_key`
4. `grep -E "FROM measurement\\b" internal/db/queries/map.sql` returns empty (only `measurement_hourly`)
</verification>

<success_criteria>
- 3 sqlc queries: ListSitesForMap, ListGatewaysForMap, TodaySiteConsumption
- Handler returns JSON with capability-gated today_consumption
- Single-capability install hides the missing-capability key
- Tests verify per-site rollups, gateway online flag, capability filtering, OSM-only invariant
</success_criteria>

<output>
After completion, create `.planning/phases/05-aggregates-reports-map-floor-plans/05-04-SUMMARY.md` recording:
- Whether install.IdentityProvider needed extending (or was already available)
- Number of test fixtures seeded for the map data tests
- Confirmation that response shape matches plan 05-08 frontend expectations
</output>
