package http

import "testing"

// TestTestConn_Happy — POST /api/install/test-connection against a v4
// ChirpStack mock + reachable Mosquitto returns gRPC=reachable AND
// MQTT=reachable.
// Implementation: Plan 17 (test-connection).
func TestTestConn_Happy(t *testing.T) {
	t.Skip("Plan 17: happy-path test-connection pending")
}

// TestTestConn_V3Refused — POST /api/install/test-connection against a v3
// ChirpStack mock returns gRPC=unreachable AND MQTT=skipped (we never probe
// the broker if the network server is rejected).
// Implementation: Plan 17 (test-connection).
func TestTestConn_V3Refused(t *testing.T) {
	t.Skip("Plan 17: v3-refusal short-circuit pending")
}

// TestTestConn_BothFail — POST /api/install/test-connection against a closed
// gRPC server AND unreachable broker returns gRPC=unreachable AND
// MQTT=unreachable with distinct error reasons in each row.
// Implementation: Plan 17 (test-connection).
func TestTestConn_BothFail(t *testing.T) {
	t.Skip("Plan 17: both-fail rendering pending")
}
