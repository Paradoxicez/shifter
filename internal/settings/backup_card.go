// Package settings — backup status + threshold handlers (Plan 06-10 / SETT-05).
//
// GET  /api/settings/backup           — admin + viewer (ActionBackupRead)
// PATCH /api/settings/backup/thresholds — admin only   (ActionBackupConfigure)
//
// Reads backup freshness thresholds from retention_config (columns added by
// migration 0047_backup_thresholds) and backup history from the backup_run
// table (Plan 06-08). The freshness-dot color logic lives in the frontend;
// this handler provides the raw age_seconds + threshold values.

package settings

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/shifter-io/shifter/internal/audit"
	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/backup"
)

// BackupCardConfig holds the operator-level configuration the backup card needs
// (primarily the destination directory). Sourced from cfg.BackupDir at wire-up.
type BackupCardConfig struct {
	BackupDir string
}

// BackupStatusResponse is the wire shape for GET /api/settings/backup.
type BackupStatusResponse struct {
	NeverRun           bool            `json:"never_run"`
	Last               *BackupSummary  `json:"last,omitempty"`
	Recent             []BackupSummary `json:"recent"`
	WarnThresholdHours int32           `json:"warn_threshold_hours"`
	CritThresholdHours int32           `json:"crit_threshold_hours"`
	DestinationDir     string          `json:"destination_dir"`
}

// BackupSummary is the per-run wire shape used in both .Last and .Recent.
type BackupSummary struct {
	ID          string  `json:"id"`
	FileName    string  `json:"file_name"`
	SizeBytes   int64   `json:"size_bytes"`
	SHA256      string  `json:"sha256"`
	StartedAt   string  `json:"started_at"`
	FinishedAt  *string `json:"finished_at"`
	Status      string  `json:"status"`
	AgeSeconds  int64   `json:"age_seconds"`
	TriggerKind string  `json:"trigger_kind"`
}

// BackupThresholdsPatch is the wire shape for PATCH /api/settings/backup/thresholds.
type BackupThresholdsPatch struct {
	WarnThresholdHours *int32 `json:"warn_threshold_hours"`
	CritThresholdHours *int32 `json:"crit_threshold_hours"`
}

// RegisterBackupRoutes mounts the backup settings endpoints on r.
//
//	GET   /api/settings/backup              — admin + viewer (ActionBackupRead)
//	PATCH /api/settings/backup/thresholds   — admin only    (ActionBackupConfigure)
func RegisterBackupRoutes(r chi.Router, deps Deps, sm *scs.SessionManager, store *backup.Store, cfg BackupCardConfig) {
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(sm, auth.ActionBackupRead))
		rt.Get("/api/settings/backup", GetBackupStatusHandler(deps, store, cfg))
	})
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(sm, auth.ActionBackupConfigure))
		rt.Patch("/api/settings/backup/thresholds", PatchBackupThresholdsHandler(deps, sm))
	})
}

