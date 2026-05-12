import '@testing-library/jest-dom/vitest'

// Mock matchMedia — jsdom does not implement it; the theme-provider tests
// need it to read the system color-scheme preference (Plan 06).
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  }),
})

// jsdom polyfills for Radix UI primitives (Plan 02-14 Task 2).
// Radix Select / Popover / etc. probe these Element methods on pointer events;
// jsdom doesn't implement them, which surfaces as "target.hasPointerCapture is
// not a function" + "scrollIntoView is not a function" during userEvent.click
// of a SelectTrigger. Mocking them as no-ops is the canonical workaround
// recommended in Radix's own test docs.
// ResizeObserver polyfill — cmdk (Command component) uses ResizeObserver internally;
// jsdom does not implement it. Mock as a no-op so Popover+Command tests don't crash.
if (typeof ResizeObserver === 'undefined') {
  global.ResizeObserver = class ResizeObserver {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
}

if (typeof Element !== 'undefined') {
  if (!Element.prototype.hasPointerCapture) {
    Element.prototype.hasPointerCapture = () => false
  }
  if (!Element.prototype.setPointerCapture) {
    Element.prototype.setPointerCapture = () => {}
  }
  if (!Element.prototype.releasePointerCapture) {
    Element.prototype.releasePointerCapture = () => {}
  }
  if (!Element.prototype.scrollIntoView) {
    Element.prototype.scrollIntoView = () => {}
  }
}
