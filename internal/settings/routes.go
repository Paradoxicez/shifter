package settings

import (
	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"

	"github.com/shifter-io/shifter/internal/auth"
)

// RegisterRoutes mounts the settings endpoints on r.
//
// Route table (plan 05-11):
//
//	GET   /api/settings/retention  — admin + viewer (read)
//	PATCH /api/settings/retention  — admin only (T-05-11-01)
//
// The GET route is mounted inside a viewer-accessible group (ActionConnectionTest
// is used as the "any authenticated user" gate — same pattern as the ChirpStack
// read route). The PATCH route is gated by ActionSettingsUpdate (admin only).
func RegisterRoutes(r chi.Router, deps Deps, sm *scs.SessionManager) {
	// Read: any authenticated user (admin + viewer).
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(sm, auth.ActionConnectionTest))
		rt.Get("/api/settings/retention", GetHandler(deps))
	})

	// Write: admin only (T-05-11-01).
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(sm, auth.ActionSettingsUpdate))
		rt.Patch("/api/settings/retention", PatchHandler(deps, sm))
	})
}