// GetBackupStatusHandler serves GET /api/settings/backup.
// Admin + viewer (ActionBackupRead — viewer sees status + history but cannot
// mutate thresholds or trigger a backup; D-46 / T-06-10-04).
func GetBackupStatusHandler(deps Deps, store *backup.Store, cfg BackupCardConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Read thresholds from retention_config singleton.
		row, err := deps.Queries.GetRetentionConfig(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "load_failed")
			return
		}

		// Read recent backup runs (up to 5).
		recent, err := store.ListRecent(r.Context(), 5)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list_failed")
			return
		}

		now := time.Now().UTC()

		// Build response.
		resp := BackupStatusResponse{
			WarnThresholdHours: row.BackupWarnThresholdHours,
			CritThresholdHours: row.BackupCritThresholdHours,
			DestinationDir:     cfg.BackupDir,
			Recent:             make([]BackupSummary, 0, len(recent)),
		}

		if len(recent) == 0 {
			resp.NeverRun = true
		} else {
			// Most recent row is first (ListRecent orders by started_at DESC).
			resp.Last = toBackupSummary(recent[0], now)
		}

		for _, row := range recent {
			resp.Recent = append(resp.Recent, *toBackupSummary(row, now))
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// PatchBackupThresholdsHandler serves PATCH /api/settings/backup/thresholds.
// Admin-only (ActionBackupConfigure / T-06-10-02).
//
// Validates: warn > 0 && crit > 0 && warn < crit && both <= 8760.
// Persists in retention_config in a SERIALIZABLE tx; writes audit row.
func PatchBackupThresholdsHandler(deps Deps, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.GetUser(r.Context(), sm)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if !auth.Can(&user, auth.ActionBackupConfigure, nil) {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}

		var patch BackupThresholdsPatch
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json")
			return
		}
		if err := validateBackupThresholds(patch); err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}

		tx, err := deps.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "tx_begin")
			return
		}
		defer tx.Rollback(r.Context()) //nolint:errcheck

		// Snapshot before-state for audit diff.
		var beforeWarn, beforeCrit int32
		if err := tx.QueryRow(r.Context(),
			`SELECT backup_warn_threshold_hours, backup_crit_threshold_hours
			 FROM retention_config WHERE id = 1`).
			Scan(&beforeWarn, &beforeCrit); err != nil {
			writeError(w, http.StatusInternalServerError, "load_failed")
			return
		}

		// Apply patch values (only supplied fields; default to before-values).
		newWarn, newCrit := beforeWarn, beforeCrit
		if patch.WarnThresholdHours != nil {
			newWarn = *patch.WarnThresholdHours
		}
		if patch.CritThresholdHours != nil {
			newCrit = *patch.CritThresholdHours
		}

		// Re-validate after merging with existing values (handles partial patch).
		merged := BackupThresholdsPatch{WarnThresholdHours: &newWarn, CritThresholdHours: &newCrit}
		if err := validateBackupThresholds(merged); err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}

		if _, err := tx.Exec(r.Context(),
			`UPDATE retention_config
			 SET backup_warn_threshold_hours = $1,
			     backup_crit_threshold_hours = $2,
			     updated_at = now()
			 WHERE id = 1`,
			newWarn, newCrit,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "update_failed")
			return
		}

		// Audit entry.
		uid := uuid.MustParse(user.ID)
		beforeMap := map[string]any{
			"backup_warn_threshold_hours": beforeWarn,
			"backup_crit_threshold_hours": beforeCrit,
		}
		afterMap := map[string]any{
			"backup_warn_threshold_hours": newWarn,
			"backup_crit_threshold_hours": newCrit,
		}
		if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
			UserID:     uid,
			Action:     audit.ActionRetentionChange,
			EntityType: audit.EntityTypeRetention,
			EntityID:   uuid.Nil,
			Before:     beforeMap,
			After:      afterMap,
			RequestID:  middleware.GetReqID(r.Context()),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "audit_failed")
			return
		}

		if err := tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "tx_commit")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"warn_threshold_hours": newWarn,
			"crit_threshold_hours": newCrit,
		})
	}
}

// validateBackupThresholds enforces: warn > 0, crit > 0, warn < crit, both <= 8760.
// Returns an error string used as the 422 response body "error" field.
// Mirrors the DB CHECK constraints in migration 0047.
func validateBackupThresholds(p BackupThresholdsPatch) error {
	if p.WarnThresholdHours == nil || p.CritThresholdHours == nil {
		return errors.New("warn_threshold_hours and crit_threshold_hours are required")
	}
	w, c := *p.WarnThresholdHours, *p.CritThresholdHours
	if w <= 0 {
		return errors.New("warn_threshold_hours_must_be_positive")
	}
	if c <= 0 {
		return errors.New("crit_threshold_hours_must_be_positive")
	}
	if w >= c {
		return errors.New("warn_threshold_hours_must_be_less_than_crit")
	}
	if w > 8760 || c > 8760 {
		return errors.New("threshold_hours_exceeds_maximum_8760")
	}
	return nil
}

// toBackupSummary converts a backup.BackupRunRow to the wire BackupSummary shape.
func toBackupSummary(row backup.BackupRunRow, now time.Time) *BackupSummary {
	s := &BackupSummary{
		ID:          row.ID.String(),
		Status:      row.Status,
		StartedAt:   row.StartedAt.Format(time.RFC3339),
		TriggerKind: row.TriggerKind,
	}
	if row.FileName != nil {
		s.FileName = *row.FileName
	}
	if row.FileSizeBytes != nil {
		s.SizeBytes = *row.FileSizeBytes
	}
	if row.SHA256 != nil {
		s.SHA256 = *row.SHA256
	}
	if row.FinishedAt != nil {
		ts := row.FinishedAt.Format(time.RFC3339)
		s.FinishedAt = &ts
		s.AgeSeconds = int64(now.Sub(*row.FinishedAt).Seconds())
		if s.AgeSeconds < 0 {
			s.AgeSeconds = 0
		}
	}
	return s
}
