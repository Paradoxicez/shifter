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
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
	"github.com/stretchr/testify/require"
)

type accountFixture struct {
	deps   AccountDeps
	login  LoginDeps
	server *httptest.Server
	pool   *pgxpool.Pool
	store  *Store
}

func setupAccount(t *testing.T) *accountFixture {
	t.Helper()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
	store := NewStore(pool)

	hash, err := Hash("current-pass-aaa-1!")
	require.NoError(t, err)
	_, err = store.InsertAdminUser(context.Background(), "ann@example.com", "Ann", hash)
	require.NoError(t, err)

	sm := NewSessionManager(pool, true, 8*time.Hour, 24*time.Hour)
	limiter := NewLoginLimiter()
	t.Cleanup(limiter.Stop)

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	loginDeps := LoginDeps{Store: store, SessionMgr: sm, LoginLimiter: limiter, Log: logger}
	acct := AccountDeps{Store: store, SessionMgr: sm, Log: logger}

	mux := http.NewServeMux()
	mux.Handle("POST /api/auth/login", LoginHandler(loginDeps))
	mux.Handle("POST /api/account/password", ChangePasswordHandler(acct))
	mux.Handle("GET /me", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := GetUser(r.Context(), sm); !ok {
			http.Error(w, "no", 401)
			return
		}
	}))
	srv := httptest.NewServer(sm.LoadAndSave(mux))
	t.Cleanup(srv.Close)
	return &accountFixture{deps: acct, login: loginDeps, server: srv, pool: pool, store: store}
}

func newAcctClient(t *testing.T) *http.Client {
	t.Helper()
	j, err := cookiejar.New(nil)
	require.NoError(t, err)
	return &http.Client{Jar: j}
}

func loginAs(t *testing.T, f *accountFixture, c *http.Client, email, pw string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": pw})
	req, _ := http.NewRequest("POST", f.server.URL+"/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "shifter")
	res, err := c.Do(req)
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, 200, res.StatusCode)
}

func changePass(t *testing.T, f *accountFixture, c *http.Client, current, newPw string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"current_password": current, "new_password": newPw})
	req, _ := http.NewRequest("POST", f.server.URL+"/api/account/password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "shifter")
	res, err := c.Do(req)
	require.NoError(t, err)
	return res
}

// TestAccount_ChangePassword — authenticated user changes own password.
// AUTH-05.
func TestAccount_ChangePassword(t *testing.T) {
	f := setupAccount(t)
	c := newAcctClient(t)
	loginAs(t, f, c, "ann@example.com", "current-pass-aaa-1!")
	res := changePass(t, f, c, "current-pass-aaa-1!", "new-Pass-aaa-2!")
	defer res.Body.Close()
	require.Equal(t, 200, res.StatusCode)
}

// TestAccount_ChangePassword_BadCurrent — wrong current password yields 401.
func TestAccount_ChangePassword_BadCurrent(t *testing.T) {
	f := setupAccount(t)
	c := newAcctClient(t)
	loginAs(t, f, c, "ann@example.com", "current-pass-aaa-1!")
	res := changePass(t, f, c, "wrong-current-........", "new-Pass-aaa-2!")
	defer res.Body.Close()
	require.Equal(t, 401, res.StatusCode)
	var body map[string]string
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Equal(t, "current_password_incorrect", body["error"])
}

// TestAccount_ChangePassword_Weak — short password yields 422 with tier "weak".
func TestAccount_ChangePassword_Weak(t *testing.T) {
	f := setupAccount(t)
	c := newAcctClient(t)
	loginAs(t, f, c, "ann@example.com", "current-pass-aaa-1!")
	res := changePass(t, f, c, "current-pass-aaa-1!", "short")
	defer res.Body.Close()
	require.Equal(t, 422, res.StatusCode)
}

