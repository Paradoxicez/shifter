// Package api — catalog HTTP handler shims (plan 07-04).
//
// catalog_handler.go exposes 4 endpoints for the Vendor Catalog tab (plan 05)
// and the Import / Update dialogs (plan 06):
//
//	GET  /api/catalog              — list all catalog entries + per-profile status
//	GET  /api/catalog/{slug}       — fetch one catalog entry by slug
//	POST /api/catalog/import       — import a catalog entry as a device_profile row
//	POST /api/catalog/{id}/update  — apply per-field catalog diff to an existing profile
//
// Security: RBAC is enforced at the router layer (RegisterCatalogRoutes) via
// RequireAction middleware. Handler-level session reads are for audit attribution only.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/mod/semver"

	"github.com/shifter-io/shifter/internal/audit"
	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/codec"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/profile/codecs"
)

// CatalogDeps bundles the dependencies for the catalog HTTP handlers.
type CatalogDeps struct {
	Pool       *pgxpool.Pool
	SessionMgr *scs.SessionManager
}

// RegisterCatalogRoutes mounts the 4 catalog endpoints with RBAC middleware.
// Admin can call all 4; viewer can only call the 2 GET endpoints (T-07-04-01).
func RegisterCatalogRoutes(r chi.Router, deps CatalogDeps) {
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionCatalogRead))
		rt.Get("/api/catalog", ListCatalogHandler(deps))
		rt.Get("/api/catalog/{slug}", GetCatalogEntryHandler())
	})
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionCatalogImport))
		rt.Post("/api/catalog/import", ImportFromCatalogHandler(deps))
	})
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionCatalogUpdate))
		rt.Post("/api/catalog/{profile_id}/update", ApplyCatalogUpdateHandler(deps))
	})
}

// ---- response shapes -------------------------------------------------------

type catalogListResponse struct {
	Entries  []catalogListEntry   `json:"entries"`
	Profiles []catalogListProfile `json:"profiles"`
}

type catalogListEntry struct {
	Slug         string   `json:"slug"`
	Name         string   `json:"name"`
	Vendor       string   `json:"vendor"`
	Family       string   `json:"family"`
	Capabilities []string `json:"capabilities"`
	Version      string   `json:"version"`
}

type catalogListProfile struct {
	ProfileID        uuid.UUID `json:"profile_id"`
	Slug             string    `json:"slug"`
	InstalledVersion string    `json:"installed_version"`
	Status           string    `json:"status"` // not-installed | installed | update-available
	CustomerEdited   bool      `json:"customer_edited"`
	CodecJsSyncedAt  *string   `json:"codec_js_synced_at,omitempty"`
}

// D-34 allowlist — concrete; ApplyCatalogUpdateHandler enforces this.
// Keep in sync with <interfaces> §D-34 in the plan.
var catalogUpdateAllowedFields = map[string]struct{}{
	"codec_js":                         {},
	"capabilities":                     {},
	"battery_curve":                    {},
	"expected_uplink_interval_seconds": {},
	"offline_threshold_multiplier":     {},
	"anomaly_compatibility":            {},
	"counter_modulus":                  {},
	"mac_version":                      {},
}

// ---- handlers ---------------------------------------------------------------

