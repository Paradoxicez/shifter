package mapapi_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/db"
	mapapi "github.com/shifter-io/shifter/internal/map"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// setupMapTest starts Postgres, runs migrations, seeds install_identity,
// and returns a Deps + session manager ready for handler tests.
// Also seeds a single admin user and returns its ID for session injection.
func setupMapTest(t *testing.T, capabilities string) (mapapi.Deps, *scs.SessionManager, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	seedInstallIdentity(t, pool, capabilities)

	// Seed admin user for authenticated tests.
	userID := uuid.New()
	_, err := pool.Exec(ctx, `
		INSERT INTO "user" (id, email, name, password_hash, role)
		VALUES ($1, 'admin@test.com', 'Test Admin', 'hashed', 'admin')
	`, userID)
	require.NoError(t, err, "seed admin user")

	sm := auth.NewSessionManager(pool, true, 8*time.Hour, 24*time.Hour)

	deps := mapapi.Deps{
		Pool:       pool,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError})),
		SessionMgr: sm,
	}
	return deps, sm, userID.String()
}

// seedInstallIdentity inserts or updates the install_identity singleton row.
func seedInstallIdentity(t *testing.T, pool *pgxpool.Pool, capabilities string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO install_identity (id, display_name, timezone, units, capabilities)
		VALUES (1, 'Test Install', 'UTC', 'metric', $1)
		ON CONFLICT (id) DO UPDATE SET capabilities = $1
	`, capabilities)
	require.NoError(t, err, "seed install_identity")
}

// newTestServer builds a httptest.Server with sm.LoadAndSave + a /seed route
// (for session injection) + the mapapi routes. Returns the server + a cookie
// jar client. Pattern mirrors internal/http/rbac_test.go.
func newTestServer(t *testing.T, deps mapapi.Deps, sm *scs.SessionManager, userID, role string) (*httptest.Server, *http.Client) {
	t.Helper()
	r := chi.NewRouter()
	r.Use(sm.LoadAndSave)

	// /seed sets the session user (mirrors rbac_test.go seedAdmin endpoint).
	r.Post("/seed", func(w http.ResponseWriter, req *http.Request) {
		if err := auth.PutUser(req.Context(), sm, auth.User{ID: userID, Role: role}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mapapi.RegisterRoutes(r, deps)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	cli := &http.Client{Jar: jar}

	// Seed the session via the /seed endpoint so the cookie jar gets the
	// session cookie.
	res, err := cli.Post(srv.URL+"/seed", "", nil)
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusNoContent, res.StatusCode, "seed session must succeed")

	return srv, cli
}

// TestMapData verifies the core site + gateway rollup behaviour:
//   - Site without lat/lng is omitted (cannot be plotted)
//   - mp_count, online_count, offline_count are correct
//   - today_consumption has both utility keys when capabilities='both'
//   - Gateway online flag reflects stats_refreshed_at staleness
func TestMapData(t *testing.T) {
	deps, sm, userID := setupMapTest(t, "both")
	ctx := context.Background()
	pool := deps.Pool

	// Seed device profiles (valid capability values per 0009_device_profile.up.sql).
	waterProfileID := uuid.New()
	_, err := pool.Exec(ctx, `
		INSERT INTO device_profile (id, slug, name, vendor, capabilities, counter_modulus, codec_js, cs_profile_id, mac_version, expected_interval_s)
		VALUES ($1, 'test-water', 'Test Water Meter', 'TestVendor', ARRAY['cumulative'], 16777216, '', gen_random_uuid(), '1.0.0', 3600)
	`, waterProfileID)
	require.NoError(t, err)

	elecProfileID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO device_profile (id, slug, name, vendor, capabilities, counter_modulus, codec_js, cs_profile_id, mac_version, expected_interval_s)
		VALUES ($1, 'test-elec', 'Test Electricity Meter', 'TestVendor', ARRAY['instant_power'], 16777216, '', gen_random_uuid(), '1.0.0', 3600)
	`, elecProfileID)
	require.NoError(t, err)

	// Seed sites: one with lat/lng, one without (must be excluded from map).
	siteWithCoordsID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO site (id, name, lat, lng, timezone) VALUES ($1, 'Alpha Site', 13.7563, 100.5018, 'UTC')
	`, siteWithCoordsID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO site (id, name, timezone) VALUES ($1, 'No Coords Site', 'UTC')
	`, uuid.New())
	require.NoError(t, err)

	// Seed metering points on Alpha Site: 1 water + 1 electricity.
	mpWaterID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO metering_point (id, site_id, name, utility_class) VALUES ($1, $2, 'MP Water', 'water')
	`, mpWaterID, siteWithCoordsID)
	require.NoError(t, err)

	mpElecID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO metering_point (id, site_id, name, utility_class) VALUES ($1, $2, 'MP Elec', 'electricity')
	`, mpElecID, siteWithCoordsID)
	require.NoError(t, err)

	// Seed devices: one online (seen 1 min ago, within 2×3600s window), one offline (seen 4h ago).
	devOnlineID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO device (id, dev_eui, name, device_profile_id, last_seen_at)
		VALUES ($1, '0000000000000001', 'Dev Online', $2, now() - INTERVAL '1 minute')
	`, devOnlineID, waterProfileID)
	require.NoError(t, err)

	devOfflineID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO device (id, dev_eui, name, device_profile_id, last_seen_at)
		VALUES ($1, '0000000000000002', 'Dev Offline', $2, now() - INTERVAL '4 hours')
	`, devOfflineID, elecProfileID)
	require.NoError(t, err)

	// Bind both devices to their MPs (active bindings: valid_to IS NULL).
	_, err = pool.Exec(ctx, `
		INSERT INTO binding (id, metering_point_id, device_id, valid_from)
		VALUES (gen_random_uuid(), $1, $2, now() - INTERVAL '1 day')
	`, mpWaterID, devOnlineID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO binding (id, metering_point_id, device_id, valid_from)
		VALUES (gen_random_uuid(), $1, $2, now() - INTERVAL '1 day')
	`, mpElecID, devOfflineID)
	require.NoError(t, err)

	// Seed gateways: one online (recent stats_refreshed_at) + one offline (stale).
	_, err = pool.Exec(ctx, `
		INSERT INTO gateway (id, gateway_id, name, region, lat, lng, stats_refreshed_at)
		VALUES ($1, 'aabbccdd11223344', 'GW Online', 'EU868', 13.7, 100.5, now() - INTERVAL '1 minute')
	`, uuid.New())
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO gateway (id, gateway_id, name, region, lat, lng, stats_refreshed_at)
		VALUES ($1, 'aabbccdd55667788', 'GW Offline', 'EU868', 13.8, 100.6, now() - INTERVAL '30 minutes')
	`, uuid.New())
	require.NoError(t, err)

	// Seed measurements for today (UTC).  Two rows per MP in the current UTC hour
	// so cumulative_delta = last - first > 0.  With materialized_only=false the
	// CAGG returns the current-hour bucket via real-time mode (no explicit refresh
	// needed).
	// measurement requires: raw_payload BYTEA NOT NULL, decoded_object JSONB NOT NULL.
	now := time.Now().UTC()
	_, err = pool.Exec(ctx, `
		INSERT INTO measurement (time, metering_point_id, raw_value, cumulative_value, instant_value, quality, raw_payload, decoded_object)
		VALUES
		  ($1, $2, 100, 100, 1.0, 'ok', '\x'::bytea, '{}'::jsonb),
		  ($3, $2, 110, 110, 1.1, 'ok', '\x'::bytea, '{}'::jsonb),
		  ($4, $5, 200, 200, 2.0, 'ok', '\x'::bytea, '{}'::jsonb),
		  ($6, $5, 220, 220, 2.2, 'ok', '\x'::bytea, '{}'::jsonb)
	`, now.Add(-2*time.Minute), mpWaterID,
		now.Add(-1*time.Minute), // same MP, second reading (water)
		now.Add(-2*time.Minute), mpElecID,
		now.Add(-1*time.Minute)) // same MP, second reading (elec)
	require.NoError(t, err)

	// GET /api/map/data with authenticated session.
	srv, cli := newTestServer(t, deps, sm, userID, "admin")

	res, err := cli.Get(srv.URL + "/api/map/data")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var resp mapapi.Response
	require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))

	// Test 1: site without lat/lng excluded — only Alpha Site returned.
	require.Len(t, resp.Sites, 1, "only site with lat/lng must be returned")
	assert.Equal(t, "Alpha Site", resp.Sites[0].Name)

	// Test 2: mp_count, online_count, offline_count on the populated site.
	assert.Equal(t, int64(2), resp.Sites[0].MPCount, "mp_count must be 2")
	assert.Equal(t, int64(1), resp.Sites[0].OnlineCount, "online_count must be 1")
	assert.Equal(t, int64(1), resp.Sites[0].OfflineCount, "offline_count must be 1")

	// Test 3/4: capabilities='both' → both keys present in today_consumption.
	assert.Contains(t, resp.Sites[0].TodayConsumption, "water", "today_consumption must have water key")
	assert.Contains(t, resp.Sites[0].TodayConsumption, "electricity", "today_consumption must have electricity key")

	// Test 5: gateways with lat/lng: 2 returned, 1 online, 1 offline.
	require.Len(t, resp.Gateways, 2, "both gateways with lat/lng must be returned")
	onlineCount := 0
	for _, gw := range resp.Gateways {
		if gw.Online {
			onlineCount++
		}
	}
	assert.Equal(t, 1, onlineCount, "exactly one gateway must be online")
}

