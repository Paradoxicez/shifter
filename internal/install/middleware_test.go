package install

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
	"github.com/stretchr/testify/require"
)

// setupGate spins up a TimescaleDB testcontainer with migrations applied,
// wires the FirstRunGate around an inner handler that echoes the path, and
// returns the live test server plus a seedAdmin closure for tests that need
// the post-install state.
func setupGate(t *testing.T) (*httptest.Server, *pgxpool.Pool, func(string)) {
	t.Helper()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

	gate := FirstRunGate(pool, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok:" + r.URL.Path))
	})
	srv := httptest.NewServer(gate(inner))
	t.Cleanup(srv.Close)

	seedAdmin := func(email string) {
		_, err := pool.Exec(context.Background(),
			`INSERT INTO "user" (email, name, password_hash, role) VALUES ($1, $1, $2, 'admin')`,
			email, "$argon2id$v=19$m=19456,t=2,p=1$AAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
		require.NoError(t, err)
	}
	return srv, pool, seedAdmin
}

func noFollow() *http.Client {
	return &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func TestFirstRun_Gate_RedirectsHTML(t *testing.T) {
	srv, _, _ := setupGate(t)
	res, err := noFollow().Get(srv.URL + "/")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusTemporaryRedirect, res.StatusCode)
	require.Equal(t, "/install", res.Header.Get("Location"))
}

func TestFirstRun_Gate_API_Returns409(t *testing.T) {
	srv, _, _ := setupGate(t)
	res, err := http.Get(srv.URL + "/api/settings/anything")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusConflict, res.StatusCode)
	require.Contains(t, res.Header.Get("Content-Type"), "application/json")
	body, _ := io.ReadAll(res.Body)
	var payload map[string]string
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Equal(t, "install_required", payload["error"])
}

func TestFirstRun_Gate_Whitelist(t *testing.T) {
	srv, _, _ := setupGate(t)
	for _, p := range []string{
		"/install",
		"/api/install/state",
		"/health",
		"/assets/index-abc.js",
		"/login",
		"/favicon.ico",
		"/logo.svg",
	} {
		res, err := http.Get(srv.URL + p)
		require.NoError(t, err)
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode, "whitelist path %s should pass through (got %d)", p, res.StatusCode)
		require.True(t, strings.HasPrefix(string(body), "ok:"), "inner handler should run for %s", p)
	}
}

func TestPostFinish_NoWizardAccess(t *testing.T) {
	srv, _, seed := setupGate(t)
	seed("alice@example.com")
	res, err := http.Get(srv.URL + "/dashboard")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode, "post-install: gate is pass-through")
}

// TestFirstRun_Gate_Cache verifies PITFALL #10 — once admin exists, the gate
// caches the answer and subsequent requests do NOT re-query the user table.
// We assert by counting calls to the user table from the test, dropping the
// table mid-test, and confirming the next request still passes.
func TestFirstRun_Gate_Cache(t *testing.T) {
	srv, pool, seed := setupGate(t)
	seed("admin@example.com")

	// First request primes the cache.
	res, err := http.Get(srv.URL + "/dashboard")
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	// Drop the user table — if the gate were still hitting the DB, the
	// next query would error. With the cache, the request must still pass.
	_, err = pool.Exec(context.Background(), `DROP TABLE "user" CASCADE`)
	require.NoError(t, err)

	res, err = http.Get(srv.URL + "/dashboard-2")
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode, "cache should bypass DB on subsequent requests")
}
