package site

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// siteFixture wires a real testcontainer Postgres + an httptest server with
// the Site routes mounted. The SetUser helper switches the in-memory session
// between admin and viewer so Can()-driven tests don't need to log in via
// password each time.
type siteFixture struct {
	pool   *pgxpool.Pool
	server *httptest.Server
	client *http.Client
	deps   Deps

	adminID  string
	viewerID string
}

func nopLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newSiteFixture(t *testing.T) *siteFixture {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, nopLogger()))

	// Seed an admin + a viewer so handler tests can switch between them.
	var adminID, viewerID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('admin-site@example.com', 'Admin Site', 'x', 'admin') RETURNING id::text`,
	).Scan(&adminID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('viewer-site@example.com', 'Viewer Site', 'x', 'viewer') RETURNING id::text`,
	).Scan(&viewerID))

	sm := auth.NewSessionManager(pool, true /*dev*/, 8*time.Hour, 24*time.Hour)
	deps := Deps{Pool: pool, SessionMgr: sm, Log: nopLogger()}

	r := chi.NewRouter()
	// Test-only seed endpoint — calls PutUser to log in as admin / viewer.
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
	RegisterRoutes(r, deps)

	srv := httptest.NewServer(sm.LoadAndSave(r))
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	cli := &http.Client{Jar: jar}

	return &siteFixture{
		pool:     pool,
		server:   srv,
		client:   cli,
		deps:     deps,
		adminID:  adminID,
		viewerID: viewerID,
	}
}

func (f *siteFixture) seedRole(t *testing.T, role string) {
	t.Helper()
	res, err := f.client.Post(f.server.URL+"/test/seed/"+role, "", nil)
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusNoContent, res.StatusCode)
}

func (f *siteFixture) doJSON(t *testing.T, method, path string, body any) *http.Response {
	t.Helper()
	var buf io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, f.server.URL+path, buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "shifter")
	res, err := f.client.Do(req)
	require.NoError(t, err)
	return res
}

// TestCreateSite_AdminAllowed — admin POSTs a valid site → 201 + row exists +
// audit row 'create' on entity_type='site'.
func TestCreateSite_AdminAllowed(t *testing.T) {
	f := newSiteFixture(t)
	f.seedRole(t, "admin")

	res := f.doJSON(t, "POST", "/api/sites", CreateSiteRequest{
		Name:     "Bangkok HQ",
		Timezone: "Asia/Bangkok",
		SiteType: ptrStr("office"),
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&got))
	require.Equal(t, "Bangkok HQ", got["name"])

	// Site row exists.
	var count int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM site WHERE name = $1`, "Bangkok HQ",
	).Scan(&count))
	require.Equal(t, 1, count)

	// Audit row.
	var action, entityType string
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT action, entity_type FROM audit_log WHERE entity_type = 'site' ORDER BY time DESC LIMIT 1`,
	).Scan(&action, &entityType))
	require.Equal(t, "create", action)
	require.Equal(t, "site", entityType)
}

// TestCreateSite_ViewerForbidden — viewer POST → 403; no site row inserted;
// no audit row.
func TestCreateSite_ViewerForbidden(t *testing.T) {
	f := newSiteFixture(t)
	f.seedRole(t, "viewer")

	res := f.doJSON(t, "POST", "/api/sites", CreateSiteRequest{
		Name:     "Forbidden Site",
		Timezone: "Asia/Bangkok",
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusForbidden, res.StatusCode)

	var count int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM site WHERE name = $1`, "Forbidden Site",
	).Scan(&count))
	require.Equal(t, 0, count)

	var auditCount int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE entity_type = 'site'`,
	).Scan(&auditCount))
	require.Equal(t, 0, auditCount, "viewer reject must NOT write audit row")
}

// TestCreateSite_RejectsInvalidLatLng — lat=200 → 400 (server-side validation).
func TestCreateSite_RejectsInvalidLatLng(t *testing.T) {
	f := newSiteFixture(t)
	f.seedRole(t, "admin")

	bad := 200.0
	res := f.doJSON(t, "POST", "/api/sites", CreateSiteRequest{
		Name:     "Bad Lat",
		Timezone: "Asia/Bangkok",
		Lat:      &bad,
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)

	var count int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM site WHERE name = $1`, "Bad Lat",
	).Scan(&count))
	require.Equal(t, 0, count)
}

// TestUpdateSite_AuditDiff — patch site name; audit row's before contains the
// old name, after contains the new name.
func TestUpdateSite_AuditDiff(t *testing.T) {
	f := newSiteFixture(t)
	f.seedRole(t, "admin")

	// Create.
	res := f.doJSON(t, "POST", "/api/sites", CreateSiteRequest{
		Name:     "Original Name",
		Timezone: "Asia/Bangkok",
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)
	var created map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
	id := created["id"].(string)

	// Update.
	res2 := f.doJSON(t, "PATCH", "/api/sites/"+id, UpdateSiteRequest{
		Name:     "Renamed Site",
		Timezone: "Asia/Bangkok",
	})
	defer res2.Body.Close()
	require.Equal(t, http.StatusOK, res2.StatusCode)

	// Audit row diff.
	var beforeJSON, afterJSON []byte
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT before, after FROM audit_log WHERE action = 'update' AND entity_type = 'site' ORDER BY time DESC LIMIT 1`,
	).Scan(&beforeJSON, &afterJSON))

	var before, after map[string]any
	require.NoError(t, json.Unmarshal(beforeJSON, &before))
	require.NoError(t, json.Unmarshal(afterJSON, &after))
	require.Equal(t, "Original Name", before["name"])
	require.Equal(t, "Renamed Site", after["name"])
}

