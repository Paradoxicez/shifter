import { describe, it } from 'vitest'

// Implementation: Plan 06 (frontend-shell).
// ThemeProvider persists light/dark/system in localStorage and applies the
// `dark` class to <html> when dark is active. matchMedia is mocked in
// src/test-setup.ts to make `system` deterministic in jsdom.
describe.skip('ThemeProvider (Plan 06)', () => {
  it('persists choice in localStorage', () => {})
  it('applies class="dark" to root when dark', () => {})
  it('respects system when set to system', () => {})
})
