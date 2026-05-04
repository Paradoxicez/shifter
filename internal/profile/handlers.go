// Package profile — HTTP handlers for the device-profile editor (Plan 02-08
// + 02-11). This file is the HTTP surface; SaveProfile (editor.go) owns the
// Serializable transaction + audit-in-tx + ChirpStack push. Handlers here
// only:
//
//  1. RBAC + body decode + path-param parsing.
//  2. Build ProfileSaveInput from the request body + session user.
//  3. Call SaveProfile and translate error messages to HTTP status codes:
//     - "invalid capability" / mapping data_type / etc. → 400
//     - "exceeds %d bytes"                              → 413 (codec too large)
//     - "ChirpStack tenant not bootstrapped"            → 503
//     - default                                         → 500
//  4. /decoded-sample preview is read-only — walks the body's sample JSON
//     and returns every RFC 6901 leaf (used by the mapping editor's
//     click-to-bind UI, T-02-08-01 paste-bomb defense via 200-leaf cap).
//  5. /archive runs a Serializable txn that calls sqlc.ArchiveDeviceProfile
//     + audit.WriteEntry inside the same tx (mirrors site/archiveSite).
package profile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/audit"
	"github.com/shifter-io/shifter/internal/auth"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// maxDecodedSampleLeaves caps the leaf count returned by /decoded-sample so
// a paste-bomb (operator pasting a giant JSON tree) cannot DoS the editor.
// T-02-11-06.
const maxDecodedSampleLeaves = 200

// HTTPDeps bundles the shared infra a profile HTTP handler call needs.
// Distinct from profile.Deps (which is the lower-level SaveProfile input
// shape) because handlers also need the SessionMgr for RBAC.
type HTTPDeps struct {
	Pool       *pgxpool.Pool
	SessionMgr *scs.SessionManager
	Log        *slog.Logger
	CSClient   CSProfileClient
	ConnStore  ConnectionStore
}

// RegisterRoutes mounts every device-profile editor endpoint under
// /api/device-profiles on the given chi router.
//
// Route table:
//
//	GET   /api/device-profiles                     device_profile.read     — admin + viewer
//	GET   /api/device-profiles/{id}                device_profile.read     — admin + viewer
//	POST  /api/device-profiles/{id}/decoded-sample device_profile.read     — admin + viewer (POST: body carries sample)
//	POST  /api/device-profiles                     device_profile.create   — admin only
//	PATCH /api/device-profiles/{id}                device_profile.update   — admin only
//	POST  /api/device-profiles/{id}/archive        device_profile.archive  — admin only
func RegisterRoutes(r chi.Router, deps HTTPDeps) {
	r.Route("/api/device-profiles", func(r chi.Router) {
		// Read group — admin + viewer.
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionDeviceProfileRead))
			rt.Get("/", listProfiles(deps))
			rt.Get("/{id}", getProfile(deps))
			rt.Post("/{id}/decoded-sample", decodedSamplePreview(deps))
		})
		// Mutate groups — admin only.
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionDeviceProfileCreate))
			rt.Post("/", createProfile(deps))
		})
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionDeviceProfileUpdate))
			rt.Patch("/{id}", updateProfile(deps))
		})
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionDeviceProfileArchive))
			rt.Post("/{id}/archive", archiveProfile(deps))
		})
	})
}

// ----- request shapes ------------------------------------------------------

// ProfileRequest mirrors ProfileSaveInput minus UserID + RequestID + ID
// (handler injects from session/path). Scale is decimal text so the operator
// can carry full precision through JSON.
type ProfileRequest struct {
	Slug           string           `json:"slug"`
	Name           string           `json:"name"`
	Vendor         string           `json:"vendor"`
	Family         string           `json:"family,omitempty"`
	Capabilities   []string         `json:"capabilities"`
	CounterModulus int64            `json:"counter_modulus"`
	Region         string           `json:"region,omitempty"`
	MACVersion     string           `json:"mac_version"`
	CodecJS        string           `json:"codec_js,omitempty"`
	Mappings       []MappingRequest `json:"mappings"`
}

// MappingRequest is a single device_profile_mapping row in flight to/from
// the editor over JSON.
type MappingRequest struct {
	JSONPointer string `json:"json_pointer"`
	Target      string `json:"target"`
	Scale       string `json:"scale,omitempty"` // decimal text; default "1"
	DataType    string `json:"data_type"`
	Position    int32  `json:"position"`
}

