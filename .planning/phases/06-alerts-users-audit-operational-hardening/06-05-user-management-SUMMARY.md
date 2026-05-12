---
phase: 06-alerts-users-audit-operational-hardening
plan: 05
subsystem: user-management
tags: [phase-6, users, rbac, session-revoke, share-once-password, audit-in-tx, toctou-mitigation]
requires:
  - phase-1-foundation
  - phase-6-plan-01-alert-engine-substrate
provides:
  - migration: 0044_user_last_login (adds user.last_login_at TIMESTAMPTZ)
  - package: internal/user (store DTO + password generator + guards + 8 HTTP handlers)
  - rest-surface: /api/users (8 endpoints, all admin-only)
  - export: auth.IterateAndRevoke (exported alias for cross-package D-24 session-revoke)
  - authz: 9 fine-grained ActionUser* constants (split from Phase 1 umbrella)
  - frontend: /settings/users route + 8 dialogs + ShareCredentialsPanel (D-23 show-once)
affects:
  - internal/auth/users.go (extends Store with 9 new methods + 4 new UserRecord fields)
  - internal/auth/account.go (exports IterateAndRevoke)
  - internal/auth/authz.go (9 new actions; viewer can read self only)
  - internal/auth/authz_test.go (2 new test functions)
  - internal/cli/serve.go (wires user.Deps into production router)
  - internal/http/router.go (mounts user.RegisterRoutes)
  - internal/db/migrations_test.go (5 step counts updated 43 → 44)
  - internal/db/roundtrip_test.go (version 43 → 44)
  - web/src/App.tsx (lazy UsersPage + /settings/users route)
  - web/src/routes/settings.tsx (Users card with Open link, admin-only)
tech-stack:
  added: []
  patterns:
    - "SERIALIZABLE pgx tx + GetByIDForUpdate row lock for Pitfall 6 TOCTOU mitigation on last-admin demote"
    - "Plaintext password lives only in the response body of POST /api/users + POST /reset-password (D-23); persistent storage is Argon2id"
    - "queryClient.removeQueries({queryKey:['users']}) on share-panel confirm (T-06-05-06 cache-leak mitigation)"
    - "Server-side RejectSelfAction + RejectLastAdminDemote guards run BEFORE every mutating handler's DB write; UI grey-out is decorative"
    - "Audit row in same tx as user mutation; auth.session_revoked audit row in same tx as the four D-24 implicit triggers"
key-files:
  created:
    - internal/db/migrations/0044_user_last_login.up.sql
    - internal/db/migrations/0044_user_last_login.down.sql
    - internal/user/doc.go
    - internal/user/store.go
    - internal/user/store_test.go
    - internal/user/password.go
    - internal/user/password_test.go
    - internal/user/guards.go
    - internal/user/guards_test.go
    - internal/user/handler.go
    - internal/user/handler_test.go
    - internal/auth/users_test.go
    - web/src/components/ui/switch.tsx
    - web/src/hooks/useUsers.ts
    - web/src/routes/settings/users.tsx
    - web/src/routes/settings/users.test.tsx
    - web/src/routes/settings/AddUserDialog.tsx
    - web/src/routes/settings/EditUserDialog.tsx
    - web/src/routes/settings/ShareCredentialsPanel.tsx
    - web/src/routes/settings/RoleChangeDialog.tsx
    - web/src/routes/settings/DisableUserDialog.tsx
    - web/src/routes/settings/ReEnableUserDialog.tsx
    - web/src/routes/settings/LogoutEverywhereDialog.tsx
    - web/src/routes/settings/ResetPasswordDialog.tsx
    - web/playwright/specs/user-management.spec.ts
  modified:
    - internal/auth/users.go
    - internal/auth/account.go
    - internal/auth/authz.go
    - internal/auth/authz_test.go
    - internal/cli/serve.go
    - internal/http/router.go
    - internal/db/migrations_test.go
    - internal/db/roundtrip_test.go
    - web/src/App.tsx
    - web/src/routes/settings.tsx
