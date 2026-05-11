package api

// Phase 3 Wave 2 — /api/gateways HTTP handlers (chi).
//
// D-01 / D-03: CRUD + region default from the install config.
// Skeleton t.Skip placeholders so Wave 2 has a verification target.

import "testing"

// TestListGatewaysHandler — GET /api/gateways returns paginated list with
// region badge + last-uplink + sparkline pre-computed via metrics cache.
func TestListGatewaysHandler(t *testing.T) {
	t.Skip("Wave 2: awaiting internal/api/gateways_handler.go (03-VALIDATION row gateways_handler_test.TestListGatewaysHandler)")
}

// TestCreateGatewayHandler_RegionDefault — D-03: when the request body omits
// region, the install-config region (`AS923_2` for Thailand) is applied.
func TestCreateGatewayHandler_RegionDefault(t *testing.T) {
	t.Skip("Wave 2: awaiting internal/api/gateways_handler.go (03-VALIDATION row gateways_handler_test.TestCreateGatewayHandler_RegionDefault)")
}

// TestUpdateGatewayHandler — PUT /api/gateways/:id updates name/description
// preserving gateway_id; rejects body with mismatched gateway_id.
func TestUpdateGatewayHandler(t *testing.T) {
	t.Skip("Wave 2: awaiting internal/api/gateways_handler.go (03-VALIDATION row gateways_handler_test.TestUpdateGatewayHandler)")
}

// TestArchiveGateway — POST /api/gateways/:id/archive soft-deletes in PG
// while leaving the CS-side gateway intact (decommission flow handles CS).
func TestArchiveGateway(t *testing.T) {
	t.Skip("Wave 2: awaiting internal/api/gateways_handler.go (03-VALIDATION row gateways_handler_test.TestArchiveGateway)")
}
