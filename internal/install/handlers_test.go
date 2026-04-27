package install

import "testing"

// TestInstallState_Reentrant — A user can reload mid-wizard and resume from
// the persisted draft. Each step's GET handler hydrates the form from the
// install_state row.
// Implementation: Plan 15 (install-handlers).
func TestInstallState_Reentrant(t *testing.T) {
	t.Skip("Plan 15: wizard re-entry hydration pending")
}
