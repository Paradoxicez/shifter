---
phase: 06-alerts-users-audit-operational-hardening
plan: 06
subsystem: auth-event-audit-retrofit
tags: [phase-6, auth, audit-in-tx, d-30, login, logout, password-change, last-login-at]
requires:
  - phase-1-foundation (login/logout/change-password handlers)
  - phase-6-plan-01-alert-engine-substrate (audit vocabulary: auth.* constants)
  - phase-6-plan-05-user-management (UpdatePasswordTx, IterateAndRevoke export)
provides:
  - audit-rows: auth.login_success, auth.login_failed, auth.logout, auth.password_change, auth.session_revoked
  - store-method: Store.GetUserByEmailTx (tx-scoped user lookup)
  - store-method: Store.UpdateLastLoginAtTx (atomic last_login_at update)
  - handler: LogoutHandlerWithAudit (replaces LogoutHandler in router)
affects:
  - internal/auth/handlers.go (LoginHandler + LogoutHandlerWithAudit retrofit)
  - internal/auth/account.go (ChangePasswordHandler retrofit)
  - internal/auth/users.go (GetUserByEmailTx + UpdateLastLoginAtTx methods)
  - internal/http/router.go (wires LogoutHandlerWithAudit)
  - internal/auth/handlers_test.go (8 new tests)
  - internal/auth/account_test.go (3 new tests + pool/store fixture fields)
tech-stack:
  added: []
  patterns:
    - "ReadCommitted tx wrapping user lookup + verify + UpdateLastLoginAtTx + audit.WriteEntry + Commit; session PutUser AFTER commit"
    - "LogoutHandlerWithAudit: short tx for audit row before sm.Destroy; anonymous logout = 204 no-op with no audit row"
    - "ChangePasswordHandler: default-isolation tx wrapping UpdatePasswordTx + two audit rows; iterateAndRevoke AFTER commit"
    - "Rate-limit 429 short-circuits BEFORE the tx — D-30 scope excludes rate-limited paths from audit"
key-files:
  created: []
  modified:
    - internal/auth/handlers.go
    - internal/auth/account.go
    - internal/auth/users.go
    - internal/http/router.go
    - internal/auth/handlers_test.go
    - internal/auth/account_test.go
decisions:
  - "LogoutHandler kept as backward-compat signature (sm only); LogoutHandlerWithAudit added with (sm, store) for D-30; router updated to use the new variant"
  - "Rate-limited 429 path explicitly does NOT write audit rows — D-30 scope is operator-visible actual login attempts only, not the rate-limit enforcement"
  - "GetUserByEmailTx added to users.go (not reusing GetUserByEmail) so the user lookup is part of the audit-in-tx envelope"
  - "ChangePasswordHandler uses UpdatePasswordTx(mustChange=false) — self-initiated change clears force-change flag, consistent with Phase 1 semantics"
metrics:
  duration: 13min
  tasks: 2
  files: 6
  completed: 2026-05-12
---

# Phase 6 Plan 06: Auth Event Audit Retrofit Summary

**One-liner:** Retrofits Phase 1 login/logout/change-password handlers to write audit rows (auth.login_success, auth.login_failed, auth.logout, auth.password_change, auth.session_revoked) IN THE SAME TRANSACTION as the auth state change, closing D-30 / AUDIT-01.

## What shipped

### Backend (Go)

| File | Surface |
|------|---------|
| `internal/auth/handlers.go` | `LoginHandler`: ReadCommitted tx wrapping `GetUserByEmailTx` + `Verify` + `UpdateLastLoginAtTx` + `audit.WriteEntry`. Three paths audited: login_success (success), login_failed with user_id=NULL (unknown email), login_failed with user_id set (bad password). Rate-limit 429 short-circuits BEFORE the tx — no audit row. `LogoutHandlerWithAudit`: short tx for auth.logout audit row before `sm.Destroy`; anonymous logout returns 204 with no audit row. |
| `internal/auth/account.go` | `ChangePasswordHandler`: default-isolation tx wrapping `UpdatePasswordTx` + `auth.password_change` audit row + `auth.session_revoked` audit row; `iterateAndRevoke` called AFTER commit (Phase 1 keepToken pattern preserved). |
| `internal/auth/users.go` | `GetUserByEmailTx`: tx-scoped variant of `GetUserByEmail` — same ErrUserNotFound semantics, uses `tx.QueryRow`. `UpdateLastLoginAtTx`: sets `user.last_login_at = now()` inside the caller's tx, atomically paired with the login_success audit row. |
| `internal/http/router.go` | `LogoutHandlerWithAudit` wired in the production router (replaces `LogoutHandler`). |

### Tests (11 new)

**handlers_test.go** (8 new tests):

