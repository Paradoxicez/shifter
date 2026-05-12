// Package mapapi serves the map view's data layer (Phase 5 MAP-01..04).
//
// Endpoint: GET /api/map/data
//
// Response: { sites: [...], gateways: [...] } with all coordinates as IEEE-754
// floats. Only sites/gateways with both lat AND lng populated are returned — a
// marker without coordinates cannot be plotted on the Leaflet map.
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
// # Schema note
//
// site and gateway tables use columns `lat`/`lng` (not latitude/longitude).
// gateway has no `last_seen_at`; gateway online state is derived from
// `stats_refreshed_at` (updated by the cache_refresher goroutine every ~60 s).
//
// Go pkg name is `mapapi` because `map` is a Go keyword. Directory is
// `internal/map/` for path consistency.
package mapapi
