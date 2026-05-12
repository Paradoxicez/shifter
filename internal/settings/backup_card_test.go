package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/backup"
)

// buildBackupRouter builds a test chi.Router with both retention + backup
// settings routes registered.
func buildBackupRouter(t *testing.T, sm *scs.SessionManager, backupDir string) (http.Handler, *backup.Store) {
	t.Helper()
	pool := startTestDB(t)
	bStore := backup.NewStore(pool)
	deps := Deps{Pool: pool, Queries: sqlc.New(pool)}
	backupCfg := BackupCardConfig{BackupDir: backupDir}
	r := chi.NewRouter()
	r.Use(sm.LoadAndSave)
	RegisterRoutes(r, deps, sm)
	RegisterBackupRoutes(r, deps, sm, bStore, backupCfg)
	return r, bStore
}

// TestGetBackupStatus_NeverRun verifies that the endpoint returns never_run=true
// when the backup_run table is empty.
func TestGetBackupStatus_NeverRun(t *testing.T) {
	sm := scs.New()
	router, _ := buildBackupRouter(t, sm, "/var/lib/shifter/backups")

	adminID := uuid.New().String()
	pool := startTestDB(t)
	seedTestUser(t, pool, adminID, "admin")
	adminCookie := injectSession(t, sm, adminID, "admin")

	w := doReq(t, router, http.MethodGet, "/api/settings/backup", nil, adminCookie)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/settings/backup expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp BackupStatusResponse
	decodeJSON(t, w, &resp)
	if !resp.NeverRun {
		t.Errorf("expected never_run=true, got false")
	}
	if resp.WarnThresholdHours != 24 {
		t.Errorf("warn_threshold_hours: want 24, got %d", resp.WarnThresholdHours)
	}
	if resp.CritThresholdHours != 168 {
		t.Errorf("crit_threshold_hours: want 168, got %d", resp.CritThresholdHours)
	}
	if resp.DestinationDir != "/var/lib/shifter/backups" {
		t.Errorf("destination_dir: want /var/lib/shifter/backups, got %q", resp.DestinationDir)
	}
	if len(resp.Recent) != 0 {
		t.Errorf("recent: want empty, got %d entries", len(resp.Recent))
	}
}

// TestGetBackupStatus_PopulatedAndAge verifies that when backup_run rows exist,
// the response includes last and recent fields with correct age_seconds.
func TestGetBackupStatus_PopulatedAndAge(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: requires TimescaleDB container (-short)")
	}
	pool := startTestDB(t)
	sm := scs.New()
	bStore := backup.NewStore(pool)
	deps := Deps{Pool: pool, Queries: sqlc.New(pool)}
	backupCfg := BackupCardConfig{BackupDir: "/backups"}
	r := chi.NewRouter()
	r.Use(sm.LoadAndSave)
	RegisterRoutes(r, deps, sm)
	RegisterBackupRoutes(r, deps, sm, bStore, backupCfg)

	// Seed 3 backup_run rows at known times.
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		finishedAt := now.Add(time.Duration(-i) * time.Hour)
		startedAt := finishedAt.Add(-5 * time.Minute)
		_, err := pool.Exec(context.Background(),
			`INSERT INTO backup_run
				(trigger_kind, status, destination_dir, file_name, file_size_bytes, sha256, started_at, finished_at)
			 VALUES ('cli', 'completed', '/backups', $1, 1024, 'abc123', $2, $3)`,
			"backup-"+uuid.New().String()+".tar.gz",
			startedAt,
			finishedAt,
		)
		if err != nil {
			t.Fatalf("seed backup_run row %d: %v", i, err)
		}
	}

	adminID := uuid.New().String()
	seedTestUser(t, pool, adminID, "admin")
	adminCookie := injectSession(t, sm, adminID, "admin")

	w := doReq(t, r, http.MethodGet, "/api/settings/backup", nil, adminCookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp BackupStatusResponse
	decodeJSON(t, w, &resp)
	if resp.NeverRun {
		t.Error("expected never_run=false when rows exist")
	}
	if resp.Last == nil {
		t.Fatal("expected last != nil")
	}
	if resp.Last.AgeSeconds < 0 {
		t.Errorf("age_seconds should be >= 0, got %d", resp.Last.AgeSeconds)
	}
	if len(resp.Recent) != 3 {
		t.Errorf("recent: want 3, got %d", len(resp.Recent))
	}
}

