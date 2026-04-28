import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { StatusRow } from './status-row'

describe('StatusRow', () => {
  it('renders reachable state with success color and CheckCircle icon', () => {
    const { container } = render(<StatusRow status="reachable" label="gRPC" detail="47 ms" />)
    expect(screen.getByText('Reachable')).toBeInTheDocument()
    expect(screen.getByText('gRPC')).toBeInTheDocument()
    expect(screen.getByText('47 ms')).toBeInTheDocument()
    expect(container.querySelector('.text-success')).not.toBeNull()
  })

  it('renders unreachable state with destructive color and XCircle icon', () => {
    const { container } = render(
      <StatusRow status="unreachable" label="MQTT" detail="connect: refused" />,
    )
    expect(screen.getByText('Unreachable')).toBeInTheDocument()
    expect(container.querySelector('.text-destructive')).not.toBeNull()
  })

  it('renders skipped state with muted color', () => {
    const { container } = render(<StatusRow status="skipped" label="MQTT" />)
    expect(screen.getByText('Skipped')).toBeInTheDocument()
    expect(container.querySelector('.text-muted-foreground')).not.toBeNull()
  })

  it('detail renders in a font-mono element', () => {
    const { container } = render(<StatusRow status="reachable" label="gRPC" detail="42 ms" />)
    const monoEl = container.querySelector('.font-mono')
    expect(monoEl).not.toBeNull()
    expect(monoEl?.textContent).toBe('42 ms')
  })
})
