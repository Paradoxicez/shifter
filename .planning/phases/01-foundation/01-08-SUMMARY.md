---
phase: 01-foundation
plan: 08
subsystem: auth
tags: [go, scs, pgxstore, sessions, cookies, csrf, owasp, asvs-v3, auth-02]

requires:
  - phase: 01-foundation
    plan: 03
    provides: sessions table (migration 0003) + pgxpool helper
  - phase: 01-foundation
    plan: 04
    provides: Config.Session (Key, IdleTimeout, Lifetime) and Config.IsDev() — Plan 09 will pass these to NewSessionManager
provides:
  - internal/auth.NewSessionManager(pool, devMode, idleTimeout, lifetime) *scs.SessionManager
  - internal/auth.User struct (ID, Role) + IsAdmin / IsViewer helpers
  - internal/auth.PutUser, GetUser, Destroy, RotateOnLogin, EnsureCSRFToken
  - internal/auth.UserFromContext + ErrNoUser sentinel (context.go)
  - Locked cookie attribute set: shifter_session / HttpOnly / SameSite=Lax / Path=/ / Domain unset / Secure=!devMode
affects:
  - 01-09-login-ratelimit (wires sm.LoadAndSave into chi router; calls PutUser after password verify)
  - 01-10-authz (uses UserFromContext in role middleware)
  - 01-11-account-ui (calls Destroy on logout; password change must invalidate other sessions)
  - 01-14-install-middleware (gates wizard endpoints by session presence)
  - 01-15-install-handlers (calls PutUser to log the bootstrap admin in atomically at wizard finish)
  - 01-18-router-health (wires sm.LoadAndSave once, on the chi router)

tech-stack:
  added:
    - github.com/alexedwards/scs/v2 v2.9.0
    - github.com/alexedwards/scs/pgxstore v0.0.0-20251002162104-209de6e426de
  patterns:
    - "pgxstore.New(pool) reuses the application pgxpool — single connection pool for sessions + everything else (D-06: no Redis dependency)"
    - "Cookie.Secure = !devMode (D-23): plain http://localhost works in dev; production always sets Secure"
    - "SameSite=Lax + X-Requested-With header (RESEARCH §Security): defends against CSRF without breaking SPA login redirect; Plan 06 apiFetch sends the header, Plan 11 middleware enforces it"
    - "Session payload is primitives only: user_id (string), role (string), csrf_token (string). No gob-serialized structs (RESEARCH §Pickled deserialization)"
    - "PutUser calls sm.RenewToken — every successful login rotates the token (ASVS V3 / T-08-01 session-fixation defense)"
    - "GetUser is panic-safe: bare context.Background() returns ok=false instead of panicking with 'scs: no session data in context' — lets unauthenticated handlers and start-up code call GetUser without panic guards"