// TestPatchBackupThresholds_Validation verifies the warn/crit validation logic.
func TestPatchBackupThresholds_Validation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: requires TimescaleDB container (-short)")
	}
	pool := startTestDB(t)
	sm := scs.New()
	bStore := backup.NewStore(pool)
	deps := Deps{Pool: pool, Queries: sqlc.New(pool)}
	backupCfg := BackupCardConfig{BackupDir: "/var/lib/shifter/backups"}
	r := chi.NewRouter()
	r.Use(sm.LoadAndSave)
	RegisterRoutes(r, deps, sm)
	RegisterBackupRoutes(r, deps, sm, bStore, backupCfg)

	adminID := uuid.New().String()
	seedTestUser(t, pool, adminID, "admin")
	adminCookie := injectSession(t, sm, adminID, "admin")

	cases := []struct {
		name       string
		body       map[string]any
		wantStatus int
	}{
		{"valid_defaults", map[string]any{"warn_threshold_hours": 24, "crit_threshold_hours": 168}, http.StatusOK},
		{"warn_gte_crit_rejected", map[string]any{"warn_threshold_hours": 200, "crit_threshold_hours": 100}, http.StatusUnprocessableEntity},
		{"warn_equals_crit_rejected", map[string]any{"warn_threshold_hours": 100, "crit_threshold_hours": 100}, http.StatusUnprocessableEntity},
		{"warn_negative_rejected", map[string]any{"warn_threshold_hours": -1, "crit_threshold_hours": 168}, http.StatusUnprocessableEntity},
		{"warn_zero_rejected", map[string]any{"warn_threshold_hours": 0, "crit_threshold_hours": 168}, http.StatusUnprocessableEntity},
		{"crit_too_large_rejected", map[string]any{"warn_threshold_hours": 24, "crit_threshold_hours": 9000}, http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tc.body)
			req := httptest.NewRequest(http.MethodPatch, "/api/settings/backup/thresholds", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(&adminCookie)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.wantStatus {
				t.Errorf("case %s: want %d, got %d: %s", tc.name, tc.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

// TestAuthz_BackupConfigure_AdminOnly verifies viewer cannot PATCH thresholds.
func TestAuthz_BackupConfigure_AdminOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: requires TimescaleDB container (-short)")
	}
	pool := startTestDB(t)
	sm := scs.New()
	bStore := backup.NewStore(pool)
	deps := Deps{Pool: pool, Queries: sqlc.New(pool)}
	backupCfg := BackupCardConfig{BackupDir: "/var/lib/shifter/backups"}
	r := chi.NewRouter()
	r.Use(sm.LoadAndSave)
	RegisterRoutes(r, deps, sm)
	RegisterBackupRoutes(r, deps, sm, bStore, backupCfg)

	viewerCookie := injectSession(t, sm, uuid.New().String(), "viewer")
	adminID := uuid.New().String()
	seedTestUser(t, pool, adminID, "admin")
	adminCookie := injectSession(t, sm, adminID, "admin")

	// Viewer PATCH → 403.
	w := doReq(t, r, http.MethodPatch, "/api/settings/backup/thresholds",
		map[string]any{"warn_threshold_hours": 24, "crit_threshold_hours": 168}, viewerCookie)
	if w.Code != http.StatusForbidden {
		t.Errorf("viewer PATCH thresholds: want 403, got %d", w.Code)
	}

	// Admin PATCH → 200.
	w2 := doReq(t, r, http.MethodPatch, "/api/settings/backup/thresholds",
		map[string]any{"warn_threshold_hours": 24, "crit_threshold_hours": 168}, adminCookie)
	if w2.Code != http.StatusOK {
		t.Errorf("admin PATCH thresholds: want 200, got %d: %s", w2.Code, w2.Body.String())
	}
}

// TestMigration0047_CreatesThresholdsColumns verifies migration 0047 adds the
// expected columns to retention_config with correct defaults.
func TestMigration0047_CreatesThresholdsColumns(t *testing.T) {
	pool := startTestDB(t)

	// Migration 0047 should have run via startTestDB (RunMigrations).
	// Verify columns and defaults.
	var warnDefault, critDefault int
	err := pool.QueryRow(context.Background(),
		`SELECT backup_warn_threshold_hours, backup_crit_threshold_hours
		 FROM retention_config WHERE id = 1`).
		Scan(&warnDefault, &critDefault)
	if err != nil {
		t.Fatalf("read thresholds from retention_config: %v", err)
	}
	if warnDefault != 24 {
		t.Errorf("backup_warn_threshold_hours default: want 24, got %d", warnDefault)
	}
	if critDefault != 168 {
		t.Errorf("backup_crit_threshold_hours default: want 168, got %d", critDefault)
	}
}
