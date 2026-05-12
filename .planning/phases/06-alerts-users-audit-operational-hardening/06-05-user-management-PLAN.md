---
phase: 06-alerts-users-audit-operational-hardening
plan: 05
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/db/migrations/0044_user_last_login.up.sql
  - internal/db/migrations/0044_user_last_login.down.sql
  - internal/auth/users.go
  - internal/auth/users_test.go
  - internal/user/store.go
  - internal/user/store_test.go
  - internal/user/handler.go
  - internal/user/handler_test.go
  - internal/user/password.go
  - internal/user/password_test.go
  - internal/user/guards.go
  - internal/user/guards_test.go
  - internal/auth/account.go
  - internal/auth/authz.go
  - internal/auth/authz_test.go
  - internal/http/router.go
  - web/src/routes/settings/users.tsx
  - web/src/routes/settings/users.test.tsx
  - web/src/routes/settings/AddUserDialog.tsx
  - web/src/routes/settings/EditUserDialog.tsx
  - web/src/routes/settings/ShareCredentialsPanel.tsx
  - web/src/routes/settings/ResetPasswordDialog.tsx
  - web/src/routes/settings/LogoutEverywhereDialog.tsx
  - web/src/routes/settings/DisableUserDialog.tsx
  - web/src/routes/settings/ReEnableUserDialog.tsx
  - web/src/routes/settings/RoleChangeDialog.tsx
  - web/src/hooks/useUsers.ts
  - web/playwright/specs/user-management.spec.ts
autonomous: true
requirements: [USER-01, USER-02, USER-03, USER-04]
must_haves:
  truths:
    - "Admin can list active users; toggle Show disabled to see soft-deleted users (D-27)"
    - "Admin Add User dialog generates random ≥16-char strong password and shows it ONCE in a share-credentials panel (D-23); must_change_password=true on the created user"
    - "Admin Edit User dialog changes name + role; role change triggers Logout-everywhere confirm + revokes all sessions for the user (D-25)"
    - "Admin Logout-everywhere row action deletes all SCS sessions for that user_id (D-24); same code path triggers on Disable, Role change, Reset password"
    - "Admin Disable sets disabled_at=now(); user immediately signed out + cannot sign in (D-27 soft-delete; AUDIT preserved)"
    - "Admin Reset password generates new random + shows-once + sets must_change_password=true + revokes sessions"
    - "Admin Re-enable preserves original credentials + state (same password hash + must_change_password flag) (D-27)"
    - "Server REJECTS self-disable, self-role-change, and last-admin-demote with 422 (D-26 server-side enforcement)"
    - "UI greys out self-action and last-admin-demote actions (D-26 client-side enforcement; both must exist, not one)"
    - "Random password generator uses crypto/rand + alphabet excluding ambiguous chars 1lI0O + length ≥ 16; re-rolls if Phase 1 strength evaluator fails (D-28 reuse)"
  artifacts:
    - path: internal/user/store.go
      provides: "List, Create, Update, Disable, Enable, ChangeRole user store methods"
      exports: ["Store","List","Create","Update","Disable","Enable","ChangeRole","ResetPassword"]
    - path: internal/user/handler.go
      provides: "/api/users HTTP CRUD + /logout-everywhere endpoint"
      exports: ["ListHandler","CreateHandler","UpdateHandler","DisableHandler","EnableHandler","ChangeRoleHandler","LogoutEverywhereHandler","ResetPasswordHandler"]
    - path: internal/user/password.go
      provides: "GenerateRandomPassword respecting D-28 strength evaluator"
      exports: ["GenerateRandomPassword"]
    - path: internal/user/guards.go
      provides: "Self-action + last-admin guards (D-26 server-side)"
      exports: ["RejectSelfAction","RejectLastAdminDemote","ErrSelfAction","ErrLastAdmin"]
    - path: web/src/routes/settings/users.tsx
      provides: "Users table + dialogs"
    - path: web/src/routes/settings/AddUserDialog.tsx
      provides: "2-step add (form + share-credentials)"
  key_links:
    - from: internal/user/handler.go
      to: internal/auth/account.go::iterateAndRevoke
      via: "LogoutEverywhereHandler + ChangeRoleHandler + DisableHandler + ResetPasswordHandler ALL call iterateAndRevoke(user_id, keepToken='')"
      pattern: "iterateAndRevoke"
    - from: internal/user/guards.go
      to: internal/user/handler.go
      via: "every state-changing handler calls RejectSelfAction OR RejectLastAdminDemote BEFORE mutating"
      pattern: "RejectSelfAction\\|RejectLastAdminDemote"
---

<objective>
Ship user management: list/create/update/disable/enable/role-change/reset-password/logout-everywhere — server-enforced (D-26 guards + D-24 session-revoke) + UI dialogs with show-once password panel (D-23) + Settings → Users tab (D-29). Closes USER-01..04.

Purpose: until this plan, only the install wizard's admin can sign in. After this plan, the operator can onboard team members with random-show-once passwords (no SMTP needed, per project constraint), demote/disable them, and force them out when a role change implies they need to re-authenticate.

Output: new `internal/user/` package (store + handlers + password + guards) + 8 React dialogs + Settings → Users route + Playwright E2E covering add-user → share-once → first-login force-rotate.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-UI-SPEC.md
@internal/auth/users.go
@internal/auth/account.go
@internal/auth/handlers.go
@internal/auth/session.go
@internal/auth/authz.go
@internal/auth/password_strength.go
@internal/auth/argon2id.go
@internal/audit/log.go
@internal/http/router.go
@web/src/components/responsive-dialog.tsx
@web/src/components/ui/alert-dialog.tsx
@web/src/lib/use-current-user.ts

<interfaces>
internal/auth/users.go EXISTING (Plan 01-09):
```go
type UserRecord struct {
    ID, Email, Name, PasswordHash, Role string
    MustChangePassword bool
}
func (s *Store) GetUserByEmail(ctx, email) (*UserRecord, error)
func (s *Store) GetUserByID(ctx, id) (*UserRecord, error)
func (s *Store) AdminExists(ctx) (bool, error)
func (s *Store) InsertAdminUser(ctx, email, name, hash) (string, error)
func (s *Store) UpdatePassword(ctx, userID, hash) error
func (s *Store) Pool() *pgxpool.Pool
```
Plan 06-05 EXTENDS this Store with: List(ctx, scope), Create(ctx, tx, email, name, role, hash, mustChange), Update(ctx, tx, id, name), Disable(ctx, tx, id), Enable(ctx, tx, id), ChangeRole(ctx, tx, id, newRole), UpdatePasswordTx(ctx, tx, id, hash, mustChange).

