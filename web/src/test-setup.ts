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
