import { act, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { ThemeProvider, useTheme } from './theme-provider'

function Probe() {
  const { theme, setTheme } = useTheme()
  return (
    <div>
      <span data-testid="current">{theme}</span>
      <button data-testid="dark" type="button" onClick={() => setTheme('dark')}>
        dark
      </button>
      <button data-testid="light" type="button" onClick={() => setTheme('light')}>
        light
      </button>
    </div>
  )
}

describe('ThemeProvider', () => {
  beforeEach(() => {
    localStorage.clear()
    document.documentElement.classList.remove('dark')
  })
  afterEach(() => {
    localStorage.clear()
    document.documentElement.classList.remove('dark')
  })

  it('default theme is system', () => {
    const { getByTestId } = render(
      <ThemeProvider>
        <Probe />
      </ThemeProvider>,
    )
    expect(getByTestId('current').textContent).toBe('system')
  })

  it('setting dark adds class="dark" to root and persists', () => {
    const { getByTestId } = render(
      <ThemeProvider>
        <Probe />
      </ThemeProvider>,
    )
    act(() => {
      getByTestId('dark').click()
    })
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(localStorage.getItem('shifter-theme')).toBe('dark')
  })

  it('setting light removes the dark class', () => {
    const { getByTestId } = render(
      <ThemeProvider>
        <Probe />
      </ThemeProvider>,
    )
    act(() => {
      getByTestId('dark').click()
    })
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    act(() => {
      getByTestId('light').click()
    })
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    expect(localStorage.getItem('shifter-theme')).toBe('light')
  })
})
