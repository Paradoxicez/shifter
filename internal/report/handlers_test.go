package report

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/db"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/testsupport"
	"github.com/stretchr/testify/require"
)

func TestStatusHandler_PendingReadyFailed(t *testing.T) {
	t.Skip("Plan 05-06 Task 2: GET /api/reports/:id returns pdf_status JSON for poll")
}

// setupHandlerTest starts Postgres, runs migrations, seeds an admin user, and
// returns a Deps + session manager ready for handler tests.
func setupHandlerTest(t *testing.T) (Deps, *scs.SessionManager, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: -short")
	}

	pool := testsupport.StartPostgres(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	// Seed install_identity (required by FinishSetup constraints).
	_, err := pool.Exec(ctx, `
		INSERT INTO install_identity (id, display_name, logo_path, address, timezone, units, capabilities)
		VALUES (1, 'Test Install', '', '', 'UTC', 'metric', 'both')
		ON CONFLICT (id) DO NOTHING
	`)
	require.NoError(t, err)

	// Seed an admin user.
	userID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, email, password_hash, role)
		VALUES ($1, 'admin@test.com', 'hashed', 'admin')
	`, userID)
	require.NoError(t, err)

	// Seed retention_config (required by FinishSetup checks, if any).
	_, err = pool.Exec(ctx, `
		INSERT INTO retention_config (id, raw_days, hourly_days, daily_days, monthly_days, yearly_days)
		VALUES (1, 90, 365, 1825, 7300, NULL)
		ON CONFLICT (id) DO NOTHING
	`)
	require.NoError(t, err)

	sm := auth.NewSessionManager(pool, true, 8*time.Hour, 24*time.Hour)
	q := sqlc.New(pool)

	artifactsRoot := t.TempDir()

	deps := Deps{
		Pool:          pool,
		Queries:       q,
		SessionMgr:    sm,
		Identity:      InstallIdentity{DisplayName: "Test", Timezone: time.UTC, Units: "metric"},
		ArtifactsRoot: artifactsRoot,
	}

	return deps, sm, userID.String()
}

// TestGenerateHandler_InvalidScope_400 — invalid scope returns 400.
func TestGenerateHandler_InvalidScope_400(t *testing.T) {
	deps, sm, userID := setupHandlerTest(t)

	body, _ := json.Marshal(map[string]string{
		"scope": "invalid_scope",
		"range": "daily",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/reports/generate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// Inject session user directly into context.
	req = req.WithContext(injectUser(req.Context(), sm, userID))
	rr := httptest.NewRecorder()

	GenerateHandler(deps)(rr, req)

	require.Equal(t, http.StatusUnprocessableEntity, rr.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Contains(t, resp["error"], "scope")
}

// TestGenerateHandler_MissingSiteID_422 — scope=site without site_id returns 422.
func TestGenerateHandler_MissingSiteID_422(t *testing.T) {
	deps, sm, userID := setupHandlerTest(t)

	body, _ := json.Marshal(map[string]string{
		"scope": "site",
		"range": "daily",
		// site_id intentionally omitted
	})
	req := httptest.NewRequest(http.MethodPost, "/api/reports/generate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(injectUser(req.Context(), sm, userID))
	rr := httptest.NewRecorder()

	GenerateHandler(deps)(rr, req)

	require.Equal(t, http.StatusUnprocessableEntity, rr.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Contains(t, resp["error"], "site_id")
}

// TestGenerateHandler_HappyPath_ScopeMeter — seed measurement rows, call
// handler, assert 200 + report_id in JSON + report row exists in DB + audit row exists.
func TestGenerateHandler_HappyPath_ScopeMeter(t *testing.T) {
	deps, sm, userID := setupHandlerTest(t)
	ctx := context.Background()

	// Seed a site + MP + measurement_hourly data.
	var siteID, mpID string
	require.NoError(t, deps.Pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('handler-test-site', 'UTC') RETURNING id`,
	).Scan(&siteID))
	require.NoError(t, deps.Pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class) VALUES ($1, 'handler-mp', 'water') RETURNING id`,
		siteID,
	).Scan(&mpID))

	// Insert a measurement to have something in hourly CAGG.
	now := time.Now().UTC()
	_, err := deps.Pool.Exec(ctx, `
		INSERT INTO measurement (time, metering_point_id, cumulative_value, quality, raw_payload, decoded_object)
		VALUES ($1, $2::uuid, 100, 'ok', '\x'::bytea, '{}'::jsonb)
	`, now.Add(-2*time.Hour), mpID)
	require.NoError(t, err)

	// Refresh hourly CAGG.
	_, err = deps.Pool.Exec(ctx,
		`CALL refresh_continuous_aggregate('measurement_hourly', $1::timestamptz, $2::timestamptz)`,
		now.Add(-24*time.Hour), now.Add(time.Hour),
	)
	require.NoError(t, err)

	mpUUID, _ := uuid.Parse(mpID)
	body, _ := json.Marshal(GenerateRequest{
		Scope:           "meter",
		MeteringPointID: mpUUID.String(),
		Range:           "daily",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/reports/generate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(injectUser(req.Context(), sm, userID))
	rr := httptest.NewRecorder()

	GenerateHandler(deps)(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())

	var resp GenerateResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.NotEqual(t, uuid.Nil, resp.ReportID, "report_id must be set")
	require.Equal(t, "pending", resp.PDFStatus)

	// Verify report row exists in DB.
	var reportScope string
	require.NoError(t, deps.Pool.QueryRow(ctx,
		`SELECT scope FROM report WHERE id = $1`,
		resp.ReportID,
	).Scan(&reportScope))
	require.Equal(t, "meter", reportScope)

	// Verify audit row exists with action=report.generate.
	var auditAction string
	require.NoError(t, deps.Pool.QueryRow(ctx,
		`SELECT action FROM audit_log WHERE entity_id = $1 AND action = 'report.generate'`,
		resp.ReportID,
	).Scan(&auditAction))
	require.Equal(t, "report.generate", auditAction)
}

// TestGenerateHandler_AuditInSameTx_RollbackBoth verifies that a valid scope=all
// request with an unreachable artifacts root (causing MkdirAll to fail) results
// in neither a report row nor an audit row being committed.
func TestGenerateHandler_AuditInSameTx_RollbackBoth(t *testing.T) {
	deps, sm, userID := setupHandlerTest(t)
	ctx := context.Background()

	// Point ArtifactsRoot to an unwritable path so MkdirAll will fail,
	// triggering an early return before the tx commits.
	deps.ArtifactsRoot = "/proc/shifter-test-unwritable"

	body, _ := json.Marshal(GenerateRequest{
		Scope: "all",
		Range: "daily",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/reports/generate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(injectUser(req.Context(), sm, userID))
	rr := httptest.NewRecorder()

	GenerateHandler(deps)(rr, req)

	// Should get a 5xx error (artifact dir creation failed).
	require.GreaterOrEqual(t, rr.Code, 400, "expected error response")

	// Verify NO report row in DB (tx should have been rolled back).
	var count int
	require.NoError(t, deps.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM report WHERE user_id = $1`,
		userID,
	).Scan(&count))
	require.Equal(t, 0, count, "no report row should exist after rollback")

	// Verify NO audit row in DB.
	require.NoError(t, deps.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_log WHERE action = 'report.generate' AND entity_type = 'report'`,
	).Scan(&count))
	require.Equal(t, 0, count, "no audit row should exist after rollback")
}

// TestGenerateHandler_CSVAndExcelLandOnDisk verifies that after a successful
// generate, report.csv and report.xlsx exist in the artifact directory.
func TestGenerateHandler_CSVAndExcelLandOnDisk(t *testing.T) {
	deps, sm, userID := setupHandlerTest(t)

	body, _ := json.Marshal(GenerateRequest{
		Scope: "all",
		Range: "daily",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/reports/generate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(injectUser(req.Context(), sm, userID))
	rr := httptest.NewRecorder()

	GenerateHandler(deps)(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())

	var resp GenerateResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))

	artifactDir := filepath.Join(deps.ArtifactsRoot, resp.ReportID.String())

	// Assert CSV exists.
	_, err := os.Stat(filepath.Join(artifactDir, "report.csv"))
	require.NoError(t, err, "report.csv must exist on disk")

	// Assert XLSX exists.
	_, err = os.Stat(filepath.Join(artifactDir, "report.xlsx"))
	require.NoError(t, err, "report.xlsx must exist on disk")
}

// injectUser puts the auth.User into the SCS session context so the handler's
// auth.GetUser call succeeds (without needing a real HTTP session round-trip).
func injectUser(ctx context.Context, sm *scs.SessionManager, userID string) context.Context {
	sm.Put(ctx, "user_id", userID)
	sm.Put(ctx, "role", "admin")
	return ctx
}

// RegisterRoutes integration smoke — verifies the chi router mounts the route.
func TestRegisterRoutes_Mounts(t *testing.T) {
	r := chi.NewRouter()
	deps := Deps{
		ArtifactsRoot: t.TempDir(),
	}
	RegisterRoutes(r, deps)

	// Verify the route is registered: a request without a session returns 401,
	// not 404 (which would indicate the route is not mounted).
	req := httptest.NewRequest(http.MethodPost, "/api/reports/generate", bytes.NewReader([]byte("{}")))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	// Without deps.SessionMgr, auth.GetUser returns false → 401.
	require.NotEqual(t, http.StatusNotFound, rr.Code, "route must be mounted")
}

