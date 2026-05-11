package api

// Phase 3 Wave 2 — GET /api/devices (filter / sort / paginate).
// D-12 / D-13 / D-14: server-side filtering, sorting, offset pagination.

import "testing"

// TestListDevicesFiltered_Site — ?site=A,B returns only devices bound to
// those sites (multi-select).
func TestListDevicesFiltered_Site(t *testing.T) {
	t.Skip("Wave 2: awaiting internal/api/devices_list.go (03-VALIDATION row devices_list_test.TestListDevicesFiltered_Site)")
}

// TestListDevicesFiltered_Status — ?status=online|offline filters by the
// computed last_seen_at threshold (D-12).
func TestListDevicesFiltered_Status(t *testing.T) {
	t.Skip("Wave 2: awaiting internal/api/devices_list.go (03-VALIDATION row devices_list_test.TestListDevicesFiltered_Status)")
}

// TestListDevicesFiltered_LastSeen — ?last_seen=24h|7d|30d filters by
// uplink recency.
func TestListDevicesFiltered_LastSeen(t *testing.T) {
	t.Skip("Wave 2: awaiting internal/api/devices_list.go (03-VALIDATION row devices_list_test.TestListDevicesFiltered_LastSeen)")
}

// TestListDevicesFiltered_TextSearch — ?q=foo searches name + dev_eui
// substring (case-insensitive).
func TestListDevicesFiltered_TextSearch(t *testing.T) {
	t.Skip("Wave 2: awaiting internal/api/devices_list.go (03-VALIDATION row devices_list_test.TestListDevicesFiltered_TextSearch)")
}

// TestListDevicesSorted — ?sort=name,-last_seen multi-column sort with
// direction prefix.
func TestListDevicesSorted(t *testing.T) {
	t.Skip("Wave 2: awaiting internal/api/devices_list.go (03-VALIDATION row devices_list_test.TestListDevicesSorted)")
}

// TestListDevicesPaginated — ?limit=50&offset=100 returns the third page.
func TestListDevicesPaginated(t *testing.T) {
	t.Skip("Wave 2: awaiting internal/api/devices_list.go (03-VALIDATION row devices_list_test.TestListDevicesPaginated)")
}
