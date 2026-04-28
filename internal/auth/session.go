package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"time"

	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Session keys (lowercase, primitive-only — never gob-serialize complex types).
const (
	sessionCookieName = "shifter_session"
	sessionUserIDKey  = "user_id"
	sessionRoleKey    = "role"
	sessionCSRFKey    = "csrf_token"
)

// NewSessionManager wires alexedwards/scs/v2 with the pgxstore backend.
//
// devMode (true when SHIFTER_ENV=dev per D-23) disables Cookie.Secure so login
// works on plain http://localhost:5173. In production (devMode=false) the
// session cookie always sets Secure=true (D-22 forbids plain HTTP).
//
// Cookie attributes (locked, RESEARCH §Pattern 6 + §Security):
//   - Name:     "shifter_session"
//   - HttpOnly: true (defends against XSS exfiltration; T-08-03)
//   - Secure:   !devMode (D-23; T-08-02)
//   - SameSite: Lax (NOT Strict — would break SPA login redirect; T-08-04)
//   - Path:     "/"
//   - Domain:   "" (unset — scopes the cookie to the issuing host only;
//     PITFALL §"Cookie.Domain")
//
// Storage: alexedwards/scs/pgxstore.New(pool) — uses the sessions table from
// migration 0003. The default 5-minute background cleanup goroutine is enabled
// (T-08-05 mitigation; PITFALL §3 prevention).
func NewSessionManager(pool *pgxpool.Pool, devMode bool, idleTimeout, lifetime time.Duration) *scs.SessionManager {
	sm := scs.New()
	sm.Store = pgxstore.New(pool)
	sm.Lifetime = lifetime       // absolute (e.g. 24h) per AUTH-02
	sm.IdleTimeout = idleTimeout // idle (e.g. 8h) per AUTH-02
	sm.Cookie.Name = sessionCookieName
	sm.Cookie.HttpOnly = true
	sm.Cookie.Secure = !devMode
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.Path = "/"
	sm.ErrorFunc = func(w http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(w, "session error", http.StatusInternalServerError)
	}
	return sm
}

// User is the in-session representation. Keep it small + primitive only.
//
// Storing only (ID, Role) — never email, never password hash, never anything
// that would force a session bump on profile change (T-08-06).
type User struct {
	ID   string // UUID string
	Role string // "admin" | "viewer"
}

// IsAdmin reports whether the user has the admin role.
func (u User) IsAdmin() bool { return u.Role == "admin" }

// IsViewer reports whether the user has the viewer role.
func (u User) IsViewer() bool { return u.Role == "viewer" }

// PutUser writes the user to the session and rotates the token (mitigates
// session fixation per ASVS V3 / T-08-01). SCS rotates by default on
// RenewToken — the old token becomes invalid.
//
// Plan 09 (login handler) calls this immediately after a successful password
// verify; Plan 15 (install wizard finish) calls it to log the bootstrap admin
// in atomically.
func PutUser(ctx context.Context, sm *scs.SessionManager, u User) error {
	sm.Put(ctx, sessionUserIDKey, u.ID)
	sm.Put(ctx, sessionRoleKey, u.Role)
	return sm.RenewToken(ctx)
}

// GetUser pulls the user from the session if present. Returns the zero User
// and ok=false when no session is loaded or the required fields are absent.
//
// Defensive: if the context did not pass through sm.LoadAndSave (e.g. an
// unauthenticated request that bypassed the middleware, or a bare
// context.Background() at start-up), SCS would normally panic with "scs: no
// session data in context". We recover and return ok=false instead so callers
// can write straight-line code without panic guards.
func GetUser(ctx context.Context, sm *scs.SessionManager) (user User, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			user, ok = User{}, false
		}
	}()
	id := sm.GetString(ctx, sessionUserIDKey)
	role := sm.GetString(ctx, sessionRoleKey)
	if id == "" || role == "" {
		return User{}, false
	}
	return User{ID: id, Role: role}, true
}

// Destroy invalidates the current session.
//
// Plan 11 (account UI / logout) calls this; the operator's cookie still exists
// in their browser but maps to a destroyed server-side row.
func Destroy(ctx context.Context, sm *scs.SessionManager) error {
	return sm.Destroy(ctx)
}

// RotateOnLogin rotates the session token without changing values. Use right
// after successful login to mitigate session fixation when the caller does NOT
// want to update session data (rare — PutUser already rotates).
//
// RESEARCH §Security domain: ASVS V3 — login MUST rotate the session
// identifier.
func RotateOnLogin(ctx context.Context, sm *scs.SessionManager) error {
	return sm.RenewToken(ctx)
}

// EnsureCSRFToken populates a per-session CSRF token if absent and returns it.
//
// The token isn't used in Phase 1 enforcement (we rely on SameSite=Lax +
// X-Requested-With per RESEARCH §Security; Plan 06's apiFetch sends the
// header). Exposing the token now lets later phases tighten the CSRF posture
// without a session-data migration.
func EnsureCSRFToken(ctx context.Context, sm *scs.SessionManager) string {
	if t := sm.GetString(ctx, sessionCSRFKey); t != "" {
		return t
	}
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	token := base64.RawURLEncoding.EncodeToString(buf)
	sm.Put(ctx, sessionCSRFKey, token)
	return token
}
