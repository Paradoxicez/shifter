package install

import "testing"

// TestStep1_PersistsAdmin — Wizard step 1 (admin identity) persists the draft
// admin email/password into the install_state row.
// Implementation: Plan 15 (install-handlers).
func TestStep1_PersistsAdmin(t *testing.T) {
	t.Skip("Plan 15: step 1 persistence pending")
}

// TestStep2_CapturesCS — Wizard step 2 captures the ChirpStack mode (bundled
// vs external), gRPC URL, API key, and MQTT URL into the draft. Includes a
// successful v4 probe before persisting.
// Implementation: Plan 15 (install-handlers).
func TestStep2_CapturesCS(t *testing.T) {
	t.Skip("Plan 15: step 2 ChirpStack capture pending")
}

// TestStep2_RejectsV3 — When the v3 probe (GetVersion → Unimplemented) hits,
// step 2 returns a destructive banner and does NOT persist the draft.
// Implementation: Plan 15 (install-handlers).
func TestStep2_RejectsV3(t *testing.T) {
	t.Skip("Plan 15: step 2 v3 rejection pending")
}

// TestStep3_PersistsRegion — Wizard step 3 (LoRaWAN region picker) writes the
// chosen `(name, common_name)` pair to the draft chirpstack_connection row.
// Implementation: Plan 15 (install-handlers).
func TestStep3_PersistsRegion(t *testing.T) {
	t.Skip("Plan 15: step 3 region persistence pending")
}

// TestStep4_PersistsIdentity — Wizard step 4 (install identity) writes the
// install_identity row (display name, organization). Filled by the install
// wizard UI in Plan 16.
// Implementation: Plan 15 (install-handlers); test data driven by Plan 14.
func TestStep4_PersistsIdentity(t *testing.T) {
	t.Skip("Plan 15: step 4 identity persistence pending")
}

// TestFinishSetup_Atomic — The wizard "Finish setup" action commits all
// staged drafts (admin user, chirpstack_connection, install_identity) inside
// a single transaction; concurrent finish requests collapse to a single
// committed state, never partial.
// Implementation: Plan 15 (install-handlers).
func TestFinishSetup_Atomic(t *testing.T) {
	t.Skip("Plan 15: atomic finish pending")
}