// decodedSampleRequest is the JSON body of POST /decoded-sample. The endpoint
// is read-only despite the POST verb because the body carries the JSON tree
// the operator pasted (impractical to fit into a query string).
type decodedSampleRequest struct {
	Sample any `json:"sample"`
}

// ----- handlers -----------------------------------------------------------

func listProfiles(deps HTTPDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := sqlc.New(deps.Pool)
		rows, err := q.ListActiveDeviceProfiles(r.Context())
		if err != nil {
			internalError(deps.Log, w, "list profiles", err)
			return
		}
		writeJSON(w, http.StatusOK, profilesToJSON(rows))
	}
}

func getProfile(deps HTTPDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id")
		if !ok {
			return
		}
		q := sqlc.New(deps.Pool)
		row, err := q.GetDeviceProfile(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
			return
		}
		if err != nil {
			internalError(deps.Log, w, "get profile", err)
			return
		}
		mappings, err := q.ListMappingsByProfile(r.Context(), pgUUID(id))
		if err != nil {
			internalError(deps.Log, w, "list mappings", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"profile":  profileToJSON(row),
			"mappings": mappingsToJSON(mappings),
		})
	}
}

func createProfile(deps HTTPDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAdmin(deps.SessionMgr, w, r, auth.ActionDeviceProfileCreate)
		if !ok {
			return
		}
		var in ProfileRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_request", Detail: "invalid JSON"})
			return
		}
		mappings, err := parseMappings(in.Mappings)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "validation", Detail: err.Error()})
			return
		}

		input := ProfileSaveInput{
			UserID:         mustParseUUID(user.ID),
			RequestID:      middleware.GetReqID(r.Context()),
			ID:             uuid.Nil,
			Slug:           in.Slug,
			Name:           in.Name,
			Vendor:         in.Vendor,
			Family:         in.Family,
			Capabilities:   in.Capabilities,
			CounterModulus: in.CounterModulus,
			Region:         in.Region,
			MACVersion:     in.MACVersion,
			CodecJS:        in.CodecJS,
			Mappings:       mappings,
		}

		id, err := SaveProfile(r.Context(), Deps{
			Pool:      deps.Pool,
			CSClient:  deps.CSClient,
			ConnStore: deps.ConnStore,
			Log:       deps.Log,
		}, input)
		if err != nil {
			writeSaveProfileError(deps.Log, w, "create profile", err)
			return
		}

		writeJSON(w, http.StatusCreated, map[string]string{"id": id.String()})
	}
}

func updateProfile(deps HTTPDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAdmin(deps.SessionMgr, w, r, auth.ActionDeviceProfileUpdate)
		if !ok {
			return
		}
		id, ok := parseUUIDParam(w, r, "id")
		if !ok {
			return
		}
		var in ProfileRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_request", Detail: "invalid JSON"})
			return
		}
		mappings, err := parseMappings(in.Mappings)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "validation", Detail: err.Error()})
			return
		}

		// Slug is immutable on update — load the persisted slug from the row
		// and use it instead of the body's value (defense-in-depth on top of
		// SaveProfile's lower-level slug guard, which checks via the same path).
		q := sqlc.New(deps.Pool)
		existing, err := q.GetDeviceProfile(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
			return
		}
		if err != nil {
			internalError(deps.Log, w, "load existing profile", err)
			return
		}

		input := ProfileSaveInput{
			UserID:         mustParseUUID(user.ID),
			RequestID:      middleware.GetReqID(r.Context()),
			ID:             id,
			Slug:           existing.Slug, // immutable — ignore body
			Name:           in.Name,
			Vendor:         in.Vendor,
			Family:         in.Family,
			Capabilities:   in.Capabilities,
			CounterModulus: in.CounterModulus,
			Region:         in.Region,
			MACVersion:     in.MACVersion,
			CodecJS:        in.CodecJS,
			Mappings:       mappings,
		}

		updatedID, err := SaveProfile(r.Context(), Deps{
			Pool:      deps.Pool,
			CSClient:  deps.CSClient,
			ConnStore: deps.ConnStore,
			Log:       deps.Log,
		}, input)
		if err != nil {
			writeSaveProfileError(deps.Log, w, "update profile", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"id": updatedID.String()})
	}
}

