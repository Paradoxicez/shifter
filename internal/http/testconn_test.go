package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/config"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
	"github.com/stretchr/testify/require"
	gogrpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// tcWrapper satisfies chirpStackConn against a bufconn-backed *grpc.ClientConn.
// Per Warning #6, exposes Conn() *grpc.ClientConn directly — no interface{}
// round-trip.
type tcWrapper struct{ c *gogrpc.ClientConn }

func (w *tcWrapper) Conn() *gogrpc.ClientConn { return w.c }
func (w *tcWrapper) Close() error             { return w.c.Close() }

// dialMockTC builds a TestConnDeps.Dial that returns a chirpStackConn wrapping
// a bufconn-backed gRPC client connected to a NewChirpStackMockBuf(mode) mock.
func dialMockTC(t *testing.T, mode string) func(context.Context, config.CSConfig) (chirpStackConn, error) {
	t.Helper()
	dial, _ := testsupport.NewChirpStackMockBuf(t, mode)
	return func(_ context.Context, _ config.CSConfig) (chirpStackConn, error) {
		c, err := gogrpc.NewClient("passthrough:///bufnet",
			gogrpc.WithContextDialer(dial),
			gogrpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, err
		}
		return &tcWrapper{c: c}, nil
	}
}

// dialErrorTC returns a Dial that always errors — simulates a refused gRPC URL.
func dialErrorTC() func(context.Context, config.CSConfig) (chirpStackConn, error) {
	return func(_ context.Context, _ config.CSConfig) (chirpStackConn, error) {
		return nil, errors.New("dial: connection refused")
	}
}

// setupTestConn builds an httptest.Server exposing only POST /api/settings/chirpstack/test
// and seeds a chirpstack_connection row so the server can resolve stored creds
// (kept for symmetry; the handler today reads creds from the request body).
func setupTestConn(t *testing.T, mode string, mqttErr error) *httptest.Server {
	t.Helper()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

	deps := TestConnDeps{
		Pool: pool,
		Log:  slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Dial: dialMockTC(t, mode),
		PingMQTT: func(_ context.Context, _, _, _ string) error {
			return mqttErr
		},
	}
	mux := http.NewServeMux()
	mux.Handle("POST /api/settings/chirpstack/test", TestConnHandler(deps))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func postTC(t *testing.T, srv *httptest.Server, body any) testConnResponse {
	t.Helper()
	buf, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", srv.URL+"/api/settings/chirpstack/test", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "shifter")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, 200, res.StatusCode)
	var out testConnResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&out))
	return out
}

// TestTestConn_Happy — v4 mock + reachable Mosquitto returns both reachable.
func TestTestConn_Happy(t *testing.T) {
	srv := setupTestConn(t, "v4", nil)
	out := postTC(t, srv, map[string]string{"grpc_url": "x", "api_token": "t", "mqtt_url": "tcp://m"})
	require.Equal(t, "reachable", out.GRPC.Status)
	require.Equal(t, "reachable", out.MQTT.Status)
}

// TestTestConn_V3Refused — v3 mock yields gRPC unreachable + MQTT skipped.
func TestTestConn_V3Refused(t *testing.T) {
	srv := setupTestConn(t, "v3", nil)
	out := postTC(t, srv, map[string]string{"grpc_url": "x", "api_token": "t", "mqtt_url": "tcp://m"})
	require.Equal(t, "unreachable", out.GRPC.Status)
	require.Contains(t, out.GRPC.Detail, "v3", "INST-05/CHIRP-03: gRPC detail must mention v3")
	require.Equal(t, "skipped", out.MQTT.Status, "RESEARCH §Pattern 14: MQTT skipped when gRPC fails")
}

// TestTestConn_BothFail — bogus URLs: gRPC unreachable; MQTT skipped (gRPC failed first).
func TestTestConn_BothFail(t *testing.T) {
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
	deps := TestConnDeps{
		Pool: pool,
		Log:  slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Dial: dialErrorTC(),
		PingMQTT: func(_ context.Context, _, _, _ string) error {
			return errors.New("connect: refused")
		},
	}
	mux := http.NewServeMux()
	mux.Handle("POST /api/settings/chirpstack/test", TestConnHandler(deps))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	out := postTC(t, srv, map[string]string{"grpc_url": "x", "api_token": "t", "mqtt_url": "tcp://m"})
	require.Equal(t, "unreachable", out.GRPC.Status)
	require.Equal(t, "skipped", out.MQTT.Status,
		"RESEARCH §Pattern 14: MQTT skipped when gRPC fails first — even when MQTT itself would also fail")
}

// TestTestConn_RequiresCSRFHeader — POST without X-Requested-With → 400.
func TestTestConn_RequiresCSRFHeader(t *testing.T) {
	srv := setupTestConn(t, "v4", nil)
	buf, _ := json.Marshal(map[string]string{"grpc_url": "x"})
	req, _ := http.NewRequest("POST", srv.URL+"/api/settings/chirpstack/test", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, 400, res.StatusCode)
}

// seedChirpStackConnection inserts a baseline chirpstack_connection row so
// GET handlers can read it. Mirrors what FinishSetup commits in production.
func seedChirpStackConnection(t *testing.T, srvDeps TestConnDeps) {
	t.Helper()
	_, err := srvDeps.Pool.Exec(context.Background(),
		`INSERT INTO chirpstack_connection
		    (id, mode, grpc_url, api_token_ref, mqtt_url, mqtt_user, mqtt_password_ref, region_name, region_common_name)
		    VALUES (1, 'bundled', 'chirpstack:8080', '/run/secrets/chirpstack_api_token',
		            'tcp://mosquitto:1883', 'shifter', NULL, 'as923_2', 'AS923_2')
		    ON CONFLICT (id) DO UPDATE SET grpc_url = EXCLUDED.grpc_url`)
	require.NoError(t, err)
}

