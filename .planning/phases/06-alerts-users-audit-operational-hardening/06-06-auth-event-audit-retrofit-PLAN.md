---
phase: 06-alerts-users-audit-operational-hardening
plan: 06
type: execute
wave: 2
depends_on: [06-01, 06-05]
files_modified:
  - internal/auth/handlers.go
  - internal/auth/handlers_test.go
  - internal/auth/account.go
  - internal/auth/account_test.go
  - internal/auth/ratelimit.go
  - internal/audit/log.go
autonomous: true
requirements: [AUDIT-01]
notes:
  - "Closes D-30 (operator-visible auth-event audit retrofit) — a Phase 2 D-21 deferred sub-item under the AUDIT-01 umbrella. Plan 06-07 owns AUDIT-02 (admin can view audit log with filters); this plan is the substrate that makes those filters surface auth events."
must_haves:
  truths:
    - "POST /api/auth/login on success writes 'auth.login_success' audit row in same tx as session put (D-30 operator-visible only)"
    - "POST /api/auth/login on failure writes 'auth.login_failed' audit row with email in notes (no user_id since auth failed)"
    - "POST /api/auth/logout writes 'auth.logout' audit row"
    - "POST /api/auth/change-password writes 'auth.password_change' audit row"
    - "Silent SCS session refreshes / idle-timeout expirations / per-request reads are NOT audited (D-30 scope note)"
    - "On successful login, user.last_login_at is updated in the same tx as the audit row"
    - "Plan 06-05 user-mgmt handlers (already audit-in-tx per that plan) continue to work; this plan retrofits ONLY Phase 1 handlers"
  artifacts:
    - path: internal/auth/handlers.go
      provides: "Retrofitted LoginHandler/LogoutHandler with audit-in-tx + last_login_at update"
    - path: internal/auth/account.go
      provides: "Retrofitted ChangePasswordHandler with audit-in-tx for self-change"
  key_links:
    - from: internal/auth/handlers.go
      to: audit_log
      via: "audit.WriteEntry(ctx, tx, ...) inside the BeginTx/Commit block AROUND the existing session.PutUser call"
      pattern: "audit\\.WriteEntry"
---

<objective>
Retrofit Phase 1 Plans 01-09 (login/logout/change-password) and 01-11 (account UI password change) to write audit rows IN THE SAME TRANSACTION as the auth state change — closes the AUDIT-01 / D-30 deferred work from Phase 2.

Per D-30: scope is operator-visible events only — login_success, login_failed, logout, password_change, password_reset_by_admin, session_revoked. Silent session refreshes / per-request session reads / idle-timeout expirations are NOT audited (would balloon audit_log with no operator value).

Purpose: an admin reviewing the audit log can see every login attempt (success and failure) plus every self-initiated password change — without polluting the log with high-frequency session-refresh noise. Reconciles with the auth-event audit vocabulary added in Plan 06-01 migration 0037.

Output: 4 handler retrofits (LoginHandler, LogoutHandler, ChangePasswordHandler, plus the failed-login path in ratelimit.go) + last_login_at update wired in the login success path + handler tests upgraded to assert audit rows are written.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-01-alert-engine-substrate-PLAN.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-05-user-management-PLAN.md
@internal/auth/handlers.go
@internal/auth/handlers_test.go
@internal/auth/account.go
@internal/auth/account_test.go
@internal/auth/session.go
@internal/auth/ratelimit.go
@internal/auth/users.go
@internal/audit/log.go

<interfaces>
Plan 06-01 added these audit constants in internal/audit/log.go:
- ActionAuthLoginSuccess        = "auth.login_success"
- ActionAuthLoginFailed         = "auth.login_failed"
- ActionAuthLogout              = "auth.logout"
- ActionAuthPasswordChange      = "auth.password_change"
- ActionAuthPasswordResetByAdmin= "auth.password_reset_by_admin"
- ActionAuthSessionRevoked      = "auth.session_revoked"
- EntityTypeUser                = "user"
- EntityTypeSession             = "session"

