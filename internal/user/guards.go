package user

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/shifter-io/shifter/internal/auth"
)

// ErrSelfAction is returned by RejectSelfAction when an admin attempts a
// self-targeting mutation (disable / role-change / sign-out-everywhere /
// reset-own-password) via this surface. The lockout escape hatch remains
// the Phase 1 `shifter create-admin --reset` CLI (D-26).
var ErrSelfAction = errors.New("user: cannot perform this action on yourself")

// ErrLastAdmin is returned by RejectLastAdminDemote when the requested role
// change would leave zero active admins. The system maintains the invariant
// "at least one active admin exists at all times" so a future admin-driven
// recovery flow is always possible.
var ErrLastAdmin = errors.New("user: cannot leave zero admins")

// RejectSelfAction blocks self-disable, self-role-change, self-reset-password,
// and self-logout-everywhere at the server (D-26). The UI greys out the
// affected row actions but this function is the authoritative lock — a
// crafted HTTP request still 422s.
func RejectSelfAction(actingUserID, targetUserID string) error {
	if actingUserID == "" || targetUserID == "" {
		// Defensive: an empty acting id implies an unauthenticated request,
		// which the RequireAction middleware should have caught earlier. Be
		// fail-open in this guard — the middleware is the lock. (Returning
		// nil here means downstream guards keep their normal semantics.)
		return nil
	}
	if actingUserID == targetUserID {
		return ErrSelfAction
	}
	return nil
}

// RejectLastAdminDemote blocks any role change that would leave zero active
// admins. Called INSIDE a Serializable transaction (Pitfall 6 TOCTOU
// mitigation): two parallel demotes both observing one-other-admin would
// both succeed under READ COMMITTED; under SERIALIZABLE one tx serialization-
// errors on Commit and the caller can retry / report the conflict.
//
// newRole 'admin' is always allowed — only demote-from-admin can violate the
// last-admin invariant.
//
// The check counts active admins EXCLUDING the target (CountActiveAdminsExcluding):
// if at least one OTHER active admin exists, the demote is safe.
func RejectLastAdminDemote(ctx context.Context, tx pgx.Tx, store *auth.Store, targetUserID, newRole string) error {
	if newRole == "admin" {
		return nil
	}
	n, err := store.CountActiveAdminsExcluding(ctx, tx, targetUserID)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrLastAdmin
	}
	return nil
}
