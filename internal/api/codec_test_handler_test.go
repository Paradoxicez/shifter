package api

// Tests for POST /api/device-profiles/{id}/test-codec handler.
// V2-VEND-02 backend half — goja sandbox test runner HTTP surface.

import (
	_ "embed"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
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

//go:embed testdata/axioma_w1.js
var axiomaCodecJS string

// axiomaTestHex is an 11-byte Axioma W1 frame (fPort=100):
//   byte 0: status=0x00
//   bytes 1-4: log_time LE
//   bytes 5-8: cumulative_l = 1000 LE
//   byte 9:  battery_pct = 75
//   byte 10: temperature_c = 24
var axiomaTestHex = hex.EncodeToString([]byte{
	0x00, 0x01, 0x02, 0x03, 0x04,
	0xE8, 0x03, 0x00, 0x00,
	0x4B, 0x18,
})

// codecHandlerSetup creates a real Postgres testcontainer, seeds one user,
// and returns the httptest server, cookie-jar HTTP client, and pgxpool.
func codecHandlerSetup(t *testing.T, suffix string, role string) (srv *httptest.Server, cli *http.Client, pool *pgxpool.Pool) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	pgPool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pgPool, noopLog()))

	var userID string
	require.NoError(t, pgPool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('`+role+`-`+suffix+`@ex.com', 'U', 'x', '`+role+`') RETURNING id::text`,
	).Scan(&userID))

	sm := auth.NewSessionManager(pgPool, true, 8*time.Hour, 24*time.Hour)
	deps := CodecTestDeps{Pool: pgPool, SessionMgr: sm, Log: noopLog()}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/test/seed", func(w http.ResponseWriter, req *http.Request) {
		if err := auth.PutUser(req.Context(), sm, auth.User{ID: userID, Role: role}); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	RegisterCodecTestRoute(r, deps)

	server := httptest.NewServer(sm.LoadAndSave(r))
	t.Cleanup(server.Close)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	// Seed session.
	res, err := client.Post(server.URL+"/test/seed", "", nil)
	require.NoError(t, err)
	res.Body.Close()

	return server, client, pgPool
}

// TestCodecTestHandler_AxiomaHappyPath — admin POST with valid axioma hex → 200 + decoded_json.
func TestCodecTestHandler_AxiomaHappyPath(t *testing.T) {
	srv, cli, pool := codecHandlerSetup(t, "happy", "admin")
	ctx := context.Background()

	// Seed codec_js into the axioma profile (the migration leaves it empty;
	// seed.go would fill it at boot with a live ChirpStack, but tests skip that).
	_, err := pool.Exec(ctx,
		`UPDATE device_profile SET codec_js = $1 WHERE slug = 'axioma_w1'`,
		axiomaCodecJS)
	require.NoError(t, err)

	var profileID string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT id::text FROM device_profile WHERE slug = 'axioma_w1'`,
	).Scan(&profileID))

	res, err := cli.Post(srv.URL+"/api/device-profiles/"+profileID+"/test-codec",
		"application/json",
		bytes.NewBufferString(`{"hex":"`+axiomaTestHex+`","fPort":100}`))
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
	require.Empty(t, resp["error_message"], "expected no error, got: %v", resp["error_message"])
	decoded, ok := resp["decoded_json"].(map[string]any)
	require.True(t, ok, "decoded_json should be object")
	require.Contains(t, decoded, "cumulative_l")
}

