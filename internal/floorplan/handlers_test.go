package floorplan_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/db"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/floorplan"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// ─── Fixture helpers ──────────────────────────────────────────────────────────

type fixture struct {
	deps      floorplan.Deps
	server    *httptest.Server
	client    *http.Client
	siteID    uuid.UUID
	userID    string
	imageRoot string
}

func setupFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()

	pool := testsupport.StartPostgres(t)
	require.NoError(t,
		db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(os.Stderr, nil))),
	)

	// Create a site row so FK constraints on floor_plan.site_id are satisfied.
	siteID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO site (id, name, lat, lng, timezone) VALUES ($1, 'Test Site', 0.0, 0.0, 'UTC')`,
		siteID,
	)
	require.NoError(t, err)

	// Create an admin user + log them in to get a real session cookie.
	store := auth.NewStore(pool)
	hash, err := auth.Hash("test-password-123")
	require.NoError(t, err)
	userID, err := store.InsertAdminUser(ctx, "admin@test.example", "Admin", hash)
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

	// Register routes under the session middleware.
	r := chi.NewRouter()
	r.Use(sm.LoadAndSave)
	// Login endpoint so tests can get a real session cookie.
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

	// Log in to get the session cookie.
	jar, _ := cookiejar.New(nil)
	cli := &http.Client{Jar: jar}
	loginBody, _ := json.Marshal(map[string]string{
		"email":    "admin@test.example",
		"password": "test-password-123",
	})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/auth/login", bytes.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "shifter")
	resp, err := cli.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "login must succeed before floor-plan tests")

	return &fixture{
		deps:      deps,
		server:    srv,
		client:    cli,
		siteID:    siteID,
		userID:    userID,
		imageRoot: tmpDir,
	}
}

// ─── Image byte helpers ───────────────────────────────────────────────────────

// smallPNG returns a valid 4×4 px PNG encoded as bytes.
func smallPNG(t *testing.T) []byte {
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

// smallJPEG returns a valid 4×4 px JPEG encoded as bytes.
func smallJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, nil))
	return buf.Bytes()
}

// fakePDFBytes returns the first 8 bytes of a PDF header, padded to 512.
// http.DetectContentType sniffs this as "application/pdf".
func fakePDFBytes() []byte {
	b := make([]byte, 512)
	copy(b, []byte("%PDF-1.4"))
	return b
}

// buildMultipart builds a multipart body for the upload endpoint.
// filename defaults to "image" if empty.
func buildMultipart(t *testing.T, imageBytes []byte, label, sortOrder, filename string) (body io.Reader, contentType string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	if filename == "" {
		filename = "image"
	}
	fw, err := mw.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = fw.Write(imageBytes)
	require.NoError(t, err)

	if label != "" {
		require.NoError(t, mw.WriteField("label", label))
	}
	if sortOrder != "" {
		require.NoError(t, mw.WriteField("sort_order", sortOrder))
	}
	require.NoError(t, mw.Close())
	return &buf, mw.FormDataContentType()
}

// doUpload posts to POST /api/sites/{siteID}/floor-plans.
func doUpload(t *testing.T, f *fixture, imageBytes []byte, label, sortOrder, filename string) *http.Response {
	t.Helper()
	body, ct := buildMultipart(t, imageBytes, label, sortOrder, filename)
	req, _ := http.NewRequest(http.MethodPost,
		f.server.URL+"/api/sites/"+f.siteID.String()+"/floor-plans",
		body)
	req.Header.Set("Content-Type", ct)
	resp, err := f.client.Do(req)
	require.NoError(t, err)
	return resp
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestImageUpload_PNG_HappyPath — POST a valid PNG; expect 201, DB row created,
// file present on disk, and label reflected in response.
func TestImageUpload_PNG_HappyPath(t *testing.T) {
	f := setupFixture(t)

	resp := doUpload(t, f, smallPNG(t), "Ground Floor", "1", "ground.png")
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var plan struct {
		ID        string `json:"ID"`
		Label     string `json:"Label"`
		SortOrder int    `json:"SortOrder"`
		ImagePath string `json:"ImagePath"`
		ImageW    int    `json:"ImageW"`
		ImageH    int    `json:"ImageH"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&plan))
	require.Equal(t, "Ground Floor", plan.Label)
	require.Equal(t, 1, plan.SortOrder)
	require.Greater(t, plan.ImageW, 0)
	require.Greater(t, plan.ImageH, 0)
	require.NotEmpty(t, plan.ImagePath)

	// Verify file exists on disk.
	absPath := filepath.Join(f.imageRoot, plan.ImagePath)
	_, err := os.Stat(absPath)
	require.NoError(t, err, "uploaded image file must exist on disk")

	// Verify DB row exists.
	ctx := context.Background()
	planID, err := uuid.Parse(plan.ID)
	require.NoError(t, err)
	row, err := f.deps.Queries.GetFloorPlan(ctx, pgtype.UUID{Bytes: planID, Valid: true})
	require.NoError(t, err)
	require.Equal(t, "Ground Floor", row.Label)
}

