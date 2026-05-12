// Package auth — account-management HTTP handlers (AUTH-05).
//
// ChangePasswordHandler implements POST /api/account/password. Behavior is
// described in 01-09-login-ratelimit-PLAN.md <interfaces>.
//
// Defense-in-depth: a successful password change revokes every OTHER active
// session for the same user. The current session (the operator's own device)
// stays valid because we keep its token; SCS payloads are gob-encoded inside
// the `data` BYTEA column, so the revoke pass uses scs.SessionManager.Iterate
// to inspect each session's user_id and DELETE the matching tokens directly
// against the sessions table via pgx.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"

	"github.com/shifter-io/shifter/internal/audit"
)

// AccountDeps bundles dependencies for account.go handlers.
type AccountDeps struct {
	Store      *Store
	SessionMgr *scs.SessionManager
	Log        *slog.Logger
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type weakPasswordResp struct {
	Error string `json:"error"`
	Tier  string `json:"tier"`
}

// ChangePasswordHandler implements AUTH-05.
//
// Flow:
//  1. CSRF guard.
//  2. Require authenticated user via GetUser.
//  3. Decode body; reject empty / oversize new_password.
//  4. Reject weak passwords (Strength == StrengthWeak) → 422.
//  5. Reload user record by ID, Verify current_password.
//  6. Hash new_password; open tx: UpdatePasswordTx + audit rows (D-30) + commit.
//  7. Post-commit: revoke all OTHER sessions for this user (defense-in-depth).
//  8. 200 OK.
//
// D-30 scope: two audit rows per successful change, both in the same tx as the
// password update — atomicity means the audit record cannot exist without the
// password change, and vice versa:
//   - auth.password_change  (self-initiated change)
//   - auth.session_revoked  (other sessions will be revoked post-commit)
func ChangePasswordHandler(deps AccountDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !csrfHeaderPresent(r) {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "missing_csrf_header"})
			return
		}
		u, ok := GetUser(r.Context(), deps.SessionMgr)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errorResp{Error: "unauthorized"})
			return
		}
		var req changePasswordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_request"})
			return
		}
		if req.CurrentPassword == "" || req.NewPassword == "" ||
			len(req.CurrentPassword) > maxPasswordLength ||
			len(req.NewPassword) > maxPasswordLength {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_request"})
			return
		}
		if PasswordStrength(req.NewPassword) == StrengthWeak {
			writeJSON(w, http.StatusUnprocessableEntity, weakPasswordResp{
				Error: "weak_password",
				Tier:  StrengthWeak.String(),
			})
			return
		}

		userRow, err := deps.Store.GetUserByID(r.Context(), u.ID)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				// User row was deleted/disabled between login and this request.
				writeJSON(w, http.StatusUnauthorized, errorResp{Error: "unauthorized"})
				return
			}
			deps.Log.Error("change-password: load user", "err", err, "user_id", u.ID)
			writeJSON(w, http.StatusInternalServerError, errorResp{Error: "internal"})
			return
		}
		ok, vErr := Verify(req.CurrentPassword, userRow.PasswordHash)
		if vErr != nil {
			deps.Log.Warn("change-password: verify", "err", vErr, "user_id", u.ID)
		}
		if vErr != nil || !ok {
			writeJSON(w, http.StatusUnauthorized, errorResp{Error: "current_password_incorrect"})
			return
		}

		newHash, err := Hash(req.NewPassword)
		if err != nil {
			deps.Log.Error("change-password: hash", "err", err, "user_id", u.ID)
			writeJSON(w, http.StatusInternalServerError, errorResp{Error: "internal"})
			return
		}

		ctx := r.Context()
		tx, err := deps.Store.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			deps.Log.Error("change-password: begin tx", "err", err, "user_id", u.ID)
			writeJSON(w, http.StatusInternalServerError, errorResp{Error: "internal"})
			return
		}
		defer func() { _ = tx.Rollback(ctx) }()

		// UpdatePasswordTx (Plan 06-05): updates password_hash + must_change_password
		// inside the caller's tx. mustChange=false because this is a self-initiated
		// change (the user chose their own new password).
		if err := deps.Store.UpdatePasswordTx(ctx, tx, u.ID, newHash, false); err != nil {
			deps.Log.Error("change-password: update tx", "err", err, "user_id", u.ID)
			writeJSON(w, http.StatusInternalServerError, errorResp{Error: "internal"})
			return
		}

		userUUID := mustParseUUID(u.ID)
		reqID := middleware.GetReqID(ctx)

		// Audit: self-initiated password change (D-30).
		if err := audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     userUUID,
			Action:     audit.ActionAuthPasswordChange,
			EntityType: audit.EntityTypeUser,
			EntityID:   userUUID,
			Notes:      "self_initiated",
			RequestID:  reqID,
		}); err != nil {
			deps.Log.Error("change-password: audit password_change", "err", err, "user_id", u.ID)
			writeJSON(w, http.StatusInternalServerError, errorResp{Error: "internal"})
			return
		}

		// Audit: other sessions will be revoked after commit (D-30).
		if err := audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     userUUID,
			Action:     audit.ActionAuthSessionRevoked,
			EntityType: audit.EntityTypeSession,
			EntityID:   userUUID,
			Notes:      "other_sessions_revoked_on_password_change",
			RequestID:  reqID,
		}); err != nil {
			deps.Log.Error("change-password: audit session_revoked", "err", err, "user_id", u.ID)
			writeJSON(w, http.StatusInternalServerError, errorResp{Error: "internal"})
			return
		}

		if err := tx.Commit(ctx); err != nil {
			deps.Log.Error("change-password: commit tx", "err", err, "user_id", u.ID)
			writeJSON(w, http.StatusInternalServerError, errorResp{Error: "internal"})
			return
		}

		// Defense-in-depth: revoke every OTHER session for this user AFTER commit.
		// The current session stays valid because we pass the current token as
		// keepToken. Operator's current cookie continues to work.
		currentToken := deps.SessionMgr.Token(ctx)
		if err := iterateAndRevoke(ctx, deps.SessionMgr, deps.Store, u.ID, currentToken); err != nil {
			// Log but do not fail the request — the password is already
			// changed; an unrelated session-cleanup hiccup must not surface
			// as "your password change failed".
			deps.Log.Warn("change-password: revoke other sessions", "err", err, "user_id", u.ID)
		}

		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// IterateAndRevoke is the exported alias of iterateAndRevoke for Phase 6 user