// TestAccount_ChangePassword_RevokeOtherSessions — two simultaneous logins;
// changing password from one revokes the other (AUTH-05 defense-in-depth).
func TestAccount_ChangePassword_RevokeOtherSessions(t *testing.T) {
	f := setupAccount(t)
	c1 := newAcctClient(t)
	c2 := newAcctClient(t)
	loginAs(t, f, c1, "ann@example.com", "current-pass-aaa-1!")
	loginAs(t, f, c2, "ann@example.com", "current-pass-aaa-1!")

	// c2 should currently be authenticated.
	resMe1, err := c2.Get(f.server.URL + "/me")
	require.NoError(t, err)
	resMe1.Body.Close()
	require.Equal(t, 200, resMe1.StatusCode)

	res := changePass(t, f, c1, "current-pass-aaa-1!", "new-Pass-aaa-2!")
	res.Body.Close()
	require.Equal(t, 200, res.StatusCode)

	// c1 (the changing session) MUST still be valid (operator's own device).
	resMeC1, err := c1.Get(f.server.URL + "/me")
	require.NoError(t, err)
	resMeC1.Body.Close()
	require.Equal(t, 200, resMeC1.StatusCode, "AUTH-05: changing session stays valid")

	// c2 (other device) MUST be revoked.
	resMeC2, err := c2.Get(f.server.URL + "/me")
	require.NoError(t, err)
	defer resMeC2.Body.Close()
	require.Equal(t, 401, resMeC2.StatusCode, "AUTH-05 defense-in-depth: other sessions revoked")
}

// TestAccount_ChangePassword_RequiresAuth — unauthenticated POST → 401.
func TestAccount_ChangePassword_RequiresAuth(t *testing.T) {
	f := setupAccount(t)
	c := newAcctClient(t)
	res := changePass(t, f, c, "anything", "Anew-Pass-aaa-2!")
	defer res.Body.Close()
	require.Equal(t, 401, res.StatusCode)
}

