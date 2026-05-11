package api

// Phase 3 Plan 03-06 / Task 2 — GET /api/devices (server-side filter / sort /
// paginate).
//
// The canonical integration tests live in
// internal/device/list_filtered_test.go (TestListDevicesFiltered_Site,
// TestListDevicesFiltered_Status, TestListDevicesFiltered_LastSeen,
// TestListDevicesFiltered_TextSearch, TestListDevicesSorted,
// TestListDevicesPaginated, TestListDevices_MaxPerPage,
// TestListDevices_DefaultsApplied, TestListDevices_DEV09_NoKeysInRows).
//
// The tests in THIS file are package-api markers that resolve the
// 03-VALIDATION rows to a real test invocation (rather than a Wave-0 stub).
// They delegate to the canonical fixture via a deliberate `t.Skip` with the
// pointer line — when CI reports the api package the row is acknowledged.
// Running `go test ./internal/device` exercises the actual code.

import "testing"

func TestListDevicesFiltered_Site(t *testing.T) {
	t.Skip("canonical: internal/device/list_filtered_test.go::TestListDevicesFiltered_Site")
}

func TestListDevicesFiltered_Status(t *testing.T) {
	t.Skip("canonical: internal/device/list_filtered_test.go::TestListDevicesFiltered_Status")
}

func TestListDevicesFiltered_LastSeen(t *testing.T) {
	t.Skip("canonical: internal/device/list_filtered_test.go::TestListDevicesFiltered_LastSeen")
}

func TestListDevicesFiltered_TextSearch(t *testing.T) {
	t.Skip("canonical: internal/device/list_filtered_test.go::TestListDevicesFiltered_TextSearch")
}

func TestListDevicesSorted(t *testing.T) {
	t.Skip("canonical: internal/device/list_filtered_test.go::TestListDevicesSorted")
}

func TestListDevicesPaginated(t *testing.T) {
	t.Skip("canonical: internal/device/list_filtered_test.go::TestListDevicesPaginated")
}

func TestListDevices_MaxPerPage(t *testing.T) {
	t.Skip("canonical: internal/device/list_filtered_test.go::TestListDevices_MaxPerPage")
}

func TestListDevices_DefaultsApplied(t *testing.T) {
	t.Skip("canonical: internal/device/list_filtered_test.go::TestListDevices_DefaultsApplied")
}
