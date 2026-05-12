package floorplan

import (
	"github.com/go-chi/chi/v5"

	"github.com/shifter-io/shifter/internal/auth"
)

// RegisterRoutes mounts the floor-plan endpoints on r.
//
// Route table (plan 05-05 + plan 05-07):
//
//	POST   /api/sites/{siteID}/floor-plans              site.create  — admin only
//	GET    /api/sites/{siteID}/floor-plans              site.read    — admin + viewer
//	GET    /api/floor-plans/{id}                        site.read    — admin + viewer
//	GET    /api/floor-plans/{id}/image                  site.read    — admin + viewer (auth-gated image serve)
//	GET    /api/floor-plans/{id}/placements             site.read    — admin + viewer
//	PATCH  /api/floor-plans/{id}                        site.update  — admin only (replace image)
//	PATCH  /api/floor-plans/{id}/label                  site.update  — admin only (rename)
//	PATCH  /api/floor-plans/{id}/placements/{deviceID}  site.update  — admin only (drag-to-nudge)
//	DELETE /api/floor-plans/{id}                        site.archive — admin only
//	DELETE /api/floor-plans/{id}/placements/{deviceID}  site.archive — admin only (remove pin)
//	POST   /api/floor-plans/{id}/placements             site.create  — admin only (pin device)
func RegisterRoutes(r chi.Router, deps Deps) {
	// Read-only group — admin + viewer.
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionSiteRead))
		rt.Get("/api/sites/{siteID}/floor-plans", ListBySiteHandler(deps))
		rt.Get("/api/floor-plans/{id}", GetHandler(deps))
		rt.Get("/api/floor-plans/{id}/placements", ListPlacementsHandler(deps))
		rt.Get("/api/floor-plans/{id}/image", ServeImageHandler(deps))
	})

	// Mutate group — admin only (site.create for upload + pin).
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionSiteCreate))
		rt.Post("/api/sites/{siteID}/floor-plans", UploadImageHandler(deps))
		rt.Post("/api/floor-plans/{id}/placements", UpsertPlacementHandler(deps))
	})

	// Mutate group — admin only (site.update for replace image, rename, nudge).
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionSiteUpdate))
		rt.Patch("/api/floor-plans/{id}", ReplaceImageHandler(deps))
		rt.Patch("/api/floor-plans/{id}/label", RenameHandler(deps))
		rt.Patch("/api/floor-plans/{id}/placements/{deviceID}", UpdatePlacementHandler(deps))
	})

	// Archive group — admin only (site.archive for delete plan + remove pin).
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionSiteArchive))
		rt.Delete("/api/floor-plans/{id}", DeleteHandler(deps))
		rt.Delete("/api/floor-plans/{id}/placements/{deviceID}", DeletePlacementHandler(deps))
	})
}
