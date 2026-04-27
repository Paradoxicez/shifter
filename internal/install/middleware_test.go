package install

import "testing"

// TestFirstRun_Gate — When no admin user exists yet, requests to any non-/install
// path are 302-redirected to /install. Static assets and the /install routes
// pass through unchanged.
// Implementation: Plan 14 (install-middleware).
func TestFirstRun_Gate(t *testing.T) {
	t.Skip("Plan 14: first-run redirect pending")
}

// TestPostFinish_NoWizardAccess — After wizard finish (admin user exists in
// the database), GET /install returns 302 to /, preventing repeated reset.
// Implementation: Plan 14 (install-middleware).
func TestPostFinish_NoWizardAccess(t *testing.T) {
	t.Skip("Plan 14: post-finish wizard lockout pending")
}