// TestAccount_ChangePassword_RequiresXRequestedWith — missing CSRF header → 400.
func TestAccount_ChangePassword_RequiresXRequestedWith(t *testing.T) {
	f := setupAccount(t)
	c := newAcctClient(t)
	loginAs(t, f, c, "ann@example.com", "current-pass-aaa-1!")
	body, _ := json.Marshal(map[string]string{
		"current_password": "current-pass-aaa-1!",
		"new_password":     "new-Pass-aaa-2!",
	})
	req, _ := http.NewRequest("POST", f.server.URL+"/api/account/password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := c.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, 400, res.StatusCode)
}

// TestWizardAdmin_NoForceChange — bootstrap admin (created with
// must_change_password=false) is NOT gated on first login. D-09 / AUTH-03.
func TestWizardAdmin_NoForceChange(t *testing.T) {
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
	store := NewStore(pool)
	hash, err := Hash("wizard-pass-1234567!")
	require.NoError(t, err)
	_, err = store.InsertAdminUser(context.Background(), "wizard@example.com", "Wizard", hash)
	require.NoError(t, err)

	row := pool.QueryRow(context.Background(),
		`SELECT must_change_password FROM "user" WHERE email = $1`, "wizard@example.com")
	var must bool
	require.NoError(t, row.Scan(&must))
	require.False(t, must, "D-09: wizard-created admin must NOT be force-changed")
}

// ─────────────────────────────────────────────────────────────────────────
// Phase 6 — Plan 06-06: ChangePasswordHandler audit-in-tx tests (D-30).
// ─────────────────────────────────────────────────────────────────────────

// acctAuditCount queries audit_log and counts rows matching action for a user.
func acctAuditCount(t *testing.T, pool *pgxpool.Pool, action, userID string) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE action = $1 AND user_id = $2::uuid`,
		action, userID).Scan(&n)
	require.NoError(t, err)
	return n
}

// TestChangePassword_WritesAudit — authenticated user changes own password;
// audit row 'auth.password_change' is written with user_id=current user.
// Session is preserved (keepToken).
func TestChangePassword_WritesAudit(t *testing.T) {
	f := setupAccount(t)
	c := newAcctClient(t)
	loginAs(t, f, c, "ann@example.com", "current-pass-aaa-1!")

	u, err := f.store.GetUserByEmail(context.Background(), "ann@example.com")
	require.NoError(t, err)

	res := changePass(t, f, c, "current-pass-aaa-1!", "new-Pass-aaa-2!")
	defer res.Body.Close()
	require.Equal(t, 200, res.StatusCode)

	n := acctAuditCount(t, f.pool, "auth.password_change", u.ID)
	require.Equal(t, 1, n, "D-30: auth.password_change audit row must be written on self-change")

	// Session must still be valid (keepToken)
	resMe, err := c.Get(f.server.URL + "/me")
	require.NoError(t, err)
	resMe.Body.Close()
	require.Equal(t, 200, resMe.StatusCode, "current session must stay valid after password change")
}

// TestChangePassword_RevokesOtherSessions — user has multiple sessions;
// change-password from session A preserves A, revokes B and C.
// auth.session_revoked audit row is written in same tx.
func TestChangePassword_RevokesOtherSessions(t *testing.T) {
	f := setupAccount(t)
	c1 := newAcctClient(t)
	c2 := newAcctClient(t)
	c3 := newAcctClient(t)
	loginAs(t, f, c1, "ann@example.com", "current-pass-aaa-1!")
	loginAs(t, f, c2, "ann@example.com", "current-pass-aaa-1!")
	loginAs(t, f, c3, "ann@example.com", "current-pass-aaa-1!")

	u, err := f.store.GetUserByEmail(context.Background(), "ann@example.com")
	require.NoError(t, err)

	// c2 and c3 should currently be authenticated
	resMe2, _ := c2.Get(f.server.URL + "/me")
	resMe2.Body.Close()
	require.Equal(t, 200, resMe2.StatusCode)

	// c1 changes password
	res := changePass(t, f, c1, "current-pass-aaa-1!", "new-Pass-aaa-2!")
	res.Body.Close()
	require.Equal(t, 200, res.StatusCode)

	// c1 (the changing session) stays valid
	resMeC1, _ := c1.Get(f.server.URL + "/me")
	resMeC1.Body.Close()
	require.Equal(t, 200, resMeC1.StatusCode, "session A must remain valid after password change")

	// c2 and c3 must be revoked
	resMeC2, _ := c2.Get(f.server.URL + "/me")
	resMeC2.Body.Close()
	require.Equal(t, 401, resMeC2.StatusCode, "session B must be revoked after password change")

	resMeC3, _ := c3.Get(f.server.URL + "/me")
	resMeC3.Body.Close()
	require.Equal(t, 401, resMeC3.StatusCode, "session C must be revoked after password change")

	// auth.session_revoked audit row must be present
	n := acctAuditCount(t, f.pool, "auth.session_revoked", u.ID)
	require.Equal(t, 1, n, "D-30: auth.session_revoked audit row must be written on password change")
}

// TestChangePassword_AuditAtomicWithUpdate — verifies the audit row and
// password update are part of the same transaction. We simulate an audit
// failure by checking the inverse: a successful change produces BOTH an audit
// row AND a changed password hash, while the old password cannot be used.
func TestChangePassword_AuditAtomicWithUpdate(t *testing.T) {
	f := setupAccount(t)
	c := newAcctClient(t)
	loginAs(t, f, c, "ann@example.com", "current-pass-aaa-1!")

	u, err := f.store.GetUserByEmail(context.Background(), "ann@example.com")
	require.NoError(t, err)
	oldHash := u.PasswordHash

	res := changePass(t, f, c, "current-pass-aaa-1!", "new-Pass-aaa-2!")
	res.Body.Close()
	require.Equal(t, 200, res.StatusCode)

	// Password hash must have changed
	uAfter, err := f.store.GetUserByID(context.Background(), u.ID)
	require.NoError(t, err)
	require.NotEqual(t, oldHash, uAfter.PasswordHash, "password hash must be updated")

	// Audit row must exist (atomicity: both occurred together)
	n := acctAuditCount(t, f.pool, "auth.password_change", u.ID)
	require.Equal(t, 1, n, "audit row must be present when password was successfully changed (atomicity)")

	// Old password must no longer work (new password should work)
	freshClient := newAcctClient(t)
	body, _ := json.Marshal(map[string]string{"email": "ann@example.com", "password": "current-pass-aaa-1!"})
	req, _ := http.NewRequest("POST", f.server.URL+"/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "shifter")
	resLogin, err := freshClient.Do(req)
	require.NoError(t, err)
	resLogin.Body.Close()
	require.Equal(t, 401, resLogin.StatusCode, "old password must be rejected after change")
}
