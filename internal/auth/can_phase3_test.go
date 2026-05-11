package auth

// Phase 3 Wave 1 — RBAC matrix for the new Phase 3 actions (D-26).
// These tests extend the Phase 2 authz_test.go matrix without rewriting
// existing rows. They verify the `Can` checker admits admin and denies
// viewer for every Phase 3 mutating action; gateway.read is the single
// exception viewers retain (mirrors Phase 2 site.read / device.read).

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// adminUser / viewerUser fixtures match the existing authz_test.go pattern.
func adminUser() *User  { return &User{ID: "u-admin", Role: "admin"} }
func viewerUser() *User { return &User{ID: "u-viewer", Role: "viewer"} }

// TestCan_GatewayCreate_AdminOK_ViewerForbidden — gateway.create
func TestCan_GatewayCreate_AdminOK_ViewerForbidden(t *testing.T) {
	require.True(t, Can(adminUser(), ActionGatewayCreate, nil), "admin must create gateways")
	require.False(t, Can(viewerUser(), ActionGatewayCreate, nil), "viewer must NOT create gateways")
}

// TestCan_GatewayUpdate_AdminOK_ViewerForbidden — gateway.update
func TestCan_GatewayUpdate_AdminOK_ViewerForbidden(t *testing.T) {
	require.True(t, Can(adminUser(), ActionGatewayUpdate, nil))
	require.False(t, Can(viewerUser(), ActionGatewayUpdate, nil))
}

// TestCan_GatewayArchive_AdminOK_ViewerForbidden — gateway.archive
func TestCan_GatewayArchive_AdminOK_ViewerForbidden(t *testing.T) {
	require.True(t, Can(adminUser(), ActionGatewayArchive, nil))
	require.False(t, Can(viewerUser(), ActionGatewayArchive, nil))
}

// TestCan_GatewayRestore_AdminOK_ViewerForbidden — gateway.restore (D-32
// admin-only restore matches archive — viewer cannot undo an archive).
func TestCan_GatewayRestore_AdminOK_ViewerForbidden(t *testing.T) {
	require.True(t, Can(adminUser(), ActionGatewayRestore, nil))
	require.False(t, Can(viewerUser(), ActionGatewayRestore, nil))
}

// TestCan_DeviceBulkImport_AdminOK_ViewerForbidden — device.bulk_import
func TestCan_DeviceBulkImport_AdminOK_ViewerForbidden(t *testing.T) {
	require.True(t, Can(adminUser(), ActionDeviceBulkImport, nil))
	require.False(t, Can(viewerUser(), ActionDeviceBulkImport, nil))
}

// TestCan_DeviceRevealSecrets_AdminOK_ViewerForbidden — device.reveal_secrets
// (T-3-13 mitigation — secret material visibility is admin-only).
func TestCan_DeviceRevealSecrets_AdminOK_ViewerForbidden(t *testing.T) {
	require.True(t, Can(adminUser(), ActionDeviceRevealSecrets, nil))
	require.False(t, Can(viewerUser(), ActionDeviceRevealSecrets, nil))
}

// TestCan_GatewayRead_AdminAndViewer_Phase3 — gateway.read is the ONE
// Phase 3 action viewers retain (mirrors Phase 2 site.read / device.read).
func TestCan_GatewayRead_AdminAndViewer_Phase3(t *testing.T) {
	require.True(t, Can(adminUser(), ActionGatewayRead, nil))
	require.True(t, Can(viewerUser(), ActionGatewayRead, nil),
		"viewer must READ gateways (parity with Phase 2 read actions)")
}

// TestCan_AnonymousDeniedAllPhase3 — nil user denied for every Phase 3
// action (T-3-10 fail-closed mitigation). RequireAction middleware also
// 401s before Can() runs, but defense-in-depth at the Can() level matters
// when handlers call it directly.
func TestCan_AnonymousDeniedAllPhase3(t *testing.T) {
	for _, a := range []Action{
		ActionGatewayCreate,
		ActionGatewayUpdate,
		ActionGatewayArchive,
		ActionGatewayRestore,
		ActionGatewayRead,
		ActionDeviceBulkImport,
		ActionDeviceRevealSecrets,
	} {
		require.False(t, Can(nil, a, nil), "anonymous (nil) must be denied for %s", a)
		require.False(t, Can(&User{}, a, nil), "zero-User must be denied for %s", a)
	}
}
