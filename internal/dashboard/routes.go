package dashboard

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Deps groups every dependency the dashboard handlers need. Constructed in
// cmd/shifter/serve.go and threaded into NewRouter via DashboardDeps.
type Deps struct {
	// Pool is the shared pgxpool used by all dashboard queries.
	Pool *pgxpool.Pool

	// Logger is a *slog.Logger (optionally namespaced with
	// slog.Logger.With("component", "dashboard")).
	Logger *slog.Logger
}

// RegisterRoutes mounts the three dashboard endpoints on the provided chi.Router.
//
// All three routes MUST be called inside the authenticated chi group in
// internal/http/router.go so that both admin and viewer roles can access them
// (D-23) and unauthenticated requests receive 401 from the session middleware
// before reaching any handler (T-04-04-05).
//
// Routes mounted:
//
//	GET /api/dashboard/scope      — D-09 + D-21
//	GET /api/dashboard/snapshot   — KPI tiles + latest readings
//	GET /api/dashboard/timeseries — D-12 time-bucket chart series
func RegisterRoutes(r chi.Router, deps Deps) {
	r.Get("/api/dashboard/scope", deps.handleScope)
	r.Get("/api/dashboard/snapshot", deps.handleSnapshot)
	r.Get("/api/dashboard/timeseries", deps.handleTimeseries)
}
