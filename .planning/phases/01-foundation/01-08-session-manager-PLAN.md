---
phase: 01-foundation
plan: 08
type: execute
wave: 6
depends_on: [03, 04]
files_modified:
  - go.mod
  - go.sum
  - internal/auth/session.go
  - internal/auth/session_test.go
  - internal/auth/context.go
autonomous: true
requirements:
  - AUTH-02
must_haves:
  truths:
    - "NewSessionManager(pool, devMode=false) sets Cookie.Secure=true; devMode=true sets Cookie.Secure=false (D-23)"
    - "Cookie.HttpOnly=true and Cookie.SameSite=Lax always (RESEARCH §Pattern 6)"
    - "Cookie.Name='shifter_session' (UI-SPEC + RESEARCH)"
    - "IdleTimeout drives session expiry; sessions older than IdleTimeout return zero-value user"
    - "Session data stored in pgxstore (alexedwards/scs/pgxstore.New(pool)) — uses sessions table from Plan 03"
    - "PutUser / GetUser store/load (userID uuid.UUID, role string) tuple"
    - "RotateOnLogin invalidates the previous session token (mitigates session fixation)"
  artifacts:
    - path: "internal/auth/session.go"
      provides: "NewSessionManager + helpers PutUser, GetUser, RotateOnLogin, Destroy"
      contains: "scs.New"
    - path: "internal/auth/context.go"
      provides: "Context-bound user retrieval (UserFromContext)"
      contains: "UserFromContext"
  key_links:
    - from: "internal/auth/session.go"
      to: "alexedwards/scs/pgxstore"
      via: "pgxstore.New(pool)"
      pattern: "pgxstore\\.New"
    - from: "Cookie.Secure"
      to: "config.Config.IsDev()"
      via: "devMode toggle (D-23)"
      pattern: "!devMode"
---

<objective>
Implement Shifter's session manager (alexedwards/scs/v2 + pgxstore backend, Postgres-only), exposing a `NewSessionManager(pool, devMode bool)` constructor that sets HttpOnly + SameSite=Lax always, and Secure=true except when SHIFTER_ENV=dev (D-23). Provide helpers `PutUser`, `GetUser`, `Destroy`, `RotateOnLogin` that Plan 09/11/14 use directly.

Purpose: AUTH-02 (session persistence + idle timeout). Plan 09 wires `LoadAndSave` into the chi router; Plan 11 invalidates other sessions on password change; Plan 15 logs the wizard admin in atomically after wizard finish.

Output: `go test ./internal/auth -run TestSession_` passes including idle-timeout test using a synthetic time source.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/01-foundation/01-RESEARCH.md
@.planning/phases/01-foundation/01-CONTEXT.md
@01-03-database-layer-PLAN.md
@01-04-config-secrets-PLAN.md

<interfaces>
RESEARCH §Pattern 6 (lines 570-610) — verbatim NewSessionManager pattern.

Cookie config (locked):
- Name: `shifter_session`
- HttpOnly: `true`
- Secure: `!devMode` (D-23)
- SameSite: `http.SameSiteLaxMode` (NOT Strict — would break SPA login redirect)
- Path: `/`
- Domain: unset (PITFALL §"Cookie.Domain" — leave unset)

Session payload (small, primitive only — never gob-serialize complex types):
- `user_id` (string, UUID)
- `role` (string, "admin" | "viewer")
- `csrf_token` (string, random — populated by Plan 09 on login; consumed by middleware in Plan 11)

