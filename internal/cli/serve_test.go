package cli

import "testing"

// TestServe_RefusesV3 — `shifter serve` invoked against a v3 ChirpStack
// (GetVersion returns Unimplemented) exits with a non-zero code and prints
// the v3-rejection banner. v3 must never be silently allowed.
// Implementation: Plan 18 (router-health) wires this guard into serve.
func TestServe_RefusesV3(t *testing.T) {
	t.Skip("Plan 18: serve v3 refusal pending")
}

// TestServe_AutoMigrate — On `shifter serve` startup, pending migrations
// are applied automatically before the HTTP listener binds; if migrations
// fail, serve exits with the migration error rather than starting half-way.
// Implementation: Plan 18 (router-health) + Plan 03 (database-layer).
func TestServe_AutoMigrate(t *testing.T) {
	t.Skip("Plan 03/18: serve startup auto-migrate pending")
}
