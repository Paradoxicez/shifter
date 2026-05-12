// Package settings — install identity GET + PATCH handlers (Plan 06-12 / SETT-02).
//
// GET  /api/settings/identity  — admin + viewer (ActionConnectionTest)
// PATCH /api/settings/identity — admin only (ActionSettingsIdentityUpdate)
//
// PATCH follows the load-merge-validate pattern from backup_card.go:
//  1. Load current install_identity row.
//  2. Merge supplied nullable fields over the current values.
//  3. Validate merged result.
//  4. UpsertInstallIdentity inside Serializable tx + audit.WriteEntry.
//
// The GET response shape matches the existing InstallIdentityCard.tsx interface:
//
//	{ install_id, site_name, display_name, address, timezone, units, version }
//
// install_id returns the integer id=1 as a string.
// version returns the build version from the version package.
package settings

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/shifter-io/shifter/internal/audit"
	"github.com/shifter-io/shifter/internal/auth"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/version"
)

// IdentityResponse is the wire shape for GET /api/settings/identity.
// Field names are locked by InstallIdentityCard.tsx — do not rename.
type IdentityResponse struct {
	InstallID   string  `json:"install_id"`
	SiteName    string  `json:"site_name"`
	DisplayName string  `json:"display_name"`
	Address     *string `json:"address,omitempty"`
	Timezone    string  `json:"timezone"`
	Units       string  `json:"units"`
	Version     string  `json:"version"`
}

// IdentityPatch is the wire shape for PATCH /api/settings/identity.
// All fields are optional — omitted fields retain their current value.
type IdentityPatch struct {
	DisplayName *string `json:"display_name"`
	Address     *string `json:"address"`
	Timezone    *string `json:"timezone"`
	Units       *string `json:"units"`
}

// RegisterIdentityRoutes mounts the identity endpoints on r.
//
//	GET   /api/settings/identity — ActionConnectionTest (admin + viewer)
//	PATCH /api/settings/identity — ActionSettingsIdentityUpdate (admin only)
func RegisterIdentityRoutes(r chi.Router, deps Deps, sm *scs.SessionManager) {
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(sm, auth.ActionConnectionTest))
		rt.Get("/api/settings/identity", GetIdentityHandler(deps))
	})
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(sm, auth.ActionSettingsIdentityUpdate))
		rt.Patch("/api/settings/identity", PatchIdentityHandler(deps, sm))
	})
}

// GetIdentityHandler serves GET /api/settings/identity.
// Accessible to any authenticated user (admin + viewer).
func GetIdentityHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		row, err := deps.Queries.GetInstallIdentity(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "load_failed")
			return
		}
		resp := IdentityResponse{
			InstallID:   strconv.Itoa(int(row.ID)),
			SiteName:    row.DisplayName,
			DisplayName: row.DisplayName,
			Address:     row.Address,
			Timezone:    row.Timezone,
			Units:       string(row.Units),
			Version:     version.Version,
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// PatchIdentityHandler serves PATCH /api/settings/identity.
// Admin-only (ActionSettingsIdentityUpdate gate in RegisterIdentityRoutes).
// Follows load-merge-validate → Serializable tx → UpsertInstallIdentity + audit.WriteEntry.
func PatchIdentityHandler(deps Deps, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var patch IdentityPatch
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json")
			return
		}

		// Load current identity for merge.
		current, err := deps.Queries.GetInstallIdentity(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "load_failed")
			return
		}

		// Merge: apply supplied fields over current values.
		merged := sqlc.UpsertInstallIdentityParams{
			DisplayName: current.DisplayName,
			LogoPath:    current.LogoPath,
			Address:     current.Address,
			Timezone:    current.Timezone,
			Units:       current.Units,
		}
		if patch.DisplayName != nil {
			merged.DisplayName = *patch.DisplayName
		}
		if patch.Address != nil {
			merged.Address = patch.Address
		}
		if patch.Timezone != nil {
			merged.Timezone = *patch.Timezone
		}
		if patch.Units != nil {
			u := sqlc.UnitsSystem(*patch.Units)
			if u != sqlc.UnitsSystemMetric && u != sqlc.UnitsSystemImperial {
				writeError(w, http.StatusBadRequest, "invalid_units")
				return
			}
			merged.Units = u
		}

		// Validate merged values.
		if len(merged.DisplayName) == 0 || len(merged.DisplayName) > 200 {
			writeError(w, http.StatusBadRequest, "display_name_invalid")
			return
		}
		if len(merged.Timezone) == 0 || len(merged.Timezone) > 64 {
			writeError(w, http.StatusBadRequest, "timezone_invalid")
			return
		}

		// Caller identity for audit row — mirrors backup_card.go PatchBackupThresholdsHandler.
		// auth.GetUser always succeeds here: RequireAction already returned 403 if no user.
		user, ok := auth.GetUser(r.Context(), sm)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		callerID := uuid.MustParse(user.ID)
		reqID := middleware.GetReqID(r.Context())

		// Build before/after for audit diff.
		before := map[string]any{
			"display_name": current.DisplayName,
			"address":      current.Address,
			"timezone":     current.Timezone,
			"units":        string(current.Units),
		}

		// Serializable tx: upsert + audit in one atomic unit (D-30 pattern).
		tx, err := deps.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "tx_begin_failed")
			return
		}
		defer tx.Rollback(r.Context()) //nolint:errcheck

		qtx := deps.Queries.WithTx(tx)
		updated, err := qtx.UpsertInstallIdentity(r.Context(), merged)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "upsert_failed")
			return
		}

		after := map[string]any{
			"display_name": updated.DisplayName,
			"address":      updated.Address,
			"timezone":     updated.Timezone,
			"units":        string(updated.Units),
		}

		// EntityID: use uuid.Nil (no UUID column on install_identity; id=1 integer).
		if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
			UserID:     callerID,
			Action:     audit.ActionSettingsIdentityUpdate,
			EntityType: audit.EntityTypeInstallIdentity,
			EntityID:   uuid.Nil,
			RequestID:  reqID,
			Before:     before,
			After:      after,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "audit_failed")
			return
		}

		if err := tx.Commit(r.Context()); err != nil {
			if isTxSerializationFailure(err) {
				writeError(w, http.StatusConflict, "serialization_failure")
				return
			}
			writeError(w, http.StatusInternalServerError, "tx_commit_failed")
			return
		}

		resp := IdentityResponse{
			InstallID:   strconv.Itoa(int(updated.ID)),
			SiteName:    updated.DisplayName,
			DisplayName: updated.DisplayName,
			Address:     updated.Address,
			Timezone:    updated.Timezone,
			Units:       string(updated.Units),
			Version:     version.Version,
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// isTxSerializationFailure returns true when err is a Postgres 40001
// serialization_failure (two concurrent Serializable txns conflicted).
// This helper is identity.go-local — no equivalent exists elsewhere in the
// settings package (backup_card.go and retention.go do not handle 40001).
func isTxSerializationFailure(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "40001"
}
