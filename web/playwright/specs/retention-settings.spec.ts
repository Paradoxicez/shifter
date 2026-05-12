import { test, expect } from '@playwright/test'

/**
 * Plan 05-11 — Settings → Data Retention (D-09 / DATA-13)
 *
 * These specs validate the Data Retention card end-to-end against a running
 * Shifter instance (bundled or external compose stack). The admin session is
 * pre-authenticated via admin-session.json storageState (same pattern as
 * dashboard-live-update.spec.ts).
 *
 * Test matrix:
 *   1. Admin edits raw retention 90→60 days: card updates, success toast shown.
 *   2. Viewer sees read-only values with no Edit buttons (AUTH-06 frontend hide).
 */

test.describe('Settings — Data Retention (D-09 / DATA-13)', () => {
  test(
    'admin edits raw retention 90→60 days → card reflects new value',
    async ({ page }) => {
      test.use({ storageState: 'playwright/fixtures/admin-session.json' })

      await page.goto('/settings')

      // Data Retention card must be visible
      await expect(page.getByText('Data Retention')).toBeVisible()
      await expect(page.getByText('Raw measurements')).toBeVisible()

      // Click the Edit button for Raw measurements row
      await page
        .getByRole('button', { name: 'Edit Raw measurements' })
        .click()

      // Dialog opens
      await expect(page.getByText('Edit Raw measurements')).toBeVisible()

      // Clear the input and type 60
      const input = page.getByRole('spinbutton')
      await input.fill('60')

      // Save
      await page.getByRole('button', { name: 'Save retention settings' }).click()

      // Success toast
      await expect(page.getByText('Retention settings saved.')).toBeVisible()

      // Card reflects the new value
      await expect(page.getByText('60 days')).toBeVisible()
    }
  )

  test(
    'viewer sees read-only retention values, no Edit button (AUTH-06 frontend hide)',
    async ({ page }) => {
      test.use({ storageState: 'playwright/fixtures/viewer-session.json' })

      await page.goto('/settings')

      // Data Retention card must be visible with values
      await expect(page.getByText('Data Retention')).toBeVisible()
      await expect(page.getByText('Raw measurements')).toBeVisible()

      // No Edit buttons should be present for any retention row
      await expect(
        page.getByRole('button', { name: /^Edit /i })
      ).toHaveCount(0)

      // Yearly row should show "Never expires" (or a formatted value)
      await expect(page.getByText('Yearly aggregate')).toBeVisible()
    }
  )
})