// TestImageUpload_RejectsPDF — a PDF body (magic bytes %PDF) must yield 415
// (Unsupported Media Type). Server never processes PDF (D-17); client converts
// PDF→PNG via pdf.js before uploading.
func TestImageUpload_RejectsPDF(t *testing.T) {
	f := setupFixture(t)

	resp := doUpload(t, f, fakePDFBytes(), "Floor", "", "plan.pdf")
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnsupportedMediaType, resp.StatusCode, "D-17: PDF must be rejected 415")

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Equal(t, "unsupported_mime_type", body["error"])

	// No file must have been written to disk.
	entries, err := os.ReadDir(f.imageRoot)
	require.NoError(t, err)
	require.Empty(t, entries, "no file must be written when PDF is rejected")
}

// TestImageUpload_RejectsOversizedFile — body larger than 10 MiB must yield
// 413 Request Entity Too Large (MaxBytesReader cap, D-19).
func TestImageUpload_RejectsOversizedFile(t *testing.T) {
	f := setupFixture(t)

	// Build a body that exceeds MaxUploadBytes (10 MiB).
	// Wrap valid PNG header + 11 MiB of zeros so it clearly exceeds the cap.
	png4 := smallPNG(t)
	oversized := make([]byte, floorplan.MaxUploadBytes+1)
	copy(oversized, png4)

	resp := doUpload(t, f, oversized, "Too Big", "", "big.png")
	defer resp.Body.Close()
	require.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode,
		"D-19: body > 10 MiB must yield 413")
}

// TestImageUpload_RejectsOversizedDimensions — a valid PNG whose dimensions
// exceed MaxDimension (8192 px) must yield 422. Check is header-only
// (image.DecodeConfig), no full decode (T-05-05-01).
func TestImageUpload_RejectsOversizedDimensions(t *testing.T) {
	f := setupFixture(t)

	// Synthesise a PNG with 9000×9000 declared dimensions.
	// We use png.Encode on an image.NRGBA with 9000×9000 allocation — this will
	// create a real, parseable PNG header that declares 9000×9000. The actual
	// encode writes scanlines, but ProbeDimensions only reads the IHDR chunk via
	// image.DecodeConfig (fast). We create a small image but override the size
	// declaration via a custom encoder approach.
	//
	// Simpler: create a real 9000×9000 image (0 alloc per pixel — NRGBA zero
	// value is transparent). Encoding a 9000×9000 solid-transparent PNG fits well
	// within 10 MiB when compressed.
	img := image.NewNRGBA(image.Rect(0, 0, floorplan.MaxDimension+1, floorplan.MaxDimension+1))
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	// Sanity: must be under MaxUploadBytes so the file-size check passes first.
	require.Less(t, buf.Len(), floorplan.MaxUploadBytes, "oversized-dim PNG must fit within upload cap")

	resp := doUpload(t, f, buf.Bytes(), "Huge Dims", "", "huge.png")
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode,
		"D-19: >8192 px dims must yield 422")

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Equal(t, "dimensions_too_large", body["error"])
}

