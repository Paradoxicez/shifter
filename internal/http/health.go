// Package http — health endpoints.
//
// Plan 18 ships the D-18 / D-19 split:
//
//   - GET /health           public, no auth, minimal payload (status, version,
//                           uptime_seconds). Anonymous probes (Caddy upstream
//                           checks, Docker HEALTHCHECK via `shifter healthcheck`,
//                           cloud LB probes) MUST work without a session.
//   - GET /health/detailed  admin-only via auth.RequireAction(sm,
//                           ActionHealthDetailed). Includes DB ping result;
//                           Phase 6 will extend with CS/MQTT/disk/last-uplink
//                           checks. Splitting at the action boundary lets
//                           operator monitoring use /health while internal
//                           observability uses /health/detailed.
//
// Threat model:
//   - T-18-01 (Information Disclosure on /health): mitigated by minimal public
//     payload — DB / CS / MQTT internals NEVER leak via /health.
package http

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/version"
)

// startedAt is captured at process start for the uptime_seconds field. It is
// a package-level var (not const) so test code can override via
// ResetStartedAtForTest if needed.
var startedAt = time.Now()

// Health returns the PUBLIC /health handler (D-18). No auth; minimal payload
// so anonymous probes cannot enumerate install internals.
//
// Body shape:
//
//	{ "status": "ok", "version": {...build info...}, "uptime_seconds": 1234 }
//
// status is always "ok" — when the binary is unable to serve this endpoint
// the request never reaches the handler in the first place. Detailed health
// (DB, CS, MQTT) lives behind /health/detailed.
func Health() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":         "ok",
			"version":        version.Info(),
			"uptime_seconds": int(time.Since(startedAt).Seconds()),
		})
	}
}

// HealthDetailed returns the ADMIN /health/detailed handler (D-19).
//
// Phase 1 minimum: DB ping + version + uptime. CS/MQTT/disk/last-uplink-age
// move to Phase 6 (operations dashboard surface).
//
// Status semantics:
//   - "ok"        — every check passed.
//   - "degraded"  — at least one check failed; the binary is still serving
//     traffic but operators should investigate.
//
// The handler is wrapped at the chi router by
// auth.RequireAction(sm, ActionHealthDetailed); the handler itself does NOT
// re-check authorization (the middleware already did).
func HealthDetailed(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dbOK := pool.Ping(r.Context()) == nil
		status := "ok"
		if !dbOK {
			status = "degraded"
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":         status,
			"checks":         map[string]any{"db": dbOK},
			"version":        version.Info(),
			"uptime_seconds": int(time.Since(startedAt).Seconds()),
		})
	}
}

// ResetStartedAtForTest overrides the package-level startedAt sentinel. Tests
// that assert specific uptime_seconds values use this to make assertions
// deterministic without sleeping.
func ResetStartedAtForTest(t time.Time) { startedAt = t }
