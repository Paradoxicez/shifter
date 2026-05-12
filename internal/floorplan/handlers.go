package floorplan

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

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

// Deps bundles the shared infrastructure a floorplan handler call needs.
// Plan 05-07 wiring (cmd/serve) constructs a Deps and passes it to RegisterRoutes.
type Deps struct {
	Pool       *pgxpool.Pool
	Queries    *sqlc.Queries
	SessionMgr *scs.SessionManager
	// ImageRoot is the absolute path to the floor-plans volume on disk
	// (e.g. /var/lib/shifter/floor-plans, or t.TempDir() in tests).
	// image_path rows store the RELATIVE path; the handler joins ImageRoot + relPath.
	ImageRoot string
}

// errorResp is the standard JSON error envelope.
type errorResp struct {
	Error  string `json:"error"`
	Detail string `json:"detail,omitempty"`
}

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResp{Error: code})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// userUUID parses a session User.ID string (UUID form) into a uuid.UUID.
// Returns uuid.Nil on parse failure, which WriteEntry encodes as SQL NULL
// (acceptable for system-level fallback; session middleware guarantees
// User.ID is always a well-formed UUID string for authenticated calls).
func userUUID(id string) uuid.UUID {
	u, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil
	}
	return u
}

// UploadImageHandler handles POST /api/sites/{siteID}/floor-plans.
//
// Multipart form fields:
//   - file     (required) — image bytes
//   - label    (required) — human name (e.g. "Ground Floor")
//   - sort_order (optional) — integer; defaults to max existing + 1 within site
//
// Validation order (T-05-05-01 disk-exhaustion defence):
//  1. http.MaxBytesReader caps body at MaxUploadBytes (10 MiB)
//  2. ParseMultipartForm — parse after cap
//  3. ValidateImageHeader — sniff MIME, reject PDF (D-17), reject non-PNG/JPEG
//  4. ProbeDimensions — read image header only, reject > 8192×8192 (D-19)
//  5. Write to disk (ImageRoot/<uuid>.<ext>)
//  6. INSERT floor_plan + audit.WriteEntry in the same pgx.Tx (D-23)
func UploadImageHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		user, ok := auth.GetUser(ctx, deps.SessionMgr)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		siteID, err := uuid.Parse(chi.URLParam(r, "siteID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_site_id")
			return
		}

		// Step 1: cap body before any read.
		r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBytes)

		// Step 2: parse multipart.
		if err := r.ParseMultipartForm(MaxUploadBytes); err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				writeError(w, http.StatusRequestEntityTooLarge, "file_too_large")
				return
			}
			writeError(w, http.StatusBadRequest, "multipart_parse_failed")
			return
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "file_missing")
			return
		}
		defer file.Close()

		label := r.FormValue("label")
		if label == "" {
			writeError(w, http.StatusUnprocessableEntity, "label_required")
			return
		}

		// Step 3: MIME sniff — sniffed type drives ext, not Content-Type header.
		mime, body1, err := ValidateImageHeader(file)
		if err != nil {
			if errors.Is(err, ErrUnsupportedMIME) {
				writeError(w, http.StatusUnsupportedMediaType, "unsupported_mime_type")
			} else {
				writeError(w, http.StatusBadRequest, "image_validation_failed")
			}
			return
		}

		// Step 4: dimension probe — header-only, no full pixel decode.
		width, height, body2, err := ProbeDimensions(body1)
		if err != nil {
			switch {
			case errors.Is(err, ErrDimensionTooLarge):
				writeError(w, http.StatusUnprocessableEntity, "dimensions_too_large")
			case errors.Is(err, ErrImageDecodeFailed):
				writeError(w, http.StatusUnprocessableEntity, "image_decode_failed")
			default:
				writeError(w, http.StatusBadRequest, "image_validation_failed")
			}
			return
		}

		// Step 5: write to disk. UUID is server-generated (T-05-05-03 path traversal defence).
		planID := uuid.New()
		ext := extFromMIME(mime)
		relPath := fmt.Sprintf("%s.%s", planID.String(), ext)
		absPath := filepath.Join(deps.ImageRoot, relPath)

		out, err := os.Create(absPath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "fs_create_failed")
			return
		}
		if _, err := io.Copy(out, body2); err != nil {
			out.Close()
			os.Remove(absPath)
			writeError(w, http.StatusInternalServerError, "fs_write_failed")
			return
		}
		out.Close()

		// Step 6: INSERT floor_plan + audit.WriteEntry in a single tx (D-23).
		sortOrder := parseSortOrderOrNext(r.FormValue("sort_order"))

		tx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		if err != nil {
			os.Remove(absPath)
			writeError(w, http.StatusInternalServerError, "tx_begin")
			return
		}
		rolledBack := true
		defer func() {
			if rolledBack {
				tx.Rollback(ctx)
				os.Remove(absPath)
			}
		}()

		q := deps.Queries.WithTx(tx)
		plan, err := q.CreateFloorPlan(ctx, sqlc.CreateFloorPlanParams{
			ID:        pgtype.UUID{Bytes: planID, Valid: true},
			SiteID:    pgtype.UUID{Bytes: siteID, Valid: true},
			Label:     label,
			SortOrder: sortOrder,
			ImagePath: relPath,
			ImageW:    int32(width),
			ImageH:    int32(height),
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "db_create_failed")
			return
		}

		if err := audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     userUUID(user.ID),
			Action:     audit.ActionFloorPlanUpload,
			EntityType: audit.EntityTypeFloorPlan,
			EntityID:   planID,
			After: map[string]any{
				"site_id":    siteID.String(),
				"label":      label,
				"image_w":    width,
				"image_h":    height,
				"image_path": relPath,
			},
			RequestID: middleware.GetReqID(ctx),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "audit_failed")
			return
		}

		if err := tx.Commit(ctx); err != nil {
			writeError(w, http.StatusInternalServerError, "tx_commit")
			return
		}
		rolledBack = false

		writeJSON(w, http.StatusCreated, plan)
	}
}

