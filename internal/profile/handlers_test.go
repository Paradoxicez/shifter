package profile

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// profileHandlerFixture wires a real testcontainer Postgres + an httptest
// server with the profile editor routes mounted. Uses fakeCSClient +
// fakeConnStore from editor_test.go (same package).
type profileHandlerFixture struct {
	pool   *pgxpool.Pool
	server *httptest.Server
	client *http.Client
	deps   HTTPDeps

	cs    *fakeCSClient
	store *fakeConnStore

	adminID  string
	viewerID string
}

func newProfileHandlerFixture(t *testing.T, suffix string) *profileHandlerFixture {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, nopLogger()))

	// Seed admin + viewer.
	var adminID, viewerID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('admin-prof-`+suffix+`@example.com', 'Admin Prof', 'x', 'admin') RETURNING id::text`,
	).Scan(&adminID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('viewer-prof-`+suffix+`@example.com', 'Viewer Prof', 'x', 'viewer') RETURNING id::text`,
	).Scan(&viewerID))

	cs := &fakeCSClient{}
	store := &fakeConnStore{tenantID: uuid.NewString(), appID: uuid.NewString()}

	sm := auth.NewSessionManager(pool, true /*dev*/, 8*time.Hour, 24*time.Hour)
	deps := HTTPDeps{
		Pool:       pool,
		SessionMgr: sm,
		Log:        nopLogger(),
		CSClient:   cs,
		ConnStore:  store,
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
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

	return &profileHandlerFixture{
		pool:     pool,
		server:   srv,
		client:   cli,
		deps:     deps,
		cs:       cs,
		store:    store,
		adminID:  adminID,
		viewerID: viewerID,
	}
}

func (f *profileHandlerFixture) seedRole(t *testing.T, role string) {
	t.Helper()
	res, err := f.client.Post(f.server.URL+"/test/seed/"+role, "", nil)
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusNoContent, res.StatusCode)
}

func (f *profileHandlerFixture) doJSON(t *testing.T, method, path string, body any) *http.Response {
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

// TestProfileHandler_List_BothRoles — admin GET /api/device-profiles → 200
// with array including the 3 seeded profiles. Viewer GET → same.
func TestProfileHandler_List_BothRoles(t *testing.T) {
	f := newProfileHandlerFixture(t, "list")
	f.seedRole(t, "admin")

	res := f.doJSON(t, "GET", "/api/device-profiles", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var listed []map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&listed))
	require.GreaterOrEqual(t, len(listed), 3, "at least 3 seeded profiles")
	slugs := map[string]bool{}
	for _, p := range listed {
		if s, ok := p["slug"].(string); ok {
			slugs[s] = true
		}
	}
	require.True(t, slugs["axioma_w1"])
	require.True(t, slugs["acrel_adl200"])
	require.True(t, slugs["acrel_adw300"])

	// Viewer can also list.
	f.seedRole(t, "viewer")
	res2 := f.doJSON(t, "GET", "/api/device-profiles", nil)
	defer res2.Body.Close()
	require.Equal(t, http.StatusOK, res2.StatusCode)
}

// TestProfileHandler_Get_BothRoles — admin GET /api/device-profiles/{id} →
// 200 with profile + mapping rows. Viewer → same.
func TestProfileHandler_Get_BothRoles(t *testing.T) {
	f := newProfileHandlerFixture(t, "get")
	f.seedRole(t, "admin")

	// Look up a seeded profile id.
	var seededID string
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT id::text FROM device_profile WHERE slug = 'axioma_w1'`,
	).Scan(&seededID))

	res := f.doJSON(t, "GET", "/api/device-profiles/"+seededID, nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&got))
	prof, ok := got["profile"].(map[string]any)
	require.True(t, ok, "response must contain profile object")
	require.Equal(t, "axioma_w1", prof["slug"])
	mappings, ok := got["mappings"].([]any)
	require.True(t, ok, "response must contain mappings array")
	require.NotNil(t, mappings)

	// Viewer can also get.
	f.seedRole(t, "viewer")
	res2 := f.doJSON(t, "GET", "/api/device-profiles/"+seededID, nil)
	defer res2.Body.Close()
	require.Equal(t, http.StatusOK, res2.StatusCode)
}

// TestProfileHandler_Create_AdminOnly — admin POST /api/device-profiles with
// valid body → 201 with {"id": "<uuid>"}. Viewer POST → 403.
func TestProfileHandler_Create_AdminOnly(t *testing.T) {
	f := newProfileHandlerFixture(t, "create")

	body := ProfileRequest{
		Slug:           "newvendor_x99",
		Name:           "NewVendor X99",
		Vendor:         "NewVendor",
		Capabilities:   []string{"cumulative", "battery"},
		CounterModulus: 4294967296,
		MACVersion:     "LORAWAN_1_0_3",
		CodecJS:        validCodec(),
		Mappings: []MappingRequest{
			{JSONPointer: "/cumul", Target: "cumulative_value", DataType: "numeric", Position: 0},
		},
	}

	// Viewer rejected.
	f.seedRole(t, "viewer")
	resV := f.doJSON(t, "POST", "/api/device-profiles", body)
	defer resV.Body.Close()
	require.Equal(t, http.StatusForbidden, resV.StatusCode)

	// Admin succeeds.
	f.seedRole(t, "admin")
	res := f.doJSON(t, "POST", "/api/device-profiles", body)
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)

	var resp map[string]string
	require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
	require.NotEmpty(t, resp["id"])
	_, err := uuid.Parse(resp["id"])
	require.NoError(t, err)
}

// TestProfileHandler_Create_AuditWritten — after create, exactly 1 audit_log
// row with action='profile_create', entity_type='device_profile'.
func TestProfileHandler_Create_AuditWritten(t *testing.T) {
	f := newProfileHandlerFixture(t, "audit")
	f.seedRole(t, "admin")

	body := ProfileRequest{
		Slug:           "audit_vendor_v1",
		Name:           "Audit Vendor V1",
		Vendor:         "AuditVendor",
		Capabilities:   []string{"cumulative"},
		CounterModulus: 100000,
		MACVersion:     "LORAWAN_1_0_3",
		CodecJS:        validCodec(),
	}
	res := f.doJSON(t, "POST", "/api/device-profiles", body)
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)

	var resp map[string]string
	require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
	id := resp["id"]

	var action, entityType string
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT action, entity_type FROM audit_log WHERE entity_id = $1::uuid`,
		id,
	).Scan(&action, &entityType))
	require.Equal(t, "profile_create", action)
	require.Equal(t, "device_profile", entityType)
}

