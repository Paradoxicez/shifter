package chirpstack

import "testing"

// TestProbeVersion_v4 — Against a v4 mock that returns a successful
// GetVersion response, ProbeVersion returns the version string and nil error.
// Implementation: Plan 12 (chirpstack-grpc).
func TestProbeVersion_v4(t *testing.T) {
	t.Skip("Plan 12: v4 probe pending")
}

// TestProbeVersion_v3 — Against a v3 mock that returns codes.Unimplemented
// for GetVersion, ProbeVersion returns the typed sentinel
// ErrChirpStackV3OrUnknown so the wizard / serve startup can refuse v3.
// Implementation: Plan 12 (chirpstack-grpc).
func TestProbeVersion_v3(t *testing.T) {
	t.Skip("Plan 12: v3 probe rejection pending")
}