decisions:
  - "Plain (non-Serializable) tx is used for the post-commit reload after Create/Update — the heavy guard work is finished and a long-lived Serializable read would unnecessarily hold a connection"
  - "ResetPasswordHandler does NOT call RejectSelfAction — admins may reset their own forgotten password from this surface; the destructive UI hides the action on the self row but server-side defense leans on the still-valid session as the authorization signal (the Phase 1 shifter create-admin --reset CLI remains the lockout escape hatch)"
  - "Migration 0044 backfills last_login_at = updated_at for legacy rows so the Users table doesn't render the bootstrap admin as 'never logged in' on day 1 of Phase 6 (Plan 06-06 will start writing real login timestamps)"
  - "ToDTO struct intentionally OMITS PasswordHash field; T-06-05-10 verified by grep returning 0 for PasswordHash in internal/user/store.go"
  - "9-action split (instead of one umbrella) lets future Phase 6 audit rows record the specific verb a request was authorized for"
metrics:
  duration: 39min
  tasks: 3
  files: 25 created + 10 modified
  completed: 2026-05-12
---

# Phase 6 Plan 05: User Management Summary

**One-liner:** Ships USER-01..04 — admin-only `/api/users` CRUD with random-show-once initial passwords (D-23), the four-callers/one-code-path session-revoke pattern via the newly-exported `auth.IterateAndRevoke` (D-24), the Settings → Users surface with TanStack Table + 8 dialogs, and Pitfall 6 TOCTOU mitigation via Serializable tx + `SELECT FOR UPDATE` on the last-admin demote race.

## What shipped

### Backend (Go)

| File | Surface |
|------|---------|
| `internal/auth/users.go` | Extended Store with `List`, `CreateTx`, `UpdateTx`, `DisableTx`, `EnableTx`, `ChangeRoleTx`, `UpdatePasswordTx`, `CountActiveAdminsExcluding`, `GetByIDForUpdate`. Extended `UserRecord` with `DisabledAt`, `CreatedAt`, `UpdatedAt`, `LastLoginAt`. New typed errors `ErrDuplicateEmail`, `ErrInvalidScope`. |
| `internal/auth/account.go` | Exported `IterateAndRevoke` — thin wrapper over the existing `iterateAndRevoke`. Lets `internal/user/handler.go` funnel all four D-24 callers (LogoutEverywhere, Disable, ChangeRole, ResetPassword) through one code path. |
| `internal/auth/authz.go` | 9 new actions: `ActionUserList`, `ActionUserCreate`, `ActionUserUpdate`, `ActionUserDisable`, `ActionUserEnable`, `ActionUserChangeRole`, `ActionUserResetPassword`, `ActionUserLogoutEverywhere`, `ActionUserReadSelf`. Admin gets all 9; viewer gets only `ActionUserReadSelf`. |
| `internal/user/doc.go` | Package overview: thin orchestration over `auth.Store` + `auth.IterateAndRevoke` + `audit.WriteEntry`. |
| `internal/user/store.go` | `UserDTO` JSON wire shape (no `PasswordHash` field — T-06-05-10), `ToDTO` / `ToDTOs`. |
| `internal/user/password.go` | `GenerateRandomPassword` — `crypto/rand` rejection-sampled over a 65-char alphabet excluding `1lI0O`. Re-rolls up to 10 times against `auth.PasswordStrength >= StrengthGood` (D-28 reuse). |
| `internal/user/guards.go` | `RejectSelfAction` (handler-level) + `RejectLastAdminDemote` (inside-Serializable-tx, Pitfall 6 TOCTOU mitigation). |
| `internal/user/handler.go` | 8 handlers + `RegisterRoutes`. Every mutation: Serializable tx → `GetByIDForUpdate` → guard chain → Store mutation → audit row(s) in tx → commit → `auth.IterateAndRevoke` (where applicable). |

### Migration

| # | Name | Purpose |
|---|------|---------|
| 0044 | `user_last_login` | Adds `user.last_login_at TIMESTAMPTZ` + best-effort backfill `last_login_at = updated_at`. Plan 06-06 (auth-event retrofit) will overwrite on every successful login. |

### Frontend (React/TS)