key-files:
  created:
    - internal/auth/session.go (NewSessionManager + User + PutUser/GetUser/Destroy/RotateOnLogin/EnsureCSRFToken)
    - internal/auth/context.go (UserFromContext + ErrNoUser)
  modified:
    - internal/auth/session_test.go (replaces Plan 02's t.Skip stub with 12 real tests)
    - go.mod (adds scs/v2 + scs/pgxstore)
    - go.sum

key-decisions:
  - "GetUser recovers from SCS's 'no session data in context' panic. SCS panics by design when its context key is absent (i.e. no LoadAndSave middleware ran). Catching the panic and returning ok=false lets package consumers write straight-line `u, ok := GetUser(ctx, sm); if !ok { ... }` code regardless of context provenance — matches the idiomatic 'comma-ok' pattern in stdlib map lookups. Without the recover, every Plan 09/11 handler would need a panic guard around every GetUser call."
  - "pgxstore.New(pool) — NOT NewWithConfig — uses the default 5-minute background cleanup. T-08-05 (sessions-table-grows-unbounded) is mitigated by this default; explicitly setting cleanup to 0 would be a regression. PITFALL §3 calls out the same idiom."
  - "Lifetime parameter exposed in NewSessionManager signature even though Plan 04's Config.Session.Lifetime feeds it. Plan 09 builds the SessionManager from Config; keeping Lifetime as a parameter (vs. hardcoded 24h) lets future config changes flow through without touching auth/."
  - "User struct stores only (ID, Role). Storing email/display_name in the session would force a session bump every time a user edits their profile — T-08-06 mitigation. Plan 11 (account UI) joins to the user table on every authenticated request; the cost (1 indexed PK lookup) is negligible vs. the operational simplicity of 'session is the smallest stable identity'."
  - "EnsureCSRFToken ships now even though Phase 1 enforcement is SameSite=Lax + X-Requested-With. Pre-emptively populating the token means later phases (Plan 11 password change, Plan 14 install middleware tightening) can adopt token-pair CSRF without a session-data migration. The base64.RawURLEncoding choice matches RESEARCH §Pattern 6 verbatim — URL-safe so it can travel as a query param if needed."
  - "RotateOnLogin is exposed even though PutUser already calls RenewToken internally. Plan 11 needs RotateOnLogin to invalidate other sessions of the same user after a password change without writing a new user blob to the current session — and Plan 09's login flow uses PutUser, which both writes user data AND rotates. Two entry points keep the call sites readable."

requirements-completed:
  - AUTH-02

duration: 5min
completed: 2026-04-28
---

# Phase 01 Plan 08: Session Manager Summary

**alexedwards/scs/v2 + pgxstore session manager with HttpOnly + SameSite=Lax cookies, dev/prod Secure toggle (D-23), and primitive-only payload storing (user_id, role, csrf_token) — implements AUTH-02 and unblocks Plans 09/11/14/15 for login + logout + role middleware + install-finish atomic login.**

## Performance

- **Duration:** ~5 min
- **Started:** 2026-04-28T00:46:25Z
- **Completed:** 2026-04-28
- **Tasks:** 1 / 1 (TDD: RED → GREEN)
- **Commits:** 2 (1 RED + 1 GREEN)
- **Files created:** 2 (`internal/auth/session.go`, `internal/auth/context.go`)
- **Files modified:** 3 (`internal/auth/session_test.go`, `go.mod`, `go.sum`)
- **Tests added:** 12 (was 2 t.Skip stubs from Plan 02)

## Accomplishments

- `NewSessionManager(pool, devMode, idleTimeout, lifetime) *scs.SessionManager` returns a fully-configured SCS instance backed by the existing pgxpool. No second pool, no Redis, no separate config files.
- Six helpers (`PutUser`, `GetUser`, `Destroy`, `RotateOnLogin`, `EnsureCSRFToken`, `UserFromContext`) cover every wiring path Plans 09/10/11/14/15 need; no plan is forced to call SCS directly.
- 12 tests pass against a TimescaleDB 2.26.0-pg16 testcontainer including the AUTH-02 idle-timeout assertion (50ms timeout + 150ms sleep) and the session-fixation regression (replay an old cookie from a fresh client → 401).
- `go test ./... -short -race` exits 0 across 12 packages, 60 tests — no regressions in `db`, `config`, `logging`, or pre-existing `auth` (Argon2id, password strength).
- `grep -rn 'postgresstore' internal/auth/` returns 0 hits (verifies pgxstore-not-postgresstore acceptance criterion).
- `grep -n 'subtle.ConstantTimeCompare\|argon2.IDKey' internal/auth/session.go` returns 0 hits (session manager doesn't reach into crypto primitives — those live in argon2id.go from Plan 07).

## Public API

```go
package auth

// NewSessionManager wires alexedwards/scs/v2 with the pgxstore backend.
//
// devMode (true when SHIFTER_ENV=dev per D-23) disables Cookie.Secure so
// login works on plain http://localhost:5173. Cookie attributes are locked:
//   Name="shifter_session", HttpOnly=true, SameSite=Lax, Path="/",
//   Domain unset, Secure=!devMode.
//
// Storage: pgxstore.New(pool) — uses the sessions table from migration 0003.
// Default 5-minute background cleanup goroutine is enabled (T-08-05).
func NewSessionManager(pool *pgxpool.Pool, devMode bool, idleTimeout, lifetime time.Duration) *scs.SessionManager

// User is the in-session representation. Primitive-only payload (T-08-06).
type User struct {
    ID   string // UUID string
    Role string // "admin" | "viewer"
}
func (u User) IsAdmin() bool
func (u User) IsViewer() bool

// PutUser writes (ID, Role) and rotates the session token (T-08-01).
func PutUser(ctx context.Context, sm *scs.SessionManager, u User) error

// GetUser pulls (User, true) or (zero, false). Panic-safe on bare context.
func GetUser(ctx context.Context, sm *scs.SessionManager) (User, bool)

// Destroy invalidates the current session.
func Destroy(ctx context.Context, sm *scs.SessionManager) error

// RotateOnLogin renames the session token without changing data (rare path).
func RotateOnLogin(ctx context.Context, sm *scs.SessionManager) error

// EnsureCSRFToken sets a per-session CSRF token if absent and returns it.
func EnsureCSRFToken(ctx context.Context, sm *scs.SessionManager) string

// UserFromContext is the context-helper sibling of GetUser.
var ErrNoUser = errors.New("auth: no user in context")
func UserFromContext(ctx context.Context, sm *scs.SessionManager) (User, error)
```

## Cookie Attribute Table

| Attribute  | Value                                | Source                                    |
| ---------- | ------------------------------------ | ----------------------------------------- |
| Name       | `shifter_session`                    | UI-SPEC + RESEARCH §Pattern 6             |
| HttpOnly   | `true`                               | T-08-03 / ASVS V3 (XSS exfiltration)      |
| Secure     | `!devMode`                           | D-23 (dev toggle) / D-22 (prod-only HTTPS)|
| SameSite   | `http.SameSiteLaxMode`               | T-08-04 + PITFALL §5 (Strict breaks SSO)  |
| Path       | `/`                                  | RESEARCH §Pattern 6                       |
| Domain     | `""` (unset)                         | PITFALL §"Cookie.Domain" — host-scoped    |

## Plan 09 / 11 / 14 / 15 Wiring Instructions

### Plan 09 — login-ratelimit + chi router wiring

```go
// In Plan 09's serve.go (or wherever the router is built):
sm := auth.NewSessionManager(pool, cfg.IsDev(),
    cfg.Session.IdleTimeout, cfg.Session.Lifetime)

router := chi.NewRouter()
router.Use(sm.LoadAndSave)              // wraps EVERY request
router.Post("/api/auth/login", loginHandler(db, sm))
```

Inside `loginHandler` after a successful Argon2id Verify:

```go
if err := auth.PutUser(r.Context(), sm, auth.User{
    ID: user.ID.String(),
    Role: string(user.Role),
}); err != nil {
    http.Error(w, "internal error", 500)
    return
}
// 200 OK; cookie is set by sm.LoadAndSave on response write
```

### Plan 11 — account UI logout + password-change

```go
// Logout
func logoutHandler(sm *scs.SessionManager) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        _ = auth.Destroy(r.Context(), sm)
        w.WriteHeader(http.StatusNoContent)
    }
}

// Password-change should invalidate ALL sessions for the user.
// Plan 11 will:
//   1. UPDATE user SET password_hash=$1 WHERE id=$2
//   2. DELETE FROM sessions WHERE data::jsonb -> 'user_id' = ...   (Plan 11 will derive)
//   3. auth.PutUser(r.Context(), sm, currentUser)  -- restore current device
//      (calls RenewToken internally, so the operator's current cookie still works)
```

### Plan 14 — install middleware (gating wizard endpoints)

```go
// Plan 14 middleware (one-line):
func RequireSession(sm *scs.SessionManager) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if _, err := auth.UserFromContext(r.Context(), sm); err != nil {
                http.Error(w, "unauthorized", http.StatusUnauthorized)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

### Plan 15 — install wizard finish (atomic admin login)

```go
// After INSERT INTO "user" (...) RETURNING id:
_ = auth.PutUser(r.Context(), sm, auth.User{
    ID: bootstrapID.String(),
    Role: "admin",
})
// install_state.current_step = 5 (done) — Plan 15 owns this transition.
// PutUser rotates the token, so the operator's wizard-pre-login session
// is invalidated and replaced with a freshly-rotated logged-in session.
```

## Decisions Made

- **GetUser is panic-safe.** SCS panics with `"scs: no session data in context"` when the context key is absent (e.g. tests calling `GetUser(context.Background(), sm)`). The defensive `recover()` returns `ok=false` instead, matching the comma-ok idiom of stdlib `m[k]`. Without it, every plan would need to wrap GetUser in a panic guard, which would in turn defeat the purpose of having a clean middleware-based context flow.
- **pgxstore.New (default 5-min cleanup) instead of NewWithConfig.** The default cleanup goroutine is the T-08-05 mitigation; explicitly setting `CleanUpInterval=0` would be a regression. NewWithConfig stays available for tests that need to disable cleanup, but the production path uses the default.
- **Lifetime parameter even though Plan 04 owns the value.** Plan 09 will build SessionManager from `cfg.Session.Lifetime`; Plan 04 already validates it (`>0`). Keeping Lifetime as a NewSessionManager parameter (rather than hardcoding 24h) lets the operator tune absolute session lifetime via `SHIFTER_SESSION_LIFETIME` without an auth-package change.
- **User struct primitives only.** No email, no display name. Plan 11 joins to the user table on every authenticated request (1 indexed lookup per request — negligible). Storing email in the session would force every profile edit to invalidate sessions. T-08-06 mitigation.
- **EnsureCSRFToken ships pre-emptively.** Phase 1 enforcement is SameSite=Lax + X-Requested-With (RESEARCH §Security). Adding the token now means Plans 11/14 can tighten to a token-pair pattern without a session-data migration. base64.RawURLEncoding chosen to match RESEARCH §Pattern 6 — URL-safe and 32-byte entropy.
- **Two rotate entry points (PutUser + RotateOnLogin).** PutUser bundles "write user blob + rotate token" for the common login path (Plans 09, 15). RotateOnLogin exposes pure rotation for Plan 11's password-change flow, where the user blob isn't changing but the session ID must.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] GetUser panic on bare context**

- **Found during:** Task 1 GREEN phase, when `TestGetUser_NoSession_ReturnsZero` panicked with `"scs: no session data in context"`.
- **Issue:** Plan's verbatim `GetUser` calls `sm.GetString(ctx, ...)`, which internally calls SCS's `getSessionDataFromContext` — and that function panics when the SCS context key is absent (i.e. the request didn't go through `LoadAndSave`). The plan's behavior block specified `TestGetUser_NoSession_ReturnsZero: empty context → ok=false`, so the panic violated the plan's own acceptance criteria.
- **Fix:** Wrapped GetUser body in `defer recover()` returning the zero User + ok=false. The function still works identically inside a LoadAndSave-wrapped handler (no panic to recover) and now also handles bare contexts cleanly.
- **Files modified:** `internal/auth/session.go`
- **Tested by:** `TestGetUser_NoSession_ReturnsZero` and `TestUserFromContext_NoSession_ReturnsErrNoUser`.
- **Commit:** `373a633`

**2. [Rule 2 - Missing Critical] Added TestDestroy_RemovesSession**

- **Found during:** Task 1 RED phase, while writing tests against the plan's behavior block.
- **Issue:** Plan listed `Destroy` as a public helper but had no test for it. Plan 11 (account UI) is going to call `Destroy` for the logout endpoint; if Destroy is silently broken, the regression would surface in Plan 11 instead of being caught here.
- **Fix:** Added `TestDestroy_RemovesSession` — login, verify session is live, Destroy, assert subsequent GetUser returns false.
- **Files modified:** `internal/auth/session_test.go`
- **Commit:** `0b37eed` (RED) + `373a633` (GREEN)

**3. [Rule 2 - Missing Critical] Added TestEnsureCSRFToken_Stable**

- **Found during:** Task 1 RED phase.
- **Issue:** Plan listed `EnsureCSRFToken` as a public helper but had no test. The function has a subtle "set-once-then-stable" semantic (returns the existing token if one exists, generates+stores otherwise) that's easy to break later.
- **Fix:** Added `TestEnsureCSRFToken_Stable` — call twice within the same session, assert identical tokens.
- **Files modified:** `internal/auth/session_test.go`
- **Commit:** `0b37eed` (RED) + `373a633` (GREEN)

**4. [Rule 2 - Missing Critical] Added TestUserFromContext_* pair**

- **Found during:** Task 1 RED phase.
- **Issue:** Plan defined `UserFromContext + ErrNoUser` in `context.go` but listed no behavior tests for it. Plan 14's middleware wraps every authenticated route in `UserFromContext`, so a regression here would silently break authz across the entire app.
- **Fix:** Added `TestUserFromContext_NoSession_ReturnsErrNoUser` and `TestUserFromContext_WithSession_ReturnsUser`.
- **Files modified:** `internal/auth/session_test.go`
- **Commit:** `0b37eed` (RED) + `373a633` (GREEN)

---

**Total deviations:** 4 auto-fixed (1 Rule 1 bug, 3 Rule 2 missing-critical-tests).
**Impact on plan:** None to the public API or architectural intent. The bug fix (Rule 1) is required for the plan's own acceptance criterion (`TestGetUser_NoSession_ReturnsZero`); the test additions (Rule 2) lock in behavior that downstream plans depend on.

## Issues Encountered

- **First test run panicked on `GetUser(context.Background(), sm)`.** Root-caused to SCS's `getSessionDataFromContext` panicking when the context key is absent. The plan's behavior block expected ok=false for that path; the verbatim `GetUser` body would have crashed every test that didn't go through LoadAndSave. Fixed with a `defer recover()` (see Deviation 1).
- **`pgxstore` package version is a pseudo-version.** Latest tagged version is from October 2025 (`v0.0.0-20251002162104-209de6e426de`). Pseudo-versions in go.mod are normal for Go modules without a proper release tag; the import path remains stable.
- **rtk Bash tool sometimes truncates `go test -v` output.** Used `rtk proxy` to get the raw `go test` stream confirming all 12 PASS lines. The tool's summary count (`12 passed in 1 packages`) was correct but the per-test detail was filtered.

## Threat Surface Notes

No new threat surface beyond the plan's `<threat_model>`. All six register entries (T-08-01..06) are mitigated by code shipped in this plan:

| Threat | Mitigation |
|--------|------------|
| T-08-01 (session fixation at login) | `PutUser` calls `sm.RenewToken`; `TestRotateOnLogin_InvalidatesOldToken` regression-tests the path |
| T-08-02 (cookie sniffed over HTTP) | `Cookie.Secure = !devMode`; D-22 forbids plain HTTP in production; D-23 toggles for dev only |
| T-08-03 (XSS exfiltration of cookie) | `Cookie.HttpOnly = true` |
| T-08-04 (cross-site CSRF on state changes) | `SameSite=Lax` + Plan 06 apiFetch's `X-Requested-With: shifter` header (Plan 11/15 will enforce) |
| T-08-05 (sessions table grows unbounded) | `pgxstore.New(pool)` uses the default 5-minute cleanup goroutine |
| T-08-06 (session payload includes sensitive fields) | `User` struct holds only `ID` + `Role`; primitives only; CSRF token is base64 ASCII |

## Known Stubs

None — Plan 08 is fully implemented. Future enhancements (token-pair CSRF, alternative storage backends) are explicit follow-ups documented above; the current code does not contain TODO/FIXME placeholders or empty-state UI fallbacks that would mask incomplete behavior.

## User Setup Required

None. Plan 08 is plumbing for Plans 09/11/14/15; the operator never sees session-manager code directly.

For local smoke-testing today (after Plan 09 ships):

```bash
export SHIFTER_SESSION_KEY=$(openssl rand -base64 48)
export SHIFTER_SESSION_IDLE_TIMEOUT=8h
export SHIFTER_SESSION_LIFETIME=24h
shifter serve   # then POST /api/auth/login + cookie returned with HttpOnly + SameSite=Lax
```

## Next Phase Readiness

- `NewSessionManager(pool, devMode, idle, lifetime)` is the canonical entry point. Plans 09/11/14/15/18 import it directly; no plan reaches into `scs.New()` itself.
- `internal/auth.User` is the canonical session-bound identity. Plan 11 joins to the `user` table when display name / email is needed; Plan 10 uses `User.Role` for authz.
- `sm.LoadAndSave` is wired exactly once, in Plan 09's chi router setup. No plan beyond 09 should call `LoadAndSave` again.
- `auth.UserFromContext` is the only sanctioned way for handlers to get the current user. Plan 10's role middleware and Plan 14's install middleware both build on top of it.
- The `sessions` table from migration 0003 is now the live session store. Plan 11's password-change flow will issue a `DELETE FROM sessions WHERE ...` to invalidate other devices; the schema is stable.

## Self-Check: PASSED

Files verified to exist:
- FOUND: `internal/auth/session.go`
- FOUND: `internal/auth/context.go`
- FOUND: `internal/auth/session_test.go`

Commits verified to exist:
- FOUND: `0b37eed` (Task 1 RED — failing session manager tests)
- FOUND: `373a633` (Task 1 GREEN — session manager + user context)

Behavior verified:
- `go build ./...` exits 0
- `go vet ./...` exits 0
- `go test ./internal/auth -run 'TestSession|TestNewSessionManager|TestPutGetUser|TestGetUser|TestRotate|TestDestroy|TestEnsureCSRF|TestUserFromContext' -race -count=1` passes 12 tests
- `go test ./... -short -race -count=1` passes 60 tests across 12 packages
- `grep -rn 'postgresstore' internal/auth/` returns 0 hits (pgxstore-only acceptance)
- `grep -n 'pgxstore.New' internal/auth/session.go` returns 1 hit on line 43
- `grep -n '!devMode' internal/auth/session.go` returns 1 hit on line 48
- `grep -E '^func (NewSessionManager|PutUser|GetUser|Destroy|RotateOnLogin|EnsureCSRFToken)' internal/auth/session.go` returns 6 hits
- `grep -E '^func UserFromContext' internal/auth/context.go` returns 1 hit
- `grep -n 'ErrNoUser' internal/auth/context.go` returns 4 hits

---
*Phase: 01-foundation*
*Plan: 08-session-manager*
*Completed: 2026-04-28*
