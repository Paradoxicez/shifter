package meteringpoint

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

type mpFixture struct {
	pool   *pgxpool.Pool
	server *httptest.Server
	client *http.Client
	deps   Deps

	adminID  string
	viewerID string
	siteID   string // a pre-seeded site to attach MPs to
}

func nopLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newMPFixture(t *testing.T) *mpFixture {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, nopLogger()))

	var adminID, viewerID, siteID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('admin-mp@example.com', 'Admin MP', 'x', 'admin') RETURNING id::text`,
	).Scan(&adminID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('viewer-mp@example.com', 'Viewer MP', 'x', 'viewer') RETURNING id::text`,
	).Scan(&viewerID))

	// Seed a site so MP creates have a target.
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('MP Test Site', 'Asia/Bangkok') RETURNING id::text`,
	).Scan(&siteID))

	sm := auth.NewSessionManager(pool, true /*dev*/, 8*time.Hour, 24*time.Hour)
	deps := Deps{Pool: pool, SessionMgr: sm, Log: nopLogger()}

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
	RegisterRoutes(r, deps)

	srv := httptest.NewServer(sm.LoadAndSave(r))
	t.Cleanup(srv.Close)

	jar, _ := cookiejar.New(nil)
	cli := &http.Client{Jar: jar}

	return &mpFixture{
		pool: pool, server: srv, client: cli, deps: deps,
		adminID: adminID, viewerID: viewerID, siteID: siteID,
	}
}

func (f *mpFixture) seedRole(t *testing.T, role string) {
	t.Helper()
	res, err := f.client.Post(f.server.URL+"/test/seed/"+role, "", nil)
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusNoContent, res.StatusCode)
}

func (f *mpFixture) doJSON(t *testing.T, method, path string, body any) *http.Response {
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

// TestCreateMP_AdminAllowed — admin POST → 201; row exists; audit row 'create'
// on entity_type='metering_point'.
func TestCreateMP_AdminAllowed(t *testing.T) {
	f := newMPFixture(t)
	f.seedRole(t, "admin")

	res := f.doJSON(t, "POST", "/api/metering-points", CreateMPRequest{
		SiteID:       f.siteID,
		Name:         "Main Water Inlet",
		UtilityClass: "water",
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&got))
	require.Equal(t, "Main Water Inlet", got["name"])
	require.Equal(t, "water", got["utility_class"])

	var auditCount int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE entity_type = 'metering_point' AND action = 'create'`,
	).Scan(&auditCount))
	require.Equal(t, 1, auditCount)
}

// TestCreateMP_RejectsInvalidUtilityClass — utility_class='gas' → 400.
func TestCreateMP_RejectsInvalidUtilityClass(t *testing.T) {
	f := newMPFixture(t)
	f.seedRole(t, "admin")

	res := f.doJSON(t, "POST", "/api/metering-points", CreateMPRequest{
		SiteID:       f.siteID,
		Name:         "Bad Class",
		UtilityClass: "gas",
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)

	var count int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM metering_point WHERE name = $1`, "Bad Class",
	).Scan(&count))
	require.Equal(t, 0, count)
}

// TestCreateMP_RejectsDupeNameOnSameSite — second create with same (site_id, name)
// returns 409.
func TestCreateMP_RejectsDupeNameOnSameSite(t *testing.T) {
	f := newMPFixture(t)
	f.seedRole(t, "admin")

	res1 := f.doJSON(t, "POST", "/api/metering-points", CreateMPRequest{
		SiteID:       f.siteID,
		Name:         "MP-A",
		UtilityClass: "water",
	})
	defer res1.Body.Close()
	require.Equal(t, http.StatusCreated, res1.StatusCode)

	res2 := f.doJSON(t, "POST", "/api/metering-points", CreateMPRequest{
		SiteID:       f.siteID,
		Name:         "MP-A",
		UtilityClass: "water",
	})
	defer res2.Body.Close()
	require.Equal(t, http.StatusConflict, res2.StatusCode)
}

// TestGetMPDetail_NoActiveBinding — fresh MP without binding → active_binding null.
// Updated for Phase 4 detail endpoint shape (DETL-01): metering_point.site_name,
// active_binding=null, latest_reading=null, online=null (D-22 empty case).
func TestGetMPDetail_NoActiveBinding(t *testing.T) {
	f := newMPFixture(t)
	f.seedRole(t, "admin")

	res1 := f.doJSON(t, "POST", "/api/metering-points", CreateMPRequest{
		SiteID:       f.siteID,
		Name:         "Unbound MP",
		UtilityClass: "water",
	})
	defer res1.Body.Close()
	require.Equal(t, http.StatusCreated, res1.StatusCode)
	var created map[string]any
	require.NoError(t, json.NewDecoder(res1.Body).Decode(&created))
	id := created["id"].(string)

	res := f.doJSON(t, "GET", "/api/metering-points/"+id, nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	var got DetailResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&got))
	require.Nil(t, got.ActiveBinding, "fresh MP must have null active_binding")
	require.Nil(t, got.LatestReading, "fresh MP must have null latest_reading")
	require.Nil(t, got.Online, "fresh MP must have null online flag")
	require.Equal(t, "MP Test Site", got.MeteringPoint.SiteName)
}

// TestGetMPDetail_WithActiveBinding — pre-create MP + active binding + device →
// active_binding populated.
func TestGetMPDetail_WithActiveBinding(t *testing.T) {
	f := newMPFixture(t)
	ctx := context.Background()
	f.seedRole(t, "admin")

	res1 := f.doJSON(t, "POST", "/api/metering-points", CreateMPRequest{
		SiteID:       f.siteID,
		Name:         "Bound MP",
		UtilityClass: "water",
	})
	defer res1.Body.Close()
	require.Equal(t, http.StatusCreated, res1.StatusCode)
	var created map[string]any
	require.NoError(t, json.NewDecoder(res1.Body).Decode(&created))
	mpID := created["id"].(string)

	// Pre-seed: pull a device profile from migration 0010 seed.
	var profileID, deviceID string
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT id::text FROM device_profile WHERE slug = 'axioma_w1' LIMIT 1`,
	).Scan(&profileID))
	require.NoError(t, f.pool.QueryRow(ctx,
		`INSERT INTO device (dev_eui, name, device_profile_id)
		 VALUES ('0102030405060708', 'Test Device', $1::uuid) RETURNING id::text`, profileID,
	).Scan(&deviceID))

	// Open binding.
	_, err := f.pool.Exec(ctx,
		`INSERT INTO binding (metering_point_id, device_id, valid_from, reading_offset)
		 VALUES ($1::uuid, $2::uuid, now(), 0)`, mpID, deviceID)
	require.NoError(t, err)

	res := f.doJSON(t, "GET", "/api/metering-points/"+mpID, nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	// Phase 4 detail endpoint shape (DETL-01): flat active_binding with dev_eui + device_profile_name.
	var got DetailResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&got))
	require.NotNil(t, got.ActiveBinding, "MP with binding must have non-null active_binding")
	require.Equal(t, "0102030405060708", got.ActiveBinding.DevEUI)
	require.NotEmpty(t, got.ActiveBinding.DeviceProfileName)
}