// TestImageUpload_MimeSniffOverridesHeader — send JPEG bytes but claim
// Content-Type: image/png in the multipart part. Server must sniff the actual
// bytes and store the file with .jpg extension (T-05-05-02).
func TestImageUpload_MimeSniffOverridesHeader(t *testing.T) {
	f := setupFixture(t)

	jpegBytes := smallJPEG(t)

	// Build a multipart body that lies in the filename extension (says .png)
	// but contains JPEG bytes.
	body, ct := buildMultipart(t, jpegBytes, "Liar Floor", "", "lie.png")
	req, _ := http.NewRequest(http.MethodPost,
		f.server.URL+"/api/sites/"+f.siteID.String()+"/floor-plans",
		body)
	req.Header.Set("Content-Type", ct)
	resp, err := f.client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var plan struct {
		ImagePath string `json:"ImagePath"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&plan))

	// The stored file must use the sniffed extension (.jpg), not the lied extension.
	require.Equal(t, ".jpg", filepath.Ext(plan.ImagePath),
		"T-05-05-02: MIME sniff must override client-provided extension")
}

// TestImageUpload_AuditAndDBRollback — if the DB insert fails (e.g. duplicate
// sort_order / site_id UNIQUE violation), the file written to disk must be
// cleaned up and no file left behind.
//
// We simulate a conflict by uploading the same sort_order twice for the same
// site. The second upload should fail at the DB layer (23505 unique_violation)
// and the handler must remove the newly written file.
func TestImageUpload_AuditAndDBRollback(t *testing.T) {
	f := setupFixture(t)

	// First upload at sort_order=5 succeeds.
	resp1 := doUpload(t, f, smallPNG(t), "Floor A", "5", "a.png")
	defer resp1.Body.Close()
	require.Equal(t, http.StatusCreated, resp1.StatusCode)

	// Count files on disk after first upload.
	entries1, err := os.ReadDir(f.imageRoot)
	require.NoError(t, err)
	require.Len(t, entries1, 1)

	// Second upload at the same sort_order=5 → DB UNIQUE violation.
	resp2 := doUpload(t, f, smallPNG(t), "Floor B", "5", "b.png")
	defer resp2.Body.Close()
	require.Equal(t, http.StatusInternalServerError, resp2.StatusCode,
		"UNIQUE violation on (site_id, sort_order) should yield 500 db_create_failed")

	// The file count must remain 1 — the failed upload's file was cleaned up.
	entries2, err := os.ReadDir(f.imageRoot)
	require.NoError(t, err)
	require.Len(t, entries2, 1,
		"file written for failed upload must be cleaned up on DB rollback")
}

// TestMultiFloor — upload two floor plans; list must return them in sort_order
// order. Plan 05-05 requirement: ListBySiteHandler returns ordered results.
func TestMultiFloor(t *testing.T) {
	f := setupFixture(t)

	// Upload sort_order=2 first, sort_order=1 second.
	resp2 := doUpload(t, f, smallPNG(t), "Second Floor", "2", "second.png")
	defer resp2.Body.Close()
	require.Equal(t, http.StatusCreated, resp2.StatusCode)

	resp1 := doUpload(t, f, smallPNG(t), "Ground Floor", "1", "ground.png")
	defer resp1.Body.Close()
	require.Equal(t, http.StatusCreated, resp1.StatusCode)

	// List.
	req, _ := http.NewRequest(http.MethodGet,
		f.server.URL+"/api/sites/"+f.siteID.String()+"/floor-plans", nil)
	listResp, err := f.client.Do(req)
	require.NoError(t, err)
	defer listResp.Body.Close()
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	var plans []struct {
		Label     string `json:"Label"`
		SortOrder int    `json:"SortOrder"`
	}
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&plans))
	require.Len(t, plans, 2)
	require.Equal(t, 1, plans[0].SortOrder, "lower sort_order must come first")
	require.Equal(t, "Ground Floor", plans[0].Label)
	require.Equal(t, 2, plans[1].SortOrder)
	require.Equal(t, "Second Floor", plans[1].Label)
}

// TestReplaceKeepsPins — PATCH /api/floor-plans/{id} replaces the image but
// ALL device_floor_plan_placement rows survive (D-24 fractional-coord
// preservation across image swap).
func TestReplaceKeepsPins(t *testing.T) {
	f := setupFixture(t)
	ctx := context.Background()

	// Upload the initial floor plan.
	resp := doUpload(t, f, smallPNG(t), "Main Floor", "0", "main.png")
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var plan struct {
		ID        string `json:"ID"`
		ImagePath string `json:"ImagePath"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&plan))
	planID, err := uuid.Parse(plan.ID)
	require.NoError(t, err)
	oldImagePath := plan.ImagePath

	// Insert a device_profile + device so the placement FK is satisfied.
	profileID := uuid.New()
	_, err = f.deps.Pool.Exec(ctx,
		`INSERT INTO device_profile (id, slug, name, vendor, capabilities)
		 VALUES ($1, 'test-profile-01', 'Test Profile', 'Acme', '{}')`,
		profileID,
	)
	require.NoError(t, err, "device_profile insert must succeed")

	deviceID := uuid.New()
	_, err = f.deps.Pool.Exec(ctx,
		`INSERT INTO device (id, dev_eui, name, device_profile_id)
		 VALUES ($1, 'aabbccddeeff0011', 'Test Device', $2)`,
		deviceID, profileID,
	)
	require.NoError(t, err, "device insert must succeed")

	_, err = f.deps.Pool.Exec(ctx,
		`INSERT INTO device_floor_plan_placement (device_id, floor_plan_id, x_frac, y_frac)
		 VALUES ($1, $2, 0.3, 0.7)`,
		deviceID, planID,
	)
	require.NoError(t, err, "placement insert must succeed")

	// Verify pin exists.
	pinCount, err := f.deps.Queries.CountPinsOnFloorPlan(ctx, pgtype.UUID{Bytes: planID, Valid: true})
	require.NoError(t, err)
	require.Equal(t, int64(1), pinCount)

	// Replace the image.
	newPNG := smallJPEG(t) // different bytes / format
	body, ct := buildMultipart(t, newPNG, "", "", "replacement.jpg")
	patchReq, _ := http.NewRequest(http.MethodPatch,
		f.server.URL+"/api/floor-plans/"+plan.ID,
		body)
	patchReq.Header.Set("Content-Type", ct)
	patchResp, err := f.client.Do(patchReq)
	require.NoError(t, err)
	defer patchResp.Body.Close()
	require.Equal(t, http.StatusOK, patchResp.StatusCode)

	var updated struct {
		ImagePath string `json:"ImagePath"`
	}
	require.NoError(t, json.NewDecoder(patchResp.Body).Decode(&updated))
	require.NotEqual(t, oldImagePath, updated.ImagePath,
		"image_path must change after replace")

	// Pin count must be unchanged.
	pinCount2, err := f.deps.Queries.CountPinsOnFloorPlan(ctx, pgtype.UUID{Bytes: planID, Valid: true})
	require.NoError(t, err)
	require.Equal(t, int64(1), pinCount2, "D-24: placement rows must survive image replacement")

	// Old image file must have been unlinked.
	_, statErr := os.Stat(filepath.Join(f.imageRoot, oldImagePath))
	require.True(t, os.IsNotExist(statErr),
		"old image file must be unlinked after successful PATCH")

	// New image file must exist.
	_, statErr2 := os.Stat(filepath.Join(f.imageRoot, updated.ImagePath))
	require.NoError(t, statErr2, "new image file must exist after PATCH")
}
