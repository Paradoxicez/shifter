package backup

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/shifter-io/shifter/internal/auth"
)

// Deps groups the dependencies the backup HTTP handlers need.
type Deps struct {
	Runner     *Runner
	Store      *Store
	SessionMgr *scs.SessionManager
	Log        *slog.Logger
}

// RegisterRoutes mounts the four /api/backup/* routes onto the chi Router.
//
//	GET  /api/backup/list        — ActionBackupRead (admin + viewer per D-46)
//	GET  /api/backup/last        — ActionBackupRead (admin + viewer)
//	GET  /api/backup/jobs/{id}   — ActionBackupRead (admin + viewer)
//	POST /api/backup/run-now     — ActionBackupRun  (admin only; T-06-08-01)
func RegisterRoutes(r chi.Router, d Deps) {
	r.Route("/api/backup", func(rt chi.Router) {
		rt.With(auth.RequireAction(d.SessionMgr, auth.ActionBackupRead)).
			Get("/list", ListRecentHandler(d))
		rt.With(auth.RequireAction(d.SessionMgr, auth.ActionBackupRead)).
			Get("/last", LastHandler(d))
		rt.With(auth.RequireAction(d.SessionMgr, auth.ActionBackupRead)).
			Get("/jobs/{id}", GetJobHandler(d))
		rt.With(auth.RequireAction(d.SessionMgr, auth.ActionBackupRun)).
			Post("/run-now", RunNowHandler(d))
	})
}

// RunNowHandler handles POST /api/backup/run-now.
//
// It kicks off Runner.Backup in a background goroutine and returns 202
// immediately.  Because we need the backup_run.id synchronously (to give
// the caller something to poll), the handler uses a channel to receive the
// id that Runner.Backup creates inside its first transaction, then returns
// 202 once the id is available.
//
// T-06-08-01: ActionBackupRun is admin-only; viewer POST → 403 enforced by
// RequireAction in RegisterRoutes.
func RunNowHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.GetUser(r.Context(), d.SessionMgr)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		uid := uuid.MustParse(user.ID)
		dest := DefaultBackupDir

		// idCh receives the backup_run.id as soon as Runner inserts the
		// 'running' row (before pg_dump starts), so we can return 202 promptly.
		idCh := make(chan uuid.UUID, 1)

		go func() {
			// Use context.Background() so HTTP request cancellation does not
			// abort a backup that has already started dumping to disk.
			bgCtx := r.Context()
			backupID, _, err := d.Runner.BackupWithNotify(bgCtx, dest, "api", &uid, idCh)
			if err != nil {
				d.Log.Error("backup/run-now: backup failed", "err", err, "run_id", backupID)
			}
		}()

		// Wait for the run ID (created before pg_dump starts; should be fast).
		select {
		case backupID := <-idCh:
			writeJSON(w, http.StatusAccepted, map[string]any{
				"job_id": backupID.String(),
				"status": "running",
			})
		case <-r.Context().Done():
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "request cancelled"})
		}
	}
}

// ListRecentHandler handles GET /api/backup/list?limit=5.
func ListRecentHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 5
		rows, err := d.Store.ListRecent(r.Context(), limit)
		if err != nil {
			d.Log.Error("backup/list: ListRecent", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, toAPIRows(rows))
	}
}

// LastHandler handles GET /api/backup/last.
//
// Returns {never_run: true} when no backups have run yet (SETT-05 freshness
// dot needs to handle the cold-start state).
func LastHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		row, err := d.Store.Last(r.Context())
		if err != nil {
			d.Log.Error("backup/last: Last", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		if row == nil {
			writeJSON(w, http.StatusOK, map[string]any{"never_run": true})
			return
		}
		out := toAPIRow(*row)
		out["age_seconds"] = int64(time.Since(row.StartedAt).Seconds())
		writeJSON(w, http.StatusOK, out)
	}
}

// GetJobHandler handles GET /api/backup/jobs/{id}.
func GetJobHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		id, err := uuid.Parse(idStr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
			return
		}
		row, err := d.Store.Get(r.Context(), id)
		if err != nil {
			d.Log.Error("backup/jobs: Get", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		if row == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusOK, toAPIRow(*row))
	}
}

// toAPIRows converts a slice of BackupRunRow to the API wire shape.
func toAPIRows(rows []BackupRunRow) []map[string]any {
	out := make([]map[string]any, len(rows))
	for i, r := range rows {
		out[i] = toAPIRow(r)
	}
	return out
}

func toAPIRow(r BackupRunRow) map[string]any {
	m := map[string]any{
		"id":              r.ID.String(),
		"trigger_kind":    r.TriggerKind,
		"status":          r.Status,
		"destination_dir": r.DestinationDir,
		"started_at":      r.StartedAt.Format(time.RFC3339),
	}
	if r.FileName != nil {
		m["file_name"] = *r.FileName
	}
	if r.FileSizeBytes != nil {
		m["file_size_bytes"] = *r.FileSizeBytes
	}
	if r.SHA256 != nil {
		m["sha256"] = *r.SHA256
	}
	if r.FinishedAt != nil {
		m["finished_at"] = r.FinishedAt.Format(time.RFC3339)
	}
	if r.ErrorMessage != nil {
		m["error_message"] = *r.ErrorMessage
	}
	if r.ChirpStackMode != nil {
		m["chirpstack_mode"] = *r.ChirpStackMode
	}
	if r.SchemaVersion != nil {
		m["schema_version"] = *r.SchemaVersion
	}
	return m
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
