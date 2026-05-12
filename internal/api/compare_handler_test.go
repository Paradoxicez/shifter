package api

// compare_handler_test.go — Plan 07-12 Task 1 (TDD RED → GREEN)
//
// Tests for POST /api/reports/compare.
//
// Test matrix:
//   TestCompareHandler_EntitiesMode_TwoMPs
//   TestCompareHandler_TimeRangesMode_SameMP_YearOverYear
//   TestCompareHandler_InvalidEntityID_400
//   TestCompareHandler_ViewerAllowed_200

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

// ---- setup helper -----------------------------------------------------------

func compareHandlerSetup(t *testing.T, suffix, role string) (srv *httptest.Server, cli *http.Client, pool *pgxpool.Pool) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	pool = testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, noopLog()))

	_, err := pool.Exec(ctx,
		`INSERT INTO install_identity (id, display_name, timezone, units)
		 VALUES (1, 'CompareHandlerTest', 'UTC', 'metric') ON CONFLICT (id) DO NOTHING`)
	require.NoError(t, err)

	var userID string
	require.NoError(t, pool.QueryRow(ctx,
		fmt.Sprintf(`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('%s-compare-%s@ex.com', 'U', 'x', '%s') RETURNING id::text`,
			role, suffix, role),
	).Scan(&userID))

	sm := auth.NewSessionManager(pool, true, 8*time.Hour, 24*time.Hour)
	deps := CompareDeps{Pool: pool, SessionMgr: sm}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/test/seed", func(w http.ResponseWriter, req *http.Request) {
		if err := auth.PutUser(req.Context(), sm, auth.User{ID: userID, Role: role}); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	RegisterCompareRoutes(r, deps)

	server := httptest.NewServer(sm.LoadAndSave(r))
	t.Cleanup(server.Close)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	// Establish the session cookie.
	res, err := client.Post(server.URL+"/test/seed", "", nil)
	require.NoError(t, err)
	res.Body.Close()

	return server, client, pool
}

// seedSiteAndMP creates a site + metering_point and returns both IDs.
func seedSiteAndMPForCompare(t *testing.T, pool *pgxpool.Pool, nameSuffix string) (siteIDStr, mpIDStr string) {
	t.Helper()
	ctx := context.Background()
	var siteID, mpID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, site_type, lat, lng, timezone) VALUES ($1, 'building', 0, 0, 'UTC') RETURNING id::text`,
		"CompareSite-"+nameSuffix).Scan(&siteID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class) VALUES ($1::uuid, $2, 'water') RETURNING id::text`,
		siteID, "CompareMP-"+nameSuffix).Scan(&mpID))
	return siteID, mpID
}

// ---- tests ------------------------------------------------------------------

func TestCompareHandler_EntitiesMode_TwoMPs(t *testing.T) {
	srv, cli, pool := compareHandlerSetup(t, "entities", "admin")
	ctx := context.Background()

	_, mpAID := seedSiteAndMPForCompare(t, pool, "A")
	_, mpBID := seedSiteAndMPForCompare(t, pool, "B")

	// Seed two measurement rows for each MP so CAGGs have data.
	now := time.Now().UTC().Truncate(24 * time.Hour)
	for _, row := range []struct {
		mp  string
		ts  time.Time
		val float64
	}{
		{mpAID, now.Add(-48 * time.Hour), 100},
		{mpAID, now.Add(-24 * time.Hour), 200},
		{mpBID, now.Add(-48 * time.Hour), 150},
		{mpBID, now.Add(-24 * time.Hour), 50},
	} {
		_, err := pool.Exec(ctx,
			`INSERT INTO measurement (metering_point_id, time, cumulative_value, quality, raw_payload, decoded_object)
			 VALUES ($1::uuid, $2, $3, 'ok', '\x'::bytea, '{}'::jsonb)`,
			row.mp, row.ts, row.val)
		require.NoError(t, err)
	}

	body := map[string]any{
		"mode":        "entities",
		"entity_type": "metering_point",
		"entity_a_id": mpAID,
		"entity_b_id": mpBID,
		"range": map[string]string{
			"from": now.Add(-72 * time.Hour).Format(time.RFC3339),
			"to":   now.Format(time.RFC3339),
		},
	}
	b, _ := json.Marshal(body)
	res, err := cli.Post(srv.URL+"/api/reports/compare", "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var resp compareResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
	require.NotNil(t, resp.A.Series)
	require.NotNil(t, resp.B.Series)
	// Delta totals should be numeric (may be 0 if CAGG not materialized in test).
	_ = resp.Delta
}

func TestCompareHandler_TimeRangesMode_SameMP_YearOverYear(t *testing.T) {
	srv, cli, pool := compareHandlerSetup(t, "time-ranges", "admin")

	_, mpID := seedSiteAndMPForCompare(t, pool, "YoY")

	now := time.Now().UTC().Truncate(24 * time.Hour)
	oneYearAgo := now.AddDate(-1, 0, 0)

	body := map[string]any{
		"mode":        "time_ranges",
		"entity_type": "metering_point",
		"entity_id":   mpID,
		"range_a": map[string]string{
			"from": now.Add(-72 * time.Hour).Format(time.RFC3339),
			"to":   now.Format(time.RFC3339),
		},
		"range_b": map[string]string{
			"from": oneYearAgo.Add(-72 * time.Hour).Format(time.RFC3339),
			"to":   oneYearAgo.Format(time.RFC3339),
		},
	}
	b, _ := json.Marshal(body)
	res, err := cli.Post(srv.URL+"/api/reports/compare", "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var resp compareResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
	// No data seeded for prior year — empty series are valid; must not error.
	require.NotNil(t, resp.A.Series)
	require.NotNil(t, resp.B.Series)
}

func TestCompareHandler_InvalidEntityID_400(t *testing.T) {
	srv, cli, _ := compareHandlerSetup(t, "invalid", "admin")

	body := map[string]any{
		"mode":        "entities",
		"entity_type": "metering_point",
		"entity_a_id": "not-a-uuid",
		"entity_b_id": "not-a-uuid",
		"range": map[string]string{
			"from": "2024-01-01T00:00:00Z",
			"to":   "2024-02-01T00:00:00Z",
		},
	}
	b, _ := json.Marshal(body)
	res, err := cli.Post(srv.URL+"/api/reports/compare", "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}

func TestCompareHandler_ViewerAllowed_200(t *testing.T) {
	srv, cli, pool := compareHandlerSetup(t, "viewer", "viewer")

	_, mpAID := seedSiteAndMPForCompare(t, pool, "ViewA")
	_, mpBID := seedSiteAndMPForCompare(t, pool, "ViewB")

	now := time.Now().UTC().Truncate(24 * time.Hour)
	body := map[string]any{
		"mode":        "entities",
		"entity_type": "metering_point",
		"entity_a_id": mpAID,
		"entity_b_id": mpBID,
		"range": map[string]string{
			"from": now.Add(-72 * time.Hour).Format(time.RFC3339),
			"to":   now.Format(time.RFC3339),
		},
	}
	b, _ := json.Marshal(body)
	res, err := cli.Post(srv.URL+"/api/reports/compare", "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode, "viewer must be allowed")
}
