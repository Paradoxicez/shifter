import { describe, it } from 'vitest'

// Implementation: Plan 11 (account-ui).
// AccountMenu hides admin-only items from viewer sessions. This is the
// frontend half of AUTH-06 (the backend half is TestRBAC_ViewerForbidden in
// internal/http/rbac_test.go).
describe.skip('AccountMenu (Plan 11)', () => {
  it('hides admin-only items from viewer', () => {})
  it('shows Change password and Sign out', () => {})
  it('opens change-password dialog on click', () => {})
})
