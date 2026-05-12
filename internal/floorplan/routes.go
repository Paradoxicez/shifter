package floorplan

import (
	"github.com/go-chi/chi/v5"

	"github.com/shifter-io/shifter/internal/auth"
)

// RegisterRoutes mounts the floor-plan endpoints on r.
//
// Route table:
//
//	POST   /api/sites/{siteID}/floor-plans          site.create  — admin only
//	GET    /api/sites/{siteID}/floor-plans          site.read    — admin + viewer
//	GET    /api/floor-plans/{id}                    site.read    — admin + viewer
//	PATCH  /api/floor-plans/{id}                    site.update  — admin only (replace image)
//	PATCH  /api/floor-plans/{id}/label              site.update  — admin only (rename)
//	DELETE /api/floor-plans/{id}                    site.archive — admin only
func RegisterRoutes(r chi.Router, deps Deps) {
	// Read-only group — admin + viewer.
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionSiteRead))
		rt.Get("/api/sites/{siteID}/floor-plans", ListBySiteHandler(deps))
		rt.Get("/api/floor-plans/{id}", GetHandler(deps))
	})

	// Mutate group — admin only (site.create for upload, site.update for edits,
	// site.archive for delete — reuses existing site authz actions as floor plans
	// are a sub-resource of site; plan 05-07 may introduce dedicated floor_plan actions).
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionSiteCreate))
		rt.Post("/api/sites/{siteID}/floor-plans", UploadImageHandler(deps))
	})

	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionSiteUpdate))
		rt.Patch("/api/floor-plans/{id}", ReplaceImageHandler(deps))
		rt.Patch("/api/floor-plans/{id}/label", RenameHandler(deps))
	})

	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionSiteArchive))
		rt.Delete("/api/floor-plans/{id}", DeleteHandler(deps))
	})
}
