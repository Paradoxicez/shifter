import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import LoginScreen from './login'

vi.mock('@/lib/auth', async (orig) => {
  const real = await orig<typeof import('@/lib/auth')>()
  return {
    ...real,
    login: vi.fn().mockResolvedValue({
      id: 'u1',
      email: 'a@x',
      role: 'admin',
      must_change_password: false,
    }),
  }
})

function renderLogin() {
  return render(
    <MemoryRouter initialEntries={['/login']}>
      <LoginScreen />
    </MemoryRouter>,
  )
}

describe('Login screen (Plan 23)', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('renders the verbatim heading and description (UI-SPEC §Phase 1 copy table)', () => {
    renderLogin()
    expect(screen.getByText('Sign in to Shifter')).toBeInTheDocument()
    expect(screen.getByText('Enter your email and password to continue.')).toBeInTheDocument()
  })

  it('renders the verbatim form labels', () => {
    renderLogin()
    expect(screen.getByLabelText('Email address')).toBeInTheDocument()
    expect(screen.getByLabelText('Password')).toBeInTheDocument()
  })

  it('renders the verbatim "Sign in" button (English-only, UX-02)', () => {
    renderLogin()
    expect(screen.getByRole('button', { name: 'Sign in' })).toBeInTheDocument()
  })

  it('renders the verbatim footer copy', () => {
    renderLogin()
    expect(
      screen.getByText('Forgot password? Contact your administrator.'),
    ).toBeInTheDocument()
  })

  it('uses the navy primary class on the submit button (UX-02 navy palette)', () => {
    renderLogin()
    const btn = screen.getByRole('button', { name: 'Sign in' })
    // shadcn default Button variant uses bg-primary which resolves to OKLCH
    // navy via theme.css. Verify the class is present rather than the runtime
    // resolution (jsdom doesn't compute OKLCH).
    expect(btn.className).toMatch(/bg-primary/)
  })

  it('does not override font-family inline (UX-02 Inter Variable from index.css)', () => {
    renderLogin()
    // Plan 06 sets `body { font-family: 'Inter Variable', system-ui, ... }`
    // in index.css. jsdom doesn't process imported stylesheets at render time;
    // assert the component does not override font-family inline so the body
    // rule wins in the production browser.
    const heading = screen.getByText('Sign in to Shifter')
    expect(heading.style.fontFamily).toBe('')
  })

  it('shows the verbatim 401 error message', async () => {
    const lib = await import('@/lib/auth')
    const { ApiError } = await import('@/lib/api')
    ;(lib.login as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ApiError(401, 'bad', { error: 'bad_credentials' }),
    )
    renderLogin()
    await userEvent.type(screen.getByLabelText('Email address'), 'a@x')
    await userEvent.type(screen.getByLabelText('Password'), 'wrong')
    await userEvent.click(screen.getByRole('button', { name: 'Sign in' }))
    expect(
      await screen.findByText("That email and password don't match. Try again."),
    ).toBeInTheDocument()
  })

  it('shows the verbatim 429 rate-limit message (AUTH-04)', async () => {
    const lib = await import('@/lib/auth')
    const { ApiError } = await import('@/lib/api')
    ;(lib.login as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ApiError(429, 'rate', { error: 'rate_limited' }),
    )
    renderLogin()
    await userEvent.type(screen.getByLabelText('Email address'), 'a@x')
    await userEvent.type(screen.getByLabelText('Password'), 'wrong')
    await userEvent.click(screen.getByRole('button', { name: 'Sign in' }))
    expect(
      await screen.findByText('Too many failed attempts. Try again in 5 minutes.'),
    ).toBeInTheDocument()
  })
})
