package api

// Phase 3 Wave 2 — POST /api/devices/bulk-decommission.
// D-17: N rows submitted, each CS+PG transaction is atomic per device,
// partial success reported.

import "testing"

// TestBulkDecommissionDevices_PartialSuccess — submit 5 EUIs where the 3rd
// CS-side Delete fails; response shows ok=4, errored=1 with the specific
// EUI flagged and a remediation hint.
func TestBulkDecommissionDevices_PartialSuccess(t *testing.T) {
	t.Skip("Wave 2: awaiting internal/api/devices_bulk_decommission.go (03-VALIDATION row devices_bulk_decommission_test.TestBulkDecommissionDevices_PartialSuccess)")
}
