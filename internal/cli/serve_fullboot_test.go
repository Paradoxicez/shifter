package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/argon2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/chirpstack"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/device"
	httpapi "github.com/shifter-io/shifter/internal/http"
	"github.com/shifter-io/shifter/internal/ingest"
	"github.com/shifter-io/shifter/internal/profile"
	"github.com/shifter-io/shifter/internal/resolver"
	"github.com/shifter-io/shifter/internal/swap"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// fullBootHarness boots the entire Phase 2 wiring (matching what
// internal/cli/serve.go's RunE constructs) against {Postgres testcontainer,
// Mosquitto testcontainer, bufconn ChirpStack v4 mock} and a real httptest
// listener. Each test owns its own harness so test isolation is preserved.
type fullBootHarness struct {
	ctx        context.Context
	cancel     context.CancelFunc
	pool       *pgxpool.Pool
	mqttURL    string
	csClient   *chirpstack.Client
	csTenantID string
	csAppID    string
	resolver   *resolver.Resolver
	httpSrv    *httptest.Server
	mqttSub    *chirpstack.MQTTSubscriber
	adminEmail string
	adminPass  string

	// Seeded fixtures (populated by seedFixture).
	siteID    uuid.UUID
	mpID      uuid.UUID
	profileID uuid.UUID
	devEUI    string
	deviceID  uuid.UUID
	bindingID uuid.UUID
}

func (h *fullBootHarness) close() {
	if h.cancel != nil {
		h.cancel()
	}
	if h.httpSrv != nil {
		h.httpSrv.Close()
	}
	if h.mqttSub != nil {
		h.mqttSub.Shutdown(2 * time.Second)
	}
}

