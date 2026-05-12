package http_test

// Plan 05-07 Task 2: floor-plan static image serve tests.
//
// Tests the ServeImageHandler's auth gating (T-05-07-02) and path traversal
// defense (T-05-07-03). Uses a self-contained test server with the floorplan
// package routes mounted — no dependency on the full HTTP router.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/db"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/floorplan"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// staticFixture is a minimal test bench for the image serve handler.
type staticFixture struct {
	server    *httptest.Server
	authCli   *http.Client // logged-in client
	anonCli   *http.Client // no session
	pool      *pgxpool.Pool
	queries   *sqlc.Queries
	siteID    uuid.UUID
	imageRoot string
}

func setupStaticFixture(t *testing.T) *staticFixture {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()

	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

	// Create a site row so FK constraints on floor_plan.site_id are satisfied.
	siteID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO site (id, name, lat, lng, timezone) VALUES ($1, 'Static Test Site', 0.0, 0.0, 'UTC')`,
		siteID,
	)
	require.NoError(t, err)

	// Create an admin user + log them in.
	store := auth.NewStore(pool)
	hash, err := auth.Hash("test-password-123")
	require.NoError(t, err)
	userID, err := store.InsertAdminUser(ctx, "static@test.example", "Static", hash)
	require.NoError(t, err)

	sm := auth.NewSessionManager(pool, true /*devMode*/, 8*time.Hour, 24*time.Hour)
	tmpDir := t.TempDir()
	q := sqlc.New(pool)

	deps := floorplan.Deps{
		Pool:       pool,
		Queries:    q,
		SessionMgr: sm,
		ImageRoot:  tmpDir,
	}

	r := chi.NewRouter()
	r.Use(sm.LoadAndSave)

	limiter := auth.NewLoginLimiter()
	t.Cleanup(limiter.Stop)
	loginDeps := auth.LoginDeps{
		Store:        store,
		SessionMgr:   sm,
		LoginLimiter: limiter,
		Log:          slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	r.Post("/api/auth/login", auth.LoginHandler(loginDeps))
	floorplan.RegisterRoutes(r, deps)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	// Authenticated client — login to get session cookie.
	jar, _ := cookiejar.New(nil)
	authCli := &http.Client{Jar: jar}
	loginBody, _ := json.Marshal(map[string]string{
		"email":    "static@test.example",
		"password": "test-password-123",
	})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/auth/login", bytes.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "shifter")
	resp, err := authCli.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = userID

	// Anon client — no cookie jar.
	anonCli := &http.Client{}

	return &staticFixture{
		server:    srv,
		authCli:   authCli,
		anonCli:   anonCli,
		pool:      pool,
		queries:   q,
		siteID:    siteID,
		imageRoot: tmpDir,
	}
}

// smallPNGBytes returns a minimal valid 4×4 PNG.
func smallPNGBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 100, G: 150, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

// uploadTestPlan uploads a floor plan via the API and returns its DB ID string.
func uploadTestPlan(t *testing.T, f *staticFixture) string {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "test.png")
	require.NoError(t, err)
	_, err = fw.Write(smallPNGBytes(t))
	require.NoError(t, err)
	require.NoError(t, mw.WriteField("label", "Test Plan"))
	require.NoError(t, mw.WriteField("sort_order", "0"))
	require.NoError(t, mw.Close())

	req, _ := http.NewRequest(http.MethodPost,
		f.server.URL+"/api/sites/"+f.siteID.String()+"/floor-plans",
		&buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := f.authCli.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	ctx := context.Background()
	var planID string
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT id::text FROM floor_plan WHERE site_id = $1 LIMIT 1`, f.siteID,
	).Scan(&planID))
	return planID
}

// TestFloorPlanImageAuthGated — GET /api/floor-plans/:id/image:
//   - No session → 401
//   - Valid session → 200 + Content-Type image/png + non-empty body
//   - Non-UUID id → 400 (before any DB or filesystem touch)
//   - Valid UUID for non-existent plan → 404
func TestFloorPlanImageAuthGated(t *testing.T) {
	f := setupStaticFixture(t)
	planID := uploadTestPlan(t, f)

	imageURL := f.server.URL + "/api/floor-plans/" + planID + "/image"

	// Test 2: GET with no session → 401.
	anonResp, err := f.anonCli.Get(imageURL)
	require.NoError(t, err)
	defer anonResp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, anonResp.StatusCode)

	// Test 3: GET with valid session → 200 + image/png.
	authResp, err := f.authCli.Get(imageURL)
	require.NoError(t, err)
	defer authResp.Body.Close()
	require.Equal(t, http.StatusOK, authResp.StatusCode)
	require.Equal(t, "image/png", authResp.Header.Get("Content-Type"))
	body, err := io.ReadAll(authResp.Body)
	require.NoError(t, err)
	require.NotEmpty(t, body, "image body must be non-empty")

	// Test 4: Non-UUID id → 400 before any filesystem touch.
	invalidIDResp, err := f.authCli.Get(f.server.URL + "/api/floor-plans/not-a-uuid/image")
	require.NoError(t, err)
	defer invalidIDResp.Body.Close()
	require.Equal(t, http.StatusBadRequest, invalidIDResp.StatusCode)

	// Test 5: Valid UUID for non-existent plan → 404.
	missingPlanResp, err := f.authCli.Get(f.server.URL + "/api/floor-plans/" + uuid.NewString() + "/image")
	require.NoError(t, err)
	defer missingPlanResp.Body.Close()
	require.Equal(t, http.StatusNotFound, missingPlanResp.StatusCode)
}

// TestFloorPlanImageAuthGated_PathTraversalRejected — T-05-07-03:
// Injects image_path = '../malicious.png' directly via SQL UPDATE (bypassing
// the upload handler which enforces server-generated paths). The static handler's
// filepath.Clean + strings.HasPrefix containment check must catch this and
// return 400 with body containing "invalid_image_path".
func TestFloorPlanImageAuthGated_PathTraversalRejected(t *testing.T) {
	f := setupStaticFixture(t)
	planID := uploadTestPlan(t, f)

	ctx := context.Background()
	// Directly seed a malicious image_path to simulate a DB bypass.
	_, err := f.pool.Exec(ctx,
		`UPDATE floor_plan SET image_path = '../malicious.png' WHERE id = $1::uuid`,
		planID,
	)
	require.NoError(t, err)

	resp, err := f.authCli.Get(fmt.Sprintf("%s/api/floor-plans/%s/image", f.server.URL, planID))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "invalid_image_path")
}
