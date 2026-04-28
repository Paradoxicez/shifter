package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
	"github.com/stretchr/testify/require"
)

// TestHealth_Public — GET /health returns 200 with JSON
// {status, version, uptime_seconds} and does NOT require authentication.
// Verifies INST-06 reframed per D-19: the public endpoint must NOT include
// any "checks" payload (DB / CS / MQTT internals are admin-only via
// /health/detailed).
func TestHealth_Public(t *testing.T) {
	handler := Health()
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	handler(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	require.Equal(t, "ok", body["status"])
	require.NotNil(t, body["version"], "/health must carry version metadata")
	require.NotNil(t, body["uptime_seconds"], "/health must carry uptime_seconds")
	_, hasChecks := body["checks"]
	require.False(t, hasChecks, "D-18: /health must NOT include detailed checks (admin-only via /health/detailed)")
}

// TestHealthDetailed_RequiresAdmin — GET /health/detailed requires an
// admin session; it returns DB ping result.
// Anonymous returns 401, viewer returns 403, admin returns 200 with
// `checks.db = true`.
// Implements D-19: /health/detailed is wrapped in
// auth.RequireAction(sm, ActionHealthDetailed).
func TestHealthDetailed_RequiresAdmin(t *testing.T) {
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
	sm := auth.NewSessionManager(pool, true /*dev*/, time.Hour, 24*time.Hour)
	protected := auth.RequireAction(sm, auth.ActionHealthDetailed)(HealthDetailed(pool))

	mux := http.NewServeMux()
	mux.Handle("POST /seed", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := r.URL.Query().Get("role")
		if err := auth.PutUser(r.Context(), sm, auth.User{ID: "u1", Role: role}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.Handle("GET /health/detailed", protected)

	srv := httptest.NewServer(sm.LoadAndSave(mux))
	t.Cleanup(srv.Close)

	// No session → 401.
	res, err := http.Get(srv.URL + "/health/detailed")
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode, "T-10-02: anonymous must be 401")

	// Viewer → 403.
	j1, err := cookiejar.New(nil)
	require.NoError(t, err)
	cli1 := &http.Client{Jar: j1}
	seed1, err := cli1.Post(srv.URL+"/seed?role=viewer", "", nil)
	require.NoError(t, err)
	seed1.Body.Close()
	res, err = cli1.Get(srv.URL + "/health/detailed")
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusForbidden, res.StatusCode, "AUTH-06: viewer must not access /health/detailed")

	// Admin → 200 + body has checks.db = true.
	j2, err := cookiejar.New(nil)
	require.NoError(t, err)
	cli2 := &http.Client{Jar: j2}
	seed2, err := cli2.Post(srv.URL+"/seed?role=admin", "", nil)
	require.NoError(t, err)
	seed2.Body.Close()
	res, err = cli2.Get(srv.URL + "/health/detailed")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	var body map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	checks, ok := body["checks"].(map[string]any)
	require.True(t, ok, "/health/detailed body must carry checks object")
	require.Equal(t, true, checks["db"], "DB ping must succeed against the testcontainer pool")
	require.Equal(t, "ok", body["status"])
	require.NotNil(t, body["version"])
	require.NotNil(t, body["uptime_seconds"])
}
