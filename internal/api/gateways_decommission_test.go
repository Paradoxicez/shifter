package api

// Phase 3 Wave 2 — POST /api/gateways/:id/decommission + restore.
// D-30 / D-31 / D-32: soft-delete + CS DeleteGateway (atomic) + 24h-uplink
// warning + restore. Pending Open Q #1 resolution on whether the CS-side
// delete is part of the contract (the test names are stable either way).

import "testing"

// TestDecommissionGateway_SoftDelete — happy path: PG row gets archived_at
// timestamp; CS gateway is deleted; both succeed atomically.
func TestDecommissionGateway_SoftDelete(t *testing.T) {
	t.Skip("Wave 2: awaiting internal/api/gateways_decommission.go (03-VALIDATION row gateways_decommission_test.TestDecommissionGateway_SoftDelete)")
}

// TestDecommissionGateway_24hWarning — if last_seen_at within 24h, the
// handler returns 409 + warning JSON envelope; client confirms with
// ?force=true to proceed (D-31).
func TestDecommissionGateway_24hWarning(t *testing.T) {
	t.Skip("Wave 2: awaiting internal/api/gateways_decommission.go (03-VALIDATION row gateways_decommission_test.TestDecommissionGateway_24hWarning)")
}

// TestRestoreGateway — POST /api/gateways/:id/restore clears archived_at
// and re-creates the gateway in CS using the cached config (D-32).
func TestRestoreGateway(t *testing.T) {
	t.Skip("Wave 2: awaiting internal/api/gateways_decommission.go (03-VALIDATION row gateways_decommission_test.TestRestoreGateway)")
}
