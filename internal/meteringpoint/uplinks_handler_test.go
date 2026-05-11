package meteringpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

type uplinksFixture struct {
	*mpFixture
}

func newUplinksFixture(t *testing.T) *uplinksFixture {
	t.Helper()
	return &uplinksFixture{newMPFixture(t)}
}

// seedMPForUplinks creates a metering point and returns its ID.
func (f *uplinksFixture) seedMPForUplinks(t *testing.T) string {
	t.Helper()
	var id string
	err := f.pool.QueryRow(context.Background(),
		`INSERT INTO metering_point (site_id, name, utility_class)
		 VALUES ($1, $2, $3) RETURNING id::text`,
		f.siteID, "Uplinks Test MP", "water",
	).Scan(&id)
	require.NoError(t, err)
	return id
}

// seedNMeasurements inserts n measurements for mpID with sequential cumulative values.
// Newest first (time = now - i*minute).
func seedNMeasurements(t *testing.T, pool *pgxpool.Pool, mpID string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		ts := time.Now().Add(-time.Duration(i+1) * time.Minute)
		_, err := pool.Exec(context.Background(),
			`INSERT INTO measurement
			     (time, metering_point_id, cumulative_value, raw_payload, decoded_object, quality)
			 VALUES ($1, $2::uuid, $3, '\xdeadbeef'::bytea, '{}'::jsonb, 'ok')`,
			ts, mpID, float64(n-i),
		)
		require.NoError(t, err)
	}
}

// TestUplinksHandler_HappyPath — returns uplinks with correct fields.
func TestUplinksHandler_HappyPath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, nopLogger()))

	f := &uplinksFixture{newMPFixtureWithPool(t, pool)}
	f.seedRole(t, "viewer")

	mpID := f.seedMPForUplinks(t)
	seedNMeasurements(t, pool, mpID, 5)

	res := f.doJSON(t, "GET", "/api/metering-points/"+mpID+"/uplinks?limit=10", nil)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)

	var body UplinkListResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))

	assert.Len(t, body.Uplinks, 5)
	assert.False(t, body.HasMore)
	assert.Nil(t, body.NextBefore)

	// Verify hex format of raw_payload (0xdeadbeef → "de ad be ef").
	assert.Equal(t, "de ad be ef", body.Uplinks[0].RawPayloadHex)
}

// TestUplinksHandler_LimitExceeded — limit=501 → 400.
func TestUplinksHandler_LimitExceeded(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, nopLogger()))

	f := &uplinksFixture{newMPFixtureWithPool(t, pool)}
	f.seedRole(t, "viewer")

	mpID := f.seedMPForUplinks(t)

	res := f.doJSON(t, "GET", "/api/metering-points/"+mpID+"/uplinks?limit=501", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}

// TestUplinksHandler_InvalidQuality — quality=bogus → 400.
func TestUplinksHandler_InvalidQuality(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, nopLogger()))

	f := &uplinksFixture{newMPFixtureWithPool(t, pool)}
	f.seedRole(t, "viewer")

	mpID := f.seedMPForUplinks(t)

	res := f.doJSON(t, "GET", "/api/metering-points/"+mpID+"/uplinks?quality=bogus", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}

// TestUplinksHandler_InvalidUUID — malformed MP ID → 400.
func TestUplinksHandler_InvalidUUID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, nopLogger()))

	f := &uplinksFixture{newMPFixtureWithPool(t, pool)}
	f.seedRole(t, "viewer")

	res := f.doJSON(t, "GET", "/api/metering-points/not-a-uuid/uplinks", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}

// TestUplinksHandler_CursorPagination — verifies cursor-based pagination:
// - first page: has_more=true, next_before set
// - second page with cursor: has_more=false
func TestUplinksHandler_CursorPagination(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, nopLogger()))

	f := &uplinksFixture{newMPFixtureWithPool(t, pool)}
	f.seedRole(t, "viewer")

	mpID := f.seedMPForUplinks(t)
	// Insert 15 measurements.
	seedNMeasurements(t, pool, mpID, 15)

	// First page: limit=10.
	res1 := f.doJSON(t, "GET", "/api/metering-points/"+mpID+"/uplinks?limit=10", nil)
	defer res1.Body.Close()
	require.Equal(t, http.StatusOK, res1.StatusCode)

	var page1 UplinkListResponse
	require.NoError(t, json.NewDecoder(res1.Body).Decode(&page1))

	assert.Len(t, page1.Uplinks, 10)
	assert.True(t, page1.HasMore, "first page should have more")
	require.NotNil(t, page1.NextBefore)

	// Second page: use cursor from first page.
	res2 := f.doJSON(t, "GET", fmt.Sprintf(
		"/api/metering-points/%s/uplinks?limit=10&before=%s",
		mpID, *page1.NextBefore,
	), nil)
	defer res2.Body.Close()
	require.Equal(t, http.StatusOK, res2.StatusCode)

	var page2 UplinkListResponse
	require.NoError(t, json.NewDecoder(res2.Body).Decode(&page2))

	assert.Len(t, page2.Uplinks, 5, "remaining 5 rows on second page")
	assert.False(t, page2.HasMore, "second page has no more")

	// Verify no duplicate rows across pages.
	seen := make(map[string]bool)
	for _, u := range page1.Uplinks {
		seen[u.Time] = true
	}
	for _, u := range page2.Uplinks {
		assert.False(t, seen[u.Time], "row %s should not appear in both pages", u.Time)
	}
}

// TestUplinksHandler_QualityFilter — quality=decode_fail returns only flagged rows.
func TestUplinksHandler_QualityFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, nopLogger()))

	f := &uplinksFixture{newMPFixtureWithPool(t, pool)}
	f.seedRole(t, "viewer")

	mpID := f.seedMPForUplinks(t)

	// 3 ok + 2 decode_fail.
	for i := 0; i < 3; i++ {
		seedMeasurementAt(t, pool, mpID, float64(i), "ok", time.Now().Add(-time.Duration(i+1)*time.Minute))
	}
	seedMeasurementAt(t, pool, mpID, 10, "decode_fail", time.Now().Add(-5*time.Minute))
	seedMeasurementAt(t, pool, mpID, 11, "decode_fail", time.Now().Add(-6*time.Minute))

	res := f.doJSON(t, "GET", "/api/metering-points/"+mpID+"/uplinks?quality=decode_fail", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var body UplinkListResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))

	assert.Len(t, body.Uplinks, 2)
	for _, u := range body.Uplinks {
		assert.Equal(t, "decode_fail", u.Quality)
	}
}

// newMPFixtureWithPool creates an mpFixture using an already-migrated pool.
// Used by uplinks tests that need a fresh pool with migrations already applied.
func newMPFixtureWithPool(t *testing.T, pool *pgxpool.Pool) *mpFixture {
	t.Helper()
	ctx := context.Background()

	var adminID, viewerID, siteID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('admin-uplinks@example.com', 'Admin Uplinks', 'x', 'admin') RETURNING id::text`,
	).Scan(&adminID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('viewer-uplinks@example.com', 'Viewer Uplinks', 'x', 'viewer') RETURNING id::text`,
	).Scan(&viewerID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('Uplinks Test Site', 'UTC') RETURNING id::text`,
	).Scan(&siteID))

	sm := auth.NewSessionManager(pool, true, 8*time.Hour, 24*time.Hour)
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
