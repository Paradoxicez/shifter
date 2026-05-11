package chirpstack

// Phase 3 Wave 1 — internal/chirpstack/gateway.go wrapper tests.
//
// These are skeleton t.Skip placeholders so Wave 0 leaves a compile-able
// test file referenced by every Wave 1+ task before production code lands.
// See 03-VALIDATION.md §Required Test Files for the expected behavior of
// each TestX once Wave 1 implements the wrapper.

import "testing"

// TestCreateGateway — wraps GatewayService.Create with retry / error
// translation. GW-01: empty gateway_id should surface as ErrInvalidGateway.
func TestCreateGateway(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/chirpstack/gateway.go (03-VALIDATION row gateway_test.TestCreateGateway)")
}

// TestGetGateway — Get wrapper translates codes.NotFound to ErrNotFound.
func TestGetGateway(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/chirpstack/gateway.go (03-VALIDATION row gateway_test.TestGetGateway)")
}

// TestUpdateGateway — Update wrapper preserves tenant_id, rejects empty id.
func TestUpdateGateway(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/chirpstack/gateway.go (03-VALIDATION row gateway_test.TestUpdateGateway)")
}

// TestDeleteGateway — Delete wrapper is idempotent (NotFound → nil error).
func TestDeleteGateway(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/chirpstack/gateway.go (03-VALIDATION row gateway_test.TestDeleteGateway)")
}

// TestListGateways — List wrapper paginates correctly + applies tenant filter.
func TestListGateways(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/chirpstack/gateway.go (03-VALIDATION row gateway_test.TestListGateways)")
}

// TestGetMetrics — GetMetrics wrapper returns 24-bucket rx/tx series; default
// to all-zero when no uplinks seen (D-02 sparkline empty state).
func TestGetMetrics(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/chirpstack/gateway.go (03-VALIDATION row gateway_test.TestGetMetrics)")
}
