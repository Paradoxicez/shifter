package auth

import "testing"

// TestAccount_ChangePassword — An authenticated user can change their own
// password by providing the current password and a new password; subsequent
// logins succeed only with the new password.
// Implementation: Plan 11 (account-ui backend handler).
func TestAccount_ChangePassword(t *testing.T) {
	t.Skip("Plan 11: change-password endpoint pending")
}

// TestAccount_ChangePassword_RevokeOtherSessions — After a successful password
// change, all OTHER active sessions for that user are revoked
// (defense-in-depth). The session that performed the change remains valid.
// Implementation: Plan 11 (account-ui backend handler).
func TestAccount_ChangePassword_RevokeOtherSessions(t *testing.T) {
	t.Skip("Plan 11: revoke other sessions on password change pending")
}

// TestWizardAdmin_NoForceChange — The admin user created by the install
// wizard does NOT see the force-change-password gate on first login (their
// password was set by them, not by an operator).
// Implementation: Plan 11 (account-ui backend handler).
func TestWizardAdmin_NoForceChange(t *testing.T) {
	t.Skip("Plan 11: wizard admin force-change exemption pending")
}
