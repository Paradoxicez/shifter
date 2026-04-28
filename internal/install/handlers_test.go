package install

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/shifter-io/shifter/internal/config"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// realConnWrapper wraps a *grpc.ClientConn so it satisfies the csConn interface
// used by Step2Handler. Per Warning #6, exposes Conn() *grpc.ClientConn directly —
// no interface{} round-trip.
type realConnWrapper struct{ c *grpc.ClientConn }

func (r *realConnWrapper) Conn() *grpc.ClientConn { return r.c }
func (r *realConnWrapper) Close() error           { return r.c.Close() }

func dialMockFor(t *testing.T, mode string) func(context.Context, config.CSConfig) (csConn, error) {
	t.Helper()
	dial, _ := testsupport.NewChirpStackMockBuf(t, mode)
	return func(_ context.Context, _ config.CSConfig) (csConn, error) {
		conn, err := grpc.NewClient("passthrough:///bufnet",
			grpc.WithContextDialer(dial),
			grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, err
		}
		return &realConnWrapper{c: conn}, nil
	}
}

func setupHandlers(t *testing.T, mockMode string) (*httptest.Server, *Store, string, Deps) {
	t.Helper()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

	store := NewStore(pool)
	secretsDir := filepath.Join(t.TempDir(), "secrets")
	deps := Deps{
		Pool:       pool,
		Store:      store,
		SecretsDir: secretsDir,
		Log:        slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Dial:       dialMockFor(t, mockMode),
	}
	mux := http.NewServeMux()
	mux.Handle("GET /api/install/state", StateHandler(deps))
	mux.Handle("POST /api/install/step/1", Step1Handler(deps))
	mux.Handle("POST /api/install/step/2", Step2Handler(deps))
	mux.Handle("POST /api/install/step/3", Step3Handler(deps))
	mux.Handle("POST /api/install/step/4", Step4Handler(deps))
	mux.Handle("POST /api/install/finish", FinishHandler(deps))

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, store, secretsDir, deps
}

func post(t *testing.T, srv *httptest.Server, path string, body any) *http.Response {
	t.Helper()
	buf, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", srv.URL+path, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "shifter")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return res
}

func TestState_ReturnsSingleton(t *testing.T) {
	srv, _, _, _ := setupHandlers(t, "v4")
	res, err := http.Get(srv.URL + "/api/install/state")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, 200, res.StatusCode)
	var body map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Equal(t, float64(1), body["CurrentStep"])
}

func TestStep1_Hashes_Persists(t *testing.T) {
	srv, store, _, _ := setupHandlers(t, "v4")
	res := post(t, srv, "/api/install/step/1", map[string]string{
		"email": "Alice@Example.com", "name": "Alice", "password": "Strong-Pass-1!",
	})
	defer res.Body.Close()
	require.Equal(t, 200, res.StatusCode)

	st, _ := store.GetOrCreate(context.Background())
	require.Contains(t, string(st.Step1Admin), "$argon2id$", "AUTH-01: stored as Argon2id hash")
	require.Contains(t, string(st.Step1Admin), "alice@example.com", "lowercased email")
}

func TestStep1_RejectsWeakPassword(t *testing.T) {
	srv, _, _, _ := setupHandlers(t, "v4")
	res := post(t, srv, "/api/install/step/1", map[string]string{
		"email": "a@x", "name": "A", "password": "short",
	})
	defer res.Body.Close()
	require.Equal(t, 422, res.StatusCode)
}

func TestStep2_CapturesCS_v4(t *testing.T) {
	srv, _, secretsDir, _ := setupHandlers(t, "v4")
	_ = post(t, srv, "/api/install/step/1", map[string]string{
		"email": "a@x", "name": "A", "password": "Strong-Pass-1!",
	})
	res := post(t, srv, "/api/install/step/2", map[string]string{
		"mode": "bundled", "grpc_url": "test:8080", "api_token": "t",
		"mqtt_url": "tcp://test:1883",
	})
	defer res.Body.Close()
	require.Equal(t, 200, res.StatusCode)
	var body map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Equal(t, "v4.17.0", body["chirpstack_version"])

	_, err := os.Stat(filepath.Join(secretsDir, "chirpstack_api_token"))
	require.NoError(t, err, "step 2 must write api_token to secrets dir")
}

func TestStep2_RejectsV3(t *testing.T) {
	srv, _, _, _ := setupHandlers(t, "v3")
	_ = post(t, srv, "/api/install/step/1", map[string]string{
		"email": "a@x", "name": "A", "password": "Strong-Pass-1!",
	})
	res := post(t, srv, "/api/install/step/2", map[string]string{
		"mode": "bundled", "grpc_url": "test:8080", "api_token": "t",
		"mqtt_url": "tcp://test:1883",
	})
	defer res.Body.Close()
	require.Equal(t, 422, res.StatusCode, "INST-05: v3 must be refused")
	var body map[string]string
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Equal(t, "v3_detected", body["error"])
}

func TestStep3_PersistsRegionHandler(t *testing.T) {
	srv, store, _, _ := setupHandlers(t, "v4")
	res := post(t, srv, "/api/install/step/3", map[string]string{"name": "as923_2"})
	defer res.Body.Close()
	require.Equal(t, 200, res.StatusCode)
	st, _ := store.GetOrCreate(context.Background())
	require.Contains(t, string(st.Step3Region), "AS923_2")
}

func TestStep3_UnknownRegion(t *testing.T) {
	srv, _, _, _ := setupHandlers(t, "v4")
	res := post(t, srv, "/api/install/step/3", map[string]string{"name": "atlantis"})
	defer res.Body.Close()
	require.Equal(t, 422, res.StatusCode)
}

func TestStep4_PersistsIdentityHandler(t *testing.T) {
	srv, store, _, _ := setupHandlers(t, "v4")
	res := post(t, srv, "/api/install/step/4", map[string]any{
		"display_name": "Acme", "timezone": "Asia/Bangkok", "units": "metric",
	})
	defer res.Body.Close()
	require.Equal(t, 200, res.StatusCode)
	st, _ := store.GetOrCreate(context.Background())
	require.Contains(t, string(st.Step4Identity), "Acme")
}

func TestStep4_InvalidTimezone(t *testing.T) {
	srv, _, _, _ := setupHandlers(t, "v4")
	res := post(t, srv, "/api/install/step/4", map[string]any{
		"display_name": "Acme", "timezone": "Mars/Olympus", "units": "metric",
	})
	defer res.Body.Close()
	require.Equal(t, 422, res.StatusCode)
}

func TestCSRF_Required(t *testing.T) {
	srv, _, _, _ := setupHandlers(t, "v4")
	buf, _ := json.Marshal(map[string]string{"email": "a", "name": "b", "password": "Strong-Pass-1!"})
	req, _ := http.NewRequest("POST", srv.URL+"/api/install/step/1", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, 400, res.StatusCode)
}

func TestState_ReturnsGoneAfterFinish(t *testing.T) {
	srv, store, _, deps := setupHandlers(t, "v4")
	seedAllFourSteps(t, store)
	require.NoError(t, FinishSetup(context.Background(), deps))

	res, err := http.Get(srv.URL + "/api/install/state")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusGone, res.StatusCode)
}