// TestArchiveMP_HidesFromList — archive MP; GET /api/metering-points doesn't
// include it; ?archived=true does.
func TestArchiveMP_HidesFromList(t *testing.T) {
	f := newMPFixture(t)
	f.seedRole(t, "admin")

	res1 := f.doJSON(t, "POST", "/api/metering-points", CreateMPRequest{
		SiteID:       f.siteID,
		Name:         "To Archive MP",
		UtilityClass: "water",
	})
	defer res1.Body.Close()
	var created map[string]any
	require.NoError(t, json.NewDecoder(res1.Body).Decode(&created))
	id := created["id"].(string)

	// Archive.
	res2 := f.doJSON(t, "POST", "/api/metering-points/"+id+"/archive", nil)
	defer res2.Body.Close()
	require.Equal(t, http.StatusOK, res2.StatusCode)

	// /api/metering-points DOES NOT include it.
	res3 := f.doJSON(t, "GET", "/api/metering-points", nil)
	defer res3.Body.Close()
	require.Equal(t, http.StatusOK, res3.StatusCode)
	var listed []map[string]any
	require.NoError(t, json.NewDecoder(res3.Body).Decode(&listed))
	for _, mp := range listed {
		require.NotEqual(t, id, mp["id"])
	}

	// ?archived=true does.
	res4 := f.doJSON(t, "GET", "/api/metering-points?archived=true", nil)
	defer res4.Body.Close()
	require.Equal(t, http.StatusOK, res4.StatusCode)
	var archived []map[string]any
	require.NoError(t, json.NewDecoder(res4.Body).Decode(&archived))
	var found bool
	for _, mp := range archived {
		if mp["id"] == id {
			found = true
		}
	}
	require.True(t, found)
}

// TestUpdateMP_ViewerForbidden — viewer PATCH → 403.
func TestUpdateMP_ViewerForbidden(t *testing.T) {
	f := newMPFixture(t)
	// Admin create.
	f.seedRole(t, "admin")
	res1 := f.doJSON(t, "POST", "/api/metering-points", CreateMPRequest{
		SiteID:       f.siteID,
		Name:         "Admin MP",
		UtilityClass: "water",
	})
	defer res1.Body.Close()
	var created map[string]any
	require.NoError(t, json.NewDecoder(res1.Body).Decode(&created))
	id := created["id"].(string)

	// Viewer attempt update.
	f.seedRole(t, "viewer")
	res := f.doJSON(t, "PATCH", "/api/metering-points/"+id, UpdateMPRequest{
		Name:         "Hacked",
		UtilityClass: "water",
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusForbidden, res.StatusCode)

	var name string
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT name FROM metering_point WHERE id = $1::uuid`, id,
	).Scan(&name))
	require.Equal(t, "Admin MP", name)
}

// TestQualitySummary_ReturnsZeros — fresh MP, no measurements → all counters 0.
func TestQualitySummary_ReturnsZeros(t *testing.T) {
	f := newMPFixture(t)
	f.seedRole(t, "admin")

	res1 := f.doJSON(t, "POST", "/api/metering-points", CreateMPRequest{
		SiteID:       f.siteID,
		Name:         "Quality MP",
		UtilityClass: "water",
	})
	defer res1.Body.Close()
	var created map[string]any
	require.NoError(t, json.NewDecoder(res1.Body).Decode(&created))
	id := created["id"].(string)

	res := f.doJSON(t, "GET", "/api/metering-points/"+id+"/quality", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&got))
	require.EqualValues(t, 0, got["total"])
	require.EqualValues(t, 0, got["decode_fail"])
}

// TestGetMPDetail_NotFound — non-existent UUID returns 404.
func TestGetMPDetail_NotFound(t *testing.T) {
	f := newMPFixture(t)
	f.seedRole(t, "admin")

	res := f.doJSON(t, "GET", "/api/metering-points/"+uuid.NewString(), nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}
