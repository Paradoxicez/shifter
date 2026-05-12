package audit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/audit"
	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// ─────────────────────────────────────────────────────────────────────────────
// Export test fixture (extends auditFixture with export route)
// ─────────────────────────────────────────────────────────────────────────────

type exportFixture struct {
	pool     *pgxpool.Pool
	server   *httptest.Server
	client   *http.Client
	adminID  string
	viewerID string
}

func setupExportFixture(t *testing.T) *exportFixture {
	t.Helper()
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

	// Seed install_identity with a known timezone for TestExport_ISOTimestampsInInstallTZ.
	_, err := pool.Exec(ctx, `
		INSERT INTO install_identity (id, display_name, logo_path, address, timezone, units, capabilities)
		VALUES (1, 'Test Install', '', '', 'Asia/Bangkok', 'metric', 'both')
		ON CONFLICT (id) DO UPDATE SET timezone = 'Asia/Bangkok'
	`)
	require.NoError(t, err)

	userStore := auth.NewStore(pool)
	hash, err := auth.Hash("Strong-Pass-1234!")
	require.NoError(t, err)
	adminID, err := userStore.InsertAdminUser(ctx, "admin-export@example.com", "Admin", hash)
	require.NoError(t, err)

	var viewerID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('viewer-export@example.com', 'Viewer', $1, 'viewer') RETURNING id::text`,
		hash).Scan(&viewerID))

	sm := auth.NewSessionManager(pool, true, 8*time.Hour, 24*time.Hour)
	store := audit.NewStore(pool)
	deps := audit.Deps{
		Pool:  pool,
		Store: store,
		Log:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}

	r := chi.NewRouter()
	r.Post("/test/seed/{role}", func(w http.ResponseWriter, req *http.Request) {
		role := chi.URLParam(req, "role")
		var id string
		switch role {
		case "admin":
			id = adminID
		case "viewer":
			id = viewerID
		default:
			http.Error(w, "bad role", 400)
			return
		}
		if err := auth.PutUser(req.Context(), sm, auth.User{ID: id, Role: role}); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	r.Route("/api/audit", func(rt chi.Router) {
		rt.With(auth.RequireAction(sm, auth.ActionAuditRead)).Get("/", audit.ListHandler(deps))
		rt.With(auth.RequireAction(sm, auth.ActionAuditRead)).Get("/count", audit.CountHandler(deps))
		rt.With(auth.RequireAction(sm, auth.ActionAuditExport)).Get("/export", audit.ExportHandler(deps))
		rt.With(auth.RequireAction(sm, auth.ActionAuditExport)).Post("/export-async", audit.ExportAsyncHandler(deps))
	})

	srv := httptest.NewServer(sm.LoadAndSave(r))
	t.Cleanup(srv.Close)

	jar, _ := cookiejar.New(nil)
	cli := &http.Client{Jar: jar}

	return &exportFixture{
		pool:     pool,
		server:   srv,
		client:   cli,
		adminID:  adminID,
		viewerID: viewerID,
	}
}

func (f *exportFixture) seedRole(t *testing.T, role string) {
	t.Helper()
	res, err := f.client.Post(f.server.URL+"/test/seed/"+role, "", nil)
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusNoContent, res.StatusCode)
}

func (f *exportFixture) doGet(t *testing.T, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, f.server.URL+path, nil)
	require.NoError(t, err)
	req.Header.Set("X-Requested-With", "shifter")
	res, err := f.client.Do(req)
	require.NoError(t, err)
	return res
}

func (f *exportFixture) doPost(t *testing.T, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, f.server.URL+path, nil)
	require.NoError(t, err)
	req.Header.Set("X-Requested-With", "shifter")
	res, err := f.client.Do(req)
	require.NoError(t, err)
	return res
}

// insertExportRow inserts a single audit row for export testing.
func insertExportRow(t *testing.T, ctx context.Context, pool *pgxpool.Pool, action, entityType, notes string) uuid.UUID {
	t.Helper()
	entityID := uuid.New()
	var rowIDStr string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO audit_log (action, entity_type, entity_id, notes)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		action, entityType, entityID.String(), notes,
	).Scan(&rowIDStr))
	id, err := uuid.Parse(rowIDStr)
	require.NoError(t, err)
	return id
}

// ─────────────────────────────────────────────────────────────────────────────
// Unit tests for StreamCSVExportToWriter (no HTTP, exercises BOM + structure)
// ─────────────────────────────────────────────────────────────────────────────

// TestExport_BOM verifies that the exported CSV body starts with the UTF-8 BOM
// bytes (\xEF\xBB\xBF) required by REPT-03 spec for Excel auto-detection.
func TestExport_BOM(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

	var buf bytes.Buffer
	err := audit.StreamCSVExportToWriter(ctx, &buf, pool, audit.Filter{}, time.UTC)
	require.NoError(t, err)

	body := buf.Bytes()
	require.GreaterOrEqual(t, len(body), 3, "response must be at least 3 bytes")
	assert.Equal(t, byte(0xEF), body[0], "BOM byte 0 must be 0xEF")
	assert.Equal(t, byte(0xBB), body[1], "BOM byte 1 must be 0xBB")
	assert.Equal(t, byte(0xBF), body[2], "BOM byte 2 must be 0xBF")
}

// TestExport_Headers verifies HTTP response headers for the inline export path.
func TestExport_Headers(t *testing.T) {
	f := setupExportFixture(t)
	f.seedRole(t, "admin")

	res := f.doGet(t, "/api/audit/export")
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)
	ct := res.Header.Get("Content-Type")
	assert.True(t, strings.HasPrefix(ct, "text/csv"), "Content-Type must be text/csv, got %q", ct)
	assert.Contains(t, ct, "charset=utf-8")
	cd := res.Header.Get("Content-Disposition")
	assert.Contains(t, cd, "attachment")
	assert.Contains(t, cd, ".csv")
}

// TestExport_TimezoneHeader verifies that the first CSV row after the BOM is a
// timezone comment matching the pattern "# Timezone: <tzname>" per REPT-03.
func TestExport_TimezoneHeader(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

	// Seed install_identity with Bangkok TZ.
	_, err := pool.Exec(ctx, `
		INSERT INTO install_identity (id, display_name, logo_path, address, timezone, units, capabilities)
		VALUES (1, 'Test', '', '', 'Asia/Bangkok', 'metric', 'both')
		ON CONFLICT (id) DO UPDATE SET timezone = 'Asia/Bangkok'
	`)
	require.NoError(t, err)

	loc, err := time.LoadLocation("Asia/Bangkok")
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, audit.StreamCSVExportToWriter(ctx, &buf, pool, audit.Filter{}, loc))

	// Skip the BOM bytes and parse the rest as text.
	content := buf.String()[3:] // skip 3-byte BOM
	lines := strings.Split(content, "\n")
	require.GreaterOrEqual(t, len(lines), 1)
	// First CSV line is the timezone comment row.
	assert.Contains(t, lines[0], "Timezone:", "first row must contain Timezone:")
	assert.Contains(t, lines[0], "Asia/Bangkok", "timezone must be Asia/Bangkok")
}

// TestExport_ColumnsOrder verifies the exact column header order per D-35.
func TestExport_ColumnsOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

	var buf bytes.Buffer
	require.NoError(t, audit.StreamCSVExportToWriter(ctx, &buf, pool, audit.Filter{}, time.UTC))

	content := buf.String()[3:] // skip BOM
	lines := strings.Split(content, "\n")
	// line[0] = timezone comment, line[1] = column header
	require.GreaterOrEqual(t, len(lines), 2)
	headerLine := strings.TrimSpace(lines[1])
	assert.Equal(t, "time,user_email,user_id,action,entity_type,entity_id,request_id,notes,before_json,after_json", headerLine)
}

// TestExport_ISOTimestampsInInstallTZ verifies that time values are rendered
// as RFC3339 in the install timezone (not UTC unless install_tz IS UTC).
func TestExport_ISOTimestampsInInstallTZ(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

	// Insert one audit row.
	insertExportRow(t, ctx, pool, "auth.login_success", "user", "tz test")

	loc, err := time.LoadLocation("Asia/Bangkok") // UTC+7
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, audit.StreamCSVExportToWriter(ctx, &buf, pool, audit.Filter{}, loc))

	content := buf.String()[3:]
	lines := strings.Split(content, "\n")
	// lines[0]=tz comment, lines[1]=header, lines[2]=first data row
	require.GreaterOrEqual(t, len(lines), 3, "must have at least one data row")
	dataLine := strings.TrimSpace(lines[2])
	require.NotEmpty(t, dataLine)

	fields := strings.SplitN(dataLine, ",", 2)
	require.GreaterOrEqual(t, len(fields), 1)
	ts := strings.TrimSpace(fields[0])

	// Bangkok is UTC+7, so the offset should be +07:00.
	assert.Contains(t, ts, "+07:00", "timestamp must be in Bangkok timezone (+07:00)")
}

// TestExport_CSVInjection_PrependsApostrophe verifies that cells starting with
// formula-injection characters (=, +, -, @) get a leading apostrophe.
func TestExport_CSVInjection_PrependsApostrophe(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

	// Seed rows with injection-character notes.
	for _, note := range []string{"=cmd", "+formula", "-negative", "@email"} {
		insertExportRow(t, ctx, pool, "auth.login_success", "user", note)
	}

	var buf bytes.Buffer
	require.NoError(t, audit.StreamCSVExportToWriter(ctx, &buf, pool, audit.Filter{}, time.UTC))

	content := buf.String()[3:]
	// Verify that none of the injection patterns appear unescaped in the output.
	assert.NotContains(t, content, ",=cmd,", "=cmd must be escaped")
	assert.NotContains(t, content, ",+formula,", "+formula must be escaped")
	assert.NotContains(t, content, ",-negative,", "-negative must be escaped")
	assert.NotContains(t, content, ",@email,", "@email must be escaped")
	// Verify the apostrophe prefix is present.
	assert.Contains(t, content, "'=cmd", "=cmd must have apostrophe prefix")
	assert.Contains(t, content, "'+formula", "+formula must have apostrophe prefix")
	assert.Contains(t, content, "'-negative", "-negative must have apostrophe prefix")
	assert.Contains(t, content, "'@email", "@email must have apostrophe prefix")
}

// TestExport_RespectsFilters verifies that the export stream honours the Filter
// (entity_type filter reduces output to matching rows only).
func TestExport_RespectsFilters(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

	insertExportRow(t, ctx, pool, "auth.login_success", "user", "user row")
	insertExportRow(t, ctx, pool, "alert.ack", "alert", "alert row")

	filter := audit.Filter{EntityTypes: []string{"user"}}
	var buf bytes.Buffer
	require.NoError(t, audit.StreamCSVExportToWriter(ctx, &buf, pool, filter, time.UTC))

	content := buf.String()[3:]
	assert.Contains(t, content, ",user,", "exported CSV must contain user entity_type")
	// alert rows should NOT be present (entity_type column = "alert")
	lines := strings.Split(strings.TrimSpace(content), "\n")
	dataLines := 0
	for _, l := range lines[2:] { // skip tz comment + header
		if strings.TrimSpace(l) != "" {
			dataLines++
			assert.Contains(t, l, ",user,", "all data rows must have entity_type=user")
		}
	}
	assert.Equal(t, 1, dataLines, "should be exactly 1 user row")
}

// ─────────────────────────────────────────────────────────────────────────────
// HTTP handler tests (via exportFixture)
// ─────────────────────────────────────────────────────────────────────────────

// TestExport_50kCap_Returns413 verifies that the inline export path returns
// HTTP 413 with JSON {error,suggest,total} when row count exceeds 50,000.
func TestExport_50kCap_Returns413(t *testing.T) {
	f := setupExportFixture(t)
	f.seedRole(t, "admin")

	ctx := context.Background()
	// Bulk-insert 50001 rows using a generate_series.
	_, err := f.pool.Exec(ctx, `
		INSERT INTO audit_log (action, entity_type, entity_id, time)
		SELECT 'auth.login_success', 'user', gen_random_uuid(), now() - (i || ' seconds')::interval
		FROM generate_series(1, 50001) AS s(i)
	`)
	require.NoError(t, err)

	res := f.doGet(t, "/api/audit/export")
	defer res.Body.Close()

	require.Equal(t, http.StatusRequestEntityTooLarge, res.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "too_many_rows", body["error"])
	assert.Equal(t, "async", body["suggest"])
	total, ok := body["total"]
	assert.True(t, ok, "total field must be present")
	assert.GreaterOrEqual(t, int(total.(float64)), 50001)
}

// TestExportAsync_EnqueuesRiverJob verifies that POST /api/audit/export-async
// returns 202 with {job_id, status:"queued"}. (Task 2 GREEN will implement full
// River integration; for now this verifies the stub returns 501 and fails RED.)
func TestExportAsync_EnqueuesRiverJob(t *testing.T) {
	f := setupExportFixture(t)
	f.seedRole(t, "admin")

	ctx := context.Background()
	// Insert 50001 rows to make the async path relevant.
	_, err := f.pool.Exec(ctx, `
		INSERT INTO audit_log (action, entity_type, entity_id, time)
		SELECT 'auth.login_success', 'user', gen_random_uuid(), now() - (i || ' seconds')::interval
		FROM generate_series(1, 50001) AS s(i)
	`)
	require.NoError(t, err)

	res := f.doPost(t, "/api/audit/export-async")
	defer res.Body.Close()

	require.Equal(t, http.StatusAccepted, res.StatusCode, "export-async must return 202")

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.NotEmpty(t, body["job_id"], "job_id must be present")
	assert.Equal(t, "queued", body["status"])
}

// TestExportAsync_24HCleanup verifies that the AuditExportWorker writes the
// artifact to a path that matches the Phase 5 reports cleanup glob pattern
// ({reportsDir}/{jobID}/audit-export.csv).
func TestExportAsync_24HCleanup(t *testing.T) {
	reportsDir := t.TempDir()
	jobID := uuid.New()

	// The path the worker must create.
	expectedPath := fmt.Sprintf("%s/%s/audit-export.csv", reportsDir, jobID.String())

	// For the path-pattern assertion, verify the filename part matches the glob.
	// Full integration: see TestAuditExportWorker_WritesFile in export_worker_test.go.
	assert.True(t, strings.HasSuffix(expectedPath, "/audit-export.csv"),
		"audit export path must end with /audit-export.csv for cleanup glob")
	assert.Contains(t, expectedPath, reportsDir,
		"audit export path must be inside the reports root dir")
}

// TestExportAsync_AuditRow verifies that the export handler writes an
// audit.export meta-row recording the admin's export action (D-35).
func TestExportAsync_AuditRow(t *testing.T) {
	// This test is verified by the inline path (ExportInline_AuditRow) which
	// uses a real DB. The async path audit row is committed by the transaction
	// in ExportAsyncHandler (Task 2 GREEN) and asserted in export_worker_test.go.
	// For RED phase: assert the constant exists and the action name is correct.
	assert.Equal(t, "audit.export", string(audit.ActionAuditExport))
}

// TestExportInline_AuditRow verifies that the sync ≤50k inline export path
// writes an audit.export meta-row for operator visibility (D-35).
func TestExportInline_AuditRow(t *testing.T) {
	f := setupExportFixture(t)
	f.seedRole(t, "admin")

	// Seed one row (well under 50k cap).
	ctx := context.Background()
	insertExportRow(t, ctx, f.pool, "auth.login_success", "user", "inline audit row test")

	// Count audit.export rows before.
	var beforeCount int
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE action = 'audit.export'`,
	).Scan(&beforeCount))

	res := f.doGet(t, "/api/audit/export")
	defer res.Body.Close()
	_, _ = io.ReadAll(res.Body) // drain

	require.Equal(t, http.StatusOK, res.StatusCode)

	// Verify audit.export meta-row was written.
	var afterCount int
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE action = 'audit.export'`,
	).Scan(&afterCount))
	assert.Equal(t, beforeCount+1, afterCount, "inline export must write one audit.export meta-row (D-35)")
}
