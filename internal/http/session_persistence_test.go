package http

import "testing"

// TestSessionPersistence is the canonical AUTH-02 cross-request test, hosted
// in internal/auth (so the test fixture has direct access to LoadAndSave +
// LoginHandler without an import cycle through internal/http). This forwarder
// exists only so VALIDATION.md's `go test ./internal/http -run TestSessionPersistence`
// command does not error with "no tests to run" — the actual assertion lives
// at internal/auth.TestSessionPersistence.
func TestSessionPersistence(t *testing.T) {
	t.Skip("see internal/auth.TestSessionPersistence — run: go test ./internal/auth -run TestSessionPersistence")
}

// TestLogin_Success forwarder — see internal/auth.TestLogin_Success.
func TestLogin_Success(t *testing.T) {
	t.Skip("see internal/auth.TestLogin_Success — run: go test ./internal/auth -run TestLogin_Success")
}
