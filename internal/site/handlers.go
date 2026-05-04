// Package site — HTTP handlers for the Site CRUD surface (D-17 + D-20).
//
// Every mutation handler:
//
//  1. Calls auth.Can(user, <action>, nil) at entry; viewer → 403 (defense-
//     in-depth on top of RequireAction middleware).
//  2. Decodes + server-side validates the request body.
//  3. Opens a pgx.Serializable transaction.
//  4. Runs the sqlc mutation (CreateSite / UpdateSite / ArchiveSite /
//     RestoreSite).
//  5. Writes an audit_log row INSIDE the same tx via audit.WriteEntry —
//     D-23 + AUDIT-01 enforced by signature: audit row literally cannot
//     exist without the domain row, and vice versa.
//  6. Commits.
//  7. Encodes the persisted row as JSON.
//
// Read handlers (ListActiveSites, ListArchivedSites, GetSite, ListChildSites)
// run outside any tx and only require auth.Can(<read>, ...) — admin AND
// viewer pass.
package site

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
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

// Deps bundles the shared infra a Site handler call needs. Plan 02-15
// (cmd/serve wiring) constructs a single Deps and passes it to RegisterRoutes.
type Deps struct {
	Pool       *pgxpool.Pool
	SessionMgr *scs.SessionManager
	Log        *slog.Logger
}

// RegisterRoutes mounts every Site endpoint under /api/sites on the given
// chi router. The mutating route group additionally wraps with
// RequireAction(<action>) so a missing handler-level Can() check still 403s
// — defense-in-depth.
//
// Route table:
//
//	GET    /api/sites              site.read     — admin + viewer
//	GET    /api/sites/archived     site.read     — admin + viewer
//	GET    /api/sites/{id}         site.read     — admin + viewer
//	GET    /api/sites/{id}/children site.read    — admin + viewer
//	POST   /api/sites              site.create   — admin only
//	PATCH  /api/sites/{id}         site.update   — admin only
//	POST   /api/sites/{id}/archive site.archive  — admin only
//	POST   /api/sites/{id}/restore site.restore  — admin only
func RegisterRoutes(r chi.Router, deps Deps) {
	r.Route("/api/sites", func(r chi.Router) {
		// Read group — admin + viewer.
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionSiteRead))
			rt.Get("/", listSites(deps))
			rt.Get("/archived", listArchivedSites(deps))
			rt.Get("/{id}", getSite(deps))
			rt.Get("/{id}/children", listChildSites(deps))
		})
		// Mutate group — admin only.
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionSiteCreate))
			rt.Post("/", createSite(deps))
		})
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionSiteUpdate))
			rt.Patch("/{id}", updateSite(deps))
		})
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionSiteArchive))
			rt.Post("/{id}/archive", archiveSite(deps))
		})
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionSiteRestore))
			rt.Post("/{id}/restore", restoreSite(deps))
		})
	})
}

// ----- request shapes ------------------------------------------------------

// CreateSiteRequest is the JSON body of POST /api/sites. Optional fields use
// pointers so omitempty/null semantics flow correctly to the SQL layer.
type CreateSiteRequest struct {
	ParentID    *string  `json:"parent_id,omitempty"` // UUID string or null/missing
	Name        string   `json:"name"`
	SiteType    *string  `json:"site_type,omitempty"`
	Lat         *float64 `json:"lat,omitempty"`
	Lng         *float64 `json:"lng,omitempty"`
	Timezone    string   `json:"timezone"`
	Address     *string  `json:"address,omitempty"`
	Description *string  `json:"description,omitempty"`
}

// UpdateSiteRequest is the JSON body of PATCH /api/sites/{id}. parent_id is
// intentionally NOT updatable here (D-17 — moving a site between parents is
// a separate "reparent" workflow with audit implications, deferred to Phase 6).
type UpdateSiteRequest struct {
	Name        string   `json:"name"`
	SiteType    *string  `json:"site_type,omitempty"`
	Lat         *float64 `json:"lat,omitempty"`
	Lng         *float64 `json:"lng,omitempty"`
	Timezone    string   `json:"timezone"`
	Address     *string  `json:"address,omitempty"`
	Description *string  `json:"description,omitempty"`
}

