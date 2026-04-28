package auth

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
	"github.com/stretchr/testify/require"
)

func setupSessionManager(t *testing.T, devMode bool, idle, lifetime time.Duration) *scs.SessionManager {
	t.Helper()
	pool := testsupport.StartPostgres(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	require.NoError(t, db.RunMigrations(context.Background(), pool, logger))
	sm := NewSessionManager(pool, devMode, idle, lifetime)
	return sm
}

func newJar(t *testing.T) http.CookieJar {
	t.Helper()
	j, err := cookiejar.New(nil)
	require.NoError(t, err)
	return j
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}

// TestNewSessionManager_DevMode_InsecureCookie — devMode=true → Cookie.Secure=false (D-23).
func TestNewSessionManager_DevMode_InsecureCookie(t *testing.T) {
	sm := setupSessionManager(t, true, 8*time.Hour, 24*time.Hour)
	require.False(t, sm.Cookie.Secure, "D-23: Cookie.Secure must be false in dev")
	require.True(t, sm.Cookie.HttpOnly)
	require.Equal(t, http.SameSiteLaxMode, sm.Cookie.SameSite)
	require.Equal(t, "shifter_session", sm.Cookie.Name)
	require.Equal(t, "/", sm.Cookie.Path)
	require.Empty(t, sm.Cookie.Domain)
}

// TestNewSessionManager_ProdMode_SecureCookie — devMode=false → Cookie.Secure=true.
func TestNewSessionManager_ProdMode_SecureCookie(t *testing.T) {
	sm := setupSessionManager(t, false, 8*time.Hour, 24*time.Hour)
	require.True(t, sm.Cookie.Secure, "D-22: Cookie.Secure must be true in prod")
}

// TestNewSessionManager_CookieFlags — HttpOnly=true, SameSite=Lax, Path="/", Domain="" always.
func TestNewSessionManager_CookieFlags(t *testing.T) {
	sm := setupSessionManager(t, false, 8*time.Hour, 24*time.Hour)
	require.True(t, sm.Cookie.HttpOnly)
	require.Equal(t, http.SameSiteLaxMode, sm.Cookie.SameSite)
	require.Equal(t, "/", sm.Cookie.Path)
	require.Empty(t, sm.Cookie.Domain)
	require.Equal(t, "shifter_session", sm.Cookie.Name)
}

// TestPutGetUser_RoundTrip — PutUser stores; GetUser returns same user.
func TestPutGetUser_RoundTrip(t *testing.T) {
	sm := setupSessionManager(t, false, 8*time.Hour, 24*time.Hour)
	userID := "00000000-0000-0000-0000-000000000001"
	handler := sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			require.NoError(t, PutUser(r.Context(), sm, User{ID: userID, Role: "admin"}))
			return
		}
		u, ok := GetUser(r.Context(), sm)
		if !ok {
			http.Error(w, "no session", http.StatusUnauthorized)
			return
		}
		w.Header().Set("X-User-ID", u.ID)
		w.Header().Set("X-User-Role", u.Role)
	}))
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	cli := &http.Client{Jar: newJar(t)}

	res, err := cli.Get(srv.URL + "/login")
	require.NoError(t, err)
	res.Body.Close()

	res, err = cli.Get(srv.URL + "/me")
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Equal(t, userID, res.Header.Get("X-User-ID"))
	require.Equal(t, "admin", res.Header.Get("X-User-Role"))
}

// TestGetUser_NoSession_ReturnsZero — empty context → ok=false.
func TestGetUser_NoSession_ReturnsZero(t *testing.T) {
	sm := setupSessionManager(t, false, 8*time.Hour, 24*time.Hour)
	_, ok := GetUser(context.Background(), sm)
	require.False(t, ok)
}

// TestSession_IdleTimeout — session inactive > IdleTimeout returns no user.
func TestSession_IdleTimeout(t *testing.T) {
	sm := setupSessionManager(t, false, 50*time.Millisecond, 24*time.Hour)

	handler := sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			_ = PutUser(r.Context(), sm, User{ID: "uuid", Role: "admin"})
			return
		}
		if _, ok := GetUser(r.Context(), sm); !ok {
			http.Error(w, "expired", http.StatusUnauthorized)
			return
		}
	}))
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	cli := &http.Client{Jar: newJar(t)}
	res, err := cli.Get(srv.URL + "/login")
	require.NoError(t, err)
	res.Body.Close()

	time.Sleep(150 * time.Millisecond) // exceed IdleTimeout

	res, err = cli.Get(srv.URL + "/me")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode, "AUTH-02: idle session must expire")
}

// TestSession_DevSecureToggle — verifies the dev/prod Cookie.Secure toggle (D-23).
func TestSession_DevSecureToggle(t *testing.T) {
	dev := setupSessionManager(t, true, 8*time.Hour, 24*time.Hour)
	require.False(t, dev.Cookie.Secure)

	prod := setupSessionManager(t, false, 8*time.Hour, 24*time.Hour)
	require.True(t, prod.Cookie.Secure)
}