// ListBySiteHandler handles GET /api/sites/{siteID}/floor-plans.
// Returns all floor_plan rows for the site ordered by sort_order, uploaded_at.
func ListBySiteHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		siteID, err := uuid.Parse(chi.URLParam(r, "siteID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_site_id")
			return
		}

		plans, err := deps.Queries.ListFloorPlansBySite(ctx, pgtype.UUID{Bytes: siteID, Valid: true})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "db_query_failed")
			return
		}
		if plans == nil {
			plans = []sqlc.FloorPlan{}
		}
		writeJSON(w, http.StatusOK, plans)
	}
}

// GetHandler handles GET /api/floor-plans/{id}.
func GetHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id")
			return
		}

		plan, err := deps.Queries.GetFloorPlan(ctx, pgtype.UUID{Bytes: id, Valid: true})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusNotFound, "not_found")
			} else {
				writeError(w, http.StatusInternalServerError, "db_query_failed")
			}
			return
		}
		writeJSON(w, http.StatusOK, plan)
	}
}

// ReplaceImageHandler handles PATCH /api/floor-plans/{id} (multipart).
// Replaces the stored image while keeping all device_floor_plan_placement
// rows intact — fractional coords survive (D-24). Same validation pipeline
// as UploadImageHandler. On commit the OLD image file is unlinked; on
// rollback the NEW image file is unlinked.
func ReplaceImageHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		user, ok := auth.GetUser(ctx, deps.SessionMgr)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBytes)
		if err := r.ParseMultipartForm(MaxUploadBytes); err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				writeError(w, http.StatusRequestEntityTooLarge, "file_too_large")
				return
			}
			writeError(w, http.StatusBadRequest, "multipart_parse_failed")
			return
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "file_missing")
			return
		}
		defer file.Close()

		mime, body1, err := ValidateImageHeader(file)
		if err != nil {
			if errors.Is(err, ErrUnsupportedMIME) {
				writeError(w, http.StatusUnsupportedMediaType, "unsupported_mime_type")
			} else {
				writeError(w, http.StatusBadRequest, "image_validation_failed")
			}
			return
		}

		width, height, body2, err := ProbeDimensions(body1)
		if err != nil {
			switch {
			case errors.Is(err, ErrDimensionTooLarge):
				writeError(w, http.StatusUnprocessableEntity, "dimensions_too_large")
			case errors.Is(err, ErrImageDecodeFailed):
				writeError(w, http.StatusUnprocessableEntity, "image_decode_failed")
			default:
				writeError(w, http.StatusBadRequest, "image_validation_failed")
			}
			return
		}

		// Fetch existing plan to capture oldImagePath for audit + disk unlink.
		existing, err := deps.Queries.GetFloorPlan(ctx, pgtype.UUID{Bytes: id, Valid: true})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusNotFound, "not_found")
			} else {
				writeError(w, http.StatusInternalServerError, "db_query_failed")
			}
			return
		}
		oldRelPath := existing.ImagePath

		// Write new file.
		newExt := extFromMIME(mime)
		newRelPath := fmt.Sprintf("%s.%s", uuid.New().String(), newExt)
		newAbsPath := filepath.Join(deps.ImageRoot, newRelPath)

		out, err := os.Create(newAbsPath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "fs_create_failed")
			return
		}
		if _, err := io.Copy(out, body2); err != nil {
			out.Close()
			os.Remove(newAbsPath)
			writeError(w, http.StatusInternalServerError, "fs_write_failed")
			return
		}
		out.Close()

		tx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		if err != nil {
			os.Remove(newAbsPath)
			writeError(w, http.StatusInternalServerError, "tx_begin")
			return
		}
		rolledBack := true
		defer func() {
			if rolledBack {
				tx.Rollback(ctx)
				os.Remove(newAbsPath)
			}
		}()

		q := deps.Queries.WithTx(tx)
		updated, err := q.UpdateFloorPlanImage(ctx, sqlc.UpdateFloorPlanImageParams{
			ID:        pgtype.UUID{Bytes: id, Valid: true},
			ImagePath: newRelPath,
			ImageW:    int32(width),
			ImageH:    int32(height),
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "db_update_failed")
			return
		}

		if err := audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     userUUID(user.ID),
			Action:     audit.ActionFloorPlanReplace,
			EntityType: audit.EntityTypeFloorPlan,
			EntityID:   id,
			Before:     map[string]any{"image_path": oldRelPath},
			After:      map[string]any{"image_path": newRelPath, "image_w": width, "image_h": height},
			RequestID:  middleware.GetReqID(ctx),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "audit_failed")
			return
		}

		if err := tx.Commit(ctx); err != nil {
			writeError(w, http.StatusInternalServerError, "tx_commit")
			return
		}
		rolledBack = false

		// Commit succeeded — unlink the old image file.
		os.Remove(filepath.Join(deps.ImageRoot, oldRelPath))

		writeJSON(w, http.StatusOK, updated)
	}
}