| File | Surface |
|------|---------|
| `web/src/hooks/useUsers.ts` | `useUsersList`, `useCreateUserMutation`, `useUpdateUserMutation`, `useChangeRoleMutation`, `useDisableUserMutation`, `useEnableUserMutation`, `useResetPasswordMutation`, `useLogoutEverywhereMutation`, `useLastAdminLookup`. |
| `web/src/components/ui/switch.tsx` | shadcn-style `Switch` over `radix-ui` Switch primitive. |
| `web/src/routes/settings/users.tsx` | Surface 5: Card + Show-disabled toggle + TanStack Table + per-row DropdownMenu. Current-user pinned top with `bg-primary/5`; last-admin row `bg-secondary/40`. Viewer `<Navigate to="/" replace />` redirect. |
| `web/src/routes/settings/AddUserDialog.tsx` | 2-step: `react-hook-form + zod` form → on success flips to `ShareCredentialsPanel`. |
| `web/src/routes/settings/ShareCredentialsPanel.tsx` | UI-SPEC verbatim copy. Copy buttons for email + password (sonner toasts). "I've shared this" handler calls `queryClient.removeQueries({queryKey:['users']})` (T-06-05-06 cache purge). |
| `web/src/routes/settings/EditUserDialog.tsx` | Name + role form. Role-radio disabled for self / last-admin with helper copy. Role change opens `RoleChangeDialog` before save. |
| `web/src/routes/settings/RoleChangeDialog.tsx` | AlertDialog with UI-SPEC verbatim "Change role for {name}?" copy + destructive "Change role" button. |
| `web/src/routes/settings/DisableUserDialog.tsx` | AlertDialog verbatim "Disable {name}?" + destructive "Disable user" button. |
| `web/src/routes/settings/ReEnableUserDialog.tsx` | AlertDialog verbatim "Re-enable {name}?" + **default-variant** "Re-enable" button (D-27 — non-destructive). |
| `web/src/routes/settings/LogoutEverywhereDialog.tsx` | AlertDialog verbatim "Sign {name} out everywhere?" + destructive "Sign user out everywhere" button. |
| `web/src/routes/settings/ResetPasswordDialog.tsx` | 2-stage: AlertDialog destructive confirm → ResponsiveDialog with ShareCredentialsPanel. |
| `web/src/App.tsx` | `/settings/users` route + lazy import. |
| `web/src/routes/settings.tsx` | "Users" Card with link to `/settings/users` (admin-only). |
| `web/playwright/specs/user-management.spec.ts` | E2E: admin adds viewer → share-once panel → copy → log out → log in as viewer → forced password rotation. |

## Deviations from Plan

### Rule 1 (auto-fix bug) — `mustNewTx` connection leak in UpdateHandler

**Found during:** Task 2 (TestUpdateUser_NameOnly hung past 600s in the first test run)
**Issue:** The post-commit reload in `UpdateHandler` (and `CreateHandler`) opened a fresh `pgx.TxOptions{IsoLevel: pgx.Serializable}` tx but never called `Commit` or `Rollback`. Each call returned the connection to the pool only when the deferred rollback ran at function exit — but the per-test pool exhaustion was masking that. The intermediate `mustNewTx` helper also panicked-on-error, which would have surfaced as a 500 if any test produced an error during reload.
**Fix:** Inlined a plain (default isolation) read tx that calls `Commit` immediately after `GetByIDForUpdate`. Removed the `mustNewTx` helper and the unused `fmt` import.
**Files modified:** `internal/user/handler.go`
**Commit:** bbb6f6c (Task 2; fix included in the same commit since the first run never made it past test-time and was iterated before the commit landed)

### Rule 1 (auto-fix bug) — TooltipProvider missing in users.tsx

**Found during:** Task 3 component tests (`Tooltip must be used within TooltipProvider`)
**Issue:** The "Last login" cell rendered a bare `Tooltip` without wrapping it in `TooltipProvider`. The radix-ui primitive requires the provider in the React tree.
**Fix:** Wrapped the `Tooltip` in a `TooltipProvider` inline. Added `TooltipProvider` to the import block.
**Files modified:** `web/src/routes/settings/users.tsx`
**Commit:** 769ff57

### Rule 3 (auto-fix blocking issue) — Migration step counts

**Found during:** Task 1 (adding migration 0044 invalidated 5 step-count assertions in `migrations_test.go` + 1 in `roundtrip_test.go`)
**Issue:** Plan 06-01 left precise step constants (-17, -18, -19, -22, -23) in the round-trip / rollback tests pinned to the 43-version chain. Adding 0044 shifts each by +1.
**Fix:** Updated each step count in lockstep: -17 → -18, -18 → -19, -19 → -20, -22 → -23, -23 → -24, `uint(43)` → `uint(44)`, `require.Equal(t, 43, …)` → `require.Equal(t, 44, …)`. Comments now document the Plan 06-05 contribution.
**Files modified:** `internal/db/migrations_test.go`, `internal/db/roundtrip_test.go`
**Commit:** 15fa114

### Pre-existing failures NOT fixed (scope boundary)