func archiveProfile(deps HTTPDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAdmin(deps.SessionMgr, w, r, auth.ActionDeviceProfileArchive)
		if !ok {
			return
		}
		id, ok := parseUUIDParam(w, r, "id")
		if !ok {
			return
		}

		tx, err := deps.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			internalError(deps.Log, w, "begin tx", err)
			return
		}
		defer func() { _ = tx.Rollback(r.Context()) }()

		q := sqlc.New(tx)
		row, err := q.ArchiveDeviceProfile(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found_or_already_archived"})
			return
		}
		if err != nil {
			internalError(deps.Log, w, "archive profile", err)
			return
		}

		if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
			UserID:     mustParseUUID(user.ID),
			Action:     audit.ActionArchive,
			EntityType: audit.EntityTypeDeviceProfile,
			EntityID:   uuid.UUID(row.ID.Bytes),
			Before:     map[string]any{"archived_at": nil},
			After:      map[string]any{"archived_at": timestamptzText(row.ArchivedAt)},
			RequestID:  middleware.GetReqID(r.Context()),
		}); err != nil {
			internalError(deps.Log, w, "audit archive profile", err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			internalError(deps.Log, w, "commit archive", err)
			return
		}
		writeJSON(w, http.StatusOK, profileToJSON(row))
	}
}

// decodedSamplePreview walks the body's sample JSON and returns every RFC
// 6901 leaf as a {json_pointer, value} pair. Used by the mapping editor's
// click-to-bind UI (Plan 02-14). T-02-11-06: capped at maxDecodedSampleLeaves
// leaves; deeper trees return truncated:true.
func decodedSamplePreview(deps HTTPDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Path param is required (route shape) but we don't need to load the
		// profile — the endpoint walks the body's sample. Verify it's a UUID
		// for input hygiene.
		if _, ok := parseUUIDParam(w, r, "id"); !ok {
			return
		}
		var in decodedSampleRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_request", Detail: "invalid JSON"})
			return
		}
		leaves, truncated := flattenJSON("", in.Sample, maxDecodedSampleLeaves)
		out := map[string]any{"leaves": leaves}
		if truncated {
			out["truncated"] = true
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// ----- helpers ------------------------------------------------------------

// errorResp is the JSON envelope every Phase 2 handler emits on error.
type errorResp struct {
	Error  string `json:"error"`
	Detail string `json:"detail,omitempty"`
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func internalError(log *slog.Logger, w http.ResponseWriter, op string, err error) {
	if log != nil {
		log.Error("profile handler", "op", op, "err", err)
	}
	writeJSON(w, http.StatusInternalServerError, errorResp{Error: "internal"})
}

func parseUUIDParam(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	raw := chi.URLParam(r, name)
	id, err := uuid.Parse(raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResp{Error: "invalid_id"})
		return uuid.Nil, false
	}
	return id, true
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func requireAdmin(sm *scs.SessionManager, w http.ResponseWriter, r *http.Request, action auth.Action) (auth.User, bool) {
	user, ok := auth.GetUser(r.Context(), sm)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResp{Error: "unauthorized"})
		return auth.User{}, false
	}
	if !auth.Can(&user, action, nil) {
		writeJSON(w, http.StatusForbidden, errorResp{Error: "forbidden"})
		return auth.User{}, false
	}
	return user, true
}

func mustParseUUID(s string) uuid.UUID {
	u, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil
	}
	return u
}

// parseMappings converts a []MappingRequest into the internal []Mapping
// (with *big.Float scale parsed from decimal text).
func parseMappings(reqs []MappingRequest) ([]Mapping, error) {
	out := make([]Mapping, 0, len(reqs))
	for i, m := range reqs {
		var scale *big.Float
		if m.Scale != "" {
			f, _, err := big.ParseFloat(m.Scale, 10, 128, big.ToNearestEven)
			if err != nil {
				return nil, fmt.Errorf("mapping[%d]: invalid scale %q: %w", i, m.Scale, err)
			}
			scale = f
		}
		out = append(out, Mapping{
			JSONPointer: m.JSONPointer,
			Target:      m.Target,
			Scale:       scale,
			DataType:    m.DataType,
			Position:    m.Position,
		})
	}
	return out, nil
}

// writeSaveProfileError translates SaveProfile's error messages to HTTP status
// codes per the package preamble contract.
func writeSaveProfileError(log *slog.Logger, w http.ResponseWriter, op string, err error) {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "invalid capability"),
		strings.Contains(msg, "invalid data_type"),
		strings.Contains(msg, "json_pointer"),
		strings.Contains(msg, "target is required"),
		strings.Contains(msg, "decodeUplink"),
		strings.Contains(msg, "slug must be lowercase"),
		strings.Contains(msg, "is required"),
		strings.Contains(msg, "must be positive"):
		writeJSON(w, http.StatusBadRequest, errorResp{Error: "validation", Detail: msg})
	case strings.Contains(msg, "exceeds") && strings.Contains(msg, "bytes"):
		writeJSON(w, http.StatusRequestEntityTooLarge, errorResp{Error: "codec_too_large", Detail: msg})
	case strings.Contains(msg, "ChirpStack tenant not bootstrapped"):
		writeJSON(w, http.StatusServiceUnavailable, errorResp{Error: "chirpstack_not_bootstrapped", Detail: msg})
	default:
		if log != nil {
			log.Error("profile handler", "op", op, "err", err)
		}
		writeJSON(w, http.StatusInternalServerError, errorResp{Error: "internal"})
	}
}

