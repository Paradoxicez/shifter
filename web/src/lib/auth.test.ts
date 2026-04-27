import { describe, it } from 'vitest'

// Implementation: Plans 11 (account-ui) + 16 (install-wizard-ui).
// The fetch wrapper attaches X-Requested-With (so the server can distinguish
// XHR from form post during install) and intercepts 401 responses to redirect
// the SPA to /login.
describe.skip('auth fetch wrapper (Plan 11/16)', () => {
  it('redirects on 401', () => {})
  it('attaches X-Requested-With header', () => {})
  it('passes through 2xx JSON unchanged', () => {})
})
