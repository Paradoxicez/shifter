package chirpstack

import "testing"

// TestClient_ListDevices_Mock — Against the mockgen-generated DeviceServiceServer
// the high-level Client.ListDevices wrapper returns the same set of devices
// the mock served, with paging fields propagated.
// Implementation: Plan 12 (chirpstack-grpc).
func TestClient_ListDevices_Mock(t *testing.T) {
	t.Skip("Plan 12: ListDevices wrapper pending")
}
