package auth

import (
	"context"
	"errors"

	"github.com/alexedwards/scs/v2"
)

// ErrNoUser is returned by UserFromContext when no session-bound user is
// present in the request context (either no session at all, or a session that
// has not been associated with a user via PutUser).
var ErrNoUser = errors.New("auth: no user in context")

// UserFromContext is a convenience that pulls the user from the SCS-bound
// context and returns ErrNoUser when no session is present.
//
// The caller MUST be downstream of sm.LoadAndSave (Plan 09 wires the
// middleware on every /api route) — otherwise the SCS context key isn't
// populated and ErrNoUser is the only possible result.
func UserFromContext(ctx context.Context, sm *scs.SessionManager) (User, error) {
	u, ok := GetUser(ctx, sm)
	if !ok {
		return User{}, ErrNoUser
	}
	return u, nil
}
