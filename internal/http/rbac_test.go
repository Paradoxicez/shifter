package http

import "testing"

// TestRBAC_AdminAllowed — An admin session can POST to admin-only endpoints
// (e.g. /api/users) and receives 2xx.
// Implementation: Plan 10 (authz) + Plan 18 (router wiring).
func TestRBAC_AdminAllowed(t *testing.T) {
	t.Skip("Plan 10/18: admin allow-list pending")
}

// TestRBAC_ViewerForbidden — A viewer session POSTing to admin-only
// endpoints receives 403 (not 401 — viewer IS authenticated, just not
// authorized).
// Implementation: Plan 10 (authz) + Plan 18 (router wiring).
func TestRBAC_ViewerForbidden(t *testing.T) {
	t.Skip("Plan 10/18: viewer forbidden pending")
}
