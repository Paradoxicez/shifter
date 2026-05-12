package settings

import (
	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/backup"
)

// RegisterRoutes mounts the settings endpoints on r.
//
// Route table (plan 05-11 + plan 06-10):
//
//	GET   /api/settings/retention             — admin + viewer (read)
//	PATCH /api/settings/retention             — admin only (T-05-11-01)
//	GET   /api/settings/backup                — admin + viewer (ActionBackupRead)
//	PATCH /api/settings/backup/thresholds     — admin only (ActionBackupConfigure)
//
// The GET retention route is mounted inside a viewer-accessible group
// (ActionConnectionTest is used as the "any authenticated user" gate — same
// pattern as the ChirpStack read route). The PATCH retention route is gated
// by ActionSettingsUpdate (admin only).
//
// Backup routes delegate to RegisterBackupRoutes in backup_card.go and
// require a non-nil BackupStore + BackupCfg — the router nil-guards these
// before calling RegisterRoutes.
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

// RegisterRoutesWithBackup mounts all settings routes including the Plan 06-10
// backup status + threshold routes. Called from internal/http/router.go when
// BackupDeps is non-nil.
func RegisterRoutesWithBackup(r chi.Router, deps Deps, sm *scs.SessionManager, store *backup.Store, cfg BackupCardConfig) {
	RegisterRoutes(r, deps, sm)
	RegisterBackupRoutes(r, deps, sm, store, cfg)
}
