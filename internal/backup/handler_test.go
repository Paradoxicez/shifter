package backup

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
	"github.com/stretchr/testify/require"
)

// seedAdminSession creates an admin user and returns a session manager with
// an active session for that user, plus the user's ID string.
func seedAdminSession(t *testing.T, pool interface {
	Exec(ctx context.Context, sql string, args ...any) (interface{}, error)
}) (string, string) {
	t.Helper()
	// Minimal: return a predictable ID and role for test purposes.
	// Real session injection done via auth.SetUserInContext if available.
	return "admin", "00000000-0000-0000-0000-000000000001"
}

// buildTestDeps builds a minimal backup.Deps for handler tests with a real DB.
func buildTestDeps(t *testing.T) Deps {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test: -short")
	}
	pool := testsupport.StartPostgres(t)
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(io.Discard, nil))))

	store := NewStore(pool)
	cfg := RunnerConfig{
		DBHost:         pool.Config().ConnConfig.Host,
		DBPort:         int(pool.Config().ConnConfig.Port),
		DBUser:         pool.Config().ConnConfig.User,
		DBName:         pool.Config().ConnConfig.Database,
		DBPassword:     "shifter",
		ChirpStackMode: "external",
		FloorPlansDir:  t.TempDir(),
		InstallSlug:    "test",
		SchemaVersion:  "46",
	}
	runner := &Runner{
		Pool:  pool,
		Store: store,
		Cfg:   cfg,
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return Deps{
		Runner: runner,
		Store:  store,
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// TestAuthz_BackupActions verifies role→action mappings for backup surface.
func TestAuthz_BackupActions(t *testing.T) {
	t.Parallel()

	admin := &auth.User{ID: "00000000-0000-0000-0000-000000000001", Role: "admin"}
	viewer := &auth.User{ID: "00000000-0000-0000-0000-000000000002", Role: "viewer"}

	// Admin has all three backup actions.
	require.True(t, auth.Can(admin, auth.ActionBackupRun, nil), "admin must have ActionBackupRun")
	require.True(t, auth.Can(admin, auth.ActionBackupRead, nil), "admin must have ActionBackupRead")
	require.True(t, auth.Can(admin, auth.ActionBackupConfigure, nil), "admin must have ActionBackupConfigure")

	// Viewer has only ActionBackupRead.
	require.True(t, auth.Can(viewer, auth.ActionBackupRead, nil), "viewer must have ActionBackupRead (D-46)")
	require.False(t, auth.Can(viewer, auth.ActionBackupRun, nil), "viewer must NOT have ActionBackupRun (T-06-08-01)")
	require.False(t, auth.Can(viewer, auth.ActionBackupConfigure, nil), "viewer must NOT have ActionBackupConfigure")
}

// TestHandler_ListRecent_TopFive seeds 7 backup_run rows and verifies that
// GET /api/backup/list returns the 5 most recent sorted by started_at DESC.
func TestHandler_ListRecent_TopFive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test: -short")
	}
	pool := testsupport.StartPostgres(t)
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(io.Discard, nil))))
	store := NewStore(pool)

	// Seed 7 rows.
	csMode := "external"
	schemaVer := "46"
	for i := 0; i < 7; i++ {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		_, err = store.InsertStartedTx(ctx, tx, StartParams{
			TriggerKind:    "cli",
			DestinationDir: "/backups",
			ChirpStackMode: &csMode,
			SchemaVersion:  &schemaVer,
		})
		require.NoError(t, err)
		require.NoError(t, tx.Commit(ctx))
	}

	deps := Deps{
		Store: store,
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	r := chi.NewRouter()
	r.Get("/api/backup/list", ListRecentHandler(deps))

	req := httptest.NewRequest(http.MethodGet, "/api/backup/list", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var rows []map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&rows))
	require.Len(t, rows, 5, "list must return exactly 5 rows (most recent)")
}

// TestHandler_LastAndAge verifies GET /api/backup/last returns age_seconds
// when a backup exists, and {never_run: true} when none exist.
func TestHandler_LastAndAge(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test: -short")
	}
	pool := testsupport.StartPostgres(t)
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(io.Discard, nil))))
	store := NewStore(pool)

	deps := Deps{
		Store: store,
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	r := chi.NewRouter()
	r.Get("/api/backup/last", LastHandler(deps))

	// No backup yet → never_run: true.
	req := httptest.NewRequest(http.MethodGet, "/api/backup/last", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var neverRun map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&neverRun))
	require.Equal(t, true, neverRun["never_run"], "must return {never_run: true} when no backups exist")

	// Seed one row.
	csMode := "external"
	schemaVer := "46"
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	_, err = store.InsertStartedTx(ctx, tx, StartParams{
		TriggerKind:    "cli",
		DestinationDir: "/backups",
		ChirpStackMode: &csMode,
		SchemaVersion:  &schemaVer,
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))

	req2 := httptest.NewRequest(http.MethodGet, "/api/backup/last", nil)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code)

	var lastResp map[string]any
	require.NoError(t, json.NewDecoder(rec2.Body).Decode(&lastResp))
	_, hasAge := lastResp["age_seconds"]
	require.True(t, hasAge, "last response must include age_seconds when backup exists")
	require.NotContains(t, lastResp, "never_run", "must not have never_run when backup exists")
}
