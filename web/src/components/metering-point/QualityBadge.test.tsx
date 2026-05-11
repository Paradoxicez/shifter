/**
 * QualityBadge tests — Plan 04-09 Task 1
 */

import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { QualityBadge } from './QualityBadge'

// Mock useNavigate at module level
const navigateMock = vi.fn()
vi.mock('react-router-dom', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router-dom')>()
  return { ...actual, useNavigate: () => navigateMock }
})

function wrap(ui: React.ReactElement) {
  return render(<MemoryRouter>{ui}</MemoryRouter>)
}

describe('QualityBadge', () => {
  beforeEach(() => {
    vi.resetAllMocks()
  })

  it('renders "All ok" indicator when flaggedCount=0', () => {
    wrap(<QualityBadge flaggedCount={0} windowSize={100} />)
    expect(screen.getByText('All ok')).toBeTruthy()
  })

  it('does not render a clickable button when flaggedCount=0', () => {
    wrap(<QualityBadge flaggedCount={0} windowSize={100} />)
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('renders flagged count text when flaggedCount>0', () => {
    wrap(<QualityBadge flaggedCount={3} windowSize={100} />)
    expect(screen.getByText(/3 of last 100 uplinks flagged/)).toBeTruthy()
  })

  it('renders a clickable button when flaggedCount>0', () => {
    wrap(<QualityBadge flaggedCount={5} windowSize={100} />)
    expect(screen.getByRole('button')).toBeTruthy()
  })

  it('navigates with tab=uplinks on click when flaggedCount>0', () => {
    wrap(<QualityBadge flaggedCount={3} windowSize={100} />)
    const btn = screen.getByRole('button')
    fireEvent.click(btn)
    expect(navigateMock).toHaveBeenCalledWith(expect.stringContaining('tab=uplinks'))
  })

  it('renders "All ok" when windowSize=0', () => {
    wrap(<QualityBadge flaggedCount={0} windowSize={0} />)
    expect(screen.getByText('All ok')).toBeTruthy()
  })
})
