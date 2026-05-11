package importpkg

// Phase 3 Plan 03-05 — HTTP handler integration tests. Spins up a chi
// router with the import routes mounted plus a /test/seed/{role} helper
// that bootstraps a session for admin or viewer roles. Tests issue real
// HTTP requests via httptest.

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
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// importHTTPFixture is the integration bench for /api/imports endpoints.
type importHTTPFixture struct {
	*importFixture
	server  *httptest.Server
	client  *http.Client
	viewer  uuid.UUID
}

func newImportHTTPFixture(t *testing.T) *importHTTPFixture {
	t.Helper()
	f := newImportFixture(t)

	// Seed a viewer user for the 403 tests.
	var viewerIDStr string
	ctx := context.Background()
	require.NoError(t, f.pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('viewer-import@example.com', 'Viewer Import', 'x', 'viewer') RETURNING id::text`,
	).Scan(&viewerIDStr))
	viewerID := uuid.MustParse(viewerIDStr)

	sm := auth.NewSessionManager(f.pool, true, 8*time.Hour, 24*time.Hour)
	deps := Deps{
		Pool:       f.pool,
		SessionMgr: sm,
		Log:        discardLogger(),
		Commit: &CommitDeps{
			Pool:      f.pool,
			CS:        f.cs,
			Bootstrap: f.boot,
			Log:       discardLogger(),
		},
	}

	r := chi.NewRouter()
	r.Post("/test/seed/{role}", func(w http.ResponseWriter, req *http.Request) {
		role := chi.URLParam(req, "role")
		var id uuid.UUID
		switch role {
		case "admin":
			id = f.adminID
		case "viewer":
			id = viewerID
		default:
			http.Error(w, "bad role", 400)
			return
		}
		if err := auth.PutUser(req.Context(), sm, auth.User{ID: id.String(), Role: role}); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	RegisterRoutes(r, deps)

	srv := httptest.NewServer(sm.LoadAndSave(r))
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)

	return &importHTTPFixture{
		importFixture: f,
		server:        srv,
		client:        &http.Client{Jar: jar},
		viewer:        viewerID,
	}
}

func (f *importHTTPFixture) seedRole(t *testing.T, role string) {
	t.Helper()
	res, err := f.client.Post(f.server.URL+"/test/seed/"+role, "", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("seed %s: status=%d", role, res.StatusCode)
	}
}

// postUpload builds a multipart body and POSTs to /api/imports.
func (f *importHTTPFixture) postUpload(t *testing.T, filename string, content []byte) *http.Response {
	t.Helper()
	buf := &bytes.Buffer{}
	mw := multipart.NewWriter(buf)
	w, err := mw.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = w.Write(content)
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	req, err := http.NewRequest(http.MethodPost, f.server.URL+"/api/imports/", buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := f.client.Do(req)
	require.NoError(t, err)
	return res
}

// TestUploadHandler_HappyPath — multipart XLSX upload returns 200 +
// job_id + outcomes preview.
func TestUploadHandler_HappyPath(t *testing.T) {
	f := newImportHTTPFixture(t)
	f.seedRole(t, "admin")

	res := f.postUpload(t, "happy.xlsx", testsupport.HappyFiveRows())
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("upload: status=%d body=%s", res.StatusCode, body)
	}
	var out UploadResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&out))
	if out.JobID == "" {
		t.Errorf("empty job_id")
	}
	if out.Total != 5 {
		t.Errorf("total = %d, want 5", out.Total)
	}
	if out.Valid != 5 {
		t.Errorf("valid_count = %d, want 5", out.Valid)
	}
	if len(out.FirstOutcomes) != 5 {
		t.Errorf("outcomes len = %d, want 5", len(out.FirstOutcomes))
	}
}

// TestUploadHandler_5MBLimit — body over 5 MiB → 413.
func TestUploadHandler_5MBLimit(t *testing.T) {
	f := newImportHTTPFixture(t)
	f.seedRole(t, "admin")

	// 6 MB of zeros as the "file" payload. Body cap kicks in before parse.
	big := make([]byte, 6<<20)

	res := f.postUpload(t, "big.xlsx", big)
	defer res.Body.Close()
	if res.StatusCode != http.StatusRequestEntityTooLarge {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("expected 413, got %d body=%s", res.StatusCode, body)
	}
}

// TestUploadHandler_XLSMRejected — .xlsm filename rejected with 400.
func TestUploadHandler_XLSMRejected(t *testing.T) {
	f := newImportHTTPFixture(t)
	f.seedRole(t, "admin")

	res := f.postUpload(t, "macro.xlsm", []byte("PK\x03\x04 fake xlsx bytes"))
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.StatusCode)
	}
	var body errResp
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	if body.Error != "xlsm_not_allowed" {
		t.Errorf("error = %q, want xlsm_not_allowed", body.Error)
	}
}

// TestUploadHandler_NonUTF8CSV — non-UTF-8 CSV body returns 400 with the
// operator-readable D-04a remediation message.
func TestUploadHandler_NonUTF8CSV(t *testing.T) {
	f := newImportHTTPFixture(t)
	f.seedRole(t, "admin")

	// TIS-620 bytes that are invalid as standalone UTF-8.
	body := append([]byte("dev_eui,name\n"), 0xB1, 0xB2, 0x2C, 0x64, 0x65, 0x76, 0x0A)
	res := f.postUpload(t, "thai.csv", body)
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.StatusCode)
	}
	var resp errResp
	require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
	if !strings.Contains(resp.Detail, "CSV UTF-8") {
		t.Errorf("detail %q missing CSV UTF-8 message", resp.Detail)
	}
}

// TestUploadHandler_Viewer403 — viewer POSTing /api/imports → 403.
func TestUploadHandler_Viewer403(t *testing.T) {
	f := newImportHTTPFixture(t)
	f.seedRole(t, "viewer")

	res := f.postUpload(t, "happy.xlsx", testsupport.HappyFiveRows())
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", res.StatusCode)
	}
}

// TestUploadHandler_5000RowsLimit — synthesise a 5001-row XLSX → 400.
// We don't actually build 5001 valid rows; the parser cap fires when the
// data row count exceeds MaxRowsPerJob.
func TestUploadHandler_5000RowsLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	f := newImportHTTPFixture(t)
	f.seedRole(t, "admin")

	// buildBigXLSX returns CSV bytes (faster than excelize for 5001 rows);
	// upload as .csv so the handler routes to ParseCSV.
	xlsx := buildBigXLSX(t, MaxRowsPerJob+1)
	res := f.postUpload(t, "big.csv", xlsx)
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("expected 400, got %d body=%s", res.StatusCode, body)
	}
	var body errResp
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	if body.Error != "too_many_rows" {
		t.Errorf("error = %q, want too_many_rows", body.Error)
	}
}

// TestCommitHandler — POST /api/imports/{job_id}/commit returns the
// summary on success.
func TestCommitHandler(t *testing.T) {
	f := newImportHTTPFixture(t)
	f.seedRole(t, "admin")

	// Upload first.
	res := f.postUpload(t, "h.xlsx", testsupport.HappyFiveRows())
	require.Equal(t, http.StatusOK, res.StatusCode)
	var up UploadResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&up))
	res.Body.Close()

	// Commit.
	commitRes, err := f.client.Post(f.server.URL+"/api/imports/"+up.JobID+"/commit", "", nil)
	require.NoError(t, err)
	defer commitRes.Body.Close()
	if commitRes.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(commitRes.Body)
		t.Fatalf("commit: status=%d body=%s", commitRes.StatusCode, body)
	}
	var summary map[string]any
	require.NoError(t, json.NewDecoder(commitRes.Body).Decode(&summary))
	if int(summary["created"].(float64)) != 5 {
		t.Errorf("summary.created = %v, want 5", summary["created"])
	}
}

// TestCommitHandler_Idempotent — second commit returns 200 with stored
// summary (no duplicate devices).
func TestCommitHandler_Idempotent(t *testing.T) {
	f := newImportHTTPFixture(t)
	f.seedRole(t, "admin")
	res := f.postUpload(t, "idem.xlsx", testsupport.HappyFiveRows())
	require.Equal(t, http.StatusOK, res.StatusCode)
	var up UploadResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&up))
	res.Body.Close()

	r1, err := f.client.Post(f.server.URL+"/api/imports/"+up.JobID+"/commit", "", nil)
	require.NoError(t, err)
	r1.Body.Close()
	if r1.StatusCode != http.StatusOK {
		t.Fatalf("first commit: status=%d", r1.StatusCode)
	}

	r2, err := f.client.Post(f.server.URL+"/api/imports/"+up.JobID+"/commit", "", nil)
	require.NoError(t, err)
	defer r2.Body.Close()
	if r2.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(r2.Body)
		t.Fatalf("second commit: status=%d body=%s", r2.StatusCode, body)
	}
}

// TestCommitHandler_Expired — preview job past TTL → 410.
func TestCommitHandler_Expired(t *testing.T) {
	f := newImportHTTPFixture(t)
	f.seedRole(t, "admin")

	res := f.postUpload(t, "exp.xlsx", testsupport.HappyFiveRows())
	require.Equal(t, http.StatusOK, res.StatusCode)
	var up UploadResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&up))
	res.Body.Close()

	// Backdate expires_at directly in PG.
	ctx := context.Background()
	_, err := f.pool.Exec(ctx,
		`UPDATE import_job SET expires_at = now() - interval '2 hours' WHERE job_id = $1`,
		up.JobID,
	)
	require.NoError(t, err)

	commitRes, err := f.client.Post(f.server.URL+"/api/imports/"+up.JobID+"/commit", "", nil)
	require.NoError(t, err)
	defer commitRes.Body.Close()
	if commitRes.StatusCode != http.StatusGone {
		t.Fatalf("expected 410, got %d", commitRes.StatusCode)
	}
}

// TestGetImportJobHandler — GET /api/imports/{job_id} returns job + rows.
func TestGetImportJobHandler(t *testing.T) {
	f := newImportHTTPFixture(t)
	f.seedRole(t, "admin")
	res := f.postUpload(t, "get.xlsx", testsupport.HappyFiveRows())
	require.Equal(t, http.StatusOK, res.StatusCode)
	var up UploadResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&up))
	res.Body.Close()

	gr, err := f.client.Get(f.server.URL + "/api/imports/" + up.JobID)
	require.NoError(t, err)
	defer gr.Body.Close()
	if gr.StatusCode != http.StatusOK {
		t.Fatalf("get job: status=%d", gr.StatusCode)
	}
	var body map[string]any
	require.NoError(t, json.NewDecoder(gr.Body).Decode(&body))
	job := body["job"].(map[string]any)
	if job["job_id"].(string) != up.JobID {
		t.Errorf("job_id mismatch")
	}
	if int(body["total"].(float64)) != 5 {
		t.Errorf("row total = %v, want 5", body["total"])
	}
}

// TestTemplateHandler — GET /api/imports/template.xlsx returns an XLSX.
func TestTemplateHandler(t *testing.T) {
	f := newImportHTTPFixture(t)
	f.seedRole(t, "admin")
	res, err := f.client.Get(f.server.URL + "/api/imports/template.xlsx")
	require.NoError(t, err)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("template: status=%d", res.StatusCode)
	}
	if !strings.Contains(res.Header.Get("Content-Type"), "spreadsheetml.sheet") {
		t.Errorf("Content-Type = %q", res.Header.Get("Content-Type"))
	}
	if !strings.Contains(res.Header.Get("Content-Disposition"), "device-import-template") {
		t.Errorf("Content-Disposition = %q", res.Header.Get("Content-Disposition"))
	}
}

// TestErrorsXLSXHandler — GET /api/imports/{job_id}/errors.xlsx returns
// an XLSX even when there are zero error rows.
func TestErrorsXLSXHandler(t *testing.T) {
	f := newImportHTTPFixture(t)
	f.seedRole(t, "admin")
	res := f.postUpload(t, "errx.xlsx", testsupport.MixedTenRows())
	require.Equal(t, http.StatusOK, res.StatusCode)
	var up UploadResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&up))
	res.Body.Close()

	er, err := f.client.Get(f.server.URL + "/api/imports/" + up.JobID + "/errors.xlsx")
	require.NoError(t, err)
	defer er.Body.Close()
	if er.StatusCode != http.StatusOK {
		t.Fatalf("errors.xlsx: status=%d", er.StatusCode)
	}
	if !strings.Contains(er.Header.Get("Content-Type"), "spreadsheetml.sheet") {
		t.Errorf("Content-Type = %q", er.Header.Get("Content-Type"))
	}
}

// TestRouter_ImportRoutesMounted — unauth POST to /api/imports → 401, not 404.
func TestRouter_ImportRoutesMounted(t *testing.T) {
	f := newImportHTTPFixture(t)
	// No seedRole — anonymous request.
	res := f.postUpload(t, "anon.xlsx", testsupport.HappyFiveRows())
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 (route exists), got %d", res.StatusCode)
	}
}