`TestPhase3Migrations_0018_Down` and `TestPhase3Migrations_0019_Down` continue to have a pre-existing off-by-one step count error that pre-dates Plan 06-01 (logged in `deferred-items.md`). The new step constants (-24 and -23 respectively) keep them internally consistent with the new chain length; the underlying off-by-one is unchanged.

## Auth gates

None — fully autonomous execution.

## Test results

- `go build ./...`: clean
- `go vet ./internal/auth/... ./internal/user/...`: clean
- `pnpm tsc --noEmit`: clean
- `pnpm vite build`: clean (4.45s)
- `go test ./internal/user/... -count=1 -timeout=600s`: 35 passed
- `go test ./internal/auth/... -run "TestIterateAndRevoke|TestCan_Phase6" -count=1`: passes (2 new authz tests + IterateAndRevoke export test)
- `go test ./... -short -count=1 -timeout=120s`: 434 passed in 37 packages
- `pnpm -C web test:run`: 337 tests passed in 53 files (full suite, including the 7 new users.test.tsx)

The Playwright spec is structural — it expects a running Shifter dev server + an admin storageState fixture. It is the spec contract that ships with this plan; the cookie material is environmental.

## Threat-model assertions verified

- **T-06-05-01** (viewer → admin via curl): TestRBAC_ViewerCannotCallAnyMutation walks all 8 mutating endpoints and asserts 403 for the viewer role. TestCan_Phase6_UserMgmt_ViewerOnlyReadSelf pins the role-bundle.
- **T-06-05-02** (password reset without audit trail): TestResetPassword_GeneratesAndRevokes asserts both `auth.password_reset_by_admin` and `auth.session_revoked` audit rows are present after a single POST.
- **T-06-05-03** (last-admin TOCTOU): TestRejectLastAdminDemote_TOCTOU_SerializableTx fires two parallel `Serializable` demotes via goroutines and asserts at least one fails (40001 or `ErrLastAdmin`) and the active-admin count stays ≥ 1.
- **T-06-05-04** (self-disable lockout): TestDisable_RejectsSelf + TestChangeRole_RejectsSelf + TestLogoutEverywhere_RejectsSelf all assert 422 for the self target.
- **T-06-05-05** (plaintext leak via server log): the `slog` call sites in `handler.go` only carry user_id / err / op — never the plaintext. Visual inspection.
- **T-06-05-06** (cache leak in React Query): `ShareCredentialsPanel.tsx` "I've shared this" handler calls `queryClient.removeQueries({queryKey:['users']})` — verified inline.
- **T-06-05-07** (session-fixation after role change): TestChangeRole_WritesAuditAndRevokesSessions asserts the audit rows are present in the same tx + the role is persisted; `IterateAndRevoke` after commit is wired and verified at the function-export level.
- **T-06-05-08** (random password too weak): TestGenerateRandomPassword_Strong runs 200 iterations and asserts every one clears `StrengthGood`.
- **T-06-05-09** (UI bypass of grey-out): TestChangeRole_RejectsSelf + TestDisable_RejectsSelf prove a crafted PATCH/POST 422s even when the row UI would be greyed out.
- **T-06-05-10** (password_hash in JSON): `grep -c "PasswordHash" internal/user/store.go` returns 0 — the `UserDTO` struct does not declare the field at all.

## Self-Check: PASSED

- All claimed files exist on disk ✓
- 3 task commits exist in git log: 15fa114 (Task 1), bbb6f6c (Task 2), 769ff57 (Task 3) ✓
- Acceptance-criteria greps:
  - `grep -c "^func IterateAndRevoke" internal/auth/account.go` → 1 ✓
  - `internal/user/password.go` contains `const passwordLen = 16` ✓
  - `internal/user/password.go` excludes `1`, `l`, `I`, `0`, `O` from alphabet ✓
  - `RejectLastAdminDemote` accepts `tx pgx.Tx` parameter ✓
  - `internal/user/store.go` has 0 occurrences of `PasswordHash` ✓
  - `internal/auth/authz.go` declares all 9 new actions ✓
  - `internal/user/handler.go` has 8 handler functions + ≥ 6 `Serializable` references + ≥ 4 `RejectSelfAction` + ≥ 4 `IterateAndRevoke` references ✓
  - Migration files `0044_user_last_login.up.sql` + `.down.sql` exist with the expected ADD/DROP COLUMN ✓
- `go build ./...` clean ✓
- `pnpm tsc --noEmit` clean ✓
- 35 user-package tests + 7 users.test.tsx tests + 337 total web tests + 434 Go short tests all pass ✓
