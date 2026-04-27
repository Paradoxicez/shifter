package auth

import "testing"

// TestCan_AdminAllowsAll — A user with role=admin returns true for every
// known action in the Can() permission matrix.
// Implementation: Plan 10 (authz).
func TestCan_AdminAllowsAll(t *testing.T) {
	t.Skip("Plan 10: admin permission matrix pending")
}

// TestCan_ViewerDenied — A user with role=viewer returns false for any
// mutating action (create/update/delete/admin) and true for read.
// Implementation: Plan 10 (authz).
func TestCan_ViewerDenied(t *testing.T) {
	t.Skip("Plan 10: viewer permission matrix pending")
}

// TestCan_NilUser — Can() on a nil/anonymous user returns false safely
// rather than panicking.
// Implementation: Plan 10 (authz).
func TestCan_NilUser(t *testing.T) {
	t.Skip("Plan 10: nil-user guard pending")
}
