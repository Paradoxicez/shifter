package floorplan

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/shifter-io/shifter/internal/auth"
)

// ServeImageHandler handles GET /api/floor-plans/{id}/image.
//
// Returns the raw image bytes with the correct Content-Type (image/png or
// image/jpeg). Only authenticated requests are served (T-05-07-02):
//   - The handler re-asserts auth.GetUser to defend against routing misconfig,
//     even when the route is mounted inside an authenticated chi middleware group.
//
// Path traversal mitigation (T-05-07-03):
//   - The `id` URL param is UUID-parsed BEFORE any filepath operation.
//   - image_path is a server-generated relative path (`<uuid>.<ext>` from plan
//     05-05 UploadImageHandler) — no client bytes reach the filesystem call.
//   - filepath.Clean + strings.HasPrefix containment check catches any DB row
//     with a malicious image_path (e.g. seeded directly via SQL).
func ServeImageHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// T-05-07-02: Auth gate — re-assert even inside an auth middleware group.
		if _, ok := auth.GetUser(r.Context(), deps.SessionMgr); !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id")
			return
		}

		plan, err := deps.Queries.GetFloorPlan(r.Context(), pgtype.UUID{Bytes: id, Valid: true})
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "plan_not_found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "plan_lookup_failed")
			return
		}

		// T-05-07-03: compose absolute path then verify containment within ImageRoot.
		absPath := filepath.Join(deps.ImageRoot, plan.ImagePath)
		cleaned := filepath.Clean(absPath)
		imageRootClean := filepath.Clean(deps.ImageRoot)
		if !strings.HasPrefix(cleaned, imageRootClean+string(filepath.Separator)) {
			// Should never happen — image_path is server-generated `<uuid>.<ext>`.
			// Defensive catch-all for any DB row with a malicious path.
			writeError(w, http.StatusBadRequest, "invalid_image_path")
			return
		}

		f, err := os.Open(cleaned)
		if err != nil {
			writeError(w, http.StatusNotFound, "image_missing")
			return
		}
		defer f.Close()

		// Sniff Content-Type from first 512 bytes (validated on upload too — double check).
		sniffBuf := make([]byte, 512)
		n, _ := f.Read(sniffBuf)
		sniffBuf = sniffBuf[:n]
		ct := http.DetectContentType(sniffBuf)
		switch ct {
		case "image/png", "image/jpeg":
			w.Header().Set("Content-Type", ct)
		default:
			writeError(w, http.StatusInternalServerError, "bad_content_type")
			return
		}

		w.Header().Set("Cache-Control", "private, max-age=600") // 10-min client cache

		// Seek back to start after sniff read.
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			writeError(w, http.StatusInternalServerError, "seek_failed")
			return
		}
		_, _ = io.Copy(w, f)
	}
}