// ----- handlers -----------------------------------------------------------

func listSites(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := sqlc.New(deps.Pool)
		rows, err := q.ListActiveSites(r.Context())
		if err != nil {
			internalError(deps.Log, w, "list active sites", err)
			return
		}
		writeJSON(w, http.StatusOK, sitesToJSON(rows))
	}
}

func listArchivedSites(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := sqlc.New(deps.Pool)
		rows, err := q.ListArchivedSites(r.Context())
		if err != nil {
			internalError(deps.Log, w, "list archived sites", err)
			return
		}
		writeJSON(w, http.StatusOK, sitesToJSON(rows))
	}
}

func getSite(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id")
		if !ok {
			return
		}
		q := sqlc.New(deps.Pool)
		row, err := q.GetSite(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
			return
		}
		if err != nil {
			internalError(deps.Log, w, "get site", err)
			return
		}
		writeJSON(w, http.StatusOK, siteToJSON(row))
	}
}

func listChildSites(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id")
		if !ok {
			return
		}
		q := sqlc.New(deps.Pool)
		rows, err := q.ListChildSites(r.Context(), pgUUID(id))
		if err != nil {
			internalError(deps.Log, w, "list child sites", err)
			return
		}
		writeJSON(w, http.StatusOK, sitesToJSON(rows))
	}
}

func createSite(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAdmin(deps.SessionMgr, w, r, auth.ActionSiteCreate)
		if !ok {
			return
		}
		var in CreateSiteRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_request"})
			return
		}
		if err := validateCreateSite(in); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "validation", Detail: err.Error()})
			return
		}

		var parentUUID pgtype.UUID
		if in.ParentID != nil && *in.ParentID != "" {
			pid, err := uuid.Parse(*in.ParentID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errorResp{Error: "invalid_parent_id"})
				return
			}
			parentUUID = pgUUID(pid)
		}

		tx, err := deps.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			internalError(deps.Log, w, "begin tx", err)
			return
		}
		defer func() { _ = tx.Rollback(r.Context()) }()

		q := sqlc.New(tx)
		row, err := q.CreateSite(r.Context(), sqlc.CreateSiteParams{
			ParentID:    parentUUID,
			Name:        strings.TrimSpace(in.Name),
			SiteType:    in.SiteType,
			Lat:         in.Lat,
			Lng:         in.Lng,
			Timezone:    in.Timezone,
			Address:     in.Address,
			Description: in.Description,
		})
		if err != nil {
			// CHECK constraint violations (lat/lng range, etc.) → 400.
			if isConstraintViolation(err) {
				writeJSON(w, http.StatusBadRequest, errorResp{Error: "constraint", Detail: err.Error()})
				return
			}
			internalError(deps.Log, w, "create site", err)
			return
		}

		after := siteToJSON(row)
		if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
			UserID:     mustParseUUID(user.ID),
			Action:     audit.ActionCreate,
			EntityType: audit.EntityTypeSite,
			EntityID:   uuid.UUID(row.ID.Bytes),
			Before:     nil,
			After:      after,
			RequestID:  middleware.GetReqID(r.Context()),
		}); err != nil {
			internalError(deps.Log, w, "audit create site", err)
			return
		}

		if err := tx.Commit(r.Context()); err != nil {
			internalError(deps.Log, w, "commit create site", err)
			return
		}

		writeJSON(w, http.StatusCreated, after)
	}
}