Public API:
```go
package auth

func NewSessionManager(pool *pgxpool.Pool, devMode bool, idleTimeout, lifetime time.Duration) *scs.SessionManager

// Convenience helpers — wrap sm.Get/Put under fixed keys.
func PutUser(ctx context.Context, sm *scs.SessionManager, u User)
func GetUser(ctx context.Context, sm *scs.SessionManager) (User, bool)
func Destroy(ctx context.Context, sm *scs.SessionManager) error
func RotateOnLogin(ctx context.Context, sm *scs.SessionManager) error
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Session manager + user context helpers + tests</name>
  <files>go.mod, go.sum, internal/auth/session.go, internal/auth/session_test.go, internal/auth/context.go</files>
  <read_first>
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pattern 6: SCS sessions with pgxstore" (lines 570-610)
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pitfall 5: Cookie.Secure=true in dev" (lines 1337-1342)
    - .planning/phases/01-foundation/01-CONTEXT.md (D-22, D-23)
    - 01-03-database-layer-PLAN.md (sessions table from migration 0003)
    - 01-04-config-secrets-PLAN.md (config.Config.IsDev())
  </read_first>
  <behavior>
    - TestNewSessionManager_DevMode_InsecureCookie: devMode=true → sm.Cookie.Secure=false.
    - TestNewSessionManager_ProdMode_SecureCookie: devMode=false → sm.Cookie.Secure=true.
    - TestNewSessionManager_CookieFlags: HttpOnly=true, SameSite=Lax, Path="/", Domain="" always.
    - TestPutGetUser_RoundTrip: PutUser stores a User; GetUser returns the same user; ok=true.
    - TestGetUser_NoSession_ReturnsZero: empty context → ok=false.
    - TestSession_IdleTimeout: simulate a session whose last-touched time exceeds IdleTimeout — GetUser returns ok=false. (Use the LoadAndSave middleware via httptest with a frozen clock or a short IdleTimeout like 50ms + sleep + second request to assert the session is gone.)
    - TestRotateOnLogin_InvalidatesOldToken: take an old token, call RotateOnLogin, old token no longer matches.
  </behavior>
  <action>
1. Install alexedwards/scs:
   ```bash
   go get github.com/alexedwards/scs/v2@latest
   go get github.com/alexedwards/scs/pgxstore@latest
   go get github.com/google/uuid@latest
   ```

2. Create `internal/auth/session.go`:
   ```go
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

   const (
       sessionCookieName = "shifter_session"
       sessionUserIDKey  = "user_id"
       sessionRoleKey    = "role"
       sessionCSRFKey    = "csrf_token"
   )

   // NewSessionManager wires alexedwards/scs/v2 with the pgxstore backend.
   //
   // devMode (true when SHIFTER_ENV=dev per D-23) disables Cookie.Secure
   // so login works on plain http://localhost:5173.
   //
   // PITFALL §"Cookie.Domain": leave Cookie.Domain unset — the session
   // is then scoped to the issuing host only.
   func NewSessionManager(pool *pgxpool.Pool, devMode bool, idleTimeout, lifetime time.Duration) *scs.SessionManager {
       sm := scs.New()
       sm.Store = pgxstore.New(pool)             // 5-min default cleanup goroutine
       sm.Lifetime = lifetime                    // absolute (e.g. 24h) per AUTH-02
       sm.IdleTimeout = idleTimeout              // idle (e.g. 8h) per AUTH-02
       sm.Cookie.Name = sessionCookieName
       sm.Cookie.HttpOnly = true
       sm.Cookie.Secure = !devMode               // D-23
       sm.Cookie.SameSite = http.SameSiteLaxMode // PITFALL §5: not Strict
       sm.Cookie.Path = "/"
       sm.ErrorFunc = func(w http.ResponseWriter, _ *http.Request, _ error) {
           http.Error(w, "session error", http.StatusInternalServerError)
       }
       return sm
   }

   // User is the in-session representation. Keep it small + primitive only.
   type User struct {
       ID   string // UUID string
       Role string // "admin" | "viewer"
   }

   func (u User) IsAdmin() bool  { return u.Role == "admin" }
   func (u User) IsViewer() bool { return u.Role == "viewer" }

   // PutUser writes the user to the session and rotates the token (mitigates fixation).
   func PutUser(ctx context.Context, sm *scs.SessionManager, u User) error {
       sm.Put(ctx, sessionUserIDKey, u.ID)
       sm.Put(ctx, sessionRoleKey, u.Role)
       return sm.RenewToken(ctx)
   }

   // GetUser pulls the user from the session if present.
   func GetUser(ctx context.Context, sm *scs.SessionManager) (User, bool) {
       id := sm.GetString(ctx, sessionUserIDKey)
       role := sm.GetString(ctx, sessionRoleKey)
       if id == "" || role == "" {
           return User{}, false
       }
       return User{ID: id, Role: role}, true
   }

   // Destroy invalidates the current session.
   func Destroy(ctx context.Context, sm *scs.SessionManager) error {
       return sm.Destroy(ctx)
   }

   // RotateOnLogin rotates the session token without changing values.
   // Use right after successful login to mitigate session fixation (RESEARCH §Security).
   func RotateOnLogin(ctx context.Context, sm *scs.SessionManager) error {
       return sm.RenewToken(ctx)
   }

   // EnsureCSRFToken populates a per-session CSRF token if absent and returns it.
   // The token isn't used in Phase 1 enforcement (we rely on SameSite=Lax + X-Requested-With
   // per RESEARCH §Security), but exposing it lets later phases tighten if needed.
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
   ```

3. Create `internal/auth/context.go`:
   ```go
   package auth

   import (
       "context"
       "errors"

       "github.com/alexedwards/scs/v2"
   )

   var ErrNoUser = errors.New("auth: no user in context")

   // UserFromContext is a convenience that pulls the user from the SCS-bound context
   // and returns ErrNoUser when no session is present.
   func UserFromContext(ctx context.Context, sm *scs.SessionManager) (User, error) {
       u, ok := GetUser(ctx, sm)
       if !ok {
           return User{}, ErrNoUser
       }
       return u, nil
   }
   ```

4. Replace `internal/auth/session_test.go` (skip-stubs from Plan 02). Tests use `internal/db.RunMigrations` to apply the sessions schema:
   ```go
   package auth

   import (
       "context"
       "net/http"
       "net/http/httptest"
       "testing"
       "time"

       "github.com/alexedwards/scs/v2"
       "github.com/shifter-io/shifter/internal/db"
       "github.com/shifter-io/shifter/internal/testsupport"
       "github.com/stretchr/testify/require"
       "log/slog"
       "os"
   )

   func setupDB(t *testing.T) *scs.SessionManager {
       t.Helper()
       pool := testsupport.StartPostgres(t)
       require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
       return NewSessionManager(pool, false, 8*time.Hour, 24*time.Hour)
   }

   func TestNewSessionManager_DevMode_InsecureCookie(t *testing.T) {
       pool := testsupport.StartPostgres(t)
       require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
       sm := NewSessionManager(pool, true, 8*time.Hour, 24*time.Hour)
       require.False(t, sm.Cookie.Secure, "D-23: Cookie.Secure must be false in dev")
       require.True(t, sm.Cookie.HttpOnly)
       require.Equal(t, http.SameSiteLaxMode, sm.Cookie.SameSite)
       require.Equal(t, "shifter_session", sm.Cookie.Name)
       require.Equal(t, "/", sm.Cookie.Path)
       require.Empty(t, sm.Cookie.Domain)
   }

   func TestNewSessionManager_ProdMode_SecureCookie(t *testing.T) {
       pool := testsupport.StartPostgres(t)
       require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
       sm := NewSessionManager(pool, false, 8*time.Hour, 24*time.Hour)
       require.True(t, sm.Cookie.Secure, "D-22: Cookie.Secure must be true in prod")
   }

   func TestPutGetUser_RoundTrip(t *testing.T) {
       sm := setupDB(t)
       userID := "00000000-0000-0000-0000-000000000001"
       handler := sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
           if r.URL.Path == "/login" {
               require.NoError(t, PutUser(r.Context(), sm, User{ID: userID, Role: "admin"}))
               return
           }
           u, ok := GetUser(r.Context(), sm)
           if !ok {
               http.Error(w, "no session", 401)
               return
           }
           w.Header().Set("X-User-Role", u.Role)
       }))
       srv := httptest.NewServer(handler)
       t.Cleanup(srv.Close)

       jar := newJar(t)
       cli := &http.Client{Jar: jar}

       res, err := cli.Get(srv.URL + "/login")
       require.NoError(t, err)
       res.Body.Close()

       res, err = cli.Get(srv.URL + "/me")
       require.NoError(t, err)
       res.Body.Close()
       require.Equal(t, "admin", res.Header.Get("X-User-Role"))
   }

   func TestGetUser_NoSession_ReturnsZero(t *testing.T) {
       sm := setupDB(t)
       _, ok := GetUser(context.Background(), sm)
       require.False(t, ok)
   }

   func TestSession_IdleTimeout(t *testing.T) {
       pool := testsupport.StartPostgres(t)
       require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
       sm := NewSessionManager(pool, false, 50*time.Millisecond, 24*time.Hour)

       handler := sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
           if r.URL.Path == "/login" {
               _ = PutUser(r.Context(), sm, User{ID: "uuid", Role: "admin"})
               return
           }
           if _, ok := GetUser(r.Context(), sm); !ok {
               http.Error(w, "expired", 401)
               return
           }
       }))
       srv := httptest.NewServer(handler)
       t.Cleanup(srv.Close)

       jar := newJar(t)
       cli := &http.Client{Jar: jar}
       res, _ := cli.Get(srv.URL + "/login")
       res.Body.Close()

       time.Sleep(150 * time.Millisecond) // exceed IdleTimeout

       res, _ = cli.Get(srv.URL + "/me")
       defer res.Body.Close()
       require.Equal(t, http.StatusUnauthorized, res.StatusCode, "AUTH-02: idle session must expire")
   }

   func TestRotateOnLogin_InvalidatesOldToken(t *testing.T) {
       sm := setupDB(t)
       handler := sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
           switch r.URL.Path {
           case "/seed":
               _ = PutUser(r.Context(), sm, User{ID: "u1", Role: "admin"})
           case "/rotate":
               _ = RotateOnLogin(r.Context(), sm)
           case "/me":
               if _, ok := GetUser(r.Context(), sm); !ok {
                   http.Error(w, "no", 401)
               }
           }
       }))
       srv := httptest.NewServer(handler)
       t.Cleanup(srv.Close)

       cli := &http.Client{Jar: newJar(t)}
       res, _ := cli.Get(srv.URL + "/seed");   res.Body.Close()
       firstCookies := cli.Jar.Cookies(mustParseURL(t, srv.URL))

       res, _ = cli.Get(srv.URL + "/rotate"); res.Body.Close()
       res, _ = cli.Get(srv.URL + "/me")
       require.Equal(t, http.StatusOK, res.StatusCode)
       res.Body.Close()

       // Reuse the OLD cookie value with a fresh client — should NOT be a valid session.
       cli2 := &http.Client{}
       req, _ := http.NewRequest("GET", srv.URL+"/me", nil)
       for _, c := range firstCookies {
           req.AddCookie(c)
       }
       res, _ = cli2.Do(req)
       defer res.Body.Close()
       require.Equal(t, http.StatusUnauthorized, res.StatusCode, "old token should be invalid after RenewToken")
   }
   ```

   Helpers (add to the same file or a `testhelpers_test.go`):
   ```go
   import (
       "net/http/cookiejar"
       "net/url"
   )

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
   ```
  </action>
  <verify>
    <automated>go test ./internal/auth -run 'TestSession|TestNewSessionManager|TestPutGetUser|TestGetUser|TestRotateOnLogin' -race -count=1 -v</automated>
  </verify>
  <acceptance_criteria>
    - File `internal/auth/session.go` exports `func NewSessionManager(pool *pgxpool.Pool, devMode bool, idleTimeout, lifetime time.Duration) *scs.SessionManager`
    - `NewSessionManager` sets exactly: `sm.Cookie.Name = "shifter_session"`, `sm.Cookie.HttpOnly = true`, `sm.Cookie.SameSite = http.SameSiteLaxMode`, `sm.Cookie.Path = "/"`, `sm.Cookie.Secure = !devMode`
    - `NewSessionManager` calls `pgxstore.New(pool)` (grep proof; not `postgresstore`)
    - File exports `func PutUser`, `func GetUser`, `func Destroy`, `func RotateOnLogin`, `func EnsureCSRFToken`
    - File exports `type User struct { ID, Role string }`
    - File `internal/auth/context.go` exports `func UserFromContext(ctx, sm) (User, error)` and `var ErrNoUser`
    - Command `go test ./internal/auth -run TestSession_IdleTimeout -race` exits 0 (per VALIDATION.md)
    - Command `go test ./internal/auth -run TestRotateOnLogin_InvalidatesOldToken -race` exits 0
    - All 6 tests in this plan pass
  </acceptance_criteria>
  <done>
    Session manager production-ready. Plan 09 wires `sm.LoadAndSave(router)` and calls `auth.PutUser` after a successful login. Plan 11 calls `auth.Destroy` on logout and uses `auth.UserFromContext` in middleware.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| browser cookie → server | Session cookie crosses on every request; server-side session table is authoritative |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-08-01 | Spoofing (session fixation) | login takes over an existing pre-login session | mitigate | `PutUser` calls `sm.RenewToken(ctx)`; `RotateOnLogin` available for explicit rotate. SCS rotates by default on `RenewToken`. ASVS V3. |
| T-08-02 | Information Disclosure | session cookie sniffed over HTTP | mitigate | `Cookie.Secure = !devMode` (D-23) — production always sets Secure; D-22 forbids plain HTTP. ASVS V3. |
| T-08-03 | Tampering (XSS exfiltration) | session cookie accessible from JS | mitigate | `Cookie.HttpOnly = true`. ASVS V3. |
| T-08-04 | Tampering (CSRF) | cross-site cookie use on state-changing endpoints | mitigate | `SameSite=Lax` + Plan 06 `apiFetch` sends `X-Requested-With: shifter`; Plan 11/15 middleware enforces the header on POST/PUT/DELETE. ASVS V13. |
| T-08-05 | Information Disclosure | sessions table grows unbounded if cleanup disabled | mitigate | `pgxstore.New(pool)` uses default 5-minute cleanup; PITFALL §3 prevention. |
| T-08-06 | Information Disclosure | session payload includes sensitive fields beyond user_id/role | mitigate | Only primitives stored: `user_id`, `role`, `csrf_token`. RESEARCH §"Pickled session deserialization". ASVS V3. |
</threat_model>

<verification>
- `internal/auth/session.go` uses `alexedwards/scs/pgxstore` (NOT `postgresstore`)
- Cookie attrs: HttpOnly, SameSite=Lax, Path=/, Domain unset, Secure=!devMode
- `NewSessionManager` accepts idleTimeout + lifetime so AUTH-02 is configurable via Config
- `PutUser` rotates the session token (session-fixation prevention)
- 6 tests pass against testcontainer Postgres
</verification>

<success_criteria>
- AUTH-02 enforced (idle timeout + persistence across requests)
- D-23 enforced (dev mode disables Secure flag)
- Session store uses Postgres only (no Redis dep — D-06 default)
- pgxstore (not postgresstore) — single pool reuse
- Session payload limited to primitives
</success_criteria>

<output>
After completion, create `.planning/phases/01-foundation/01-08-SUMMARY.md` documenting:
- NewSessionManager signature
- Helper API (`PutUser`, `GetUser`, `Destroy`, `RotateOnLogin`, `EnsureCSRFToken`)
- Cookie attribute table
- Plan 09/11/15 wiring instructions
</output>
