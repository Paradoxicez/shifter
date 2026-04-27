package http

import "testing"

// TestSessionPersistence — A second HTTP request sent with the cookie
// returned by the first login is treated as authenticated; the user identity
// in the request context matches the login.
// Implementation: Plan 09 (login-ratelimit) + Plan 08 (session-manager).
func TestSessionPersistence(t *testing.T) {
	t.Skip("Plan 08/09: session cookie persistence pending")
}

// TestLogin_Success — POST /api/login with valid credentials returns 200,
// sets the SCS session cookie (httpOnly, Path=/, SameSite=Lax), and the
// response body contains {role, email}.
// Implementation: Plan 09 (login-ratelimit).
func TestLogin_Success(t *testing.T) {
	t.Skip("Plan 09: login success path pending")
}