func updateSite(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAdmin(deps.SessionMgr, w, r, auth.ActionSiteUpdate)
		if !ok {
			return
		}
		id, ok := parseUUIDParam(w, r, "id")
		if !ok {
			return
		}
		var in UpdateSiteRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_request"})
			return
		}
		if err := validateUpdateSite(in); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "validation", Detail: err.Error()})
			return
		}

		tx, err := deps.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			internalError(deps.Log, w, "begin tx", err)
			return
		}
		defer func() { _ = tx.Rollback(r.Context()) }()

		q := sqlc.New(tx)

		// Capture before-state for the audit diff.
		existing, err := q.GetSite(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
			return
		}
		if err != nil {
			internalError(deps.Log, w, "load existing site", err)
			return
		}
		before := siteToJSON(existing)

		row, err := q.UpdateSite(r.Context(), sqlc.UpdateSiteParams{
			ID:          pgUUID(id),
			Name:        strings.TrimSpace(in.Name),
			SiteType:    in.SiteType,
			Lat:         in.Lat,
			Lng:         in.Lng,
			Timezone:    in.Timezone,
			Address:     in.Address,
			Description: in.Description,
		})
		if err != nil {
			if isConstraintViolation(err) {
				writeJSON(w, http.StatusBadRequest, errorResp{Error: "constraint", Detail: err.Error()})
				return
			}
			internalError(deps.Log, w, "update site", err)
			return
		}
		after := siteToJSON(row)

		// D-24 changed-fields-only diff for UPDATE actions.
		bDiff, aDiff := audit.ChangedFields(before, after)
		if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
			UserID:     mustParseUUID(user.ID),
			Action:     audit.ActionUpdate,
			EntityType: audit.EntityTypeSite,
			EntityID:   uuid.UUID(row.ID.Bytes),
			Before:     bDiff,
			After:      aDiff,
			RequestID:  middleware.GetReqID(r.Context()),
		}); err != nil {
			internalError(deps.Log, w, "audit update site", err)
			return
		}

		if err := tx.Commit(r.Context()); err != nil {
			internalError(deps.Log, w, "commit update site", err)
			return
		}
		writeJSON(w, http.StatusOK, after)
	}
}

func archiveSite(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAdmin(deps.SessionMgr, w, r, auth.ActionSiteArchive)
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
		row, err := q.ArchiveSite(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found_or_already_archived"})
			return
		}
		if err != nil {
			internalError(deps.Log, w, "archive site", err)
			return
		}

		if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
			UserID:     mustParseUUID(user.ID),
			Action:     audit.ActionArchive,
			EntityType: audit.EntityTypeSite,
			EntityID:   uuid.UUID(row.ID.Bytes),
			Before:     map[string]any{"archived_at": nil},
			After:      map[string]any{"archived_at": timestamptzText(row.ArchivedAt)},
			RequestID:  middleware.GetReqID(r.Context()),
		}); err != nil {
			internalError(deps.Log, w, "audit archive site", err)
			return
		}

		if err := tx.Commit(r.Context()); err != nil {
			internalError(deps.Log, w, "commit archive site", err)
			return
		}
		writeJSON(w, http.StatusOK, siteToJSON(row))
	}
}

func restoreSite(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAdmin(deps.SessionMgr, w, r, auth.ActionSiteRestore)
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
		row, err := q.RestoreSite(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found_or_not_archived"})
			return
		}
		if err != nil {
			internalError(deps.Log, w, "restore site", err)
			return
		}

		if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
			UserID:     mustParseUUID(user.ID),
			Action:     audit.ActionRestore,
			EntityType: audit.EntityTypeSite,
			EntityID:   uuid.UUID(row.ID.Bytes),
			Before:     map[string]any{"archived_at": "non-null"},
			After:      map[string]any{"archived_at": nil},
			RequestID:  middleware.GetReqID(r.Context()),
		}); err != nil {
			internalError(deps.Log, w, "audit restore site", err)
			return
		}

		if err := tx.Commit(r.Context()); err != nil {
			internalError(deps.Log, w, "commit restore site", err)
			return
		}
		writeJSON(w, http.StatusOK, siteToJSON(row))
	}
}

// ----- validation ---------------------------------------------------------

func validateCreateSite(in CreateSiteRequest) error {
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("name is required")
	}
	if in.Timezone == "" {
		return errors.New("timezone is required")
	}
	return validateLatLng(in.Lat, in.Lng)
}

