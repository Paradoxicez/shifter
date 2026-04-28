package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
	"github.com/stretchr/testify/require"
)

type loginFixture struct {
	deps   LoginDeps
	server *httptest.Server
	client *http.Client
	email  string
	passwd string
}

func setupLogin(t *testing.T) *loginFixture {
	t.Helper()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

	store := NewStore(pool)
	hash, err := Hash("good-password-1234")
	require.NoError(t, err)
	_, err = store.InsertAdminUser(context.Background(), "alice@example.com", "Alice", hash)
	require.NoError(t, err)

	sm := NewSessionManager(pool, true /*dev*/, 8*time.Hour, 24*time.Hour)
	limiter := NewLoginLimiter()
	t.Cleanup(limiter.Stop)
	deps := LoginDeps{Store: store, SessionMgr: sm, LoginLimiter: limiter, Log: slog.New(slog.NewTextHandler(os.Stderr, nil))}

	mux := http.NewServeMux()
	mux.Handle("POST /api/auth/login", LoginHandler(deps))
	mux.Handle("POST /api/auth/logout", LogoutHandler(sm))
	mux.Handle("GET /me", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := GetUser(r.Context(), sm)
		if !ok {
			http.Error(w, "no", 401)
			return
		}
		_, _ = w.Write([]byte(u.ID + ":" + u.Role))
	}))

	srv := httptest.NewServer(sm.LoadAndSave(mux))
	t.Cleanup(srv.Close)
	cli := &http.Client{Jar: mustJar(t)}
	return &loginFixture{deps: deps, server: srv, client: cli, email: "alice@example.com", passwd: "good-password-1234"}
}

func loginPost(t *testing.T, f *loginFixture, email, pw string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": pw})
	req, _ := http.NewRequest("POST", f.server.URL+"/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "shifter")
	res, err := f.client.Do(req)
	require.NoError(t, err)
	return res
}

// TestLogin_Success — valid credentials yield 200, body includes user, and the
// shifter_session cookie is set on the response. AUTH-01.
func TestLogin_Success(t *testing.T) {
	f := setupLogin(t)
	res := loginPost(t, f, f.email, f.passwd)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var found bool
	for _, c := range res.Cookies() {
		if c.Name == "shifter_session" {
			found = true
		}
	}
	require.True(t, found, "AUTH-01: shifter_session cookie must be set")

	var body struct {
		User struct {
			ID, Email, Role string
		} `json:"user"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Equal(t, "alice@example.com", body.User.Email)
	require.Equal(t, "admin", body.User.Role)
	require.NotEmpty(t, body.User.ID)
}

// TestSessionPersistence — second request with the same cookie jar is treated
// as authenticated. AUTH-02 cross-request invariant.
func TestSessionPersistence(t *testing.T) {
	f := setupLogin(t)
	res := loginPost(t, f, f.email, f.passwd)
	res.Body.Close()

	res2, err := f.client.Get(f.server.URL + "/me")
	require.NoError(t, err)
	defer res2.Body.Close()
	require.Equal(t, http.StatusOK, res2.StatusCode, "AUTH-02: session must persist across requests")
}

// TestLogin_BadPassword — wrong password yields 401 with body
// `{error: "bad_credentials"}`. AUTH-01.
func TestLogin_BadPassword(t *testing.T) {
	f := setupLogin(t)
	res := loginPost(t, f, f.email, "wrong-password-...")
	defer res.Body.Close()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	var body map[string]string
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Equal(t, "bad_credentials", body["error"])
}

// TestLogin_UserNotFound — unknown email yields the same 401 + bad_credentials
// as a wrong password (no enumeration leak). The handler still calls Verify on
// a dummy hash so timing is comparable.
func TestLogin_UserNotFound(t *testing.T) {
	f := setupLogin(t)
	res := loginPost(t, f, "nobody@example.com", "anything")
	defer res.Body.Close()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	var body map[string]string
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Equal(t, "bad_credentials", body["error"])
}

// TestLogin_RateLimit_429 — 6th attempt from same IP returns 429 with a
// Retry-After header. AUTH-04.
func TestLogin_RateLimit_429(t *testing.T) {
	f := setupLogin(t)
	for i := 0; i < loginBurst; i++ {
		res := loginPost(t, f, f.email, "wrong-password-...")
		res.Body.Close()
	}
	res := loginPost(t, f, f.email, "wrong-password-...")
	defer res.Body.Close()
	require.Equal(t, http.StatusTooManyRequests, res.StatusCode, "AUTH-04")
	require.NotEmpty(t, res.Header.Get("Retry-After"))
}

// TestLogout_Idempotent — POST /logout returns 204; subsequent /me is 401.
func TestLogout_Idempotent(t *testing.T) {
	f := setupLogin(t)
	res := loginPost(t, f, f.email, f.passwd)
	res.Body.Close()

	req, _ := http.NewRequest("POST", f.server.URL+"/api/auth/logout", nil)
	req.Header.Set("X-Requested-With", "shifter")
	res2, err := f.client.Do(req)
	require.NoError(t, err)
	res2.Body.Close()
	require.Equal(t, http.StatusNoContent, res2.StatusCode)

	res3, err := f.client.Get(f.server.URL + "/me")
	require.NoError(t, err)
	res3.Body.Close()
	require.Equal(t, http.StatusUnauthorized, res3.StatusCode, "session destroyed")
}

// TestLogin_RequiresXRequestedWith — POST without X-Requested-With header is
// rejected with 400 (CSRF guard).
func TestLogin_RequiresXRequestedWith(t *testing.T) {
	f := setupLogin(t)
	body := []byte(`{"email":"x@example.com","password":"y"}`)
	req, _ := http.NewRequest("POST", f.server.URL+"/api/auth/login", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	res, err := f.client.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}

func mustJar(t *testing.T) http.CookieJar {
	t.Helper()
	j, err := cookiejar.New(nil)
	require.NoError(t, err)
	return j
}
