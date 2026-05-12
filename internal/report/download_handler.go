package report

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/shifter-io/shifter/internal/auth"
)

// DownloadHandler serves GET /api/reports/{id}/file/{kind}.
//
// kind must be one of {csv, xlsx, pdf}. The handler:
//  1. Validates the session (401 if unauthenticated).
//  2. Parses the UUID BEFORE any path construction (T-05-06-02 path-traversal defense).
//  3. Validates the kind against a static allowlist.
//  4. Looks up the report row; 404 if not found or expired.
//  5. 425 (Too Early) if kind=pdf and pdf_status != 'ready'.
//  6. Streams the file with correct Content-Type and Content-Disposition.
func DownloadHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Auth gate — same gating as StatusHandler (viewers see own, admins all).
		user, ok := auth.GetUser(r.Context(), deps.SessionMgr)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		// UUID validated BEFORE filepath.Join (T-05-06-02 mitigation).
		idStr := chi.URLParam(r, "id")
		id, err := uuid.Parse(idStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_report_id")
			return
		}

		// kind validated against static allowlist.
		kind := chi.URLParam(r, "kind")
		ext, contentType, ok := kindToExt(kind)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid_kind")
			return
		}

		pgID := uuidToPgtype(id)
		plan, err := deps.Queries.GetReport(r.Context(), pgID)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "report_not_found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup_failed")
			return
		}

		// 404 for expired reports (D-07).
		if time.Now().After(plan.ExpiresAt.Time) || plan.PdfStatus == "expired" {
			writeError(w, http.StatusNotFound, "report_expired")
			return
		}

		// Auth scope: viewers see their own reports only; admins see all.
		// 404 (not 403) to avoid disclosing existence (T-05-06-04 pattern).
		if user.Role != "admin" && plan.UserID != uuidToPgtype(mustParseUUID(user.ID)) {
			writeError(w, http.StatusNotFound, "report_not_found")
			return
		}

		// 425 Too Early if PDF not ready.
		if kind == "pdf" && plan.PdfStatus != "ready" {
			writeError(w, http.StatusTooEarly, "pdf_not_ready")
			return
		}

		// Build path from server-controlled artifact_dir + allowlisted extension.
		// Client provides only the UUID (validated above) and kind (allowlisted above).
		// No client-controlled string segment in the path (T-05-06-02).
		absPath := filepath.Join(plan.ArtifactDir, fmt.Sprintf("report.%s", ext))
		f, err := os.Open(absPath) //nolint:gosec // path is fully server-controlled
		if err != nil {
			writeError(w, http.StatusNotFound, "file_missing")
			return
		}
		defer f.Close()

		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Disposition",
			fmt.Sprintf(`attachment; filename="shifter-report-%s.%s"`, id.String()[:8], ext))
		_, _ = io.Copy(w, f)
	}
}

// kindToExt maps a kind string to (file extension, MIME content-type).
// Returns ok=false for any kind not in the allowlist.
func kindToExt(kind string) (ext, contentType string, ok bool) {
	switch kind {
	case "csv":
		return "csv", "text/csv; charset=utf-8", true
	case "xlsx":
		return "xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", true
	case "pdf":
		return "pdf", "application/pdf", true
	default:
		return "", "", false
	}
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

// uuidToPgtype converts uuid.UUID to pgtype.UUID.
func uuidToPgtype(id uuid.UUID) pgtype.UUID {
	return toNullableUUID(id)
}
