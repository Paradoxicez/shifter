import { test, expect } from '@playwright/test'

/**
 * Plan 06-05 — User management E2E (USER-01..04 + D-23/D-24/D-25/D-26/D-27).
 *
 * Happy path: admin adds a viewer with a random-show-once password, copies
 * the password from the share-credentials panel, logs out, logs in as the
 * new viewer using the copied password, and is forced to change it on
 * first sign-in (must_change_password=true → ChangePasswordDialog).
 *
 * Storage state pre-authenticates the admin session; operators regenerate
 * the fixture via
 *   pnpm exec playwright open --save-storage=playwright/fixtures/admin-session.json
 *   http://localhost:8080
 * before this spec passes against a real server. The spec STRUCTURE —
 * locators, click sequence, assertion shapes — is the contract; the cookie
 * material is environmental.
 */

test.describe('User management (admin)', () => {
  test.use({ storageState: 'playwright/fixtures/admin-session.json' })

  test('admin adds viewer; viewer logs in + force-rotates', async ({
    page,
    browserName,
  }) => {
    // Each browser run gets a unique email so re-runs don't 409.
    const stamp = Date.now().toString(36)
    const email = `ben-${stamp}-${browserName}@acme.io`

    // --- Admin: open Users page ---
    await page.goto('/settings/users')
    await expect(page.getByRole('heading', { name: /Users \(/ })).toBeVisible()

    // --- Admin: Add user dialog (step 1) ---
    await page.getByRole('button', { name: 'Add user' }).click()
    await expect(page.getByText('Add user')).toBeVisible()
    await page.getByLabel('Name').fill('Ben Smith')
    await page.getByLabel('Email').fill(email)
    // Default role is Viewer (RadioGroup default), no need to click.
    await page.getByRole('button', { name: 'Create user' }).click()

    // --- Admin: Step 2 share-credentials panel ---
    await expect(page.getByText('User created')).toBeVisible()
    const passwordCell = page.getByTestId('password-block')
    await expect(passwordCell).toBeVisible()
    const password = (await passwordCell.textContent())?.trim() ?? ''
    expect(password.length).toBeGreaterThanOrEqual(16)

    await page.getByRole('button', { name: "I've shared this" }).click()
    await expect(page.getByText('User created')).not.toBeVisible()

    // --- Admin: Log out ---
    // The account menu drops "Sign out" — exact selector depends on shell.
    // Fall back to navigating to /login directly via API logout if no menu.
    await page.evaluate(async () => {
      await fetch('/api/auth/logout', {
        method: 'POST',
        headers: { 'X-Requested-With': 'shifter' },
        credentials: 'same-origin',
      })
    })

    // --- New viewer: log in ---
    await page.goto('/login')
    await page.getByLabel(/Email/i).fill(email)
    await page.getByLabel(/Password/i).fill(password)
    await page.getByRole('button', { name: /Sign in/i }).click()

    // --- New viewer: forced password rotation (must_change_password=true) ---
    // Phase 1's ChangePasswordDialog renders on first login. Copy taken from
    // existing 01-* tests.
    await expect(page.getByText(/Change your password|password/i)).toBeVisible({
      timeout: 10_000,
    })

    // Set a new strong password.
    const newPassword = 'NewStrongPass-1234!'
    // Inputs vary; aim for stable labels.
    const newPwInput = page
      .getByLabel(/New password/i)
      .or(page.locator('input[name="new_password"]'))
    await newPwInput.fill(newPassword)
    const confirmInput = page
      .getByLabel(/Confirm/i)
      .or(page.locator('input[name="new_password_confirm"]'))
    if (await confirmInput.count()) await confirmInput.fill(newPassword)
    await page.getByRole('button', { name: /Save|Change password/i }).click()

    // --- Viewer: lands on dashboard ---
    await expect(page).toHaveURL(/\/$/)
  })
})