// management handlers (Plan 06-05 D-24): every state-changing user-mgmt
// handler that needs to "kick the user out everywhere" funnels through this
// single code path. Callers from outside this package pass an empty
// keepToken to revoke ALL sessions for the user (no current-session
// exception).
//
// The function is intentionally a thin wrapper around the unexported
// implementation so account.go's existing internal callers stay untouched.
func IterateAndRevoke(ctx context.Context, sm *scs.SessionManager, store *Store, userID, keepToken string) error {
	return iterateAndRevoke(ctx, sm, store, userID, keepToken)
}

// iterateAndRevoke walks every session via SCS's Iterate, decodes the user_id
// payload, and DELETEs every session row whose user_id matches `userID` and
// whose token differs from `keepToken`. The DELETE goes against the raw
// sessions table via pgx because SCS's API does not expose "destroy by token
// other than mine".
func iterateAndRevoke(ctx context.Context, sm *scs.SessionManager, store *Store, userID, keepToken string) error {
	var toDelete []string
	err := sm.Iterate(ctx, func(ictx context.Context) error {
		tok := sm.Token(ictx)
		if tok == keepToken {
			return nil
		}
		if uid := sm.GetString(ictx, sessionUserIDKey); uid == userID {
			toDelete = append(toDelete, tok)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, tok := range toDelete {
		if _, err := store.Pool().Exec(ctx, `DELETE FROM sessions WHERE token = $1`, tok); err != nil {
			return err
		}
	}
	return nil
}
