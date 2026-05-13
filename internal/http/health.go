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
//                           Phase 6 extends with alert_worker array + last_backup.
//                           Splitting at the action boundary lets operator
//                           monitoring use /health while internal observability
//                           uses /health/detailed.
//
// Threat model:
//   - T-18-01 (Information Disclosure on /health): mitigated by minimal public
//     payload — DB / CS / MQTT internals NEVER leak via /health.
package http

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/doctor"
	"github.com/shifter-io/shifter/internal/version"
)

// startedAt is captured at process start for the uptime_seconds field. It is
// a package-level var (not const) so test code can override via
// ResetStartedAtForTest if needed.
var startedAt = time.Now()

// AlertWorkerHealth is one entry in the alert_workers array returned by
// /health/detailed (D-21 / Plan 06-11). One row per worker_kind seeded by
// migration 0042.
type AlertWorkerHealth struct {
	Kind           string    `json:"kind"`
	LastRunAt      time.Time `json:"last_run_at"`
	RulesEvaluated int       `json:"rules_evaluated"`
	FiresEmitted   int       `json:"fires_emitted"`
	Cleared        int       `json:"cleared"`
	DurationMs     int       `json:"duration_ms"`
	Degraded       bool      `json:"degraded"`
	LastError      string    `json:"last_error,omitempty"`
}

// LastBackupHealth summarises the most-recent backup_run row for
// /health/detailed (D-50 / Plan 06-11). nil means no backup has run yet.
type LastBackupHealth struct {
	FileName   string    `json:"file_name"`
	StartedAt  time.Time `json:"started_at"`
	AgeSeconds int64     `json:"age_seconds"`
	Status     string    `json:"status"`
	SHA256     string    `json:"sha256"`
}

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

// HealthDetailed returns the ADMIN /health/detailed handler (D-19 / Plan 06-11).
//
// Extended payload shape (Plan 06-11 additions):
//   - alert_workers: []AlertWorkerHealth — one row per worker_kind from
//     alert_worker_state (seeded by 0042); Plan 06-04 degraded flag powers
//     the shell banner.
//   - last_backup: *LastBackupHealth — most-recent backup_run row; nil when
//     no backup has run. Age check against backup_crit_threshold_hours (from
//     retention_config) drives the degraded status.
//
// Plan 07-14 addition:
//   - probe_results: map[string]doctor.ProbeResult — last-run-only probe
//     results for chirpstack, timescale, and region (D-40). Each probe has a
//     5s timeout; total worst-case 15s (T-07-14-02 accepted: admin-only, low
//     traffic). The ChirpStack API key is never included in probe messages
//     (T-07-14-03).
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
	return HealthDetailedWithCS(pool, "", "")
}

// HealthDetailedWithCS is the full constructor that accepts ChirpStack
// connection details for the probe_results block. When grpcURL is empty the
// chirpstack probe is skipped (returns an "unconfigured" result).
func HealthDetailedWithCS(pool *pgxpool.Pool, grpcURL, apiKey string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		dbOK := pool.Ping(ctx) == nil
		overallStatus := "ok"
		if !dbOK {
			overallStatus = "degraded"
		}

		// --- Alert worker states (D-21) ---
		alertWorkers := loadAlertWorkers(ctx, pool)
		for _, aw := range alertWorkers {
			if aw.Degraded {
				overallStatus = "degraded"
			}
		}

		// --- Last backup freshness (D-50) ---
		lastBackup, critThreshHours := loadLastBackupHealth(ctx, pool)
		if lastBackup != nil && critThreshHours > 0 {
			critThreshSecs := int64(critThreshHours) * 3600
			if lastBackup.AgeSeconds > critThreshSecs {
				overallStatus = "degraded"
			}
		}

		// --- Install validation probes (Plan 07-14 / D-40) ---
		// Each probe runs with a 5s timeout; last-run-only (no history).
		probeResults := runProbes(ctx, pool, grpcURL, apiKey)

		writeJSON(w, http.StatusOK, map[string]any{
			"status":         overallStatus,
			"checks":         map[string]any{"db": dbOK},
			"version":        version.Info(),
			"uptime_seconds": int(time.Since(startedAt).Seconds()),
			"alert_workers":  alertWorkers,
			"last_backup":    lastBackup,
			"probe_results":  probeResults,
		})
	}
}

