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

// TestCan_Phase2_AdminAllowsEveryAction — admin can perform every Phase 2
// mutation AND read action declared by Plan 02-10.
func TestCan_Phase2_AdminAllowsEveryAction(t *testing.T) {
	admin := &User{ID: "u1", Role: "admin"}
	for _, a := range []Action{
		ActionSiteCreate, ActionSiteUpdate, ActionSiteArchive, ActionSiteRestore, ActionSiteRead,
		ActionMeteringPointCreate, ActionMeteringPointUpdate, ActionMeteringPointArchive, ActionMeteringPointRestore, ActionMeteringPointRead,
		ActionDeviceAdd, ActionDeviceDecommission, ActionDeviceRead,
		ActionDeviceProfileCreate, ActionDeviceProfileUpdate, ActionDeviceProfileArchive, ActionDeviceProfileRead,
		ActionMeterSwap, ActionAuditRead,
	} {
		require.True(t, Can(admin, a, nil), "admin must be able to %s", a)
	}
}

// TestCan_Phase2_ViewerMutationsDenied — viewer can READ Phase 2 entities
// but every mutating action is rejected (defense-in-depth on top of
// RequireAction middleware).
//
// Plan 06-07 D-31 update: ActionAuditRead removed from the viewer-reads-allowed
// list (audit browse is admin-only; it contains PII and operator action history).
func TestCan_Phase2_ViewerMutationsDenied(t *testing.T) {
	viewer := &User{ID: "u2", Role: "viewer"}
	// Reads allowed (audit log intentionally absent — admin-only per D-31).
	for _, a := range []Action{
		ActionSiteRead, ActionMeteringPointRead, ActionDeviceRead,
		ActionDeviceProfileRead,
	} {
		require.True(t, Can(viewer, a, nil), "viewer must be able to %s", a)
	}
	// Mutations denied — every Phase 2 mutation must 403 a viewer.
	for _, a := range []Action{
		ActionSiteCreate, ActionSiteUpdate, ActionSiteArchive, ActionSiteRestore,
		ActionMeteringPointCreate, ActionMeteringPointUpdate, ActionMeteringPointArchive, ActionMeteringPointRestore,
		ActionDeviceAdd, ActionDeviceDecommission,
		ActionDeviceProfileCreate, ActionDeviceProfileUpdate, ActionDeviceProfileArchive,
		ActionMeterSwap,
		// D-31 (Plan 06-07): audit browse and export are admin-only.
		ActionAuditRead, ActionAuditExport,
	} {
		require.False(t, Can(viewer, a, nil), "viewer must NOT be able to %s", a)
	}
}

// TestCan_Phase6_AuditBrowseAdminOnly — D-31 (Plan 06-07): audit browse and
// export are admin-only. Both ActionAuditRead and ActionAuditExport must be
// denied for viewer (T-06-07-01 mitigation).
func TestCan_Phase6_AuditBrowseAdminOnly(t *testing.T) {
	admin := &User{ID: "u1", Role: "admin"}
	viewer := &User{ID: "u2", Role: "viewer"}
	// Admin can browse and export.
	require.True(t, Can(admin, ActionAuditRead, nil), "admin must be able to audit.read")
	require.True(t, Can(admin, ActionAuditExport, nil), "admin must be able to audit.export")
	// Viewer denied both (T-06-07-01).
	require.False(t, Can(viewer, ActionAuditRead, nil), "viewer must NOT be able to audit.read (D-31)")
	require.False(t, Can(viewer, ActionAuditExport, nil), "viewer must NOT be able to audit.export (D-31)")
}

// TestCan_Phase6_UserMgmt_AdminEverything — admin can perform every fine-
// grained user-management action declared by Plan 06-05 (D-30 + USER-01..04
// + RBAC defense in depth on top of RequireAction).
func TestCan_Phase6_UserMgmt_AdminEverything(t *testing.T) {
	admin := &User{ID: "u1", Role: "admin"}
	for _, a := range []Action{
		ActionUserList,
		ActionUserCreate,
		ActionUserUpdate,
		ActionUserDisable,
		ActionUserEnable,
		ActionUserChangeRole,
		ActionUserResetPassword,
		ActionUserLogoutEverywhere,
		ActionUserReadSelf,
	} {
		require.True(t, Can(admin, a, nil), "admin must be able to %s", a)
	}
}

// TestCan_Phase6_UserMgmt_ViewerOnlyReadSelf — viewer is allowed
// ActionUserReadSelf only; every mutating action 403s. T-06-05-01
// (viewer-creates-admin) mitigation verified at the role-bundle level.
func TestCan_Phase6_UserMgmt_ViewerOnlyReadSelf(t *testing.T) {
	viewer := &User{ID: "u2", Role: "viewer"}
	require.True(t, Can(viewer, ActionUserReadSelf, nil), "viewer can read self row")
	for _, a := range []Action{
		ActionUserList,
		ActionUserCreate,
		ActionUserUpdate,
		ActionUserDisable,
		ActionUserEnable,
		ActionUserChangeRole,
		ActionUserResetPassword,
		ActionUserLogoutEverywhere,
	} {
		require.False(t, Can(viewer, a, nil), "viewer must NOT be able to %s", a)
	}
}

// TestAuthz_AlertActionsAdminOnlyExceptRead — Plan 06-04 D-11:
// RoleAdmin has every alert.* action; RoleViewer has ActionAlertRead only.
// Mutating actions (ack/snooze/mute/rule_create/update/disable/enable/
// test_fire) are intentionally absent from the viewer bundle so Can()
// returns false (fail-closed) — defense-in-depth on top of RequireAction.
func TestAuthz_AlertActionsAdminOnlyExceptRead(t *testing.T) {
	admin := &User{ID: "u1", Role: "admin"}
	for _, a := range []Action{
		ActionAlertRead,
		ActionAlertAck,
		ActionAlertSnooze,
		ActionAlertMute,
		ActionAlertRuleCreate,
		ActionAlertRuleUpdate,
		ActionAlertRuleDisable,
		ActionAlertRuleEnable,
		ActionAlertTestFire,
	} {
		require.True(t, Can(admin, a, nil), "admin must be able to %s", a)
	}

	viewer := &User{ID: "u2", Role: "viewer"}
	require.True(t, Can(viewer, ActionAlertRead, nil),
		"viewer must be able to READ alerts (D-11)")
	for _, a := range []Action{
		ActionAlertAck,
		ActionAlertSnooze,
		ActionAlertMute,
		ActionAlertRuleCreate,
		ActionAlertRuleUpdate,
		ActionAlertRuleDisable,
		ActionAlertRuleEnable,
		ActionAlertTestFire,
	} {
		require.False(t, Can(viewer, a, nil),
			"viewer must NOT be able to %s (D-11 read-only)", a)
	}
}
