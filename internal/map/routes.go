package mapapi

import (
	"github.com/go-chi/chi/v5"

	"github.com/shifter-io/shifter/internal/auth"
)

// RegisterRoutes mounts the map data endpoint under an authenticated group.
//
// T-05-04-01 mitigation: the route is gated by auth.RequireAction(ActionSiteRead)
// so unauthenticated callers receive 401 before any DB query runs. Both admin
// and viewer roles carry ActionSiteRead, matching the plan requirement that
// GET /api/map/data is accessible to any authenticated user.
//
// Caller pattern mirrors internal/dashboard and internal/events: the top-level
// router in internal/http/router.go registers a MapDeps nil-guard and calls
// RegisterRoutes when deps are available.
func RegisterRoutes(r chi.Router, deps Deps) {
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionSiteRead))
		rt.Get("/api/map/data", DataHandler(deps))
	})
}
