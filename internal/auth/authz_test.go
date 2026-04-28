package auth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCan_AdminAllowsAll — A user with role=admin returns true for every
// known action in the Can() permission matrix.
// AUTH-06 / RESEARCH §Pattern 16.
func TestCan_AdminAllowsAll(t *testing.T) {
	admin := &User{ID: "u1", Role: "admin"}
	for _, a := range []Action{
		ActionConnectionEdit,
		ActionConnectionTest,
		ActionAccountSelfEdit,
		ActionHealthDetailed,
		ActionUserManage,
		ActionDeviceCreate,
		ActionDeviceUpdate,
		ActionDeviceDelete,
		ActionAuditView,
	} {
		require.True(t, Can(admin, a, nil), "admin must be able to %s", a)
	}
}

// TestCan_ViewerSelfEditAllowed — viewer role can edit own account and run
// the Test Connection probe (the read-only Settings page exposes it).
func TestCan_ViewerSelfEditAllowed(t *testing.T) {
	viewer := &User{ID: "u2", Role: "viewer"}
	require.True(t, Can(viewer, ActionAccountSelfEdit, nil))
	require.True(t, Can(viewer, ActionConnectionTest, nil))
}

// TestCan_ViewerConnectionEditDenied — viewer role denies every mutating /
// admin-only action.
func TestCan_ViewerConnectionEditDenied(t *testing.T) {
	viewer := &User{ID: "u2", Role: "viewer"}
	require.False(t, Can(viewer, ActionConnectionEdit, nil))
	require.False(t, Can(viewer, ActionUserManage, nil))
	require.False(t, Can(viewer, ActionHealthDetailed, nil))
	require.False(t, Can(viewer, ActionDeviceCreate, nil))
	require.False(t, Can(viewer, ActionDeviceUpdate, nil))
	require.False(t, Can(viewer, ActionDeviceDelete, nil))
	require.False(t, Can(viewer, ActionAuditView, nil))
}

// TestCan_NilUser — nil user (no session at all) and zero User (empty ID)
// are both denied. Fail-closed default.
func TestCan_NilUser(t *testing.T) {
	require.False(t, Can(nil, ActionAccountSelfEdit, nil))
	require.False(t, Can(&User{}, ActionAccountSelfEdit, nil)) // empty ID treated as no user
}

// TestCan_UnknownRole — a future role added to the user table without being
// declared in roleBundles is denied (T-10-04 fail-closed).
func TestCan_UnknownRole(t *testing.T) {
	u := &User{ID: "u3", Role: "ghost"}
	require.False(t, Can(u, ActionAccountSelfEdit, nil))
}
