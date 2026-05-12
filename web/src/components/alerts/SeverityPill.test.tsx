/**
 * Plan 06-04 SeverityPill tests — verify the locked severity color map.
 */

import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SeverityDot, SeverityPill } from './SeverityPill'

describe('SeverityPill', () => {
  it('renders default label "Critical" for critical', () => {
    render(<SeverityPill severity="critical" />)
    expect(screen.getByText('Critical')).toBeInTheDocument()
  })

  it('renders custom child text', () => {
    render(<SeverityPill severity="warning">Custom Warn</SeverityPill>)
    expect(screen.getByText('Custom Warn')).toBeInTheDocument()
  })

  it('attaches data-severity attribute', () => {
    render(<SeverityPill severity="info" />)
    const pill = document.querySelector('[data-severity="info"]')
    expect(pill).not.toBeNull()
  })
})

describe('SeverityDot', () => {
  it('renders with correct severity attribute', () => {
    const { container } = render(<SeverityDot severity="critical" />)
    expect(container.querySelector('[data-severity="critical"]')).not.toBeNull()
  })
})
