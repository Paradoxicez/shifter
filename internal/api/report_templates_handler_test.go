package api

// Tests for the 5 report-template HTTP handlers (Plan 07-11a).
// UX-POWER Surface 6: GET list, GET single, POST create, PATCH update, DELETE.
//
// All tests use a real Postgres testcontainer + testcontainer-backed SCS
// session store to exercise the full auth+audit stack. They are skipped with -short.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// reportTemplateSetup creates a real Postgres testcontainer, runs migrations,
// seeds a user with the given role, and returns the test server + HTTP client
// with a session cookie. The pool is returned for direct DB assertions.
func reportTemplateSetup(t *testing.T, suffix string, role string) (srv *httptest.Server, cli *http.Client, pool *pgxpool.Pool) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	pgPool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pgPool, noopLog()))

	var userID string
	require.NoError(t, pgPool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('`+role+`-tpl-`+suffix+`@ex.com', 'U', 'x', '`+role+`') RETURNING id::text`,
	).Scan(&userID))

	sm := auth.NewSessionManager(pgPool, true, 8*time.Hour, 24*time.Hour)
	deps := ReportTemplateDeps{Pool: pgPool, SessionMgr: sm}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/test/seed", func(w http.ResponseWriter, req *http.Request) {
		if err := auth.PutUser(req.Context(), sm, auth.User{ID: userID, Role: role}); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	RegisterReportTemplateRoutes(r, deps)

	server := httptest.NewServer(sm.LoadAndSave(r))
	t.Cleanup(server.Close)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	res, err := client.Post(server.URL+"/test/seed", "", nil)
	require.NoError(t, err)
	res.Body.Close()

	return server, client, pgPool
}

// postJSON is a helper that sends a POST with a JSON body and returns the response.
func postJSON(t *testing.T, cli *http.Client, url string, body any) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	resp, err := cli.Post(url, "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	return resp
}

// patchJSON is a helper that sends a PATCH with a JSON body and returns the response.
func patchJSON(t *testing.T, cli *http.Client, url string, body any) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(b))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := cli.Do(req)
	require.NoError(t, err)
	return resp
}

// deleteReq is a helper that sends a DELETE and returns the response.
func deleteReq(t *testing.T, cli *http.Client, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	require.NoError(t, err)
	resp, err := cli.Do(req)
	require.NoError(t, err)
	return resp
}

// TestCreateReportTemplateHandler_HappyPath — admin creates a template;
// response is 201 with the created template JSON.
func TestCreateReportTemplateHandler_HappyPath(t *testing.T) {
	srv, cli, _ := reportTemplateSetup(t, "create-happy", "admin")

	body := map[string]any{
		"name":        "My Monthly Report",
		"description": "monthly all scope",
		"state":       map[string]any{"scope": "all", "range": "monthly"},
	}
	resp := postJSON(t, cli, srv.URL+"/api/reports/templates", body)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	require.Equal(t, "My Monthly Report", result["name"])
	require.NotEmpty(t, result["id"])
}

// TestCreateReportTemplateHandler_DuplicateName409 — creating a second template
// with the same name returns 409 Conflict.
func TestCreateReportTemplateHandler_DuplicateName409(t *testing.T) {
	srv, cli, _ := reportTemplateSetup(t, "create-dup", "admin")

	body := map[string]any{
		"name":  "Duplicate",
		"state": map[string]any{"scope": "all", "range": "monthly"},
	}
	resp1 := postJSON(t, cli, srv.URL+"/api/reports/templates", body)
	resp1.Body.Close()
	require.Equal(t, http.StatusCreated, resp1.StatusCode)

	resp2 := postJSON(t, cli, srv.URL+"/api/reports/templates", body)
	defer resp2.Body.Close()
	require.Equal(t, http.StatusConflict, resp2.StatusCode)
}

// TestUpdateReportTemplateHandler_RenameCollision409 — PATCH that renames to an
// existing template's name returns 409 Conflict.
func TestUpdateReportTemplateHandler_RenameCollision409(t *testing.T) {
	srv, cli, _ := reportTemplateSetup(t, "update-dup", "admin")

	// Create two distinct templates.
	body1 := map[string]any{"name": "Alpha", "state": map[string]any{"scope": "all", "range": "monthly"}}
	body2 := map[string]any{"name": "Beta", "state": map[string]any{"scope": "all", "range": "yearly"}}
	r1 := postJSON(t, cli, srv.URL+"/api/reports/templates", body1)
	r1.Body.Close()
	require.Equal(t, http.StatusCreated, r1.StatusCode)

	r2 := postJSON(t, cli, srv.URL+"/api/reports/templates", body2)
	defer r2.Body.Close()
	require.Equal(t, http.StatusCreated, r2.StatusCode)
	var created map[string]any
	require.NoError(t, json.NewDecoder(r2.Body).Decode(&created))
	tplID := created["id"].(string)

	// Rename Beta → Alpha (collision).
	patch := map[string]any{"name": "Alpha", "description": "", "state": map[string]any{"scope": "all", "range": "yearly"}}
	resp := patchJSON(t, cli, srv.URL+"/api/reports/templates/"+tplID, patch)
	defer resp.Body.Close()
	require.Equal(t, http.StatusConflict, resp.StatusCode)
}

// TestDeleteReportTemplateHandler_AuditWritten — DELETE removes the template
// and an audit row is written in the same transaction (T-07-11a-03 mitigation).
func TestDeleteReportTemplateHandler_AuditWritten(t *testing.T) {
	srv, cli, pool := reportTemplateSetup(t, "delete-audit", "admin")

	// Create a template.
	body := map[string]any{"name": "ToDelete", "state": map[string]any{"scope": "all", "range": "daily"}}
	cr := postJSON(t, cli, srv.URL+"/api/reports/templates", body)
	cr.Body.Close()
	require.Equal(t, http.StatusCreated, cr.StatusCode)

	// List to get ID.
	lr, err := cli.Get(srv.URL + "/api/reports/templates")
	require.NoError(t, err)
	defer lr.Body.Close()
	require.Equal(t, http.StatusOK, lr.StatusCode)
	var list []map[string]any
	require.NoError(t, json.NewDecoder(lr.Body).Decode(&list))
	require.Len(t, list, 1)
	tplID := list[0]["id"].(string)

	// DELETE.
	dr := deleteReq(t, cli, srv.URL+"/api/reports/templates/"+tplID)
	dr.Body.Close()
	require.Equal(t, http.StatusNoContent, dr.StatusCode)

	// Assert audit row was written.
	ctx := context.Background()
	var auditCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE entity_id = $1::uuid AND action = 'report_template.deleted'`,
		tplID,
	).Scan(&auditCount))
	require.Equal(t, 1, auditCount, "DELETE must write a report_template.deleted audit row (T-07-11a-03)")
}

// TestCreateReportTemplateHandler_ViewerForbidden — viewer calling POST returns
// 403 (T-07-11a-02 mitigation: RequireAction blocks before handler runs).
func TestCreateReportTemplateHandler_ViewerForbidden(t *testing.T) {
	srv, cli, _ := reportTemplateSetup(t, "viewer-forbidden", "viewer")

	body := map[string]any{"name": "Forbidden", "state": map[string]any{"scope": "all", "range": "monthly"}}
	resp := postJSON(t, cli, srv.URL+"/api/reports/templates", body)
	defer resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// TestListReportTemplatesHandler_ViewerAllowed — viewer calling GET /api/reports/templates
// returns 200 (read is allowed for both roles).
func TestListReportTemplatesHandler_ViewerAllowed(t *testing.T) {
	srv, cli, _ := reportTemplateSetup(t, "viewer-list", "viewer")

	resp, err := cli.Get(srv.URL + "/api/reports/templates")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var list []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	require.NotNil(t, list) // empty slice, not null
}
