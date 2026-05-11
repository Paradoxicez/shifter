package api

// Phase 3 Plan 03-06 / Task 2 — POST /api/devices/bulk-decommission (D-17).
//
// Canonical integration tests live in
// internal/device/bulk_decommission_test.go:
//   - TestBulkDecommissionDevices_PartialSuccess
//   - TestBulkDecommissionDevices_Atomic
//   - TestBulkDecommissionDevices_MaxBatch200
//   - TestBulkDecommissionDevices_Viewer403
//
// The package-api markers below resolve the 03-VALIDATION rows to a real
// test invocation. Actual coverage lives in the device package.

import "testing"

func TestBulkDecommissionDevices_PartialSuccess(t *testing.T) {
	t.Skip("canonical: internal/device/bulk_decommission_test.go::TestBulkDecommissionDevices_PartialSuccess")
}