// startFullBootHarness spins up Postgres + Mosquitto + bufconn ChirpStack and
// constructs the same Phase 2 wiring serve.go's RunE constructs. degradedNoCS
// switches off the CS gRPC client construction (Test 4) — DeviceDeps + Swap
// Deps + ProfileDeps stay nil and the router only mounts Phase 1 routes.
func startFullBootHarness(t *testing.T, degradedNoCS bool) *fullBootHarness {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	h := &fullBootHarness{ctx: ctx, cancel: cancel}

	// 1. Postgres + migrations.
	h.pool = testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, h.pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

	// 2. Mosquitto.
	h.mqttURL = testsupport.StartMosquitto(t)

	// 3. ChirpStack bufconn mock + Client + bootstrap.
	if !degradedNoCS {
		dialer, _, _ := testsupport.NewChirpStackMockBufWithHandles(t)
		grpcConn, err := grpc.NewClient("passthrough:///bufnet",
			grpc.WithContextDialer(dialer),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = grpcConn.Close() })

		h.csClient = chirpstack.NewClient(grpcConn)

		// Seed chirpstack_connection row so EnsureTenantAndApplication can
		// persist its UUIDs.
		_, err = h.pool.Exec(ctx, `
			INSERT INTO chirpstack_connection
			    (id, mode, grpc_url, api_token_ref, mqtt_url, mqtt_user, mqtt_password_ref, region_name, region_common_name)
			VALUES (1, 'bundled', 'bufnet', 'cs_api_token', $1, NULL, NULL, 'AS923_2', 'AS923-2')
			ON CONFLICT (id) DO NOTHING`, h.mqttURL)
		require.NoError(t, err)

		csConnStore := NewConnectionStore(h.pool)
		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		tID, aID, err := chirpstack.EnsureTenantAndApplication(ctx, h.csClient, csConnStore, log)
		require.NoError(t, err)
		h.csTenantID = tID
		h.csAppID = aID
	} else {
		// Degraded path also still needs a chirpstack_connection row for
		// FirstRunGate / install state, but with NULL CS UUIDs.
		_, err := h.pool.Exec(ctx, `
			INSERT INTO chirpstack_connection
			    (id, mode, grpc_url, api_token_ref, mqtt_url, mqtt_user, mqtt_password_ref, region_name, region_common_name)
			VALUES (1, 'external', '', '', $1, NULL, NULL, 'AS923_2', 'AS923-2')
			ON CONFLICT (id) DO NOTHING`, h.mqttURL)
		require.NoError(t, err)
	}

	// 4. Admin user (so FirstRunGate allows /api/* + login path works +
	//    RBAC tests get a real session). Per migration 0004 the install_state
	//    row is DELETED after wizard finish, so the canonical post-install
	//    state is "admin user exists, install_state empty."
	h.adminEmail = "fullboot-admin@example.com"
	h.adminPass = "fullboot-pw-32bytes-123456789012"
	hashed := argon2HashForTest(h.adminPass)
	_, adminErr := h.pool.Exec(ctx, `
		INSERT INTO "user" (email, name, password_hash, role)
		VALUES ($1, 'Admin', $2, 'admin')
		ON CONFLICT (email) DO NOTHING`, h.adminEmail, hashed)
	require.NoError(t, adminErr)

	// 6. Resolver + listener goroutine (mirrors serve.go block 6c).
	h.resolver = resolver.New(&sqlcResolverLoader{pool: h.pool})
	go h.resolver.Run(ctx, h.pool, slog.New(slog.NewTextHandler(io.Discard, nil)))

	// 7. MQTT subscriber + ingest binding (mirrors serve.go block 6d).
	subLog := slog.New(slog.NewTextHandler(io.Discard, nil))
	sub, err := chirpstack.NewMQTTSubscriber(
		h.mqttURL, "", "", "shifter-fullboot-"+t.Name(),
		subLog, nil,
	)
	require.NoError(t, err)
	h.mqttSub = sub
	t.Cleanup(func() { sub.Shutdown(2 * time.Second) })

	ingestDeps := ingest.Deps{
		Pool:     h.pool,
		Resolver: h.resolver,
		Mappings: &ingest.SQLCMappingStore{Pool: h.pool},
		Log:      subLog,
	}
	sub.SetUplinkHandler(ingest.UplinkHandler(ingestDeps))

	// 8. SessionMgr + auth wiring (mirrors serve.go).
	sm := scs.New()
	sm.Store = pgxstore.New(h.pool)
	sm.Lifetime = 24 * time.Hour
	sm.IdleTimeout = 8 * time.Hour
	sm.Cookie.Secure = false // dev mode
	sm.Cookie.HttpOnly = true
	limiter := auth.NewLoginLimiter()
	t.Cleanup(limiter.Stop)
	userStore := auth.NewStore(h.pool)

	// 9. Phase 2 deps — only when CS is wired (mirrors serve.go block 8).
	var (
		deviceDeps  *device.Deps
		swapDeps    *swap.HTTPDeps
		profileDeps *profile.HTTPDeps
	)
	if !degradedNoCS {
		csConnStore := NewConnectionStore(h.pool)
		bootstrapper := bootstrapperFunc(func(c context.Context) (string, string, error) {
			return chirpstack.EnsureTenantAndApplication(c, h.csClient, csConnStore, slog.New(slog.NewTextHandler(io.Discard, nil)))
		})
		deviceDeps = &device.Deps{
			Pool:       h.pool,
			SessionMgr: sm,
			Log:        subLog,
			CS:         h.csClient,
			Bootstrap:  bootstrapper,
			PingGRPC: func(c context.Context, _ string) error {
				return h.csClient.PingDevices(c, h.csAppID)
			},
			PingMQTT: func(c context.Context) error {
				return chirpstack.PingMQTT(c, h.mqttURL, "", "")
			},
		}
		swapDeps = &swap.HTTPDeps{
			Pool:       h.pool,
			SessionMgr: sm,
			Log:        subLog,
			Resolver:   h.resolver,
		}
		profileDeps = &profile.HTTPDeps{
			Pool:       h.pool,
			SessionMgr: sm,
			Log:        subLog,
			CSClient:   h.csClient,
			ConnStore:  csConnStore,
		}
	}

	// 10. Router + httptest.Server.
	router := httpapi.NewRouter(httpapi.Deps{
		Pool:         h.pool,
		SessionMgr:   sm,
		Log:          subLog,
		LoginLimiter: limiter,
		UserStore:    userStore,
		InstallStore: nil, // not exercised by these tests
		// InstallDeps + TestConnDeps left zero — Phase 2 tests don't hit them.
		SecretsDir:  t.TempDir(),
		DeviceDeps:  deviceDeps,
		SwapDeps:    swapDeps,
		ProfileDeps: profileDeps,
		SPA:         nil,
	})

	h.httpSrv = httptest.NewServer(router)
	return h
}