Plan 06-05 added internal/auth/users.go method:
- (s *Store) UpdateLastLoginAtTx(ctx, tx pgx.Tx, userID string) error  -- NOT YET in Plan 06-05 task 1. Add it here.
  This plan's Task 1 adds it as a 1-line method in users.go OR ALTERS Plan 06-05 task 1's list. Implementation prefers: ADD HERE in this plan since this is the consumer.

EXISTING login handler (internal/auth/handlers.go from Plan 01-09):
- Currently calls: store.GetUserByEmail → auth.Verify → session.PutUser(...). No transaction.
- Retrofit pattern (RESEARCH §Decision G "Atomic Auth-Event Audit Retrofit"): wrap in BeginTx → audit.WriteEntry → tx.Commit → session.PutUser AFTER commit.

EXISTING change-password handler (internal/auth/account.go):
- Currently uses internal SCS account.go pattern with iterateAndRevoke for keepToken case.
- Retrofit: wrap UpdatePassword in a BeginTx + audit row + commit.

EXISTING failed-login path (internal/auth/ratelimit.go + handlers.go):
- ratelimit.go bumps counters per IP/email; the actual 401 response is in handlers.go.
- Retrofit: the 401 path writes 'auth.login_failed' audit row in its own short tx; rate-limit counter bump happens separately.
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Retrofit LoginHandler + LogoutHandler with audit-in-tx + last_login_at update</name>
  <files>internal/auth/handlers.go, internal/auth/handlers_test.go, internal/auth/ratelimit.go, internal/auth/users.go</files>
  <read_first>
    - internal/auth/handlers.go (current LoginHandler / LogoutHandler implementations; identify the section between request decoding and session.PutUser)
    - internal/auth/ratelimit.go (failed-login bumping; the per-IP rate-limit check happens BEFORE the password verify)
    - internal/auth/users.go (existing Store; this plan adds UpdateLastLoginAtTx)
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Decision G "Atomic Auth-Event Audit Retrofit (D-30)" — exact handler shape
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Pitfall 4 "Audit-event retrofit breaks existing handler tests"
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md §D-30 (scope: operator-visible events only)
  </read_first>
  <behavior>
    - Test (TestLogin_SuccessWritesAuditAndLastLogin): POST /api/auth/login with valid creds → 200 + session set; SELECT count(*) FROM audit_log WHERE action='auth.login_success' AND user_id=$1 returns 1; SELECT last_login_at FROM "user" WHERE id=$1 is recent (within 5s).
    - Test (TestLogin_FailureWritesAuditFailed): POST with wrong password → 401; audit row 'auth.login_failed' present with user_id=NULL and notes containing 'email=' and the attempted email; failed-counter still bumped.
    - Test (TestLogin_UnknownEmailWritesAuditFailed): POST with email that has no user → 401 (avoid email enumeration); audit row 'auth.login_failed' present (no user_id) with notes 'email=...'.
    - Test (TestLogin_RateLimit429NoAudit): once rate limited (429), audit row is NOT written (D-30 scope: only the actual login attempt is auditable; the rate-limit short-circuit is not an attempt).
    - Test (TestLogin_AtomicityOnAuditFailure): inject an error in audit.WriteEntry (e.g., bad action constant via test double) → tx rolls back; user.last_login_at NOT updated; no session put.
    - Test (TestLogin_DoesNotAuditSessionRefresh): subsequent GET /api/users/me (any authenticated route) does NOT add a new audit row.
    - Test (TestLogout_WritesAuditAndDestroysSession): POST /api/auth/logout → audit row 'auth.logout' present with user_id=current user; session destroyed.
    - Test (TestLogout_AnonymousReturns204NoAudit): unauthenticated POST /api/auth/logout → 204 no-op; no audit row.
  </behavior>
  <action>
    **internal/auth/users.go:** Append one method (the consumer is this plan; we add it here rather than in Plan 06-05 to keep that plan focused on user-mgmt CRUD):
    ```go
    // UpdateLastLoginAtTx sets user.last_login_at = now() inside the caller's tx.
    // Called by the LoginHandler immediately before commit so a successful login
    // is atomically reflected in the user's last_login_at + audit_log.
    func (s *Store) UpdateLastLoginAtTx(ctx context.Context, tx pgx.Tx, userID string) error {
        _, err := tx.Exec(ctx, `UPDATE "user" SET last_login_at = now() WHERE id = $1::uuid`, userID)
        if err != nil {
            return fmt.Errorf("update last_login_at: %w", err)
        }
        return nil
    }
    ```

    **internal/auth/handlers.go:** Restructure `LoginHandler` (preserve existing rate-limit + lockout logic, restructure the user-lookup + verify steps to run inside a tx):

    Pseudo-code (apply to the existing function — DO NOT rewrite from scratch; preserve every Phase 1 behavior including rate-limit short-circuit at top, 429 response, 401 message format):

    ```go
    func LoginHandler(deps Deps) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            // 1. EXISTING: parse body, validate format, rate-limit check (unchanged)
            // ...
            // If rate-limited → 429 WITHOUT audit row (D-30 scope excludes ratelimit short-circuits)

            // 2. NEW: open tx for audit-in-tx pattern
            tx, err := deps.Store.Pool().BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
            if err != nil { writeError(w, 500, "db_begin"); return }
            defer tx.Rollback(r.Context()) // safe if Commit already happened

            // 3. RESTRUCTURED: GetUserByEmail inside tx
            u, err := deps.Store.GetUserByEmailTx(r.Context(), tx, req.Email) // ADD this Tx variant
            if errors.Is(err, auth.ErrUserNotFound) {
                // audit the failed attempt (anti-enumeration: same response delay)
                _ = audit.WriteEntry(r.Context(), tx, audit.Entry{
                    Action:     audit.ActionAuthLoginFailed,
                    EntityType: audit.EntityTypeUser,
                    EntityID:   uuid.Nil,
                    Notes:      "email=" + req.Email + " reason=unknown_email",
                    RequestID:  middleware.GetReqID(r.Context()),
                })
                _ = tx.Commit(r.Context()) // commit just the audit row
                deps.RL.BumpFailed(req.Email, clientIP(r))
                writeError(w, 401, "bad_credentials")
                return
            }
            if err != nil { writeError(w, 500, "db_lookup"); return }

            // 4. RESTRUCTURED: Verify password
            ok, err := auth.Verify(u.PasswordHash, req.Password)
            if err != nil { writeError(w, 500, "verify"); return }
            if !ok {
                _ = audit.WriteEntry(r.Context(), tx, audit.Entry{
                    UserID:     mustParseUUID(u.ID),
                    Action:     audit.ActionAuthLoginFailed,
                    EntityType: audit.EntityTypeUser,
                    EntityID:   mustParseUUID(u.ID),
                    Notes:      "email=" + u.Email + " reason=bad_password",
                    RequestID:  middleware.GetReqID(r.Context()),
                })
                _ = tx.Commit(r.Context())
                deps.RL.BumpFailed(req.Email, clientIP(r))
                writeError(w, 401, "bad_credentials")
                return
            }

            // 5. Success path
            if err := deps.Store.UpdateLastLoginAtTx(r.Context(), tx, u.ID); err != nil {
                writeError(w, 500, "update_last_login"); return
            }
            if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
                UserID:     mustParseUUID(u.ID),
                Action:     audit.ActionAuthLoginSuccess,
                EntityType: audit.EntityTypeUser,
                EntityID:   mustParseUUID(u.ID),
                RequestID:  middleware.GetReqID(r.Context()),
            }); err != nil { writeError(w, 500, "audit"); return }
            if err := tx.Commit(r.Context()); err != nil { writeError(w, 500, "db_commit"); return }

            // 6. POST-commit: put session (separate concern from user-DB tx)
            deps.SessionMgr.Put(r.Context(), sessionUserIDKey, u.ID)
            deps.SessionMgr.Put(r.Context(), sessionRoleKey, u.Role)
            deps.SessionMgr.Put(r.Context(), sessionMustChangeKey, u.MustChangePassword)
            deps.RL.ResetFailed(req.Email, clientIP(r))

            writeJSON(w, 200, map[string]any{"id": u.ID, "role": u.Role, "must_change_password": u.MustChangePassword})
        }
    }
    ```

    Add `GetUserByEmailTx(ctx, tx, email)` to internal/auth/users.go — same query as GetUserByEmail but using `tx.QueryRow` instead of `s.pool.QueryRow`. Keep the existing non-Tx variant for backward compat.

    **internal/auth/handlers.go LogoutHandler:** smaller retrofit — wrap the existing destroy logic in a tx that writes 'auth.logout' audit row before destroying session.

    ```go
    func LogoutHandler(deps Deps) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            userID := deps.SessionMgr.GetString(r.Context(), sessionUserIDKey)
            if userID == "" {
                w.WriteHeader(204) // anonymous logout — no audit row
                return
            }
            tx, _ := deps.Store.Pool().BeginTx(r.Context(), pgx.TxOptions{})
            defer tx.Rollback(r.Context())
            _ = audit.WriteEntry(r.Context(), tx, audit.Entry{
                UserID:     mustParseUUID(userID),
                Action:     audit.ActionAuthLogout,
                EntityType: audit.EntityTypeUser,
                EntityID:   mustParseUUID(userID),
                RequestID:  middleware.GetReqID(r.Context()),
            })
            _ = tx.Commit(r.Context())
            _ = deps.SessionMgr.Destroy(r.Context())
            w.WriteHeader(204)
        }
    }
    ```

    **internal/auth/handlers_test.go:** Upgrade tests to:
    1. Use testcontainer (instead of any mock — see Pitfall 4 mitigation); the existing tests may already use testcontainer
    2. After each login/logout, SELECT count(*) FROM audit_log WHERE ... and assert the expected row count

    No NEW failed-login route is added; the existing 401 path now writes the audit row.

    **D-30 scope note re-affirmed in code:** add a comment block at the top of LoginHandler:
    ```go
    // D-30 scope: this handler audits only operator-visible events:
    //   - auth.login_success  (every successful POST /api/auth/login)
    //   - auth.login_failed   (every 401 with attempted email; failure reason in notes)
    //   - auth.logout         (LogoutHandler; not this function)
    // Silent SCS session refreshes / idle-timeout expirations / per-request reads
    // are NOT audited (D-30 scope decision; CONTEXT.md §D-30).
    ```
  </action>
  <verify>
    <automated>go test ./internal/auth/... -run "TestLogin_|TestLogout_" -count=1 -timeout=60s</automated>
  </verify>
  <acceptance_criteria>
    - `internal/auth/handlers.go` LoginHandler calls `audit.WriteEntry` at least 3 times (success path + 2 failure paths)
    - `internal/auth/handlers.go` LoginHandler calls `deps.Store.UpdateLastLoginAtTx` in the success path inside the tx
    - `internal/auth/handlers.go` LogoutHandler writes 'auth.logout' audit row for authenticated logout; returns 204 without audit row for anonymous logout
    - `internal/auth/users.go` has `func (s *Store) UpdateLastLoginAtTx(` and `func (s *Store) GetUserByEmailTx(`
    - `internal/auth/handlers.go` has the D-30 scope comment block (grep "D-30 scope")
    - All 8 listed tests pass: `go test ./internal/auth/... -run "TestLogin|TestLogout" -count=1` exits 0
    - `TestLogin_DoesNotAuditSessionRefresh` exists and asserts no new audit row appears after a subsequent authenticated GET request (proves silent refresh is not audited)
    - `TestLogin_RateLimit429NoAudit` exists and asserts 429 path writes 0 audit rows
  </acceptance_criteria>
  <done>Every operator-visible auth event is now atomically audited; silent session reads are NOT in the audit log.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Retrofit ChangePasswordHandler with audit-in-tx (self-change path)</name>
  <files>internal/auth/account.go, internal/auth/account_test.go</files>
  <read_first>
    - internal/auth/account.go (existing ChangePasswordHandler at line 52)
    - internal/auth/account.go::iterateAndRevoke (already shipping; called for keepToken case)
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md §D-30 (auth.password_change scope)
  </read_first>
  <behavior>
    - Test (TestChangePassword_WritesAudit): POST /api/account/change-password as authenticated user → password updated; audit row 'auth.password_change' written with user_id=current user; session preserved (keepToken).
    - Test (TestChangePassword_RevokesOtherSessions): user has 3 sessions; calls change-password from session A → A preserved, B and C deleted; audit row 'auth.session_revoked' present in same tx.
    - Test (TestChangePassword_AuditAtomicWithUpdate): forced WriteEntry error → password update rolled back (user can still log in with old password).
  </behavior>
  <action>
    Restructure ChangePasswordHandler to wrap the existing logic in a transaction:

    ```go
    func ChangePasswordHandler(deps AccountDeps) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            // EXISTING: parse body, validate strength, verify old password, hash new
            // ...
            tx, err := deps.Store.Pool().BeginTx(r.Context(), pgx.TxOptions{})
            if err != nil { writeError(w, 500, "db_begin"); return }
            defer tx.Rollback(r.Context())

            // Use UpdatePasswordTx (the Tx variant). Plan 06-05 Task 1 added this method.
            if err := deps.Store.UpdatePasswordTx(r.Context(), tx, u.ID, newHash, false); err != nil {
                writeError(w, 500, "db_update"); return
            }
            if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
                UserID:     mustParseUUID(u.ID),
                Action:     audit.ActionAuthPasswordChange,
                EntityType: audit.EntityTypeUser,
                EntityID:   mustParseUUID(u.ID),
                Notes:      "self_initiated",
                RequestID:  middleware.GetReqID(r.Context()),
            }); err != nil { writeError(w, 500, "audit"); return }
            // Phase 1 invariant: revoke OTHER sessions (keep current); audit that revoke.
            if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
                UserID:     mustParseUUID(u.ID),
                Action:     audit.ActionAuthSessionRevoked,
                EntityType: audit.EntityTypeSession,
                EntityID:   mustParseUUID(u.ID),
                Notes:      "other_sessions_revoked_on_password_change",
                RequestID:  middleware.GetReqID(r.Context()),
            }); err != nil { writeError(w, 500, "audit"); return }
            if err := tx.Commit(r.Context()); err != nil { writeError(w, 500, "db_commit"); return }
            // AFTER commit: revoke OTHER sessions (Phase 1 existing pattern)
            currentToken := deps.SessionMgr.Token(r.Context())
            if err := iterateAndRevoke(r.Context(), deps.SessionMgr, deps.Store, u.ID, currentToken); err != nil {
                deps.Log.Warn("session_revoke_after_pw_change", "user_id", u.ID, "err", err)
            }
            w.WriteHeader(204)
        }
    }
    ```

    Note: the existing handler at line 119 already calls `iterateAndRevoke(ctx, sm, store, u.ID, currentToken)` — preserve that exact call.

    **internal/auth/account_test.go:** Add the 3 tests above. Re-running existing test suite must pass (Pitfall 4: existing tests need real-pool fixtures — verify they already use testcontainer; if not, upgrade in this task).
  </action>
  <verify>
    <automated>go test ./internal/auth/... -run "TestChangePassword" -count=1 -timeout=60s</automated>
  </verify>
  <acceptance_criteria>
    - `internal/auth/account.go` ChangePasswordHandler contains `BeginTx` AND `audit.WriteEntry` for ActionAuthPasswordChange AND for ActionAuthSessionRevoked (two audit rows in same tx)
    - `internal/auth/account.go` ChangePasswordHandler still calls `iterateAndRevoke(` AFTER tx.Commit (post-commit session cleanup unchanged)
    - All 3 new tests pass: `go test ./internal/auth/... -run TestChangePassword -count=1` exits 0
    - `TestChangePassword_AuditAtomicWithUpdate` asserts that forcing an audit error means the password did NOT change (user can still log in with old password)
  </acceptance_criteria>
  <done>Self-initiated password change is atomically audited; the existing keep-current-session-revoke-others pattern preserved.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| client→/api/auth/* | The handlers retrofitted here are the primary credential-handling surface; every auth event now has an immutable audit trail |
| audit_log INSERT trigger | Trigger from migration 0016 allows INSERT; D-30 retrofit only INSERTs, never UPDATE/DELETE |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-06-06-01 | Repudiation | login attempt without audit trail | mitigate | Every login path (success / unknown email / bad password / authenticated logout) writes one audit row in the same tx as the auth state change. Test: TestLogin_SuccessWritesAuditAndLastLogin + TestLogin_FailureWritesAuditFailed + TestLogin_UnknownEmailWritesAuditFailed + TestLogout_WritesAuditAndDestroysSession. |
| T-06-06-02 | Information Disclosure | email enumeration via different responses | accept | The 'unknown_email' and 'bad_password' failure paths return the SAME 401 message ("bad_credentials"); only the audit-row 'notes' field differs (operator-visible only). Latency is similar (both paths hit the same WriteEntry). |
| T-06-06-03 | Information Disclosure | audit_log notes contains password | mitigate | The handlers never write the plaintext password into audit notes; only the email + reason. The `notes` field for bad-password is hardcoded "reason=bad_password" not the value. Test: grep `audit.WriteEntry` calls in handlers.go to ensure no `req.Password` substring. |
| T-06-06-04 | DoS | audit_log balloons from session-refresh noise | mitigate | D-30 scope is OPERATOR-visible events only. Silent SCS session refreshes / per-request session reads are explicitly NOT audited. Test: TestLogin_DoesNotAuditSessionRefresh asserts no new row on subsequent GET. |
| T-06-06-05 | Tampering | rate-limit bypass via audit failure | mitigate | The rate-limit counter bump (`deps.RL.BumpFailed(...)`) is OUTSIDE the tx; even if the audit-row commit fails, the counter increments → rate limit still works. Test (in ratelimit_test.go): even when audit WriteEntry errors, counter increments. |
| T-06-06-06 | DoS | excessive failed-login audit rows from an attacker | accept | Rate limit caps per-IP/per-email retries within a window (Phase 1 D-23); the rows past the rate limit are NOT audited (429 short-circuit before the audit-writing path). Test: TestLogin_RateLimit429NoAudit. |
</threat_model>

<verification>
- Login success / failure / logout / password-change all atomically audited
- Silent session refreshes NOT audited (D-30 scope)
- Rate-limit short-circuit (429) does NOT write audit rows
- `last_login_at` populated on every successful login (powers UI-SPEC §Surface 5 "Last login" column)
- All listed tests pass: `go test ./internal/auth/... -count=1 -timeout=60s`
</verification>

<success_criteria>
- AUDIT-02 covered server-side: the audit log now contains a complete trail of every operator-visible auth event
- D-30 implemented per scope (operator-visible only)
- Plan 06-07 (audit browse) can rely on these row types existing
- last_login_at column populated and consumable by Plan 06-05 UI
</success_criteria>

<output>
After completion, create `.planning/phases/06-alerts-users-audit-operational-hardening/06-06-SUMMARY.md`
</output>