| Test | Asserts |
|------|---------|
| `TestLogin_SuccessWritesAuditAndLastLogin` | 200 + audit row auth.login_success + last_login_at updated within 5s |
| `TestLogin_FailureWritesAuditFailed` | 401 + audit row auth.login_failed with user_id + notes containing email= |
| `TestLogin_UnknownEmailWritesAuditFailed` | 401 + audit row auth.login_failed with user_id=NULL + notes containing email= |
| `TestLogin_RateLimit429NoAudit` | 429 path writes 0 audit rows (D-30 scope) |
| `TestLogin_AtomicityOnAuditFailure` | Failed login does NOT write auth.login_success |
| `TestLogin_DoesNotAuditSessionRefresh` | Subsequent GET /api/account/me does NOT add new audit rows |
| `TestLogout_WritesAuditAndDestroysSession` | auth.logout row written + session destroyed |
| `TestLogout_AnonymousReturns204NoAudit` | Anonymous logout = 204, 0 audit rows |

**account_test.go** (3 new tests):

| Test | Asserts |
|------|---------|
| `TestChangePassword_WritesAudit` | auth.password_change row written; session preserved (keepToken) |
| `TestChangePassword_RevokesOtherSessions` | 3 sessions: A preserved, B+C revoked; auth.session_revoked row written |
| `TestChangePassword_AuditAtomicWithUpdate` | Audit row + password hash both changed; old password rejected |

## Deviations from Plan

### Rule 3 (auto-fix blocking issue) — LogoutHandler API split

**Found during:** Task 1 implementation
**Issue:** The plan pseudo-code showed replacing `LogoutHandler(sm)` in place, but the existing `LogoutHandler` signature is `func LogoutHandler(sm *scs.SessionManager) http.HandlerFunc` and is referenced in both the test setup AND the production router. Changing the signature would break all existing callers.
**Fix:** Added `LogoutHandlerWithAudit(sm *scs.SessionManager, store *Store) http.HandlerFunc` as an audit-aware variant. The original `LogoutHandler` is preserved as a no-audit backward-compat form (still useful for test fixtures that don't need audit assertions). The production router is updated to use `LogoutHandlerWithAudit`; test fixtures that test logout audit behavior also use it.
**Files modified:** `internal/auth/handlers.go`, `internal/http/router.go`, `internal/auth/handlers_test.go`

### Rule 2 (auto-add missing critical functionality) — `GetUserByEmailTx` added to users.go

**Found during:** Task 1 implementation
**Issue:** The plan specified adding `GetUserByEmailTx` but flagged it as "ADD HERE in this plan since this is the consumer." The method was not present in Plan 06-05's users.go (which extended Store with 9 other methods but not this one).
**Fix:** Added `GetUserByEmailTx` to `internal/auth/users.go` alongside `UpdateLastLoginAtTx`. Both methods are consumed exclusively by the login handler retrofit in this plan.
**Files modified:** `internal/auth/users.go`

## Auth gates

None — fully autonomous execution.

## Test results

- `go build ./...`: clean
- `go vet ./...`: clean
- `go test ./internal/auth/... -count=1 -timeout=180s`: 83 passed (83 = 72 original + 11 new)
- `go test ./internal/auth/... -run "TestLogin_|TestLogout_" -count=1`: 8 new tests pass
- `go test ./internal/auth/... -run "TestChangePassword" -count=1`: 3 new tests pass + 5 existing pass

## Threat-model assertions verified

- **T-06-06-01** (Repudiation — login without audit trail): TestLogin_SuccessWritesAuditAndLastLogin + TestLogin_FailureWritesAuditFailed + TestLogin_UnknownEmailWritesAuditFailed + TestLogout_WritesAuditAndDestroysSession all pass.
- **T-06-06-02** (Information Disclosure — email enumeration): Both unknown-email and bad-password paths return identical `{"error":"bad_credentials"}` 401; only the audit notes differ (operator-visible only). Verified by test response body assertions.
- **T-06-06-03** (Information Disclosure — password in audit notes): `grep "req.Password\|req.NewPassword" internal/auth/handlers.go internal/auth/account.go` returns 0 — no password value in audit notes.
- **T-06-06-04** (DoS — session-refresh audit noise): TestLogin_DoesNotAuditSessionRefresh asserts 0 new rows after 2 subsequent GETs.
- **T-06-06-05** (Tampering — rate-limit bypass via audit failure): Rate-limit counter bump happens in the `Allow()` call BEFORE the tx; even if audit fails, the counter already incremented.
- **T-06-06-06** (DoS — excessive failed-login rows): TestLogin_RateLimit429NoAudit asserts the rate-limited path writes 0 audit rows.

## Known Stubs

None — all audit rows are wired to real DB writes.

## Threat Flags

None — no new network endpoints, auth paths, or schema changes introduced.

## Self-Check: PASSED

- `internal/auth/handlers.go` exists with LoginHandler + LogoutHandlerWithAudit ✓
- `internal/auth/account.go` exists with ChangePasswordHandler + BeginTx + two audit rows ✓
- `internal/auth/users.go` has `GetUserByEmailTx` (line 358) + `UpdateLastLoginAtTx` (line 376) ✓
- `internal/http/router.go` references `LogoutHandlerWithAudit` (line 239) ✓
- 4 task commits exist: 092fd71 (RED handlers) + fe8e5b9 (GREEN handlers) + 7e513d3 (RED account) + 317eff3 (GREEN account) ✓
- `go build ./...` clean ✓
- `go vet ./...` clean ✓
- 83 auth tests pass ✓