// argon2HashForTest produces a phc-formatted Argon2id hash compatible with
// internal/auth's verifier. Lifted from the auth package's hash format so we
// don't import its private hash helper.
func argon2HashForTest(password string) string {
	const (
		mem    uint32 = 64 * 1024
		t              = 1
		p      uint8  = 1
		keyLen uint32 = 32
	)
	salt := []byte("fullboot-saltsalt") // fixed for test determinism
	hash := argon2.IDKey([]byte(password), salt, uint32(t), mem, p, keyLen)
	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		mem, t, p,
		base64Encode(salt), base64Encode(hash),
	)
}

func base64Encode(b []byte) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	out := make([]byte, 0, ((len(b)+2)/3)*4)
	for i := 0; i < len(b); i += 3 {
		var n uint32
		switch {
		case i+2 < len(b):
			n = uint32(b[i])<<16 | uint32(b[i+1])<<8 | uint32(b[i+2])
			out = append(out, alphabet[(n>>18)&63], alphabet[(n>>12)&63], alphabet[(n>>6)&63], alphabet[n&63])
		case i+1 < len(b):
			n = uint32(b[i])<<16 | uint32(b[i+1])<<8
			out = append(out, alphabet[(n>>18)&63], alphabet[(n>>12)&63], alphabet[(n>>6)&63])
		default:
			n = uint32(b[i]) << 16
			out = append(out, alphabet[(n>>18)&63], alphabet[(n>>12)&63])
		}
	}
	return string(out)
}

// seedFixture inserts site + MP + axioma_w1 profile + binding + mappings so a
// canonical uplink can land. Sets harness.{siteID,mpID,profileID,devEUI,
// deviceID,bindingID}.
func (h *fullBootHarness) seedFixture(t *testing.T, suffix string) {
	t.Helper()
	ctx := h.ctx

	var siteIDStr string
	require.NoError(t, h.pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ($1, 'UTC') RETURNING id`,
		"site-fb-"+suffix).Scan(&siteIDStr))
	h.siteID = uuid.MustParse(siteIDStr)

	var mpIDStr string
	require.NoError(t, h.pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class) VALUES ($1, $2, 'water') RETURNING id`,
		siteIDStr, "mp-fb-"+suffix).Scan(&mpIDStr))
	h.mpID = uuid.MustParse(mpIDStr)

	var profileIDStr string
	require.NoError(t, h.pool.QueryRow(ctx,
		`SELECT id FROM device_profile WHERE slug = 'axioma_w1'`,
	).Scan(&profileIDStr))
	h.profileID = uuid.MustParse(profileIDStr)

	// Seed the 5 axioma_w1 mappings (json_pointer → canonical column) so the
	// ingest pipeline produces a non-empty cumulative_value.
	require.NoError(t, seedAxiomaMappings(ctx, h.pool, profileIDStr))

	h.devEUI = cliPadDevEUI("fb-" + suffix)
	var deviceIDStr string
	require.NoError(t, h.pool.QueryRow(ctx,
		`INSERT INTO device (dev_eui, name, device_profile_id) VALUES ($1, $2, $3) RETURNING id`,
		h.devEUI, "dev-fb-"+suffix, profileIDStr,
	).Scan(&deviceIDStr))
	h.deviceID = uuid.MustParse(deviceIDStr)

	bindingStart := time.Now().UTC().Add(-1 * time.Hour)
	var bindingIDStr string
	require.NoError(t, h.pool.QueryRow(ctx,
		`INSERT INTO binding (metering_point_id, device_id, valid_from, reading_offset)
		 VALUES ($1, $2, $3, 0) RETURNING id`,
		mpIDStr, deviceIDStr, bindingStart,
	).Scan(&bindingIDStr))
	h.bindingID = uuid.MustParse(bindingIDStr)
}