// RenameHandler handles PATCH /api/floor-plans/{id}/label (JSON body).
// Updates label and/or sort_order; returns 409 on UNIQUE(site_id, sort_order) conflict.
func RenameHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		user, ok := auth.GetUser(ctx, deps.SessionMgr)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id")
			return
		}

		var body struct {
			Label     string `json:"label"`
			SortOrder *int32 `json:"sort_order,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json")
			return
		}
		if body.Label == "" {
			writeError(w, http.StatusUnprocessableEntity, "label_required")
			return
		}

		// Fetch current sort_order if not provided.
		var sortOrder int32
		if body.SortOrder != nil {
			sortOrder = *body.SortOrder
		} else {
			existing, err := deps.Queries.GetFloorPlan(ctx, pgtype.UUID{Bytes: id, Valid: true})
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					writeError(w, http.StatusNotFound, "not_found")
				} else {
					writeError(w, http.StatusInternalServerError, "db_query_failed")
				}
				return
			}
			sortOrder = existing.SortOrder
		}

		tx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "tx_begin")
			return
		}
		defer tx.Rollback(ctx)

		q := deps.Queries.WithTx(tx)
		updated, err := q.UpdateFloorPlanLabel(ctx, sqlc.UpdateFloorPlanLabelParams{
			ID:        pgtype.UUID{Bytes: id, Valid: true},
			Label:     body.Label,
			SortOrder: sortOrder,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "db_update_failed")
			return
		}

		if err := audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     userUUID(user.ID),
			Action:     audit.ActionFloorPlanRename,
			EntityType: audit.EntityTypeFloorPlan,
			EntityID:   id,
			After:      map[string]any{"label": body.Label, "sort_order": sortOrder},
			RequestID:  middleware.GetReqID(ctx),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "audit_failed")
			return
		}

		if err := tx.Commit(ctx); err != nil {
			writeError(w, http.StatusInternalServerError, "tx_commit")
			return
		}

		writeJSON(w, http.StatusOK, updated)
	}
}

// DeleteHandler handles DELETE /api/floor-plans/{id}.
// Cascades to device_floor_plan_placement via DB ON DELETE CASCADE (0033).
// Audit row written in the same tx; image file unlinked after commit.
func DeleteHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		user, ok := auth.GetUser(ctx, deps.SessionMgr)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id")
			return
		}

		// Fetch existing to get image path for disk cleanup.
		existing, err := deps.Queries.GetFloorPlan(ctx, pgtype.UUID{Bytes: id, Valid: true})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusNotFound, "not_found")
			} else {
				writeError(w, http.StatusInternalServerError, "db_query_failed")
			}
			return
		}
		relPath := existing.ImagePath

		tx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "tx_begin")
			return
		}
		committed := false
		defer func() {
			if !committed {
				tx.Rollback(ctx)
			}
		}()

		q := deps.Queries.WithTx(tx)
		if err := q.DeleteFloorPlan(ctx, pgtype.UUID{Bytes: id, Valid: true}); err != nil {
			writeError(w, http.StatusInternalServerError, "db_delete_failed")
			return
		}

		if err := audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     userUUID(user.ID),
			Action:     audit.ActionFloorPlanDelete,
			EntityType: audit.EntityTypeFloorPlan,
			EntityID:   id,
			Before:     map[string]any{"image_path": relPath},
			RequestID:  middleware.GetReqID(ctx),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "audit_failed")
			return
		}

		if err := tx.Commit(ctx); err != nil {
			writeError(w, http.StatusInternalServerError, "tx_commit")
			return
		}
		committed = true

		// Unlink the image file after successful commit.
		os.Remove(filepath.Join(deps.ImageRoot, relPath))

		w.WriteHeader(http.StatusNoContent)
	}
}

// parseSortOrderOrNext parses the sort_order form value, defaulting to 0 if
// absent or invalid. The UNIQUE(site_id, sort_order) constraint surfaces
// duplicates as 23505 at insert time; the handler can return 409 then.
func parseSortOrderOrNext(raw string) int32 {
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0
	}
	return int32(n)
}
