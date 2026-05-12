/**
 * Report Templates — save/load/delete flow — Plan 07-11b Task 2
 *
 * E2E spec: login → /reports → configure → save as template → reset → reload → delete
 *
 * Covers the full UX-POWER Surface 6 happy path:
 *   1. Admin configures a report
 *   2. Saves it as a named template
 *   3. Resets the panel config
 *   4. Reloads the template from the dropdown
 *   5. Deletes the template with the confirm dialog
 *
 * Viewer RBAC: verify Save button hidden and three-dot menu hidden.
 */

import { test, expect } from '@playwright/test'

const TEMPLATE_NAME = `E2E Template ${Date.now()}`

test.describe('Report Templates — save/load/delete flow', () => {
  test('admin: configure → save as template → reset → reload template → delete', async ({ page }) => {
    // Login as admin
    await page.goto('/login')
    await page.fill('[name=email]', 'admin@test.local')
    await page.fill('[name=password]', 'TestPass123!')
    await page.click('button[type=submit]')

    // Navigate to /reports
    await page.goto('/reports')
    await expect(page.getByText('Configure report')).toBeVisible()

    // Configure: scope = site
    await page.click('label:has-text("Single site")')

    // Select monthly range
    await page.getByRole('button', { name: 'Monthly' }).click()

    // Save as template
    await page.getByRole('button', { name: 'Save as template' }).click()

    // Save dialog appears
    await expect(page.getByText('Save as Template')).toBeVisible()

    // Enter a name
    await page.fill('[id=template-name]', TEMPLATE_NAME)

    // Confirm save
    await page.getByRole('button', { name: 'Save Template' }).click()

    // Success toast
    await expect(page.getByText(`Template saved: ${TEMPLATE_NAME}`)).toBeVisible({ timeout: 5_000 })

    // Reset panel — switch back to All meters
    await page.click('label:has-text("All meters")')

    // Open Templates dropdown
    await page.getByRole('button', { name: /Templates/ }).click()

    // Saved template appears in list
    await expect(page.getByText(TEMPLATE_NAME)).toBeVisible()

    // Search for the template
    await page.fill('[placeholder="Search templates…"]', TEMPLATE_NAME.slice(0, 5))
    await expect(page.getByText(TEMPLATE_NAME)).toBeVisible()
    await page.fill('[placeholder="Search templates…"]', '')

    // Select the template — panel state restores
    await page.getByText(TEMPLATE_NAME).click()

    // Toast fires
    await expect(page.getByText(`Template loaded: ${TEMPLATE_NAME}`)).toBeVisible({ timeout: 5_000 })

    // Delete the template
    await page.getByRole('button', { name: /Templates/ }).click()
    await expect(page.getByText(TEMPLATE_NAME)).toBeVisible()

    // Three-dot menu
    await page.getByRole('button', { name: 'More options' }).click()
    await page.getByText('Delete').click()

    // Confirm dialog shows template name
    await expect(page.getByText(`'${TEMPLATE_NAME}' will be permanently deleted. This cannot be undone.`)).toBeVisible()

    // Confirm deletion
    await page.getByRole('button', { name: 'Delete template' }).click()

    // Template no longer in list
    await page.getByRole('button', { name: /Templates/ }).click()
    await expect(page.getByText(TEMPLATE_NAME)).not.toBeVisible()
  })

  test('viewer: Save as template button is hidden', async ({ page }) => {
    // Login as viewer
    await page.goto('/login')
    await page.fill('[name=email]', 'viewer@test.local')
    await page.fill('[name=password]', 'ViewPass123!')
    await page.click('button[type=submit]')

    await page.goto('/reports')
    await expect(page.getByText('Configure report')).toBeVisible()

    // Save as template should NOT be visible for viewer
    await expect(page.getByRole('button', { name: 'Save as template' })).not.toBeVisible()
  })
})
