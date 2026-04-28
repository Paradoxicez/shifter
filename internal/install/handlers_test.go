package install

import "testing"

// TestInstallState_Reentrant — A user can reload mid-wizard and resume from
// the persisted draft. Each step's GET handler hydrates the form from the
// install_state row.
// Implementation: Plan 15 (install-handlers).
func TestInstallState_Reentrant(t *testing.T) {
	t.Skip("Plan 15: wizard re-entry hydration pending")
}

// TestStep2_RejectsV3 — When the v3 probe (GetVersion → Unimplemented) hits,
// step 2 returns a destructive banner and does NOT persist the draft.
// Implementation: Plan 15 (install-handlers).
func TestStep2_RejectsV3(t *testing.T) {
	t.Skip("Plan 15: step 2 v3 rejection pending")
}

// TestFinishSetup_Atomic — The wizard "Finish setup" action commits all
// staged drafts (admin user, chirpstack_connection, install_identity) inside
// a single transaction; concurrent finish requests collapse to a single
// committed state, never partial.
// Implementation: Plan 15 (install-handlers).
func TestFinishSetup_Atomic(t *testing.T) {
	t.Skip("Plan 15: atomic finish pending")
}