func validateUpdateSite(in UpdateSiteRequest) error {
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("name is required")
	}
	if in.Timezone == "" {
		return errors.New("timezone is required")
	}
	return validateLatLng(in.Lat, in.Lng)
}

// validateLatLng applies the WGS84 range checks the schema enforces in the
// CHECK constraint. Server-side validation here gives the operator a clean
// 400 with a friendly message instead of a constraint-violation 400.
func validateLatLng(lat, lng *float64) error {
	if lat != nil && (*lat < -90 || *lat > 90) {
		return fmt.Errorf("lat must be in [-90, 90] (got %v)", *lat)
	}
	if lng != nil && (*lng < -180 || *lng > 180) {
		return fmt.Errorf("lng must be in [-180, 180] (got %v)", *lng)
	}
	return nil
}

// ----- helpers ------------------------------------------------------------

// errorResp is the JSON envelope every Phase 2 handler emits on error.
// Mirrors Phase 1 install handlers' shape so the SPA's apiFetch logic doesn't
// need to special-case Phase 2 routes.
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
		log.Error("site handler", "op", op, "err", err)
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

// requireAdmin loads the current session user and verifies the action via
// auth.Can. The chi router additionally wraps the route in RequireAction
// middleware (defense-in-depth) — this in-handler check guards against future
// router refactors that accidentally drop the wrapper.
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
		// Session user IDs come from the database and are always valid UUIDs;
		// hitting this path means the session got corrupted, which we treat
		// as "system event" (uuid.Nil) so the audit row still writes.
		return uuid.Nil
	}
	return u
}

// isConstraintViolation reports whether err is a Postgres CHECK / UNIQUE /
// FK violation. We check by error string match because pgx wraps PgError in
// fmt.Errorf chains; the SQLSTATE codes appear in the message verbatim.
//
// 23514 = check_violation; 23505 = unique_violation; 23503 = foreign_key_violation.
func isConstraintViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "23514") ||
		strings.Contains(msg, "23505") ||
		strings.Contains(msg, "23503") ||
		strings.Contains(msg, "violates check constraint") ||
		strings.Contains(msg, "violates unique constraint") ||
		strings.Contains(msg, "violates foreign key constraint")
}

// ----- JSON serialization -------------------------------------------------

// siteToJSON converts a sqlc.Site into the operator-facing JSON shape. We
// avoid leaking the pgtype zero-form into the wire by hand-constructing the
// map. Returns map[string]any so audit.ChangedFields can diff before/after.
func siteToJSON(s sqlc.Site) map[string]any {
	return map[string]any{
		"id":           uuidString(s.ID),
		"parent_id":    uuidStringNullable(s.ParentID),
		"name":         s.Name,
		"site_type":    derefString(s.SiteType),
		"lat":          s.Lat,
		"lng":          s.Lng,
		"timezone":     s.Timezone,
		"address":      derefString(s.Address),
		"description":  derefString(s.Description),
		"archived_at":  timestamptzText(s.ArchivedAt),
		"created_at":   timestamptzText(s.CreatedAt),
		"updated_at":   timestamptzText(s.UpdatedAt),
	}
}

func sitesToJSON(rows []sqlc.Site) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, siteToJSON(r))
	}
	return out
}

func uuidString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return uuid.UUID(u.Bytes).String()
}

func uuidStringNullable(u pgtype.UUID) any {
	if !u.Valid {
		return nil
	}
	return uuid.UUID(u.Bytes).String()
}

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

// reuse — currently the only context-dep accessor we need lives in middleware.
// Future plans may want a helper that combines ctx + user — kept commented
// out to avoid YAGNI:
//
//	func userFromCtx(ctx context.Context, sm *scs.SessionManager) (auth.User, bool) {
//	    return auth.GetUser(ctx, sm)
//	}

// _ ensures `context` import is used even if all callers use r.Context().
var _ = context.Background
