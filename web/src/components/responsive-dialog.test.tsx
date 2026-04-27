import { describe, it } from 'vitest'

// Implementation: Plan 06 (frontend-shell).
// ResponsiveDialog wraps shadcn's Dialog (md+) and Sheet (<md) so the same
// CRUD surface auto-swaps between modal and bottom-sheet. PROJECT.md requires
// every CRUD to be a dialog; this component is the canonical implementation.
describe.skip('ResponsiveDialog (Plan 06)', () => {
  it('renders Dialog on md+ viewport', () => {})
  it('renders Sheet bottom on <md viewport', () => {})
  it('forwards open/onOpenChange to underlying primitive', () => {})
})
