package api

// report_templates_handler.go — Phase 7 Plan 11a: Saved Report Templates HTTP handlers.
//
// Exposes 5 endpoints under /api/reports/templates:
//
//	GET    /api/reports/templates        — list all (admin + viewer)
//	GET    /api/reports/templates/{id}   — get one (admin + viewer)
//	POST   /api/reports/templates        — create (admin only)
//	PATCH  /api/reports/templates/{id}   — rename/update (admin only)
//	DELETE /api/reports/templates/{id}   — delete (admin only)
//
// RBAC is enforced at the router layer via RequireAction middleware. Handler-level
// auth.GetUser is used for audit attribution only (not re-authorization).
//
// Threat model:
//   - T-07-11a-01: state JSONB is stored parameterized; never interpolated into SQL.
//   - T-07-11a-02: ActionReportTemplateCreate/Update/Delete are admin-only (RequireAction
//     blocks before handler runs; viewer gets 403).
//   - T-07-11a-03: Delete writes audit row in same tx (TemplateStore.Delete guarantee).
//   - T-07-11a-04: UNIQUE constraint on name → 23505 → 409 (last-write-wins accepted).

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/report"
)

// ReportTemplateDeps bundles the dependencies for the report template HTTP handlers.
type ReportTemplateDeps struct {
	Pool       *pgxpool.Pool
	SessionMgr *scs.SessionManager
}

// RegisterReportTemplateRoutes mounts the 5 report template endpoints with RBAC
// middleware. Admin has all 5 actions; viewer has only the 2 read endpoints.
func RegisterReportTemplateRoutes(r chi.Router, deps ReportTemplateDeps) {
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionReportTemplateRead))
		rt.Get("/api/reports/templates", ListReportTemplatesHandler(deps))
		rt.Get("/api/reports/templates/{id}", GetReportTemplateHandler(deps))
	})
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionReportTemplateCreate))
		rt.Post("/api/reports/templates", CreateReportTemplateHandler(deps))
	})
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionReportTemplateUpdate))
		rt.Patch("/api/reports/templates/{id}", UpdateReportTemplateHandler(deps))
	})
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionReportTemplateDelete))
		rt.Delete("/api/reports/templates/{id}", DeleteReportTemplateHandler(deps))
	})
}

// ---- response shape --------------------------------------------------------