// TestRotateOnLogin_InvalidatesOldToken — RenewToken invalidates the old token.
func TestRotateOnLogin_InvalidatesOldToken(t *testing.T) {
	sm := setupSessionManager(t, false, 8*time.Hour, 24*time.Hour)
	handler := sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/seed":
			_ = PutUser(r.Context(), sm, User{ID: "u1", Role: "admin"})
		case "/rotate":
			_ = RotateOnLogin(r.Context(), sm)
		case "/me":
			if _, ok := GetUser(r.Context(), sm); !ok {
				http.Error(w, "no", http.StatusUnauthorized)
			}
		}
	}))
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	cli := &http.Client{Jar: newJar(t)}
	res, err := cli.Get(srv.URL + "/seed")
	require.NoError(t, err)
	res.Body.Close()
	firstCookies := cli.Jar.Cookies(mustParseURL(t, srv.URL))
	require.NotEmpty(t, firstCookies, "expected session cookie after seed")

	res, err = cli.Get(srv.URL + "/rotate")
	require.NoError(t, err)
	res.Body.Close()

	res, err = cli.Get(srv.URL + "/me")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)
	res.Body.Close()

	// Reuse the OLD cookie value with a fresh client — should NOT be a valid session.
	cli2 := &http.Client{}
	req, err := http.NewRequest("GET", srv.URL+"/me", nil)
	require.NoError(t, err)
	for _, c := range firstCookies {
		req.AddCookie(c)
	}
	res, err = cli2.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode, "old token should be invalid after RenewToken")
}

// TestDestroy_RemovesSession — Destroy ends the session; subsequent GetUser is empty.
func TestDestroy_RemovesSession(t *testing.T) {
	sm := setupSessionManager(t, false, 8*time.Hour, 24*time.Hour)
	handler := sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			_ = PutUser(r.Context(), sm, User{ID: "uuid", Role: "admin"})
		case "/logout":
			_ = Destroy(r.Context(), sm)
		case "/me":
			if _, ok := GetUser(r.Context(), sm); !ok {
				http.Error(w, "no", http.StatusUnauthorized)
				return
			}
		}
	}))
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	cli := &http.Client{Jar: newJar(t)}
	res, err := cli.Get(srv.URL + "/login")
	require.NoError(t, err)
	res.Body.Close()

	res, err = cli.Get(srv.URL + "/me")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)
	res.Body.Close()

	res, err = cli.Get(srv.URL + "/logout")
	require.NoError(t, err)
	res.Body.Close()

	res, err = cli.Get(srv.URL + "/me")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode, "Destroy must end the session")
}

// TestEnsureCSRFToken_Stable — repeated calls within a session return the same token.
func TestEnsureCSRFToken_Stable(t *testing.T) {
	sm := setupSessionManager(t, false, 8*time.Hour, 24*time.Hour)
	var firstToken, secondToken string
	handler := sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := EnsureCSRFToken(r.Context(), sm)
		if r.URL.Path == "/first" {
			firstToken = token
		} else {
			secondToken = token
		}
		_, _ = w.Write([]byte(token))
	}))
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	cli := &http.Client{Jar: newJar(t)}
	res, _ := cli.Get(srv.URL + "/first")
	res.Body.Close()
	res, _ = cli.Get(srv.URL + "/second")
	res.Body.Close()

	require.NotEmpty(t, firstToken)
	require.Equal(t, firstToken, secondToken, "CSRF token must persist across requests in the same session")
}

// TestUserFromContext_NoSession_ReturnsErrNoUser — context-based helper returns ErrNoUser.
func TestUserFromContext_NoSession_ReturnsErrNoUser(t *testing.T) {
	sm := setupSessionManager(t, false, 8*time.Hour, 24*time.Hour)
	_, err := UserFromContext(context.Background(), sm)
	require.ErrorIs(t, err, ErrNoUser)
}

// TestUserFromContext_WithSession_ReturnsUser — when the SCS context is loaded, returns the user.
func TestUserFromContext_WithSession_ReturnsUser(t *testing.T) {
	sm := setupSessionManager(t, false, 8*time.Hour, 24*time.Hour)
	var seenID, seenRole string
	handler := sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			_ = PutUser(r.Context(), sm, User{ID: "abc", Role: "viewer"})
			return
		}
		u, err := UserFromContext(r.Context(), sm)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		seenID = u.ID
		seenRole = u.Role
	}))
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	cli := &http.Client{Jar: newJar(t)}
	res, _ := cli.Get(srv.URL + "/login")
	res.Body.Close()
	res, _ = cli.Get(srv.URL + "/me")
	res.Body.Close()

	require.Equal(t, "abc", seenID)
	require.Equal(t, "viewer", seenRole)
}
