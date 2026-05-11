package auth

// Phase 3 Wave 0/1 — RBAC matrix for the new Phase 3 actions (D-26).
// These tests extend the Phase 2 authz_test.go matrix without rewriting
// existing rows. They verify the `Can` checker admits admin and denies
// viewer for every Phase 3 action.

import "testing"

// TestCan_GatewayCreate_AdminOK_ViewerForbidden — gateway.create
func TestCan_GatewayCreate_AdminOK_ViewerForbidden(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/auth/authz.go phase-3 actions (03-VALIDATION row can_phase3_test.TestCan_GatewayCreate_AdminOK_ViewerForbidden)")
}

// TestCan_GatewayUpdate_AdminOK_ViewerForbidden — gateway.update
func TestCan_GatewayUpdate_AdminOK_ViewerForbidden(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/auth/authz.go phase-3 actions (03-VALIDATION row can_phase3_test.TestCan_GatewayUpdate_AdminOK_ViewerForbidden)")
}

// TestCan_GatewayArchive_AdminOK_ViewerForbidden — gateway.archive
func TestCan_GatewayArchive_AdminOK_ViewerForbidden(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/auth/authz.go phase-3 actions (03-VALIDATION row can_phase3_test.TestCan_GatewayArchive_AdminOK_ViewerForbidden)")
}

// TestCan_GatewayRestore_AdminOK_ViewerForbidden — gateway.restore
func TestCan_GatewayRestore_AdminOK_ViewerForbidden(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/auth/authz.go phase-3 actions (03-VALIDATION row can_phase3_test.TestCan_GatewayRestore_AdminOK_ViewerForbidden)")
}

// TestCan_DeviceBulkImport_AdminOK_ViewerForbidden — device.bulk_import
func TestCan_DeviceBulkImport_AdminOK_ViewerForbidden(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/auth/authz.go phase-3 actions (03-VALIDATION row can_phase3_test.TestCan_DeviceBulkImport_AdminOK_ViewerForbidden)")
}

// TestCan_DeviceRevealSecrets_AdminOK_ViewerForbidden — device.reveal_secrets
func TestCan_DeviceRevealSecrets_AdminOK_ViewerForbidden(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/auth/authz.go phase-3 actions (03-VALIDATION row can_phase3_test.TestCan_DeviceRevealSecrets_AdminOK_ViewerForbidden)")
}
