import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { ThemeProvider } from './theme-provider'
import { AccountMenu } from './shell/account-menu'

function renderMenu(opts: {
  role?: 'admin' | 'viewer'
  onChangePassword?: () => void
  onSignOut?: () => void
} = {}) {
  const role = opts.role ?? 'admin'
  return render(
    <ThemeProvider>
      <AccountMenu
        userEmail="alice@example.com"
        userRole={role}
        onChangePassword={opts.onChangePassword ?? (() => {})}
        onSignOut={opts.onSignOut ?? (() => {})}
      />
    </ThemeProvider>,
  )
}

describe('AccountMenu (Plan 11)', () => {
  it('shows Change password, Theme, Sign out for admin', async () => {
    renderMenu({ role: 'admin' })
    const trigger = screen.getByRole('button', { name: /account menu/i })
    await userEvent.click(trigger)
    expect(screen.getByText('Change password')).toBeInTheDocument()
    expect(screen.getByText('Theme')).toBeInTheDocument()
    expect(screen.getByText('Sign out')).toBeInTheDocument()
  })

  it('viewer also sees Change password (AUTH-06: viewers can self-edit)', async () => {
    renderMenu({ role: 'viewer' })
    const trigger = screen.getByRole('button', { name: /account menu/i })
    await userEvent.click(trigger)
    expect(screen.getByText('Change password')).toBeInTheDocument()
    expect(screen.getByText('Sign out')).toBeInTheDocument()
  })

  it('shows the email and role in the menu label', async () => {
    renderMenu({ role: 'viewer' })
    const trigger = screen.getByRole('button', { name: /account menu/i })
    await userEvent.click(trigger)
    expect(screen.getByText('alice@example.com')).toBeInTheDocument()
    expect(screen.getByText('viewer')).toBeInTheDocument()
  })

  it('opens change-password dialog on click (calls onChangePassword)', async () => {
    const onChangePassword = vi.fn()
    renderMenu({ role: 'admin', onChangePassword })
    const trigger = screen.getByRole('button', { name: /account menu/i })
    await userEvent.click(trigger)
    await userEvent.click(screen.getByText('Change password'))
    expect(onChangePassword).toHaveBeenCalledTimes(1)
  })
})