// TestSettings_GetChirpStack_HidesAPIToken — GET response NEVER carries api_token.
func TestSettings_GetChirpStack_HidesAPIToken(t *testing.T) {
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
	deps := TestConnDeps{
		Pool:     pool,
		Log:      slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Dial:     dialMockTC(t, "v4"),
		PingMQTT: func(_ context.Context, _, _, _ string) error { return nil },
	}
	seedChirpStackConnection(t, deps)
	mux := http.NewServeMux()
	mux.Handle("GET /api/settings/chirpstack", GetChirpStackHandler(deps))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/api/settings/chirpstack")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, 200, res.StatusCode)
	body, _ := readJSON(res)
	require.Contains(t, body, "mode")
	require.Contains(t, body, "grpc_url")
	require.Contains(t, body, "mqtt_url")
	// Critical V8: api_token / api_token_ref MUST NOT appear in the response.
	require.NotContains(t, body, "api_token", "T-17-01 / V8: api_token must not be in GET response")
}

// TestSettings_PutChirpStack_RequiresAdmin — viewer PUT → 403 (RequireAction).
func TestSettings_PutChirpStack_RequiresAdmin(t *testing.T) {
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
	sm := auth.NewSessionManager(pool, true /*dev*/, 8*time.Hour, 24*time.Hour)
	deps := TestConnDeps{
		Pool:     pool,
		Log:      slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Dial:     dialMockTC(t, "v4"),
		PingMQTT: func(_ context.Context, _, _, _ string) error { return nil },
	}
	seedChirpStackConnection(t, deps)
	secretsDir := filepath.Join(t.TempDir(), "secrets")

	mux := http.NewServeMux()
	mux.Handle("POST /seed", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := auth.PutUser(r.Context(), sm, auth.User{ID: "u1", Role: "viewer"}); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(204)
	}))
	mux.Handle("PUT /api/settings/chirpstack",
		auth.RequireAction(sm, auth.ActionConnectionEdit)(PutChirpStackHandler(deps, secretsDir)))
	srv := httptest.NewServer(sm.LoadAndSave(mux))
	t.Cleanup(srv.Close)

	jar, _ := cookiejar.New(nil)
	cli := &http.Client{Jar: jar}
	res, err := cli.Post(srv.URL+"/seed", "", nil)
	require.NoError(t, err)
	res.Body.Close()

	body := map[string]string{
		"mode": "bundled", "grpc_url": "x:8080", "mqtt_url": "tcp://m:1883",
		"region_name": "as923_2", "region_common_name": "AS923_2",
	}
	buf, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", srv.URL+"/api/settings/chirpstack", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "shifter")
	res, err = cli.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, 403, res.StatusCode, "viewer must NOT edit chirpstack connection (T-17-04)")
}

// TestSettings_PutChirpStack_RejectsV3 — admin PUT against v3 mock → 422 v3_detected.
func TestSettings_PutChirpStack_RejectsV3(t *testing.T) {
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
	deps := TestConnDeps{
		Pool:     pool,
		Log:      slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Dial:     dialMockTC(t, "v3"),
		PingMQTT: func(_ context.Context, _, _, _ string) error { return nil },
	}
	seedChirpStackConnection(t, deps)
	secretsDir := filepath.Join(t.TempDir(), "secrets")

	mux := http.NewServeMux()
	// No auth wrapping: this test focuses on the 422 v3 contract; admin-vs-viewer
	// gate has its own dedicated test above.
	mux.Handle("PUT /api/settings/chirpstack", PutChirpStackHandler(deps, secretsDir))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	body := map[string]string{
		"mode": "bundled", "grpc_url": "x:8080", "api_token": "t",
		"mqtt_url": "tcp://m:1883",
		"region_name": "as923_2", "region_common_name": "AS923_2",
	}
	buf, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", srv.URL+"/api/settings/chirpstack", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "shifter")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, 422, res.StatusCode, "T-17-03: PUT against v3 must be rejected")
	var resp map[string]string
	require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
	require.Equal(t, "v3_detected", resp["error"])
}

// TestSettings_GetChirpStack_EmptyTable_Returns404 — GET with no rows in
// chirpstack_connection returns 404 with {"error":"not_configured"}.
func TestSettings_GetChirpStack_EmptyTable_Returns404(t *testing.T) {
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
	deps := TestConnDeps{
		Pool:     pool,
		Log:      slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Dial:     dialMockTC(t, "v4"),
		PingMQTT: func(_ context.Context, _, _, _ string) error { return nil },
	}
	// Intentionally do NOT seed a chirpstack_connection row.
	mux := http.NewServeMux()
	mux.Handle("GET /api/settings/chirpstack", GetChirpStackHandler(deps))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/api/settings/chirpstack")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, 404, res.StatusCode, "empty chirpstack_connection table must return 404")
	var resp map[string]string
	require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
	require.Equal(t, "not_configured", resp["error"], "body must be {\"error\":\"not_configured\"}")
}

// readJSON drains the response body and returns it as a string for substring
// assertions. Keeps the GET test honest — we're proving api_token never
// appears as a JSON KEY name, regardless of nesting.
func readJSON(res *http.Response) (string, error) {
	buf, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}