// TestCodecTestHandler_OversizedPayload — 257-byte hex → 400.
func TestCodecTestHandler_OversizedPayload(t *testing.T) {
	srv, cli, pool := codecHandlerSetup(t, "oversize", "admin")
	ctx := context.Background()

	_, err := pool.Exec(ctx,
		`UPDATE device_profile SET codec_js = $1 WHERE slug = 'axioma_w1'`, axiomaCodecJS)
	require.NoError(t, err)

	var profileID string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT id::text FROM device_profile WHERE slug = 'axioma_w1'`,
	).Scan(&profileID))

	overHex := hex.EncodeToString(make([]byte, 257))
	res, err := cli.Post(srv.URL+"/api/device-profiles/"+profileID+"/test-codec",
		"application/json",
		bytes.NewBufferString(`{"hex":"`+overHex+`","fPort":100}`))
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}

// TestCodecTestHandler_UnknownProfile — random UUID → 404.
func TestCodecTestHandler_UnknownProfile(t *testing.T) {
	srv, cli, _ := codecHandlerSetup(t, "notfound", "admin")

	res, err := cli.Post(srv.URL+"/api/device-profiles/"+uuid.New().String()+"/test-codec",
		"application/json",
		bytes.NewBufferString(`{"hex":"0102","fPort":1}`))
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

// TestCodecTestHandler_ViewerForbidden — viewer session → 403.
func TestCodecTestHandler_ViewerForbidden(t *testing.T) {
	srv, cli, _ := codecHandlerSetup(t, "viewer", "viewer")

	res, err := cli.Post(srv.URL+"/api/device-profiles/"+uuid.New().String()+"/test-codec",
		"application/json",
		bytes.NewBufferString(`{"hex":"0102","fPort":1}`))
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusForbidden, res.StatusCode)
}

// TestCodecTestHandler_TimeoutCodec — infinite-loop codec → 200 + error_message containing timeout/interrupted.
func TestCodecTestHandler_TimeoutCodec(t *testing.T) {
	srv, cli, pool := codecHandlerSetup(t, "timeout", "admin")

	var profileID string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO device_profile (slug, name, vendor, family, capabilities,
		 counter_modulus, codec_js, region, mac_version)
		 VALUES ('timeout-codec-api', 'Timeout Codec', 'Test', 'Test',
		         ARRAY['cumulative'], 1,
		         'function decodeUplink(input) { while(true) {} }',
		         NULL, 'LORAWAN_1_0_3')
		 RETURNING id::text`,
	).Scan(&profileID))

	res, err := cli.Post(srv.URL+"/api/device-profiles/"+profileID+"/test-codec",
		"application/json",
		bytes.NewBufferString(`{"hex":"0102","fPort":1}`))
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
	errMsg, _ := resp["error_message"].(string)
	require.True(t,
		strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "interrupted"),
		"expected timeout/interrupted error, got: %q", errMsg,
	)
}

// TestCodecTestHandler_InvalidHex — hex: "ZZZZ" → 400.
func TestCodecTestHandler_InvalidHex(t *testing.T) {
	srv, cli, pool := codecHandlerSetup(t, "invalidhex", "admin")
	ctx := context.Background()

	_, err := pool.Exec(ctx,
		`UPDATE device_profile SET codec_js = $1 WHERE slug = 'axioma_w1'`, axiomaCodecJS)
	require.NoError(t, err)

	var profileID string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT id::text FROM device_profile WHERE slug = 'axioma_w1'`,
	).Scan(&profileID))

	res, err := cli.Post(srv.URL+"/api/device-profiles/"+profileID+"/test-codec",
		"application/json",
		bytes.NewBufferString(`{"hex":"ZZZZ","fPort":1}`))
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}

// TestCodecTestHandler_NoAuditWritten — verifies no audit row created (D-08 scratch-pad).
func TestCodecTestHandler_NoAuditWritten(t *testing.T) {
	srv, cli, pool := codecHandlerSetup(t, "noaudit", "admin")
	ctx := context.Background()

	_, err := pool.Exec(ctx,
		`UPDATE device_profile SET codec_js = $1 WHERE slug = 'axioma_w1'`, axiomaCodecJS)
	require.NoError(t, err)

	var profileID string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT id::text FROM device_profile WHERE slug = 'axioma_w1'`,
	).Scan(&profileID))

	var beforeCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM audit_log`).Scan(&beforeCount))

	res, err := cli.Post(srv.URL+"/api/device-profiles/"+profileID+"/test-codec",
		"application/json",
		bytes.NewBufferString(`{"hex":"`+axiomaTestHex+`","fPort":100}`))
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var afterCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM audit_log`).Scan(&afterCount))
	require.Equal(t, beforeCount, afterCount, "test-codec must not write any audit log rows")
}

// noopLog returns a no-op *slog.Logger for tests.
func noopLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