// runProbes executes the three install probes sequentially with 5s timeouts
// each and returns the results keyed by probe name.
func runProbes(ctx context.Context, pool *pgxpool.Pool, grpcURL, apiKey string) map[string]doctor.ProbeResult {
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	results := make(map[string]doctor.ProbeResult, 3)

	// chirpstack probe.
	if grpcURL != "" {
		csCtx, csCancel := context.WithTimeout(probeCtx, 5*time.Second)
		r := doctor.ProbeChirpStack(csCtx, grpcURL, apiKey)
		csCancel()
		results["chirpstack"] = r
	} else {
		results["chirpstack"] = doctor.ProbeResult{
			Name:    "chirpstack",
			Status:  "warn",
			Message: "ChirpStack gRPC URL not configured",
		}
	}

	// timescale probe.
	tsCtx, tsCancel := context.WithTimeout(probeCtx, 5*time.Second)
	results["timescale"] = doctor.ProbeTimescale(tsCtx, pool)
	tsCancel()

	// region probe.
	rgCtx, rgCancel := context.WithTimeout(probeCtx, 5*time.Second)
	results["region"] = doctor.ProbeRegion(rgCtx, pool)
	rgCancel()

	return results
}

// loadAlertWorkers queries all alert_worker_state rows ordered by worker_kind
// and returns them as AlertWorkerHealth values. Returns an empty non-nil slice
// on any error (graceful degradation — /health/detailed should not fail just
// because the alert_worker_state query fails).
func loadAlertWorkers(ctx context.Context, pool *pgxpool.Pool) []AlertWorkerHealth {
	rows, err := pool.Query(ctx, `
		SELECT worker_kind, last_run_at, rules_evaluated, fires_emitted,
		       cleared, duration_ms, degraded, COALESCE(last_error, '')
		FROM alert_worker_state
		ORDER BY worker_kind`)
	if err != nil {
		return []AlertWorkerHealth{}
	}
	defer rows.Close()

	var out []AlertWorkerHealth
	for rows.Next() {
		var aw AlertWorkerHealth
		if err := rows.Scan(
			&aw.Kind, &aw.LastRunAt, &aw.RulesEvaluated, &aw.FiresEmitted,
			&aw.Cleared, &aw.DurationMs, &aw.Degraded, &aw.LastError,
		); err != nil {
			continue
		}
		out = append(out, aw)
	}
	if rows.Err() != nil {
		return []AlertWorkerHealth{}
	}
	if out == nil {
		out = []AlertWorkerHealth{}
	}
	return out
}

// loadLastBackupHealth queries the most-recent backup_run row joined with
// retention_config and returns the summary and crit_threshold_hours for the
// caller's degraded-status logic. Returns nil, 0 when no backup has run or
// on any query error.
func loadLastBackupHealth(ctx context.Context, pool *pgxpool.Pool) (*LastBackupHealth, int) {
	var h LastBackupHealth
	var critThreshHours int
	err := pool.QueryRow(ctx, `
		SELECT
			COALESCE(br.file_name, ''),
			br.started_at,
			EXTRACT(EPOCH FROM (now() - br.started_at))::BIGINT,
			br.status,
			COALESCE(br.sha256, ''),
			COALESCE(rc.backup_crit_threshold_hours, 0)
		FROM backup_run br
		CROSS JOIN retention_config rc
		WHERE rc.id = 1
		ORDER BY br.started_at DESC
		LIMIT 1`,
	).Scan(&h.FileName, &h.StartedAt, &h.AgeSeconds, &h.Status, &h.SHA256, &critThreshHours)
	if err != nil {
		return nil, 0
	}
	return &h, critThreshHours
}

// ResetStartedAtForTest overrides the package-level startedAt sentinel. Tests
// that assert specific uptime_seconds values use this to make assertions
// deterministic without sleeping.
func ResetStartedAtForTest(t time.Time) { startedAt = t }
