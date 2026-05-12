// Package user — Phase 6 Plan 06-05 administrative user-management surface.
//
// The package is a thin orchestration layer on top of:
//
//   - auth.Store               — the persisted-user CRUD methods (List, CreateTx,
//     UpdateTx, DisableTx, EnableTx, ChangeRoleTx,
//     UpdatePasswordTx, CountActiveAdminsExcluding,
//     GetByIDForUpdate).
//   - auth.IterateAndRevoke    — the single "kick the user out everywhere"
//     code path (D-24).
//   - audit.WriteEntry         — every mutation writes its audit row INSIDE
//     the same transaction (D-30).
//
// The four D-24 implicit-revoke triggers (CONTEXT.md D-24) all funnel through
// auth.IterateAndRevoke and live in this package:
//
//   - LogoutEverywhereHandler  — explicit
//   - DisableHandler           — implicit (user is being signed out as part of disable)
//   - ChangeRoleHandler        — implicit (role change forces re-auth per D-25)
//   - ResetPasswordHandler     — implicit (admin-driven credential rotation)
//
// Two server-side guards enforce D-26 regardless of UI state:
//
//   - RejectSelfAction         — admin cannot disable / role-change / sign
//     themselves out everywhere / reset their own
//     password via this surface.
//   - RejectLastAdminDemote    — the system cannot reach zero active admins
//     via a role change. Pitfall 6 (TOCTOU on
//     parallel demotes) is mitigated by wrapping
//     the guard call + the ChangeRoleTx inside a
//     Serializable tx; the loser tx serialization-
//     errors on Commit.
//
// The package also owns the D-23 random password generator: GenerateRandomPassword
// uses crypto/rand + an alphabet that excludes the ambiguous chars 1lI0O and
// re-rolls up to 10 times if the Phase 1 StrengthScore evaluator (D-28)
// rejects the candidate. At length 16 from 62 characters the entropy is
// ~95 bits, so the re-roll guard is a defensive safeguard against future
// alphabet edits rather than a routine outcome.
package user
