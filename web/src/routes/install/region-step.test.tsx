import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { RegionStep } from './region-step'

// PITFALLS §8 / VALIDATION.md TestStep3 frontend equivalent: AS923-2 must be
// pre-selected for the Thailand operator base. We mock postStep3 so the test
// exercises only the rendered UI and the apiFetch boundary.
vi.mock('@/lib/install', async (orig) => {
  const real = await orig<typeof import('@/lib/install')>()
  return { ...real, postStep3: vi.fn().mockResolvedValue({ current_step: 4 }) }
})

describe('RegionStep (Plan 16)', () => {
  it('defaults AS923-2 for Thailand and shows the pre-selection hint', () => {
    render(<RegionStep onAdvance={() => {}} />)
    expect(screen.getByText(/We pre-selected AS923-2/)).toBeInTheDocument()
  })

  it('renders Next button labeled "Next" idle', () => {
    render(<RegionStep onAdvance={() => {}} />)
    expect(screen.getByRole('button', { name: /next/i })).toBeInTheDocument()
  })

  it('Next click invokes postStep3 with as923_2', async () => {
    const onAdvance = vi.fn()
    const lib = await import('@/lib/install')
    render(<RegionStep onAdvance={onAdvance} />)
    await userEvent.click(screen.getByRole('button', { name: /next/i }))
    expect(lib.postStep3).toHaveBeenCalledWith({ name: 'as923_2' })
    expect(onAdvance).toHaveBeenCalled()
  })
})