// TestMapData_CapabilityWaterOnly verifies D-09 capability gating:
// when capabilities='water', today_consumption has 'water' key only.
func TestMapData_CapabilityWaterOnly(t *testing.T) {
	deps, sm, userID := setupMapTest(t, "water")
	ctx := context.Background()
	pool := deps.Pool

	// Seed device profiles.
	waterProfileID := uuid.New()
	_, err := pool.Exec(ctx, `
		INSERT INTO device_profile (id, slug, name, vendor, capabilities, counter_modulus, codec_js, cs_profile_id, mac_version, expected_interval_s)
		VALUES ($1, 'test-cap-water', 'Test Water Meter', 'TestVendor', ARRAY['cumulative'], 16777216, '', gen_random_uuid(), '1.0.0', 3600)
	`, waterProfileID)
	require.NoError(t, err)

	// Seed one site with coords.
	siteID := uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO site (id, name, lat, lng, timezone) VALUES ($1, 'Cap Site', 1.0, 1.0, 'UTC')`, siteID)
	require.NoError(t, err)

	// Seed MPs for both utilities so that without capability filter we'd get both keys.
	mpWaterID := uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO metering_point (id, site_id, name, utility_class) VALUES ($1, $2, 'Cap Water', 'water')`, mpWaterID, siteID)
	require.NoError(t, err)

	mpElecID := uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO metering_point (id, site_id, name, utility_class) VALUES ($1, $2, 'Cap Elec', 'electricity')`, mpElecID, siteID)
	require.NoError(t, err)

	// Seed measurements and refresh CAGG (mirrors TestMapData pattern).
	// Two readings per MP in the current UTC minute so real-time CAGG returns them.
	now := time.Now().UTC()
	_, err = pool.Exec(ctx, `
		INSERT INTO measurement (time, metering_point_id, raw_value, cumulative_value, instant_value, quality, raw_payload, decoded_object)
		VALUES
		  ($1, $2, 50,  50,  0.5, 'ok', '\x'::bytea, '{}'::jsonb),
		  ($3, $2, 60,  60,  0.6, 'ok', '\x'::bytea, '{}'::jsonb),
		  ($4, $5, 200, 200, 2.0, 'ok', '\x'::bytea, '{}'::jsonb),
		  ($6, $5, 220, 220, 2.2, 'ok', '\x'::bytea, '{}'::jsonb)
	`, now.Add(-2*time.Minute), mpWaterID,
		now.Add(-1*time.Minute),
		now.Add(-2*time.Minute), mpElecID,
		now.Add(-1*time.Minute))
	require.NoError(t, err)

	srv, cli := newTestServer(t, deps, sm, userID, "admin")

	res, err := cli.Get(srv.URL + "/api/map/data")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var resp mapapi.Response
	require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))

	require.Len(t, resp.Sites, 1)

	// Test 4: capabilities='water' → only water key, NO electricity key.
	assert.Contains(t, resp.Sites[0].TodayConsumption, "water", "water key must be present when capabilities=water")
	assert.NotContains(t, resp.Sites[0].TodayConsumption, "electricity", "electricity key must NOT be present when capabilities=water")
}

// TestMapData_Unauthenticated_401 verifies T-05-04-01:
// GET /api/map/data without a session returns 401.
func TestMapData_Unauthenticated_401(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	require.NoError(t, db.RunMigrations(ctx, pool, log))
	seedInstallIdentity(t, pool, "both")

	sm := auth.NewSessionManager(pool, true, 8*time.Hour, 24*time.Hour)
	deps := mapapi.Deps{Pool: pool, SessionMgr: sm}

	r := chi.NewRouter()
	r.Use(sm.LoadAndSave)
	mapapi.RegisterRoutes(r, deps)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	// No session cookie — unauthenticated request.
	res, err := http.Get(srv.URL + "/api/map/data")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode, "unauthenticated GET /api/map/data must return 401")
}

// TestOSMTileURL verifies MAP-04: the response does NOT contain any tile URL,
// external API key reference, or map provider string.
// The OSM tile URL must only appear in the frontend MapView component, never
// in the backend API response.
func TestOSMTileURL(t *testing.T) {
	deps, sm, userID := setupMapTest(t, "both")

	srv, cli := newTestServer(t, deps, sm, userID, "admin")

	res, err := cli.Get(srv.URL + "/api/map/data")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	bodyBytes, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	body := string(bodyBytes)

	// MAP-04 invariant: no tile URL or API key in response.
	assert.NotContains(t, body, "tile.openstreetmap.org", "response must NOT contain OSM tile URL")
	assert.NotContains(t, body, "api_key", "response must NOT contain api_key string")
	assert.NotContains(t, body, "mapbox", "response must NOT contain mapbox reference")
}