// seedAxiomaMappings inserts the 5 canonical axioma_w1 mapping rows so the
// ingest pipeline maps the v4 decoded.object payload to the cumulative
// + battery + temperature canonical columns.
func seedAxiomaMappings(ctx context.Context, pool *pgxpool.Pool, profileID string) error {
	rows := []struct {
		Pointer  string
		Target   string
		DataType string
		Position int
	}{
		{"/cumulative_l", "raw_value", "numeric", 0},
		{"/battery_pct", "battery", "int", 1},
		{"/temperature_c", "temperature", "numeric", 2},
		{"/leak", "extra.leak", "bool", 3},
		{"/tamper", "extra.tamper", "bool", 4},
	}
	for _, r := range rows {
		var scale pgtype.Numeric
		_ = scale.Scan("1")
		_, err := pool.Exec(ctx, `
			INSERT INTO device_profile_mapping
			    (device_profile_id, json_pointer, target, scale, data_type, position)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT DO NOTHING`,
			profileID, r.Pointer, r.Target, scale, r.DataType, r.Position)
		if err != nil {
			return err
		}
	}
	return nil
}

// loginAdmin authenticates against /api/auth/login and returns an http.Client
// with the resulting session cookie attached. CSRF protection in the auth
// handlers (RESEARCH §V13) requires `X-Requested-With: shifter` on every
// state-changing POST, so we send it.
func (h *fullBootHarness) loginAdmin(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second}

	body := fmt.Sprintf(`{"email":%q,"password":%q}`, h.adminEmail, h.adminPass)
	req, err := http.NewRequest(http.MethodPost, h.httpSrv.URL+"/api/auth/login", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "shifter")
	resp, err := client.Do(req)
	require.NoError(t, err, "login request")
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode, "admin login must succeed; body=%s", readBodyForTest(resp))
	return client
}

// authedPost issues an authenticated POST that survives CSRF check.
func (h *fullBootHarness) authedPost(t *testing.T, client *http.Client, urlStr, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, urlStr, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "shifter")
	resp, err := client.Do(req)
	require.NoError(t, err)
	return resp
}