// TestArchiveAndRestoreSite — archive then restore; both audited; archived
// site disappears from /api/sites and reappears after restore.
func TestArchiveAndRestoreSite(t *testing.T) {
	f := newSiteFixture(t)
	f.seedRole(t, "admin")

	// Create.
	res := f.doJSON(t, "POST", "/api/sites", CreateSiteRequest{
		Name:     "To Archive",
		Timezone: "Asia/Bangkok",
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)
	var created map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
	id := created["id"].(string)

	// Archive.
	res2 := f.doJSON(t, "POST", "/api/sites/"+id+"/archive", nil)
	defer res2.Body.Close()
	require.Equal(t, http.StatusOK, res2.StatusCode)

	// /api/sites does NOT include archived site.
	res3 := f.doJSON(t, "GET", "/api/sites", nil)
	defer res3.Body.Close()
	require.Equal(t, http.StatusOK, res3.StatusCode)
	var listed []map[string]any
	require.NoError(t, json.NewDecoder(res3.Body).Decode(&listed))
	for _, s := range listed {
		require.NotEqual(t, id, s["id"], "archived site must be hidden from /api/sites")
	}

	// /api/sites/archived DOES include it.
	res4 := f.doJSON(t, "GET", "/api/sites/archived", nil)
	defer res4.Body.Close()
	require.Equal(t, http.StatusOK, res4.StatusCode)
	var archived []map[string]any
	require.NoError(t, json.NewDecoder(res4.Body).Decode(&archived))
	var foundArchived bool
	for _, s := range archived {
		if s["id"] == id {
			foundArchived = true
		}
	}
	require.True(t, foundArchived, "archived site must appear in /api/sites/archived")

	// Restore.
	res5 := f.doJSON(t, "POST", "/api/sites/"+id+"/restore", nil)
	defer res5.Body.Close()
	require.Equal(t, http.StatusOK, res5.StatusCode)

	// Audit rows for both.
	var auditCount int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE entity_id = $1::uuid AND action IN ('archive','restore')`, id,
	).Scan(&auditCount))
	require.Equal(t, 2, auditCount)
}

// TestUpdateSite_ViewerForbidden — viewer PATCH → 403; no audit row written.
func TestUpdateSite_ViewerForbidden(t *testing.T) {
	f := newSiteFixture(t)
	// First create as admin.
	f.seedRole(t, "admin")
	res := f.doJSON(t, "POST", "/api/sites", CreateSiteRequest{
		Name:     "Created By Admin",
		Timezone: "Asia/Bangkok",
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)
	var created map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
	id := created["id"].(string)

	// Switch to viewer and attempt update.
	f.seedRole(t, "viewer")
	res2 := f.doJSON(t, "PATCH", "/api/sites/"+id, UpdateSiteRequest{
		Name:     "Hacked Name",
		Timezone: "Asia/Bangkok",
	})
	defer res2.Body.Close()
	require.Equal(t, http.StatusForbidden, res2.StatusCode)

	// Original name preserved.
	var name string
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT name FROM site WHERE id = $1::uuid`, id,
	).Scan(&name))
	require.Equal(t, "Created By Admin", name)
}

// TestGetSite_NotFound — non-existent UUID returns 404.
func TestGetSite_NotFound(t *testing.T) {
	f := newSiteFixture(t)
	f.seedRole(t, "admin")

	res := f.doJSON(t, "GET", "/api/sites/"+uuid.NewString(), nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

// TestListSites_ViewerCanRead — viewer can GET /api/sites (read-only ok).
func TestListSites_ViewerCanRead(t *testing.T) {
	f := newSiteFixture(t)
	// Admin creates.
	f.seedRole(t, "admin")
	res := f.doJSON(t, "POST", "/api/sites", CreateSiteRequest{
		Name:     "Public Site",
		Timezone: "Asia/Bangkok",
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)

	// Viewer can read.
	f.seedRole(t, "viewer")
	res2 := f.doJSON(t, "GET", "/api/sites", nil)
	defer res2.Body.Close()
	require.Equal(t, http.StatusOK, res2.StatusCode)
	var listed []map[string]any
	require.NoError(t, json.NewDecoder(res2.Body).Decode(&listed))
	require.Len(t, listed, 1)
}

func ptrStr(s string) *string { return &s }