type templateResponse struct {
	ID          uuid.UUID       `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	State       json.RawMessage `json:"state"`
}

func toTemplateResponse(r report.TemplateRow) templateResponse {
	return templateResponse{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		State:       r.State,
	}
}

// ---- request shapes --------------------------------------------------------

type createTemplateRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	State       json.RawMessage `json:"state"`
}

type updateTemplateRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	State       json.RawMessage `json:"state"`
}

// ---- handlers --------------------------------------------------------------

// ListReportTemplatesHandler handles GET /api/reports/templates.
// Returns the full list of saved templates ordered alphabetically (case-insensitive).
// Accessible to admin + viewer (ActionReportTemplateRead).
func ListReportTemplatesHandler(deps ReportTemplateDeps) http.HandlerFunc {
	store := report.NewTemplateStore(deps.Pool)
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := store.List(r.Context())
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "db error")
			return
		}
		resp := make([]templateResponse, len(rows))
		for i, row := range rows {
			resp[i] = toTemplateResponse(row)
		}
		writeAPIJSON(w, http.StatusOK, resp)
	}
}

// GetReportTemplateHandler handles GET /api/reports/templates/{id}.
// Returns a single template by UUID or 404 if not found.
// Accessible to admin + viewer (ActionReportTemplateRead).
func GetReportTemplateHandler(deps ReportTemplateDeps) http.HandlerFunc {
	store := report.NewTemplateStore(deps.Pool)
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid id")
			return
		}
		row, err := store.Get(r.Context(), id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) || isChainedNoRows(err) {
				writeAPIError(w, http.StatusNotFound, "not found")
				return
			}
			writeAPIError(w, http.StatusInternalServerError, "db error")
			return
		}
		writeAPIJSON(w, http.StatusOK, toTemplateResponse(row))
	}
}

// CreateReportTemplateHandler handles POST /api/reports/templates.
// Creates a new saved template + writes audit row (same tx, D-23).
// Returns 201 on success, 409 on duplicate name (T-07-11a-04).
// Admin-only (ActionReportTemplateCreate; T-07-11a-02).
func CreateReportTemplateHandler(deps ReportTemplateDeps) http.HandlerFunc {
	store := report.NewTemplateStore(deps.Pool)
	return func(w http.ResponseWriter, r *http.Request) {
		var req createTemplateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.Name == "" {
			writeAPIError(w, http.StatusBadRequest, "name is required")
			return
		}
		if len(req.State) == 0 {
			writeAPIError(w, http.StatusBadRequest, "state is required")
			return
		}

		actor, _ := auth.GetUser(r.Context(), deps.SessionMgr)
		actorID, _ := uuid.Parse(actor.ID)

		created, err := store.Save(r.Context(), req.Name, req.Description, req.State, actorID)
		if err != nil {
			if isUniqueViolation(err) {
				writeAPIError(w, http.StatusConflict, "name already exists")
				return
			}
			writeAPIError(w, http.StatusInternalServerError, "db error")
			return
		}
		writeAPIJSON(w, http.StatusCreated, toTemplateResponse(created))
	}
}

// UpdateReportTemplateHandler handles PATCH /api/reports/templates/{id}.
// Updates name, description, and state + writes audit row (same tx, D-23).
// Returns 409 on rename collision (T-07-11a-04), 404 on missing template.
// Admin-only (ActionReportTemplateUpdate; T-07-11a-02).
func UpdateReportTemplateHandler(deps ReportTemplateDeps) http.HandlerFunc {
	store := report.NewTemplateStore(deps.Pool)
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid id")
			return
		}
		var req updateTemplateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.Name == "" {
			writeAPIError(w, http.StatusBadRequest, "name is required")
			return
		}
		if len(req.State) == 0 {
			writeAPIError(w, http.StatusBadRequest, "state is required")
			return
		}

		actor, _ := auth.GetUser(r.Context(), deps.SessionMgr)
		actorID, _ := uuid.Parse(actor.ID)

		if err := store.Update(r.Context(), id, req.Name, req.Description, req.State, actorID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) || isChainedNoRows(err) {
				writeAPIError(w, http.StatusNotFound, "not found")
				return
			}
			if isUniqueViolation(err) {
				writeAPIError(w, http.StatusConflict, "name already exists")
				return
			}
			writeAPIError(w, http.StatusInternalServerError, "db error")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// DeleteReportTemplateHandler handles DELETE /api/reports/templates/{id}.
// Hard-deletes the template + writes audit row (same tx, D-23; T-07-11a-03).
// Returns 204 on success, 404 when template does not exist.
// Admin-only (ActionReportTemplateDelete; T-07-11a-02).
func DeleteReportTemplateHandler(deps ReportTemplateDeps) http.HandlerFunc {
	store := report.NewTemplateStore(deps.Pool)
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid id")
			return
		}

		actor, _ := auth.GetUser(r.Context(), deps.SessionMgr)
		actorID, _ := uuid.Parse(actor.ID)

		if err := store.Delete(r.Context(), id, actorID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) || isChainedNoRows(err) {
				writeAPIError(w, http.StatusNotFound, "not found")
				return
			}
			writeAPIError(w, http.StatusInternalServerError, "db error")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ---- helpers ---------------------------------------------------------------

// writeAPIError writes a JSON {"error": "..."} response with the given status.
func writeAPIError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// writeAPIJSON writes a JSON response with the given status.
func writeAPIJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// isUniqueViolation returns true if err (or any wrapped error) is a PostgreSQL
// unique_violation (SQLSTATE 23505). Used to map name collisions to HTTP 409.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

// isChainedNoRows unwraps an error chain looking for pgx.ErrNoRows.
// Used because the store wraps ErrNoRows with fmt.Errorf %w.
func isChainedNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