func readBodyForTest(resp *http.Response) string {
	if resp == nil || resp.Body == nil {
		return ""
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return string(b)
}

// publishUplinkBlocking publishes a synthetic v4 uplink JSON to the broker
// on the canonical CS uplink topic, blocking until the publish ack returns.
func (h *fullBootHarness) publishUplinkBlocking(t *testing.T, devEUI string, decodedObject map[string]any) {
	t.Helper()
	opts := mqtt.NewClientOptions().
		AddBroker(h.mqttURL).
		SetClientID("fb-publisher-" + t.Name()).
		SetConnectTimeout(5 * time.Second)
	c := mqtt.NewClient(opts)
	tk := c.Connect()
	require.True(t, tk.WaitTimeout(5*time.Second), "publisher connect timed out")
	require.NoError(t, tk.Error(), "publisher connect error")
	defer c.Disconnect(250)

	event := map[string]any{
		"deduplicationId": uuid.NewString(),
		"time":            time.Now().UTC().Format(time.RFC3339Nano),
		"deviceInfo": map[string]any{
			"tenantId":      h.csTenantID,
			"applicationId": h.csAppID,
			"deviceName":    "fb-device",
			"devEui":        devEUI,
		},
		"fCnt":          uint32(1),
		"fPort":         uint32(85),
		"data":          "AAAA",
		"object":        decodedObject,
		"rxInfo": []map[string]any{{"gatewayId": "gw-1", "rssi": -60, "snr": 9.0}},
	}
	payload, err := json.Marshal(event)
	require.NoError(t, err)

	topic := fmt.Sprintf("application/%s/device/%s/event/up", h.csAppID, devEUI)
	pubTk := c.Publish(topic, 1, false, payload)
	require.True(t, pubTk.WaitTimeout(5*time.Second), "publish ack timed out")
	require.NoError(t, pubTk.Error(), "publish error")
}

// waitForMeasurementCount polls the measurement table for at least `target`
// rows on the harness's metering point. Fails the test on timeout.
func (h *fullBootHarness) waitForMeasurementCount(t *testing.T, target int, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var got int
	for time.Now().Before(deadline) {
		err := h.pool.QueryRow(h.ctx,
			`SELECT count(*) FROM measurement WHERE metering_point_id = $1`,
			h.mpID).Scan(&got)
		require.NoError(t, err)
		if got >= target {
			return got
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("waited %s for %d measurement row(s) on mp_id=%s; got %d", timeout, target, h.mpID, got)
	return got
}

// =============================================================================
// Tests
// =============================================================================

// TestServe_FullBoot_MQTTUplinkPersists is the literal proof that
// VERIFICATION.md Truth 2 + Truth 4 are now wired: a real MQTT publish on
// the canonical CS uplink topic results in a measurement row in the
// hypertable with cumulative_value populated.
func TestServe_FullBoot_MQTTUplinkPersists(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test — requires testcontainers")
	}
	h := startFullBootHarness(t, false)
	defer h.close()
	h.seedFixture(t, "uplink")

	// Wait for the resolver listener + MQTT subscriber to settle.
	time.Sleep(500 * time.Millisecond)

	h.publishUplinkBlocking(t, h.devEUI, map[string]any{
		"cumulative_l":  12345,
		"battery_pct":   87,
		"temperature_c": 23.0,
		"leak":          false,
		"tamper":        false,
	})

	count := h.waitForMeasurementCount(t, 1, 10*time.Second)
	require.GreaterOrEqual(t, count, 1, "ingest pipeline must produce at least 1 measurement row")

	// Assert the row has raw_payload non-empty + decoded_object non-empty +
	// cumulative_value populated (proves the FULL pipeline ran, not just the
	// persist step).
	var (
		rawLen   int
		decLen   int
		cumValid bool
	)
	require.NoError(t, h.pool.QueryRow(h.ctx, `
		SELECT
		    octet_length(raw_payload),
		    char_length(decoded_object::text),
		    cumulative_value IS NOT NULL
		FROM measurement
		WHERE metering_point_id = $1
		ORDER BY time DESC
		LIMIT 1`,
		h.mpID,
	).Scan(&rawLen, &decLen, &cumValid))
	require.Greater(t, rawLen, 0, "raw_payload must be non-empty")
	require.Greater(t, decLen, 2, "decoded_object must be non-empty (>'{}')")
	require.True(t, cumValid, "cumulative_value must be NOT NULL after a valid uplink")
}

// TestServe_FullBoot_SwapEndpointMounted proves Truth 3: the swap HTTP route
// is mounted on the production router and reaches CommitSwap from a real
// listener. We don't run the full swap math here — that's exercised by the
// swap package tests; we only assert "the route is reachable + 200" with
// admin auth.
func TestServe_FullBoot_SwapEndpointMounted(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test — requires testcontainers")
	}
	h := startFullBootHarness(t, false)
	defer h.close()
	h.seedFixture(t, "swap")

	// Need a SECOND device for the incoming side of the swap.
	incomingEUI := cliPadDevEUI("fb-swap-in")
	var incomingIDStr string
	require.NoError(t, h.pool.QueryRow(h.ctx,
		`INSERT INTO device (dev_eui, name, device_profile_id) VALUES ($1, $2, $3) RETURNING id`,
		incomingEUI, "dev-swap-in", h.profileID,
	).Scan(&incomingIDStr))

	client := h.loginAdmin(t)
	body := fmt.Sprintf(`{
		"incoming_device_id": %q,
		"outgoing_reading_r": "1000",
		"incoming_initial_n": "0",
		"operator_notes": "fullboot swap test"
	}`, incomingIDStr)

	urlStr := fmt.Sprintf("%s/api/metering-points/%s/swap", h.httpSrv.URL, h.mpID)
	resp := h.authedPost(t, client, urlStr, body)
	defer resp.Body.Close()
	bodyResp := readBodyForTest(resp)
	require.Equal(t, 200, resp.StatusCode, "swap endpoint must return 200; body=%s", bodyResp)
	require.Contains(t, bodyResp, "binding_id", "response must include the new binding_id")
}

// TestServe_FullBoot_ProfileEndpointMounted proves DATA-09 mapping editor has
// a backend: GET /api/device-profiles returns 200 with the seeded profiles.
func TestServe_FullBoot_ProfileEndpointMounted(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test — requires testcontainers")
	}
	h := startFullBootHarness(t, false)
	defer h.close()

	client := h.loginAdmin(t)
	resp, err := client.Get(h.httpSrv.URL + "/api/device-profiles")
	require.NoError(t, err)
	defer resp.Body.Close()
	body := readBodyForTest(resp)
	require.Equal(t, 200, resp.StatusCode, "GET /api/device-profiles must return 200; body=%s", body)

	// Seed migration 0010 inserts axioma_w1 + acrel_adw300; either should
	// appear in the response.
	require.True(t,
		strings.Contains(body, "axioma_w1") || strings.Contains(body, "acrel_adw300"),
		"profile list response must include at least one seeded profile slug; got: %s", body,
	)
}

