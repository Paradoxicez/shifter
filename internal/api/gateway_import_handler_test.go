package api

// Plan 07-13 Task 2 (TDD RED → GREEN): gateway bulk-import handler integration tests.
//
// Tests:
//   TestBulkImportValidate_HappyPath           — POST /validate returns ValidateResult
//   TestBulkImportCommit_CreatesAndAudits      — POST /commit creates gateways + writes audit
//   TestBulkImportCommit_Idempotent            — re-commit with same CSV → all skipped
//   TestBulkImportValidate_OversizedFile_413   — file > 5MB → 413
//   TestBulkImportValidate_ViewerForbidden_403 — viewer → 403

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/gateway"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// bulkImportSetup creates a test server wired with GatewayImportDeps.
func bulkImportSetup(t *testing.T) (serverURL string, client *http.Client, adminID, viewerID string) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, noopLog()))

	var aID, vID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('gwimport-admin@test.com', 'Admin', 'x', 'admin') RETURNING id::text`,
	).Scan(&aID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('gwimport-viewer@test.com', 'Viewer', 'x', 'viewer') RETURNING id::text`,
	).Scan(&vID))

	sm := auth.NewSessionManager(pool, true, 8*time.Hour, 24*time.Hour)

	deps := GatewayImportDeps{
		Pool:       pool,
		SessionMgr: sm,
		ImportSvc:  gateway.NewImportService(pool),
	}

	r := chi.NewRouter()
	r.Use(sm.LoadAndSave)

	// Seed-session endpoint for tests.
	r.Post("/test/seed/{role}", func(w http.ResponseWriter, req *http.Request) {
		role := chi.URLParam(req, "role")
		var id string
		switch role {
		case "admin":
			id = aID
		case "viewer":
			id = vID
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

	RegisterGatewayImportRoutes(r, deps)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	jar, _ := cookiejar.New(nil)
	cli := &http.Client{Jar: jar}

	return srv.URL, cli, aID, vID
}

// seedBulkSession logs into the test server as the given role.
func seedBulkSession(t *testing.T, client *http.Client, serverURL, role string) {
	t.Helper()
	res, err := client.Post(serverURL+"/test/seed/"+role, "", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusNoContent, res.StatusCode)
}

// makeCSVMultipart wraps csvBytes in a multipart/form-data body with field "file".
func makeCSVMultipart(t *testing.T, csvBytes []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "gateways.csv")
	require.NoError(t, err)
	_, err = fw.Write(csvBytes)
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	return &buf, mw.FormDataContentType()
}

func gatewayCSV(rows [][]string) []byte {
	lines := []string{"gateway_eui,name,description,latitude,longitude,region"}
	for _, r := range rows {
		cols := make([]string, 6)
		copy(cols, r)
		lines = append(lines, strings.Join(cols, ","))
	}
	return []byte(strings.Join(lines, "\n"))
}

// TestBulkImportValidate_HappyPath — POST /api/gateways/bulk-import/validate
// with a valid 3-row CSV returns 200 + ValidateResult with valid_rows=3.
func TestBulkImportValidate_HappyPath(t *testing.T) {
	url, client, _, _ := bulkImportSetup(t)
	seedBulkSession(t, client, url, "admin")

	csv := gatewayCSV([][]string{
		{"aabbccddeeff0001", "GW Alpha", "desc", "13.0", "100.0", "as923_2"},
		{"aabbccddeeff0002", "GW Beta", "", "", "", "eu868"},
		{"aabbccddeeff0003", "GW Gamma", "", "", "", "as923_2"},
	})
	body, ct := makeCSVMultipart(t, csv)

	req, err := http.NewRequest(http.MethodPost, url+"/api/gateways/bulk-import/validate", body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", ct)

	res, err := client.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)
	var result map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&result))
	require.Equal(t, float64(3), result["valid_rows"])
	require.Equal(t, float64(0), result["error_rows"])
}

// TestBulkImportCommit_CreatesAndAudits — POST /api/gateways/bulk-import/commit
// with a valid CSV creates gateways.
func TestBulkImportCommit_CreatesAndAudits(t *testing.T) {
	url, client, _, _ := bulkImportSetup(t)
	seedBulkSession(t, client, url, "admin")

	csv := gatewayCSV([][]string{
		{"aabbccddeeff0011", "GW One", "", "", "", "as923_2"},
		{"aabbccddeeff0012", "GW Two", "", "", "", "eu868"},
	})
	body, ct := makeCSVMultipart(t, csv)

	req, err := http.NewRequest(http.MethodPost, url+"/api/gateways/bulk-import/commit", body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", ct)

	res, err := client.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)
	var result map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&result))
	require.Equal(t, float64(2), result["created"])
	require.Equal(t, float64(0), result["updated"])
	require.Equal(t, float64(0), result["skipped"])
}

// TestBulkImportCommit_Idempotent — re-uploading the same CSV produces all "skipped".
func TestBulkImportCommit_Idempotent(t *testing.T) {
	url, client, _, _ := bulkImportSetup(t)
	seedBulkSession(t, client, url, "admin")

	csv := gatewayCSV([][]string{
		{"aabbccddeeff0021", "GW Idem", "", "", "", "as923_2"},
	})

	postCommit := func() map[string]any {
		body, ct := makeCSVMultipart(t, csv)
		req, err := http.NewRequest(http.MethodPost, url+"/api/gateways/bulk-import/commit", body)
		require.NoError(t, err)
		req.Header.Set("Content-Type", ct)
		res, err := client.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
		var result map[string]any
		require.NoError(t, json.NewDecoder(res.Body).Decode(&result))
		return result
	}

	r1 := postCommit()
	require.Equal(t, float64(1), r1["created"])

	r2 := postCommit()
	require.Equal(t, float64(0), r2["created"])
	require.Equal(t, float64(1), r2["skipped"])
}

// TestBulkImportValidate_OversizedFile_413 — a file > 5 MB is rejected with 413.
func TestBulkImportValidate_OversizedFile_413(t *testing.T) {
	url, client, _, _ := bulkImportSetup(t)
	seedBulkSession(t, client, url, "admin")

	const maxBytes = 5 << 20
	oversized := bytes.Repeat([]byte("a"), maxBytes+1024)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "big.csv")
	require.NoError(t, err)
	_, err = io.Copy(fw, bytes.NewReader(oversized))
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	req, err := http.NewRequest(http.MethodPost, url+"/api/gateways/bulk-import/validate", &buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	res, err := client.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusRequestEntityTooLarge, res.StatusCode)
}

// TestBulkImportValidate_ViewerForbidden_403 — viewer role → 403 on validate.
func TestBulkImportValidate_ViewerForbidden_403(t *testing.T) {
	url, client, _, _ := bulkImportSetup(t)
	seedBulkSession(t, client, url, "viewer")

	csv := gatewayCSV([][]string{
		{"aabbccddeeff0031", "GW Viewer", "", "", "", "as923_2"},
	})
	body, ct := makeCSVMultipart(t, csv)

	req, err := http.NewRequest(http.MethodPost, url+"/api/gateways/bulk-import/validate", body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", ct)

	res, err := client.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusForbidden, res.StatusCode)
}