internal/auth/account.go EXISTING (Plan 01-11):
```go
func iterateAndRevoke(ctx context.Context, sm *scs.SessionManager, store *Store, userID, keepToken string) error
```
The function is UNEXPORTED in account.go. Plan 06-05 EXPORTS it as `auth.IterateAndRevoke` (capitalized) so internal/user/handler.go can call it.

internal/auth/password_strength.go EXISTING (Plan 01-07):
```go
func StrengthScore(password string) int    // returns score
const MinScore int                          // OWASP-grade threshold
```

internal/auth/argon2id.go EXISTING (Plan 01-07):
```go
func Hash(password string) (string, error) // returns PHC string
func Verify(hash, password string) (bool, error)
```

internal/auth/authz.go EXISTING — has `ActionUserManage Action = "user.manage"` (line 65) declared since Phase 1. Plan 06-05 ADDS finer-grained actions and uses them in router.go but keeps ActionUserManage as the umbrella.

"user" table schema (Phase 1 migration 0002):
columns: id UUID PK, email TEXT UNIQUE, name TEXT, password_hash TEXT, role user_role ENUM('admin','viewer'),
must_change_password BOOLEAN DEFAULT FALSE, disabled_at TIMESTAMPTZ NULL, created_at, updated_at.

audit constants (Plan 06-01 task 1):
- ActionUserCreate, ActionUserUpdate, ActionUserDisable, ActionUserEnable, ActionUserRoleChange
- ActionAuthSessionRevoked, ActionAuthPasswordResetByAdmin
- EntityTypeUser, EntityTypeSession
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Store extensions + guards + password generator + IterateAndRevoke export</name>
  <files>internal/db/migrations/0044_user_last_login.up.sql, internal/db/migrations/0044_user_last_login.down.sql, internal/auth/users.go, internal/auth/users_test.go, internal/auth/account.go, internal/user/store.go, internal/user/store_test.go, internal/user/password.go, internal/user/password_test.go, internal/user/guards.go, internal/user/guards_test.go, internal/user/doc.go</files>
  <read_first>
    - internal/auth/users.go (existing Store surface; Plan 06-05 extends it)
    - internal/auth/account.go (iterateAndRevoke function body — needs EXPORT per Pitfall 3 RESEARCH §Decision G)
    - internal/auth/password_strength.go (StrengthScore + MinScore — used by D-28 password generator)
    - internal/auth/argon2id.go (Hash for password creation)
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Decision G "Random Password Generator (D-23)" + "Pitfall 6: Last-admin check has a TOCTOU race"
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md §D-23, D-24, D-25, D-26, D-27, D-28
  </read_first>
  <behavior>
    - Test (TestStore_ListActiveOnly): seed 2 active users + 1 disabled → List(scope='active') returns 2; List(scope='disabled') returns 1; List(scope='all') returns 3.
    - Test (TestStore_CreateUser_Viewer): Create returns new UUID; user row has role='viewer', must_change_password=true.
    - Test (TestStore_Update): Update changes name without touching password_hash or role.
    - Test (TestStore_Disable_Enable): Disable sets disabled_at=now(); Enable sets disabled_at=NULL; password_hash + must_change_password unchanged.
    - Test (TestStore_ChangeRole): ChangeRole sets role; returns the new record.
    - Test (TestStore_UpdatePasswordTx_SetsMustChangeFlag): UpdatePasswordTx(id, hash, must_change=true) persists both.
    - Test (TestGenerateRandomPassword_Strong): GenerateRandomPassword() returns a string of length 16 from the allowed alphabet, passes Phase 1 StrengthScore >= MinScore.
    - Test (TestGenerateRandomPassword_NoAmbiguousChars): generated string contains NONE of: '1', 'l', 'I', '0', 'O' (the excluded ambiguous chars per CONTEXT.md Claude's Discretion + D-23).
    - Test (TestRejectSelfAction): RejectSelfAction(currentUserID, currentUserID) returns ErrSelfAction; RejectSelfAction(a, b) where a != b returns nil.
    - Test (TestRejectLastAdminDemote_BlocksLastAdmin): seed 1 admin → RejectLastAdminDemote(tx, that-admin-id, 'viewer') returns ErrLastAdmin.
    - Test (TestRejectLastAdminDemote_AllowsWhenMultipleAdmins): seed 2 admins → demoting either returns nil.
    - Test (TestRejectLastAdminDemote_PromoteToAdminAllowed): newRole='admin' → returns nil always (only demote-from-admin is the risk).
    - Test (TestRejectLastAdminDemote_TOCTOU_SerializableTx): two parallel demotes hitting last-admin via SERIALIZABLE tx → exactly one succeeds, the other rolls back with 40001 serialization error.
    - Test (TestIterateAndRevoke_Exported): `auth.IterateAndRevoke` (capitalized) is callable from another package.
  </behavior>
  <action>
    **internal/auth/account.go:** Add an exported alias `func IterateAndRevoke(ctx context.Context, sm *scs.SessionManager, store *Store, userID, keepToken string) error { return iterateAndRevoke(ctx, sm, store, userID, keepToken) }`. The existing unexported function stays for backward compat with account.go callers.

    **internal/auth/users.go:** Append the following methods to `Store` (mirror existing pgxpool raw-SQL pattern; no sqlc):

    ```go
    // ListScope is "active" | "disabled" | "all".
    func (s *Store) List(ctx context.Context, scope string) ([]UserRecord, error) {
        var where string
        switch scope {
        case "active":   where = "WHERE disabled_at IS NULL"
        case "disabled": where = "WHERE disabled_at IS NOT NULL"
        case "all":      where = ""
        default: return nil, fmt.Errorf("auth: invalid List scope %q", scope)
        }
        rows, err := s.pool.Query(ctx,
            `SELECT id::text, email, name, password_hash, role::text, must_change_password,
                    disabled_at, created_at, updated_at, last_login_at
             FROM "user" `+where+` ORDER BY created_at DESC`)
        // ... scan into []UserRecord
    }

    func (s *Store) CreateTx(ctx context.Context, tx pgx.Tx, email, name, role, passwordHash string, mustChange bool) (string, error)
    func (s *Store) UpdateTx(ctx context.Context, tx pgx.Tx, id, name string) error
    func (s *Store) DisableTx(ctx context.Context, tx pgx.Tx, id string) error
    func (s *Store) EnableTx(ctx context.Context, tx pgx.Tx, id string) error
    func (s *Store) ChangeRoleTx(ctx context.Context, tx pgx.Tx, id, newRole string) error
    func (s *Store) UpdatePasswordTx(ctx context.Context, tx pgx.Tx, id, passwordHash string, mustChange bool) error
    func (s *Store) CountActiveAdminsExcluding(ctx context.Context, tx pgx.Tx, excludeID string) (int, error)
    func (s *Store) GetByIDForUpdate(ctx context.Context, tx pgx.Tx, id string) (*UserRecord, error) // SELECT ... FOR UPDATE for serializable last-admin
    ```

    Extend `UserRecord` struct to include `DisabledAt *time.Time`, `CreatedAt time.Time`, `UpdatedAt time.Time`, `LastLoginAt *time.Time` (the `last_login_at` column does not exist on the Phase 1 `"user"` table — verified at planning time: `grep -l "last_login_at" internal/db/migrations/*.sql` returns no matches as of 2026-05-12).

    **Migration coordination note:** Phase 6 plans claim migration numbers as follows to prevent collisions:
    - Plan 06-01: 0037 (audit vocab), 0038 (alert_rule), 0039 (alert), 0040 (retention_config_phase6), 0042 (alert_worker_state), 0043 (admin_prune_audit_rows) — 0041 reserved/unused.
    - Plan 06-05 (THIS PLAN): **0044** (user_last_login)
    - Plan 06-08: 0045 (backup_run)
    - Plan 06-10: 0046 (backup_thresholds)
    - Plan 06-11: 0047 (audit_vocab_alert_prune)

    Create migration `0044_user_last_login.up.sql` **unconditionally** (do NOT branch on Wave 0 discovery — the column is provably absent at plan time):
    ```sql
    ALTER TABLE "user" ADD COLUMN last_login_at TIMESTAMPTZ;
    -- Backfill best-effort: set last_login_at = updated_at for existing rows so
    -- the Users table renders "Last login" reasonably for pre-Phase 6 users.
    UPDATE "user" SET last_login_at = updated_at WHERE last_login_at IS NULL;
    ```
    Down migration: `ALTER TABLE "user" DROP COLUMN last_login_at;`
    The Plan 06-06 auth-retrofit will set `last_login_at = now()` on every successful login via `UpdateLastLoginAtTx`.

    **internal/user/doc.go:** Package doc explaining the package as a thin wrapper around auth.Store + iterateAndRevoke + guards, with the four D-24 trigger callers (LogoutEverywhere, Disable, ChangeRole, ResetPassword).

    **internal/user/password.go** (RESEARCH §Decision G):
    ```go
    package user

    import (
        "crypto/rand"
        "errors"
        "github.com/shifter-io/shifter/internal/auth"
    )

    // passwordAlphabet excludes ambiguous chars 1lI0O per CONTEXT.md Claude's Discretion + D-23.
    // 62 chars; with length 16 entropy ≈ 95 bits >> required ≥ 64 bit minimum.
    const passwordAlphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!@#$%^&*"
    const passwordLen = 16
    const generateMaxRetries = 10

    var ErrPasswordGenFailed = errors.New("user: failed to produce strong password after retries")

    // GenerateRandomPassword generates a random ≥16-char password using crypto/rand
    // and the explicit alphabet (no ambiguous chars). Re-rolls up to 10 times if
    // the Phase 1 strength evaluator (D-28) scores it below MinScore — should
    // basically never trigger at length 16 from this alphabet but the guard makes
    // it safe under future alphabet edits.
    func GenerateRandomPassword() (string, error) {
        for tries := 0; tries < generateMaxRetries; tries++ {
            b := make([]byte, passwordLen)
            if _, err := rand.Read(b); err != nil { return "", err }
            out := make([]byte, passwordLen)
            for i := range b {
                out[i] = passwordAlphabet[int(b[i])%len(passwordAlphabet)]
            }
            candidate := string(out)
            if auth.StrengthScore(candidate) >= auth.MinScore {
                return candidate, nil
            }
        }
        return "", ErrPasswordGenFailed
    }
    ```

    **internal/user/guards.go** (RESEARCH §Decision G Pitfall 6):
    ```go
    package user

    import (
        "context"
        "errors"
        "github.com/jackc/pgx/v5"
        "github.com/shifter-io/shifter/internal/auth"
    )

    var (
        ErrSelfAction = errors.New("user: cannot perform this action on yourself")
        ErrLastAdmin  = errors.New("user: cannot leave zero admins")
    )

    // RejectSelfAction blocks self-disable, self-role-change (D-26 server side).
    func RejectSelfAction(actingUserID, targetUserID string) error {
        if actingUserID == targetUserID { return ErrSelfAction }
        return nil
    }

    // RejectLastAdminDemote is called INSIDE a SERIALIZABLE tx (Pitfall 6
    // TOCTOU mitigation). Loads count of OTHER admins (excluding target);
    // if zero, returns ErrLastAdmin. newRole 'admin' is always allowed.
    func RejectLastAdminDemote(ctx context.Context, tx pgx.Tx, store *auth.Store, targetUserID, newRole string) error {
        if newRole == "admin" { return nil }
        n, err := store.CountActiveAdminsExcluding(ctx, tx, targetUserID)
        if err != nil { return err }
        if n == 0 { return ErrLastAdmin }
        return nil
    }
    ```

    **internal/user/store.go** is the package wrapper exposing exactly the surface internal/user/handler.go needs (re-exports of auth.Store methods + thin orchestration combining store + audit.WriteEntry inside transactions). See Task 2 for handler-tx orchestration. Store.go in this package mainly provides typed `UserDTO` for the JSON wire response (without password_hash exposed):
    ```go
    type UserDTO struct {
        ID                 string     `json:"id"`
        Email              string     `json:"email"`
        Name               string     `json:"name"`
        Role               string     `json:"role"`
        MustChangePassword bool       `json:"must_change_password"`
        DisabledAt         *time.Time `json:"disabled_at"`
        LastLoginAt        *time.Time `json:"last_login_at"`
        CreatedAt          time.Time  `json:"created_at"`
        UpdatedAt          time.Time  `json:"updated_at"`
    }
    func ToDTO(u *auth.UserRecord) UserDTO { ... } // copies fields, OMITS PasswordHash
    ```
  </action>
  <verify>
    <automated>go test ./internal/auth/... ./internal/user/... -run "TestStore_|TestGenerateRandomPassword|TestRejectSelfAction|TestRejectLastAdminDemote|TestIterateAndRevoke_Exported" -count=1 -timeout=60s</automated>
  </verify>
  <acceptance_criteria>
    - `internal/auth/users.go` exports: `List`, `CreateTx`, `UpdateTx`, `DisableTx`, `EnableTx`, `ChangeRoleTx`, `UpdatePasswordTx`, `CountActiveAdminsExcluding`, `GetByIDForUpdate` (grep each)
    - `internal/auth/account.go` exports `IterateAndRevoke` (capitalized): `grep -c "^func IterateAndRevoke" internal/auth/account.go` returns 1
    - `internal/user/password.go` contains `const passwordAlphabet = ` and the alphabet string excludes `1`, `l`, `I`, `0`, `O`: grep -E "passwordAlphabet = \"[^1lI0O]*\"$" matches
    - `internal/user/password.go` contains `const passwordLen = 16`
    - `internal/user/guards.go` contains both `RejectSelfAction` and `RejectLastAdminDemote` functions
    - `RejectLastAdminDemote` accepts `tx pgx.Tx` parameter (TOCTOU mitigation: callers wrap in SERIALIZABLE tx)
    - `internal/user/store.go` defines `UserDTO` struct without a `PasswordHash` field: `grep -c "PasswordHash" internal/user/store.go` returns 0
    - Migration `internal/db/migrations/0044_user_last_login.up.sql` exists unconditionally with `ADD COLUMN last_login_at TIMESTAMPTZ` AND a matching down migration `0044_user_last_login.down.sql` with `DROP COLUMN last_login_at`
    - All 12 listed tests pass: `go test ./internal/auth/... ./internal/user/... -count=1` exits 0
  </acceptance_criteria>
  <done>The user store is extended; the random password generator passes the same strength bar as user-typed passwords (D-28); the two guards are server-side enforceable; TOCTOU is mitigated via SERIALIZABLE tx + SELECT FOR UPDATE.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: HTTP handlers + authz + router wiring (USER-01..04)</name>
  <files>internal/user/handler.go, internal/user/handler_test.go, internal/auth/authz.go, internal/auth/authz_test.go, internal/http/router.go</files>
  <read_first>
    - internal/auth/authz.go (current ActionUserManage; add fine-grained children)
    - internal/auth/account.go (iterateAndRevoke pattern; D-24 callers in this plan all funnel through IterateAndRevoke)
    - internal/audit/log.go (Phase 6 constants from Plan 06-01)
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md §D-24 (four callers of session-revoke)
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Decision G "Pitfall 5: Self-action guards forgotten on server side"
  </read_first>
  <behavior>
    - Test (TestAuthz_UserActionsAdminOnly): RoleAdmin has all 5 user-mgmt actions true; RoleViewer has only ActionUserReadSelf true (viewer can see their own row only). 403 for any viewer mutation.
    - Test (TestListUsers_AdminOnly): GET /api/users → 200 returns list; viewer 403.
    - Test (TestListUsers_FilterByScope): GET /api/users?scope=disabled returns only disabled users; GET /api/users?scope=active returns active.
    - Test (TestCreateUser_GeneratesRandomPasswordAndReturnsOnce): POST /api/users {email, name, role} → 201 response body contains `{user: UserDTO, initial_password: "..."}` (the only time the plaintext is in the response); database user has password_hash set, must_change_password=true; audit row 'user.create' present.
    - Test (TestCreateUser_StrengthEvaluatorMustPass): the generated password score >= auth.MinScore (asserted by re-running StrengthScore on the response field — guards against future alphabet shrinkage).
    - Test (TestCreateUser_RejectsDuplicateEmail): second POST with same email returns 409 + "That email is already in use" error code.
    - Test (TestUpdateUser_NameOnly): PATCH /api/users/{id} {name:"New"} → 200; role and password unchanged; audit row 'user.update' with Before/After diff containing only the name field.
    - Test (TestChangeRole_RevokesSessions): PATCH /api/users/{id}/role {role:"viewer"} → role changed AND iterateAndRevoke called on user's sessions AND audit row 'user.role_change' + 'auth.session_revoked' both written in same tx.
    - Test (TestChangeRole_RejectsSelf): admin tries PATCH role on their own id → 422 ErrSelfAction.
    - Test (TestChangeRole_RejectsLastAdminDemote): single-admin install tries to demote that admin → 422 ErrLastAdmin.
    - Test (TestDisable_RevokesSessions): POST /api/users/{id}/disable → disabled_at set, sessions revoked, audit row 'user.disable'.
    - Test (TestDisable_RejectsSelf): admin disables themselves → 422.
    - Test (TestEnable_PreservesCredentials): POST /api/users/{id}/enable on previously-disabled user → disabled_at=NULL; password_hash and must_change_password unchanged; audit row 'user.enable'. (D-27)
    - Test (TestResetPassword_GeneratesAndRevokes): POST /api/users/{id}/reset-password → 200 with initial_password in body; user row has new hash, must_change_password=true; sessions revoked; audit row 'auth.password_reset_by_admin' + 'auth.session_revoked'.
    - Test (TestLogoutEverywhere_DeletesSessions): POST /api/users/{id}/logout-everywhere → all SCS sessions for user_id deleted; audit row 'auth.session_revoked'.
    - Test (TestLogoutEverywhere_RejectsSelf): admin logs themselves out everywhere → 422.
    - Test (TestRBAC_ViewerCannotCallAnyMutation): viewer POST /api/users (or PATCH or DELETE) → 403 for every endpoint listed above.
  </behavior>
  <action>
    **internal/auth/authz.go:** Add fine-grained const actions:
    ```go
    // Phase 6 — Plan 06-05 user management:
    const (
        ActionUserList         Action = "user.list"
        ActionUserCreate       Action = "user.create"
        ActionUserUpdate       Action = "user.update"
        ActionUserDisable      Action = "user.disable"
        ActionUserEnable       Action = "user.enable"
        ActionUserChangeRole   Action = "user.change_role"
        ActionUserResetPassword Action = "user.reset_password"
        ActionUserLogoutEverywhere Action = "user.logout_everywhere"
        ActionUserReadSelf     Action = "user.read_self"
    )
    ```
    Add all to `roleBundles[RoleAdmin]: true`. Add ONLY `ActionUserReadSelf: true` to `roleBundles[RoleViewer]`. The existing `ActionUserManage` umbrella stays for backward compat (treat as admin-only).

    **internal/user/handler.go:** orchestration pattern for each mutation — open SERIALIZABLE tx → run guards inside tx → execute Store method → write audit in tx → commit → if applicable, call IterateAndRevoke after commit. Example for ChangeRoleHandler:

    ```go
    type Deps struct {
        Pool       *pgxpool.Pool
        Store      *auth.Store
        SessionMgr *scs.SessionManager
    }

    func ChangeRoleHandler(deps Deps) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            actingUser, _ := auth.UserFromContext(r.Context())
            targetID := chi.URLParam(r, "id")
            var req struct{ Role string `json:"role"` }
            if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, 400, "bad_json"); return }
            if req.Role != "admin" && req.Role != "viewer" { writeError(w, 422, "invalid_role"); return }
            // D-26 server-side guards
            if err := user.RejectSelfAction(actingUser.ID, targetID); err != nil {
                writeError(w, 422, "self_action_forbidden"); return
            }
            // Open SERIALIZABLE tx for Pitfall 6 TOCTOU mitigation
            tx, err := deps.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.Serializable})
            if err != nil { writeError(w, 500, "db_begin"); return }
            defer tx.Rollback(r.Context())
            // Lock target row for the duration of the tx
            before, err := deps.Store.GetByIDForUpdate(r.Context(), tx, targetID)
            if err != nil { writeError(w, 404, "user_not_found"); return }
            if err := user.RejectLastAdminDemote(r.Context(), tx, deps.Store, targetID, req.Role); err != nil {
                if errors.Is(err, user.ErrLastAdmin) { writeError(w, 422, "last_admin"); return }
                writeError(w, 500, "guard_err"); return
            }
            if err := deps.Store.ChangeRoleTx(r.Context(), tx, targetID, req.Role); err != nil {
                writeError(w, 500, "db_update"); return
            }
            // Audit rows
            if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
                UserID:     mustParseUUID(actingUser.ID),
                Action:     audit.ActionUserRoleChange,
                EntityType: audit.EntityTypeUser,
                EntityID:   mustParseUUID(targetID),
                Before:     map[string]any{"role": before.Role},
                After:      map[string]any{"role": req.Role},
                RequestID:  middleware.GetReqID(r.Context()),
            }); err != nil { writeError(w, 500, "audit"); return }
            if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
                UserID:     mustParseUUID(actingUser.ID),
                Action:     audit.ActionAuthSessionRevoked,
                EntityType: audit.EntityTypeSession,
                EntityID:   mustParseUUID(targetID), // entity_id = the user whose sessions were revoked
                Notes:      "implicit revoke on role_change",
                RequestID:  middleware.GetReqID(r.Context()),
            }); err != nil { writeError(w, 500, "audit"); return }
            if err := tx.Commit(r.Context()); err != nil {
                if isSerializationError(err) { writeError(w, 409, "serialization_retry"); return }
                writeError(w, 500, "db_commit"); return
            }
            // AFTER commit: revoke sessions (the SCS sessions table is separate)
            if err := auth.IterateAndRevoke(r.Context(), deps.SessionMgr, deps.Store, targetID, ""); err != nil {
                deps.Log.Warn("session_revoke_after_role_change_failed", "user_id", targetID, "err", err)
                // Continue — role change succeeded; sessions will expire naturally
            }
            writeJSON(w, 200, map[string]any{"id": targetID, "role": req.Role})
        }
    }
    ```

    **All 8 handlers follow the same pattern.** CreateHandler:
    1. Generate random password via `user.GenerateRandomPassword()`
    2. Hash via `auth.Hash(password)`
    3. SERIALIZABLE tx → CreateTx → audit `user.create` → commit
    4. Return body `{user: ToDTO(record), initial_password: plaintextPassword}` — THIS IS THE ONLY TIME the plaintext crosses the wire.

    ResetPasswordHandler: generate new random + Hash → SERIALIZABLE tx → UpdatePasswordTx(must_change=true) → audit `auth.password_reset_by_admin` + `auth.session_revoked` → commit → IterateAndRevoke → return `{initial_password: plaintext}`.

    DisableHandler / EnableHandler: SERIALIZABLE tx → guard + DisableTx/EnableTx → audit → commit → IterateAndRevoke (disable only).

    LogoutEverywhereHandler: SERIALIZABLE tx → audit `auth.session_revoked` → commit → IterateAndRevoke. Note that this handler ALSO uses RejectSelfAction (admin cannot kick themselves out without going through proper logout).

    **internal/http/router.go:**
    ```go
    r.Route("/api/users", func(r chi.Router) {
        r.With(auth.RequireAction(sm, auth.ActionUserList)).Get("/", user.ListHandler(deps))
        r.With(auth.RequireAction(sm, auth.ActionUserCreate)).Post("/", user.CreateHandler(deps))
        r.With(auth.RequireAction(sm, auth.ActionUserUpdate)).Patch("/{id}", user.UpdateHandler(deps))
        r.With(auth.RequireAction(sm, auth.ActionUserDisable)).Post("/{id}/disable", user.DisableHandler(deps))
        r.With(auth.RequireAction(sm, auth.ActionUserEnable)).Post("/{id}/enable", user.EnableHandler(deps))
        r.With(auth.RequireAction(sm, auth.ActionUserChangeRole)).Patch("/{id}/role", user.ChangeRoleHandler(deps))
        r.With(auth.RequireAction(sm, auth.ActionUserResetPassword)).Post("/{id}/reset-password", user.ResetPasswordHandler(deps))
        r.With(auth.RequireAction(sm, auth.ActionUserLogoutEverywhere)).Post("/{id}/logout-everywhere", user.LogoutEverywhereHandler(deps))
    })
    ```

    Tests use the existing testcontainers pattern. `TestCreateUser_StrengthEvaluatorMustPass` re-evaluates the response's initial_password through `auth.StrengthScore`.
  </action>
  <verify>
    <automated>go test ./internal/user/... ./internal/auth/... -run "TestAuthz_UserActions|TestListUsers|TestCreateUser|TestUpdateUser|TestChangeRole|TestDisable|TestEnable|TestResetPassword|TestLogoutEverywhere|TestRBAC_Viewer" -count=1 -timeout=120s</automated>
  </verify>
  <acceptance_criteria>
    - `internal/auth/authz.go` declares all 9 new user-mgmt actions: grep -c "ActionUserList\|ActionUserCreate\|ActionUserUpdate\|ActionUserDisable\|ActionUserEnable\|ActionUserChangeRole\|ActionUserResetPassword\|ActionUserLogoutEverywhere\|ActionUserReadSelf" returns 9
    - `roleBundles[RoleViewer]` map has `ActionUserReadSelf: true` and does NOT have any other user-mgmt action
    - `internal/user/handler.go` contains 8 handler functions: `ListHandler`, `CreateHandler`, `UpdateHandler`, `DisableHandler`, `EnableHandler`, `ChangeRoleHandler`, `ResetPasswordHandler`, `LogoutEverywhereHandler`
    - Every mutating handler uses `pgx.TxOptions{IsoLevel: pgx.Serializable}`: grep -c "Serializable" internal/user/handler.go >= 6
    - Every handler that may reject self-action calls `user.RejectSelfAction(`: grep -c "RejectSelfAction" returns >= 4 (Disable, ChangeRole, LogoutEverywhere, ResetPassword)
    - Every "kick out" handler calls `auth.IterateAndRevoke`: grep -c "IterateAndRevoke" internal/user/handler.go returns >= 4
    - `internal/http/router.go` declares the 8 routes with correct RequireAction guards
    - All 16 listed tests in handler_test.go pass: `go test ./internal/user/... -count=1` exits 0
  </acceptance_criteria>
  <done>Every USER-* requirement covered server-side with audit-in-tx + SERIALIZABLE TOCTOU defenses + viewer-cannot-mutate RBAC + self-action server guards.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 3: Settings → Users tab + Add User / Edit User / Reset / Logout-Everywhere / Disable / Re-enable / Role-change dialogs + E2E</name>
  <files>web/src/routes/settings/users.tsx, web/src/routes/settings/users.test.tsx, web/src/routes/settings/AddUserDialog.tsx, web/src/routes/settings/EditUserDialog.tsx, web/src/routes/settings/ShareCredentialsPanel.tsx, web/src/routes/settings/ResetPasswordDialog.tsx, web/src/routes/settings/LogoutEverywhereDialog.tsx, web/src/routes/settings/DisableUserDialog.tsx, web/src/routes/settings/ReEnableUserDialog.tsx, web/src/routes/settings/RoleChangeDialog.tsx, web/src/hooks/useUsers.ts, web/playwright/specs/user-management.spec.ts, web/src/App.tsx</files>
  <read_first>
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-UI-SPEC.md §Surface 5 (complete row action / dialog spec)
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-UI-SPEC.md §Destructive Confirmations (verbatim copy table)
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-UI-SPEC.md §Toast Notifications (verbatim copy)
    - web/src/components/responsive-dialog.tsx (Add User 2-step flow uses this)
    - web/src/components/ui/alert-dialog.tsx (destructive confirms)
    - web/src/lib/use-current-user.ts (for D-26 client-side grey-out + last-admin detection)
  </read_first>
  <behavior>
    - Test (UsersTable_RendersActiveByDefault): mock GET /api/users?scope=active → renders 3 rows in created_at DESC order.
    - Test (UsersTable_ShowDisabledToggle): toggle on → URL ?show_disabled=1 + refetch with scope=disabled.
    - Test (UsersTable_CurrentUserPinnedTop): current user row appears first AND tinted bg-primary/5.
    - Test (UsersTable_LastAdminRowGreysOut): single-admin install → that row's Disable + role-radio in Edit are disabled with tooltip "At least one admin must remain."
    - Test (UsersTable_CurrentUserRowGreysOut): "Disable" and "Sign out everywhere" hidden on `row.id === currentUser.id` row.
    - Test (AddUserDialog_Step1Submit): fill {name, email, role:'viewer'}, submit → POST /api/users; response includes initial_password; dialog flips to Step 2 ShareCredentialsPanel.
    - Test (ShareCredentialsPanel_RendersOnce): password in monospace block; Copy button calls navigator.clipboard.writeText AND fires sonner toast "Password copied"; Email row separately Copyable.
    - Test (ShareCredentialsPanel_IShareedThisClearsCache): clicking "I've shared this" calls `queryClient.removeQueries({queryKey:['users']})` AND closes dialog; subsequent dialog reopen does NOT show the panel.
    - Test (EditUserDialog_RoleChangeOpensConfirm): change role from admin to viewer + save → opens RoleChangeDialog AlertDialog with copy "Change role for {name}?" + body "Changing the role will sign {name} out everywhere. They'll see the new role the next time they sign in." + destructive button "Change role".
    - Test (EditUserDialog_RoleRadioDisabledForSelf): editing current user → role radio is disabled with helper "You can't change your own role."
    - Test (EditUserDialog_RoleRadioDisabledForLastAdmin): editing last admin → role radio disabled with helper "At least one admin must remain."
    - Test (DisableUserDialog_VerbatimCopy): AlertDialog heading "Disable {name}?" body "The user will be signed out and unable to sign in. You can re-enable them later." destructive button "Disable user".
    - Test (ReEnableUserDialog_DefaultVariantConfirm): button variant="default" (not destructive); copy "Re-enable {name}?" body about existing password.
    - Test (LogoutEverywhereDialog_DestructiveCopy): heading "Sign {name} out everywhere?" body "All active sessions for this user will end immediately. They can sign back in normally." destructive button "Sign user out everywhere".
    - Test (ResetPasswordDialog_TwoStepShowOnce): confirm step → API call → step 2 shows new plaintext password identical to AddUser ShareCredentialsPanel.
    - Test (ViewerCannot403): visiting /settings/users as viewer → redirect to / (UI-SPEC Open Question #2 recommendation).
    - Playwright (user-management.spec.ts): admin logs in → /settings/users → Add user "Ben" with viewer role → share-credentials panel appears → copy password → "I've shared this" → logout admin → log in as Ben with the copied password → forced to change password on first login → set new password → can sign in.
  </behavior>
  <action>
    **web/src/hooks/useUsers.ts:**
    ```ts
    export function useUsersList(scope: 'active'|'disabled'|'all') { return useQuery({queryKey:['users',scope], queryFn: () => apiGet(`/api/users?scope=${scope}`)}) }
    export function useCreateUserMutation() { return useMutation({ mutationFn: ..., onSuccess: ... }) }
    export function useUpdateUserMutation() { ... }
    export function useChangeRoleMutation() { ... }
    export function useDisableUserMutation() { ... }
    export function useEnableUserMutation() { ... }
    export function useResetPasswordMutation() { ... }
    export function useLogoutEverywhereMutation() { ... }
    // Helper:
    export function useLastAdminLookup(): (userId: string) => boolean {
      // returns true if userId is the only active admin; UI uses this to grey out actions
    }
    ```

    **web/src/routes/settings/users.tsx:** layout per UI-SPEC §Surface 5:
    - Card titled "Users (N)" + right-aligned "+ Add user" button (admin-only)
    - "Show disabled" Toggle (URL-state ?show_disabled=0|1)
    - TanStack Table columns: Name (prefixed "You" for self), Email, Role (Badge variant per UI-SPEC §Role badges), Last login (relative + tooltip absolute), Status (only in disabled tab — outline Badge "Disabled at {date}"), Actions (DropdownMenu)
    - Row tinting: bg-primary/5 for current user; bg-secondary/40 for last admin
    - Mobile (<md): card-per-row collapsing
    - Empty state per UI-SPEC

    **AddUserDialog.tsx** (ResponsiveDialog, 2 steps in same dialog):
    - State: `{credentials: null | {email, password}}`
    - Step 1: react-hook-form with zod schema `{ name: required, email: z.string().email(), role: z.enum(['admin','viewer']).default('viewer') }`; on submit calls useCreateUserMutation; on success setCredentials to response data
    - Step 2: `<ShareCredentialsPanel email={...} password={...} onConfirm={...} />`
    - Submit button: "Create user" (step 1) / "I've shared this" (step 2)

    **ShareCredentialsPanel.tsx** (UI-SPEC §Add User dialog — step 2 verbatim):
    ```tsx
    export function ShareCredentialsPanel({ email, password, onConfirm }: Props) {
      return (
        <>
          <DialogHeader>
            <DialogTitle>User created</DialogTitle>
            <DialogDescription>Share these credentials with the user. We won't show this password again.</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <CopyRow label="Email" value={email} toastLabel="Email copied." />
            <CopyRow label="Password" value={password} mono toastLabel="Password copied." />
            <Alert variant="warning"><AlertTriangle className="h-4 w-4"/>This password is shown once...</Alert>
          </div>
          <DialogFooter><Button onClick={() => { queryClient.removeQueries({queryKey:['users']}); onConfirm(); }}>I've shared this</Button></DialogFooter>
        </>
      );
    }
    ```

    **EditUserDialog.tsx**: ResponsiveDialog with name + role form; Save triggers ChangeRole confirm AlertDialog if role changed.

    **RoleChangeDialog.tsx**, **DisableUserDialog.tsx**, **ReEnableUserDialog.tsx**, **LogoutEverywhereDialog.tsx**: shadcn AlertDialog with verbatim copy from UI-SPEC §Destructive Confirmations.

    **ResetPasswordDialog.tsx**: AlertDialog confirm → on confirm calls useResetPasswordMutation → flips to ResponsiveDialog showing step-2-shaped panel with new plaintext.

    All sonner toast strings verbatim from UI-SPEC §Toast Notifications: "User created. Share the credentials before closing this window.", "User updated.", "User disabled. Their sessions have been ended.", "User re-enabled.", "Password reset. Share the new credentials with the user.", "Role changed. The user has been signed out and must sign back in.", "{N} sessions ended."

    **web/src/App.tsx:** Add route `<Route path="/settings/users" element={<UsersPage />} />`. Settings sidebar already lists "Users" sub-tab per Plan 06-04.

    **Viewer access:** UsersPage top-level guards via `useCurrentUser()` — if role !== 'admin' return `<Navigate to="/" replace />`.

    **Playwright user-management.spec.ts:**
    ```ts
    test('admin adds viewer; viewer logs in + force-rotates', async ({ page }) => {
      await loginAsAdmin(page);
      await page.goto('/settings/users');
      await page.click('text=Add user');
      await page.fill('[name=name]', 'Ben Smith');
      await page.fill('[name=email]', 'ben@acme.io');
      await page.click('text=Viewer');
      await page.click('text=Create user');
      // Step 2: capture password
      const password = await page.locator('[data-testid=password-block]').innerText();
      await page.click('text=I\'ve shared this');
      await logout(page);
      // Log in as Ben
      await page.goto('/login');
      await page.fill('[name=email]', 'ben@acme.io');
      await page.fill('[name=password]', password);
      await page.click('text=Sign in');
      // Force-change dialog
      await expect(page.locator('text=Change your password')).toBeVisible();
      await page.fill('[name=new_password]', 'NewStrongPass1234!');
      await page.fill('[name=confirm], [name=new_password_confirm]', 'NewStrongPass1234!');
      await page.click('text=Save');
      await expect(page).toHaveURL('/');
    });
    ```

    Note: Plan 06-06 (auth-event retrofit) is what makes login_success audit fire; this E2E should work even before 06-06 if must_change_password flag is honored by the existing Phase 1 force-change flow.
  </action>
  <verify>
    <automated>pnpm -C web test --run web/src/routes/settings/users && pnpm -C web exec playwright test user-management    <automated>pnpm -C web test --run web/src/routes/settings/users && pnpm -C web exec playwright test user-management</automated>
  </verify>
  <acceptance_criteria>
    - `web/src/routes/settings/users.tsx` exists and uses TanStack Table with the 6 columns from UI-SPEC
    - `web/src/routes/settings/AddUserDialog.tsx` has a 2-step state (`credentials` state variable) and renders `ShareCredentialsPanel` only after successful create
    - `ShareCredentialsPanel.tsx` calls `queryClient.removeQueries({queryKey:['users']})` in the "I've shared this" handler (cache purge)
    - Every destructive AlertDialog has the exact UI-SPEC body text: grep each verbatim string in the corresponding .tsx file
    - `web/src/routes/settings/users.tsx` includes `<Navigate to="/" replace />` for non-admin role redirect (viewer-cannot-access)
    - Last-admin detection: `useLastAdminLookup` returns true for the only-admin row → row's "Disable" and Edit dialog's role radio disabled (grep for `last_admin` or `lastAdmin` boolean + disabled prop)
    - Current user self-row: "Sign out everywhere" + "Disable" hidden, role radio disabled with helper "You can't change your own role."
    - All listed component tests pass: `pnpm -C web test --run web/src/routes/settings/users` exits 0
    - Playwright spec passes: `pnpm -C web exec playwright test user-management` exits 0
  </acceptance_criteria>
  <done>Operator can manage the user fleet end-to-end: add (random-show-once), edit, disable (preserves audit), re-enable (preserves credentials), reset password, sign out everywhere, change role (with auto-revoke). Server enforces every guard; UI surfaces every guard.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| client→/api/users/* | Authenticated admin only for mutations; viewer access blocked at route + RBAC |
| password generator → response wire | Plaintext password crosses the wire EXACTLY ONCE (in CreateUser + ResetPassword responses); never stored anywhere except the user's password_hash via Argon2id |
| SCS session pool → IterateAndRevoke | Bulk DELETE of session rows by user_id; only callable by admin handlers |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-06-05-01 | Elevation of Privilege | viewer creates an admin user via curl | mitigate | RequireAction(ActionUserCreate) — not in roleBundles[RoleViewer]. Test: TestRBAC_ViewerCannotCallAnyMutation. |
| T-06-05-02 | Repudiation | password reset without audit trail | mitigate | ResetPasswordHandler writes 'auth.password_reset_by_admin' + 'auth.session_revoked' audit rows in the SAME tx as the password update. Test: TestResetPassword_GeneratesAndRevokes asserts both audit rows present. |
| T-06-05-03 | Tampering | last-admin demote TOCTOU race (two parallel demotes) | mitigate | SERIALIZABLE tx (pgx.TxOptions{IsoLevel: pgx.Serializable}) + GetByIDForUpdate (SELECT ... FOR UPDATE) inside the tx; CountActiveAdminsExcluding inside the same tx. One tx serialization-errors on commit; the other succeeds. Test: TestRejectLastAdminDemote_TOCTOU_SerializableTx. |
| T-06-05-04 | Spoofing | admin disables themselves and gets locked out | mitigate | D-26 server-side `RejectSelfAction` rejects with 422 BEFORE any DB write. Lockout recovery via `shifter create-admin --reset` CLI escape hatch (Phase 1 D-14, unchanged). |
| T-06-05-05 | Information Disclosure | plaintext password leaks via server log | mitigate | The plaintext is in handler-local variables ONLY; never logged (slog calls in this plan use only IDs/emails, never the password). The Argon2id Hash function is the only consumer; the response body is the only sink. |
| T-06-05-06 | Information Disclosure | plaintext password stays in React Query cache | mitigate | ShareCredentialsPanel's "I've shared this" handler calls `queryClient.removeQueries({queryKey:['users']})` — purges the create-user response (which contained initial_password) from cache. Test: ShareCredentialsPanel_IShareedThisClearsCache. |
| T-06-05-07 | Spoofing | session-fixation after role change | mitigate | D-25: role change atomically triggers IterateAndRevoke; the user MUST re-auth → SCS issues new session token; previous tokens are gone from the sessions table. Test: TestChangeRole_RevokesSessions. |
| T-06-05-08 | DoS | random password too weak (entropy collapse) | mitigate | D-23: alphabet has 62 chars; length 16 → ~95 bits entropy. Guard: re-roll up to 10 times if Phase 1 StrengthScore < MinScore. Test: TestGenerateRandomPassword_Strong. |
| T-06-05-09 | Tampering | UI bypass of self-edit grey-out via crafted HTTP | mitigate | Server-side guards (`RejectSelfAction`, `RejectLastAdminDemote`) run FIRST in every handler. UI grey-out is convenience; server is the lock. Test: TestChangeRole_RejectsSelf, TestDisable_RejectsSelf. |
| T-06-05-10 | Information Disclosure | password_hash visible in /api/users JSON | mitigate | UserDTO struct OMITS PasswordHash; ToDTO(record) explicitly does not copy it. Test: grep -c "PasswordHash" internal/user/store.go returns 0 (verified in Task 1 acceptance criteria). |
</threat_model>

<verification>
- All 4 USER-* requirements have shipping code + tests + RBAC + audit-in-tx
- The 4 D-24 triggers all funnel through `auth.IterateAndRevoke` (single code path)
- D-23 random password meets D-28 strength evaluator
- D-26 server-side guards enforced regardless of UI
- D-27 re-enable preserves original credentials
- `go test ./internal/auth/... ./internal/user/... -count=1 -timeout=120s` passes
- `pnpm -C web test --run web/src/routes/settings/users -- --reporter=verbose` passes
- `pnpm -C web exec playwright test user-management` passes
</verification>

<success_criteria>
- USER-01: admin can list/create/edit/disable users via dialogs, no hard-delete
- USER-02: admin assigns/changes user role; change triggers logout-everywhere
- USER-03: admin revokes all sessions for a user; same code path on disable/role-change/reset
- USER-04: admin sets initial password inline (random+show-once); user must change on first login
- D-23, D-24, D-25, D-26, D-27, D-28, D-29 all implemented
</success_criteria>

<output>
After completion, create `.planning/phases/06-alerts-users-audit-operational-hardening/06-05-SUMMARY.md`
</output>
