package dashboard

import (
	"encoding/json"
	"net/http"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// handleSnapshot serves GET /api/dashboard/snapshot.
//
// It reads install_identity.timezone + capabilities from DB, calls BuildSnapshot
// which assembles KPI tiles + latest readings, and returns JSON.
//
// D-23: mounted under authenticated group (any role).
// T-04-04-04: timezone comes from install_identity, never from the request.
func (d Deps) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := sqlc.New(d.Pool)

	// Read install_identity for timezone + capabilities (server-side only).
	identity, err := q.GetInstallIdentity(ctx)
	if err != nil {
		d.Logger.ErrorContext(ctx, "snapshot: get install identity", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	snap, err := BuildSnapshot(ctx, q, identity.Timezone, identity.Capabilities)
	if err != nil {
		d.Logger.ErrorContext(ctx, "snapshot: build snapshot", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(snap); err != nil {
		d.Logger.ErrorContext(ctx, "snapshot: encode response", "err", err)
	}
}