// TestProfileHandler_Update_PreservesSlug — PATCH /api/device-profiles/{id}
// with `slug` in body → 200; sqlc query confirms slug unchanged. Audit row
// action='profile_update'.
func TestProfileHandler_Update_PreservesSlug(t *testing.T) {
	f := newProfileHandlerFixture(t, "update")
	f.seedRole(t, "admin")

	// Create first.
	create := ProfileRequest{
		Slug:           "preserve_slug_v1",
		Name:           "Original",
		Vendor:         "PreserveVendor",
		Capabilities:   []string{"cumulative"},
		CounterModulus: 100,
		MACVersion:     "LORAWAN_1_0_3",
		CodecJS:        validCodec(),
	}
	res := f.doJSON(t, "POST", "/api/device-profiles", create)
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)
	var resp map[string]string
	require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
	id := resp["id"]

	// Now update with a *different* slug in the body — should be ignored,
	// the persisted slug stays "preserve_slug_v1".
	update := create
	update.Slug = "attempted_slug_change"
	update.Name = "Renamed"
	res2 := f.doJSON(t, "PATCH", "/api/device-profiles/"+id, update)
	defer res2.Body.Close()
	require.Equal(t, http.StatusOK, res2.StatusCode)

	var slug string
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT slug FROM device_profile WHERE id = $1::uuid`, id,
	).Scan(&slug))
	require.Equal(t, "preserve_slug_v1", slug, "slug must be immutable on update")

	var auditCount int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE entity_id = $1::uuid AND action = 'profile_update'`, id,
	).Scan(&auditCount))
	require.Equal(t, 1, auditCount)
}

// TestProfileHandler_BadCapability_400 — POST with capabilities=["bogus"] →
// 400 + detail contains "invalid capability".
func TestProfileHandler_BadCapability_400(t *testing.T) {
	f := newProfileHandlerFixture(t, "badcap")
	f.seedRole(t, "admin")

	body := ProfileRequest{
		Slug:           "badcap_v1",
		Name:           "Badcap",
		Vendor:         "Bad",
		Capabilities:   []string{"cumulative", "bogus"},
		CounterModulus: 1,
		MACVersion:     "LORAWAN_1_0_3",
		CodecJS:        validCodec(),
	}
	res := f.doJSON(t, "POST", "/api/device-profiles", body)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)

	var errResp map[string]string
	require.NoError(t, json.NewDecoder(res.Body).Decode(&errResp))
	require.Contains(t, errResp["detail"], "invalid capability")
}

// TestProfileHandler_DecodedSample_RoutesPointers — POST
// /api/device-profiles/{id}/decoded-sample with `{"sample": {...}}` returns
// the leaves (pointer + value).
func TestProfileHandler_DecodedSample_RoutesPointers(t *testing.T) {
	f := newProfileHandlerFixture(t, "decode")
	f.seedRole(t, "admin")

	// Need any profile id (route requires :id parameter, but the endpoint
	// itself doesn't read the profile from DB — it walks the body's sample).
	var seededID string
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT id::text FROM device_profile WHERE slug = 'axioma_w1'`,
	).Scan(&seededID))

	body := map[string]any{
		"sample": map[string]any{
			"cumul":    12345,
			"battery":  98,
			"flags":    map[string]any{"leak": false, "tamper": true},
			"readings": []any{1, 2, 3},
		},
	}
	res := f.doJSON(t, "POST", "/api/device-profiles/"+seededID+"/decoded-sample", body)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
	leaves, ok := resp["leaves"].([]any)
	require.True(t, ok, "response must include leaves array")

	pointers := map[string]any{}
	for _, l := range leaves {
		m := l.(map[string]any)
		pointers[m["json_pointer"].(string)] = m["value"]
	}
	require.Contains(t, pointers, "/cumul")
	require.Contains(t, pointers, "/battery")
	require.Contains(t, pointers, "/flags/leak")
	require.Contains(t, pointers, "/flags/tamper")
	require.Contains(t, pointers, "/readings/0")
	require.Contains(t, pointers, "/readings/1")
	require.Contains(t, pointers, "/readings/2")
}