// ListCatalogHandler handles GET /api/catalog.
// Returns all embedded catalog entries and per-profile install/update status.
func ListCatalogHandler(deps CatalogDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entries, err := codec.LoadAll()
		if err != nil {
			http.Error(w, "catalog unavailable", http.StatusInternalServerError)
			return
		}
		q := sqlc.New(deps.Pool)
		profiles, err := q.ListProfilesWithCatalogMetadata(r.Context())
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}

		resp := catalogListResponse{}
		for _, e := range entries {
			resp.Entries = append(resp.Entries, catalogListEntry{
				Slug:         e.Slug,
				Name:         e.Name,
				Vendor:       e.Vendor,
				Family:       e.Family,
				Capabilities: e.Capabilities,
				Version:      e.Version,
			})
		}

		// Index entries by slug for status comparison.
		entryBySlug := make(map[string]codec.CatalogEntry, len(entries))
		for _, e := range entries {
			entryBySlug[e.Slug] = e
		}

		// Build profile rows for catalog-sourced profiles.
		installedSlugs := map[string]struct{}{}
		for _, p := range profiles {
			if p.CatalogSource == nil {
				continue // non-catalog profile — skip for this tab
			}
			slug := derefStr(p.CatalogSource)
			ver := derefStr(p.CatalogSourceVersion)
			status := "installed"
			if entry, ok := entryBySlug[slug]; ok {
				// Compare semver ("v" prefix required by golang.org/x/mod/semver).
				if semver.Compare("v"+entry.Version, "v"+ver) > 0 {
					status = "update-available"
				}
				installedSlugs[slug] = struct{}{}
			}
			var syncedAt *string
			if p.CodecJsSyncedAt.Valid {
				s := p.CodecJsSyncedAt.Time.Format(time.RFC3339)
				syncedAt = &s
			}
			resp.Profiles = append(resp.Profiles, catalogListProfile{
				ProfileID:        uuid.UUID(p.ID.Bytes),
				Slug:             slug,
				InstalledVersion: ver,
				Status:           status,
				CustomerEdited:   p.CustomerEdited,
				CodecJsSyncedAt:  syncedAt,
			})
		}

		// Append not-installed entries (no matching profile row).
		for _, e := range entries {
			if _, ok := installedSlugs[e.Slug]; !ok {
				resp.Profiles = append(resp.Profiles, catalogListProfile{
					ProfileID: uuid.Nil,
					Slug:      e.Slug,
					Status:    "not-installed",
				})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// catalogEntryWithCodec wraps codec.CatalogEntry with the embedded JS source
// so the Import Review step can pre-populate the codec textarea without a
// separate /api/catalog/{slug}/codec round-trip.
type catalogEntryWithCodec struct {
	codec.CatalogEntry
	CodecJS string `json:"codec_js"`
}

// GetCatalogEntryHandler handles GET /api/catalog/{slug}.
// Returns the embedded CatalogEntry JSON (plus codec_js inline) or 404.
func GetCatalogEntryHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		entry, err := codec.Get(slug)
		if errors.Is(err, codec.ErrCatalogEntryNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "catalog error", http.StatusInternalServerError)
			return
		}
		resp := catalogEntryWithCodec{
			CatalogEntry: entry,
			CodecJS:      codecs.CodecBySlug(slug),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// ---- import -----------------------------------------------------------------

type importRequest struct {
	Slug string `json:"slug"`
	Name string `json:"name,omitempty"`
}

type importResponse struct {
	ProfileID uuid.UUID `json:"profile_id"`
}

// ImportFromCatalogHandler handles POST /api/catalog/import.
// Validates slug, checks for duplicates (409), inserts a new device_profile row
// from catalog data, and writes an audit row in the same transaction.
func ImportFromCatalogHandler(deps CatalogDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req importRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Slug == "" {
			http.Error(w, "slug is required", http.StatusBadRequest)
			return
		}

		// Resolve catalog entry.
		entry, err := codec.Get(req.Slug)
		if errors.Is(err, codec.ErrCatalogEntryNotFound) {
			http.Error(w, "unknown slug", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "catalog error", http.StatusInternalServerError)
			return
		}

		q := sqlc.New(deps.Pool)

		// 409 if slug already imported.
		if _, err := q.GetDeviceProfileBySlug(r.Context(), entry.Slug); err == nil {
			http.Error(w, "slug already imported", http.StatusConflict)
			return
		}

		// Resolve actor for audit.
		actor, _ := auth.GetUser(r.Context(), deps.SessionMgr)

		// Atomic tx: INSERT device_profile + audit row.
		tx, err := deps.Pool.Begin(r.Context())
		if err != nil {
			http.Error(w, "tx begin failed", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context()) //nolint:errcheck

		qtx := sqlc.New(tx)
		name := entry.Name
		if req.Name != "" {
			name = req.Name
		}
		newProfile, err := qtx.CreateDeviceProfileFromCatalog(r.Context(), sqlc.CreateDeviceProfileFromCatalogParams{
			Name:                          name,
			CodecJs:                       codecs.CodecBySlug(entry.Slug),
			Capabilities:                  entry.Capabilities,
			CatalogSource:                 strPtr(entry.Slug),
			CatalogSourceVersion:          strPtr(entry.Version),
			BatteryCurve:                  entry.BatteryCurve,
			ExpectedUplinkIntervalSeconds: int32(entry.ExpectedUplinkIntervalSeconds),
			OfflineThresholdMultiplier:    entry.OfflineThresholdMultiplier,
			AnomalyCompatibility:          entry.AnomalyCompatibility,
			Slug:                          entry.Slug,
			Vendor:                        entry.Vendor,
			Family:                        nullableStrPtr(entry.Family),
			CounterModulus:                entry.CounterModulus,
			MacVersion:                    entry.MACVersion,
			Region:                        entry.Region,
		})
		if err != nil {
			http.Error(w, "db error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		actorUUID, _ := uuid.Parse(actor.ID)
		if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
			UserID:     actorUUID,
			Action:     audit.AuditActionCatalogProfileImported,
			EntityType: audit.EntityTypeDeviceProfile,
			EntityID:   uuid.UUID(newProfile.ID.Bytes),
			After: map[string]any{
				"slug":    entry.Slug,
				"version": entry.Version,
			},
		}); err != nil {
			http.Error(w, "audit failed", http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "commit failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(importResponse{ProfileID: uuid.UUID(newProfile.ID.Bytes)})
	}
}

// ---- update -----------------------------------------------------------------

type updateRequest struct {
	AcceptedFields []string `json:"accepted_fields"` // field names to take from catalog; absent = keep mine
	TargetVersion  string   `json:"target_version"`
}

type updateResponse struct {
	UpdatedAt  string `json:"updated_at"`
	NewVersion string `json:"new_version"`
}

type unknownFieldError struct {
	Error string `json:"error"`
	Field string `json:"field"`
}

// ApplyCatalogUpdateHandler handles POST /api/catalog/{profile_id}/update.
// Validates: profile exists, profile is catalog-sourced, no downgrade, all
// accepted_fields are in the D-34 allowlist. Merges catalog fields, sets
// customer_edited flag, writes audit row atomically.
func ApplyCatalogUpdateHandler(deps CatalogDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Parse path + body.
		profileIDStr := chi.URLParam(r, "profile_id")
		profileID, err := uuid.Parse(profileIDStr)
		if err != nil {
			http.Error(w, "bad profile_id", http.StatusBadRequest)
			return
		}
		var req updateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request body", http.StatusBadRequest)
			return
		}

		// 2. D-34 allowlist enforcement — reject unknown field names with structured 400.
		for _, f := range req.AcceptedFields {
			if _, ok := catalogUpdateAllowedFields[f]; !ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(unknownFieldError{Error: "unknown_field", Field: f})
				return
			}
		}

		// 3. Load profile metadata.
		q := sqlc.New(deps.Pool)
		pgID := pgtype.UUID{Bytes: profileID, Valid: true}
		profile, err := q.GetProfileCatalogMetadata(r.Context(), pgID)
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "profile not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		if profile.CatalogSource == nil {
			http.Error(w, "profile is not catalog-sourced", http.StatusBadRequest)
			return
		}

		// Load catalog entry by slug.
		entry, err := codec.Get(*profile.CatalogSource)
		if errors.Is(err, codec.ErrCatalogEntryNotFound) {
			http.Error(w, "catalog entry no longer exists", http.StatusGone)
			return
		}
		if err != nil {
			http.Error(w, "catalog error", http.StatusInternalServerError)
			return
		}

		// 4. Reject downgrade (per T-07-04-07).
		currentVer := derefStr(profile.CatalogSourceVersion)
		if currentVer == "" {
			currentVer = "0.0.0"
		}
		targetVer := req.TargetVersion
		if targetVer == "" {
			targetVer = entry.Version
		}
		if semver.Compare("v"+targetVer, "v"+currentVer) <= 0 {
			http.Error(w, fmt.Sprintf("target_version %s is not newer than installed %s", targetVer, currentVer), http.StatusBadRequest)
			return
		}

		// 5. Build merged params: only accepted_fields come from the catalog.
		accept := make(map[string]struct{}, len(req.AcceptedFields))
		for _, f := range req.AcceptedFields {
			accept[f] = struct{}{}
		}

		// Load full profile row for fields not in GetProfileCatalogMetadataRow
		// (MacVersion, CounterModulus, Region).
		fullProfile, fpErr := q.GetDeviceProfile(r.Context(), pgID)

		merged := sqlc.ApplyCatalogUpdateParams{
			ID:                            pgID,
			CatalogSourceVersion:          strPtr(targetVer),
			CodecJs:                       profile.CodecJs,
			Capabilities:                  profile.Capabilities,
			BatteryCurve:                  profile.BatteryCurve,
			ExpectedUplinkIntervalSeconds: profile.ExpectedUplinkIntervalSeconds,
			OfflineThresholdMultiplier:    profile.OfflineThresholdMultiplier,
			AnomalyCompatibility:          profile.AnomalyCompatibility,
			CounterModulus:                1, // safe default; overridden below if fullProfile loaded
			MacVersion:                    "", // overridden below if fullProfile loaded
			Region:                        nil,
		}
		if fpErr == nil {
			merged.CounterModulus = fullProfile.CounterModulus
			merged.MacVersion = fullProfile.MacVersion
			merged.Region = fullProfile.Region
		}

		if _, ok := accept["codec_js"]; ok {
			merged.CodecJs = codecs.CodecBySlug(entry.Slug)
		}
		if _, ok := accept["capabilities"]; ok {
			merged.Capabilities = entry.Capabilities
		}
		if _, ok := accept["battery_curve"]; ok {
			merged.BatteryCurve = entry.BatteryCurve
		}
		if _, ok := accept["expected_uplink_interval_seconds"]; ok {
			merged.ExpectedUplinkIntervalSeconds = int32(entry.ExpectedUplinkIntervalSeconds)
		}
		if _, ok := accept["offline_threshold_multiplier"]; ok {
			merged.OfflineThresholdMultiplier = entry.OfflineThresholdMultiplier
		}
		if _, ok := accept["anomaly_compatibility"]; ok {
			merged.AnomalyCompatibility = entry.AnomalyCompatibility
		}
		if _, ok := accept["counter_modulus"]; ok {
			merged.CounterModulus = entry.CounterModulus
		}
		if _, ok := accept["mac_version"]; ok {
			merged.MacVersion = entry.MACVersion
		}

		// 6. customer_edited flag: true if the operator kept any field from "mine"
		//    (i.e., did not accept the full allowlist of 8 fields), per D-34.
		customerEdited := profile.CustomerEdited || len(req.AcceptedFields) < len(catalogUpdateAllowedFields)

		// 7. Atomic tx: ApplyCatalogUpdate + SetProfileCatalogSource + audit row.
		actor, _ := auth.GetUser(r.Context(), deps.SessionMgr)
		actorUUID, _ := uuid.Parse(actor.ID)

		tx, err := deps.Pool.Begin(r.Context())
		if err != nil {
			http.Error(w, "tx begin failed", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(r.Context()) //nolint:errcheck

		qtx := sqlc.New(tx)
		if err := qtx.ApplyCatalogUpdate(r.Context(), merged); err != nil {
			http.Error(w, "db update failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := qtx.SetProfileCatalogSource(r.Context(), sqlc.SetProfileCatalogSourceParams{
			ID:                   pgID,
			CatalogSource:        strPtr(entry.Slug),
			CatalogSourceVersion: strPtr(targetVer),
			CustomerEdited:       customerEdited,
		}); err != nil {
			http.Error(w, "set catalog source failed", http.StatusInternalServerError)
			return
		}
		if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
			UserID:     actorUUID,
			Action:     audit.AuditActionCatalogProfileUpdated,
			EntityType: audit.EntityTypeDeviceProfile,
			EntityID:   profileID,
			After: map[string]any{
				"slug":            entry.Slug,
				"from_version":    currentVer,
				"to_version":      targetVer,
				"accepted_fields": req.AcceptedFields,
				"customer_edited": customerEdited,
			},
		}); err != nil {
			http.Error(w, "audit failed", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "commit failed", http.StatusInternalServerError)
			return
		}

		// 8. Return {updated_at, new_version}.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(updateResponse{
			UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
			NewVersion: targetVer,
		})
	}
}

// ---- helpers ----------------------------------------------------------------

// strPtr converts a string to a *string.
func strPtr(s string) *string { return &s }

// nullableStrPtr converts a string to *string; empty string → nil.
func nullableStrPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// derefStr safely dereferences a *string; nil → "".
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