// TestServe_FullBoot_DegradedMode_NoCS proves degraded boot still serves the
// Phase 1 surface when ChirpStack is not configured: /health returns 200,
// auth still works, but Phase 2 routes (POST /api/devices) are NOT mounted
// (DeviceDeps nil-guard fires → 405 method-not-allowed for the unmounted
// POST verb on /api/devices, since the read-only routes ARE mounted by
// site/MP packages).
func TestServe_FullBoot_DegradedMode_NoCS(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test — requires testcontainers")
	}
	h := startFullBootHarness(t, true)
	defer h.close()

	// /health is public and must work without auth.
	resp, err := http.Get(h.httpSrv.URL + "/health")
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode, "/health must return 200 even in degraded-no-CS mode")
	resp.Body.Close()

	// Login still works (auth doesn't depend on CS).
	client := h.loginAdmin(t)

	// Phase 1 surface intact — GET /api/sites should be 200 (admin can read
	// sites; site routes are mounted by Plan 02-10 even without CS).
	resp, err = client.Get(h.httpSrv.URL + "/api/sites")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode, "GET /api/sites must work in degraded mode; body=%s", readBodyForTest(resp))

	// POST /api/devices is NOT mounted — DeviceDeps==nil → router skips the
	// Phase 2 mount entirely. Expect 404 (or 405 if a sibling matches).
	resp = h.authedPost(t, client, h.httpSrv.URL+"/api/devices", "{}")
	defer resp.Body.Close()
	require.True(t,
		resp.StatusCode == 404 || resp.StatusCode == 405 || resp.StatusCode == 503,
		"POST /api/devices must NOT succeed in degraded mode (got %d); DeviceDeps nil-guard should prevent the mount",
		resp.StatusCode,
	)
}

