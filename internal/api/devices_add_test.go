package api

// Phase 3 Wave 2 — POST /api/devices (add single device).
// D-19 / D-20 / D-21 / D-22 / D-23 / D-24 / D-25: OTAA / ABP flows,
// atomic CS+PG with best-effort CS rollback on PG failure.

import "testing"

// TestAddDevice_OTAA — OTAA flow: CreateDevice + CreateDeviceKeys in CS,
// PG row inserted; OTAA keys returned in response for one-time reveal.
func TestAddDevice_OTAA(t *testing.T) {
	t.Skip("Wave 2: awaiting internal/api/devices_add.go (03-VALIDATION row devices_add_test.TestAddDevice_OTAA)")
}

// TestAddDevice_ABP — ABP flow: CreateDevice + ActivateDevice (session keys
// + dev_addr + fcnt counters), PG row inserted.
func TestAddDevice_ABP(t *testing.T) {
	t.Skip("Wave 2: awaiting internal/api/devices_add.go (03-VALIDATION row devices_add_test.TestAddDevice_ABP)")
}

// TestAddDevice_ABP_FCntCarryOver — D-20: optional fcnt_up / fcnt_down
// fields preserve replacement-device counter continuity (post-swap).
func TestAddDevice_ABP_FCntCarryOver(t *testing.T) {
	t.Skip("Wave 2: awaiting internal/api/devices_add.go (03-VALIDATION row devices_add_test.TestAddDevice_ABP_FCntCarryOver)")
}
