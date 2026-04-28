import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { ResponsiveDialog } from './responsive-dialog'

function setViewport(width: number) {
  Object.defineProperty(window, 'innerWidth', { writable: true, configurable: true, value: width })
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches: query.includes('max-width') ? width < 768 : false,
    media: query,
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }))
}

describe('ResponsiveDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders Dialog with title on md+ viewport', () => {
    setViewport(1024)
    render(
      <ResponsiveDialog open onOpenChange={() => {}} title="Test title" description="Test desc">
        <div>body</div>
      </ResponsiveDialog>,
    )
    expect(screen.getByText('Test title')).toBeInTheDocument()
    expect(screen.getByText('Test desc')).toBeInTheDocument()
    expect(screen.getByText('body')).toBeInTheDocument()
  })

  it('renders Sheet content on <md viewport', () => {
    setViewport(500)
    render(
      <ResponsiveDialog open onOpenChange={() => {}} title="Mobile title">
        <div>mobile body</div>
      </ResponsiveDialog>,
    )
    expect(screen.getByText('Mobile title')).toBeInTheDocument()
    expect(screen.getByText('mobile body')).toBeInTheDocument()
  })
})
