package install

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgxpool"
)

// FirstRunGate is the canonical D-08 middleware: presence of an admin user
// is the install-completion signal (never a flag file or env var).
//
// Behavior:
//   - whitelisted path → pass-through unconditionally
//   - admin exists (cached or just-checked) → pass-through
//   - no admin yet, /api/*  → 409 + {"error":"install_required"}
//   - no admin yet, anything else → 307 redirect to /install
//
// PITFALL #10 mitigation: once `adminExists` returns true, the result is
// stored in an atomic.Bool and ALL subsequent requests skip the DB query.
// The boolean is one-way (never resets) — even if every admin row is later
// soft-deleted, the install has demonstrably been completed and the gate
// must not re-engage. This keeps the hot-path overhead at one atomic load.
//
// Plan 18 wires this gate as the FIRST middleware after RequestID/Logger.
func FirstRunGate(pool *pgxpool.Pool, log *slog.Logger) func(http.Handler) http.Handler {
	var adminExistsCache atomic.Bool

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isWhitelisted(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			if adminExistsCache.Load() {
				next.ServeHTTP(w, r)
				return
			}

			exists, err := adminExists(r.Context(), pool)
			if err != nil {
				if log != nil {
					log.Error("first-run gate: db error", "err", err, "path", r.URL.Path)
				}
				http.Error(w, "bootstrap check failed", http.StatusInternalServerError)
				return
			}

			if exists {
				adminExistsCache.Store(true)
				next.ServeHTTP(w, r)
				return
			}

			// No admin yet. Branch on API vs HTML.
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "install_required"})
				return
			}
			http.Redirect(w, r, "/install", http.StatusTemporaryRedirect)
		})
	}
}

// adminExists is the single SQL EXISTS check the gate runs at most once per
// process lifetime (cache holds for subsequent requests). Filters disabled
// rows so a soft-deleted admin doesn't satisfy the install gate — the
// operator must re-bootstrap via `shifter create-admin --reset` (Plan 09).
func adminExists(ctx context.Context, pool *pgxpool.Pool) (bool, error) {
	var v bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM "user" WHERE role = 'admin' AND disabled_at IS NULL)`,
	).Scan(&v)
	return v, err
}

// isWhitelisted returns true for paths that MUST serve regardless of install
// state. The list is intentionally small: anything operator-facing should
// either be a wizard route or a static asset of the wizard / login screen.
//
// Order is irrelevant (string equality / prefix / suffix), but the buckets
// are arranged for readability:
//  1. Wizard endpoints (/install, /api/install/*)
//  2. Login route (avoids redirect loop with /install — login screen ships
//     in Plan 23 but the path is reserved here so post-install logout
//     doesn't bounce against the gate)
//  3. Public health (/health) — D-18; monitoring must never depend on auth
//  4. SPA static assets — Vite's /assets/ build output prefix
//  5. Common static suffixes — favicons, font files, root-level images
func isWhitelisted(path string) bool {
	switch {
	case path == "/install" || strings.HasPrefix(path, "/install/"):
		return true
	case path == "/api/install" || strings.HasPrefix(path, "/api/install/"):
		return true
	case path == "/login":
		return true
	case path == "/health":
		return true
	case strings.HasPrefix(path, "/assets/"):
		return true
	}
	for _, suf := range []string{".svg", ".ico", ".png", ".woff2", ".woff", ".css", ".js", ".map"} {
		if strings.HasSuffix(path, suf) {
			return true
		}
	}
	return false
}
