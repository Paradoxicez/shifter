/**
 * JsonTree tests — Plan 04-09 Task 1
 *
 * TDD RED: write failing tests for JsonTree component.
 */

import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { JsonTree } from './JsonTree'

describe('JsonTree', () => {
  it('renders string values with success class', () => {
    render(<JsonTree value="hello" name="greeting" />)
    const el = screen.getByText('"hello"')
    expect(el.className).toContain('text-success')
  })

  it('renders number values with info class', () => {
    render(<JsonTree value={42} name="count" />)
    const el = screen.getByText('42')
    expect(el.className).toContain('text-info')
  })

  it('renders boolean values with warning class', () => {
    render(<JsonTree value={true} name="flag" />)
    const el = screen.getByText('true')
    expect(el.className).toContain('text-warning')
  })

  it('renders null values with muted italic class', () => {
    render(<JsonTree value={null} name="nothing" />)
    const el = screen.getByText('null')
    expect(el.className).toContain('text-muted-foreground')
    expect(el.className).toContain('italic')
  })

  it('renders nested object with expand/collapse button', () => {
    render(<JsonTree value={{ a: 1, b: 'x' }} name="data" defaultOpen={true} />)
    // Should show count
    expect(screen.getByText('(2 items)')).toBeTruthy()
    // Should show nested values
    expect(screen.getByText('1')).toBeTruthy()
    expect(screen.getByText('"x"')).toBeTruthy()
  })

  it('renders collapsed child objects by default (depth=1)', () => {
    // Root at depth=0 auto-opens. Child at depth=1 stays collapsed.
    render(<JsonTree value={{ child: { a: 1 } }} name="root" defaultOpen={true} />)
    // "a" is nested at depth=2 inside collapsed "child" node, so it's not rendered
    expect(screen.queryByText('1')).toBeNull()
    // Both the root and the "child" node show "(1 item)" count — use getAllByText
    const countLabels = screen.getAllByText('(1 item)')
    expect(countLabels.length).toBeGreaterThanOrEqual(1)
  })

  it('expands child object when its button clicked', () => {
    render(<JsonTree value={{ e: { f: 2 } }} name="parent" defaultOpen={true} />)
    // Root auto-opens (defaultOpen=true), showing "e" collapsed (depth=1).
    // Two buttons: root "parent(1 item)" and child "e(1 item)".
    // Find the CHILD button for "e" — its text starts with "e" not "parent".
    const buttons = screen.getAllByRole('button')
    // The child button contains "e" as key name but NOT "parent"
    const childButton = buttons.find(
      b => b.textContent?.includes('e') && !b.textContent?.includes('parent')
    )
    expect(childButton).toBeTruthy()
    if (childButton) {
      fireEvent.click(childButton)
      // After expanding "e", the value of "f" (2) should appear in the DOM
      const twos = screen.getAllByText(/^2$/)
      expect(twos.length).toBeGreaterThan(0)
    }
  })

  it('renders all 5 primitive types in complex object', () => {
    render(
      <JsonTree
        value={{ a: 1, b: 'x', c: true, d: null, e: { f: 2 } }}
        name="root"
        defaultOpen={true}
      />
    )
    expect(screen.getByText('1')).toBeTruthy()
    expect(screen.getByText('"x"')).toBeTruthy()
    expect(screen.getByText('true')).toBeTruthy()
    expect(screen.getByText('null')).toBeTruthy()
    // "e" is an object, collapsed by default (depth=1, name !== 'extra')
    expect(screen.getByText('(1 item)')).toBeTruthy()
  })

  it('auto-opens nodes named "extra" regardless of depth', () => {
    render(
      <JsonTree
        value={{ extra: { foo: 'bar' } }}
        name="root"
        defaultOpen={true}
      />
    )
    // 'extra' sub-tree should be open by default
    expect(screen.getByText('"bar"')).toBeTruthy()
  })

  it('renders singular "(1 item)" count', () => {
    render(<JsonTree value={{ x: 1 }} name="root" defaultOpen={true} />)
    expect(screen.getByText('(1 item)')).toBeTruthy()
  })

  it('renders plural "(2 items)" count', () => {
    render(<JsonTree value={{ x: 1, y: 2 }} name="root" defaultOpen={true} />)
    expect(screen.getByText('(2 items)')).toBeTruthy()
  })

  it('renders array entries with index keys', () => {
    render(<JsonTree value={[10, 20]} name="arr" defaultOpen={true} />)
    expect(screen.getByText('10')).toBeTruthy()
    expect(screen.getByText('20')).toBeTruthy()
  })
})
