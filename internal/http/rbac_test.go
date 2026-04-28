package http

import (
	"context"
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

// rbacFixture is a minimal HTTP test bench:
//   - POST /seed sets the session user to {ID: "u1", Role: <role>} via PutUser
//   - POST /protected is wrapped in auth.RequireAction(sm, ActionConnectionEdit)
//
// The seed handler runs inside sm.LoadAndSave so it can write to the session
// store. Subsequent requests reuse the same cookie jar so the session ID
// flows back into the protected endpoint.
func setupRBAC(t *testing.T, role string) (*httptest.Server, *http.Client) {
	t.Helper()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

	sm := auth.NewSessionManager(pool, true /*dev*/, 8*time.Hour, 24*time.Hour)
	mux := http.NewServeMux()
	mux.Handle("POST /seed", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := auth.PutUser(r.Context(), sm, auth.User{ID: "u1", Role: role}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	protected := auth.RequireAction(sm, auth.ActionConnectionEdit)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	mux.Handle("POST /protected", protected)

	srv := httptest.NewServer(sm.LoadAndSave(mux))
	t.Cleanup(srv.Close)
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	return srv, &http.Client{Jar: jar}
}

// TestRBAC_AdminAllowed — An admin session can POST to admin-only endpoints
// (e.g. /api/users) and receives 2xx. AUTH-06 / Pattern 16.
func TestRBAC_AdminAllowed(t *testing.T) {
	srv, cli := setupRBAC(t, "admin")
	res, err := cli.Post(srv.URL+"/seed", "", nil)
	require.NoError(t, err)
	res.Body.Close()
	res, err = cli.Post(srv.URL+"/protected", "", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
}

// TestRBAC_ViewerForbidden — A viewer session POSTing to admin-only endpoints
// receives 403 (NOT 401 — viewer IS authenticated, just not authorized).
// AUTH-06 / T-10-01.
func TestRBAC_ViewerForbidden(t *testing.T) {
	srv, cli := setupRBAC(t, "viewer")
	res, err := cli.Post(srv.URL+"/seed", "", nil)
	require.NoError(t, err)
	res.Body.Close()
	res, err = cli.Post(srv.URL+"/protected", "", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusForbidden, res.StatusCode, "AUTH-06: viewer must not POST to admin-only endpoint")
}

// TestRBAC_NoSession — Anonymous request (no session at all) is rejected at
// the middleware before the handler runs. T-10-02.
func TestRBAC_NoSession(t *testing.T) {
	srv, _ := setupRBAC(t, "admin")
	cli := &http.Client{}
	res, err := cli.Post(srv.URL+"/protected", "", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode, "no session must be 401")
}
