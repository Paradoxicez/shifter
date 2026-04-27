package auth

import "testing"

// TestSession_IdleTimeout — A session that has not been touched for the
// configured idle timeout is rejected on the next request.
// Implementation: Plan 08 (session-manager).
func TestSession_IdleTimeout(t *testing.T) {
	t.Skip("Plan 08: SCS idle timeout pending")
}

// TestSession_DevSecureToggle — In dev mode the session cookie omits the
// Secure flag (so it works over plain http://localhost); in prod it must
// always set Secure.
// Implementation: Plan 08 (session-manager).
func TestSession_DevSecureToggle(t *testing.T) {
	t.Skip("Plan 08: dev/prod Secure flag toggle pending")
}