// flattenJSON walks val and returns every leaf (non-container) as
// {json_pointer, value}. Containers are walked recursively; arrays use /N
// indexing per RFC 6901. Caps at max leaves and signals truncated=true.
func flattenJSON(prefix string, val any, max int) (leaves []map[string]any, truncated bool) {
	leaves = make([]map[string]any, 0, 16)
	walk(prefix, val, &leaves, max, &truncated)
	return leaves, truncated
}

func walk(prefix string, val any, leaves *[]map[string]any, max int, truncated *bool) {
	if *truncated {
		return
	}
	switch v := val.(type) {
	case map[string]any:
		for k, child := range v {
			if *truncated {
				return
			}
			walk(prefix+"/"+escapeJSONPointer(k), child, leaves, max, truncated)
		}
	case []any:
		for i, child := range v {
			if *truncated {
				return
			}
			walk(prefix+"/"+strconv.Itoa(i), child, leaves, max, truncated)
		}
	default:
		if len(*leaves) >= max {
			*truncated = true
			return
		}
		*leaves = append(*leaves, map[string]any{
			"json_pointer": prefix,
			"value":        v,
		})
	}
}

// escapeJSONPointer applies RFC 6901 §4 reverse mapping: ~ → ~0, / → ~1.
// Order matters: ~ MUST be encoded BEFORE / so "/" inside a key ends up
// as ~1 rather than ~01.
func escapeJSONPointer(token string) string {
	token = strings.ReplaceAll(token, "~", "~0")
	token = strings.ReplaceAll(token, "/", "~1")
	return token
}

// ----- JSON serialization -------------------------------------------------

func profileToJSON(p sqlc.DeviceProfile) map[string]any {
	out := map[string]any{
		"id":              uuidString(p.ID),
		"slug":            p.Slug,
		"name":            p.Name,
		"vendor":          p.Vendor,
		"family":          derefString(p.Family),
		"capabilities":    p.Capabilities,
		"counter_modulus": p.CounterModulus,
		"codec_js":        p.CodecJs,
		"region":          derefString(p.Region),
		"mac_version":     p.MacVersion,
		"archived_at":     timestamptzText(p.ArchivedAt),
		"created_at":      timestamptzText(p.CreatedAt),
		"updated_at":      timestamptzText(p.UpdatedAt),
	}
	if p.CsProfileID.Valid {
		out["cs_profile_id"] = uuid.UUID(p.CsProfileID.Bytes).String()
	} else {
		out["cs_profile_id"] = nil
	}
	out["codec_js_synced_at"] = timestamptzText(p.CodecJsSyncedAt)
	return out
}

func profilesToJSON(rows []sqlc.DeviceProfile) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, profileToJSON(r))
	}
	return out
}

func mappingToJSON(m sqlc.DeviceProfileMapping) map[string]any {
	scale := ""
	if m.Scale.Valid {
		// Render via Float64Value for human-readable form. Mapping editor
		// preserves precision client-side; the wire-format here is decimal
		// text suitable for display.
		f, err := m.Scale.Float64Value()
		if err == nil && f.Valid {
			scale = strconv.FormatFloat(f.Float64, 'f', -1, 64)
		}
	}
	return map[string]any{
		"id":           uuidString(m.ID),
		"json_pointer": m.JsonPointer,
		"target":       m.Target,
		"scale":        scale,
		"data_type":    m.DataType,
		"position":     m.Position,
	}
}

func mappingsToJSON(rows []sqlc.DeviceProfileMapping) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, mappingToJSON(r))
	}
	return out
}

// uuidString is declared in seed.go (same package).

func derefString(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func timestamptzText(t pgtype.Timestamptz) any {
	if !t.Valid {
		return nil
	}
	return t.Time.UTC().Format("2006-01-02T15:04:05.000000Z")
}

// silence unused-import lint when context import is only used by tests.
var _ = context.Background
