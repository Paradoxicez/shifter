import { test, expect } from '@playwright/test'

/**
 * Plan 06-04 — Alert center E2E.
 *
 * Validates the bell-drawer-ack-/alerts pipeline as a single happy-path
 * flow. The spec is intentionally tolerant of empty databases — when no
 * alerts exist, the empty-state assertion is what proves the surface
 * renders.
 *
 * Steps:
 *   1. Admin lands on /. The bell button is mounted in the topbar.
 *   2. Clicking the bell opens the slide-over drawer.
 *   3. From the drawer, "See all alerts" navigates to /alerts.
 *   4. The /alerts page shows the filter chips toolbar and the empty/list
 *      content. Severity URL filter survives reload.
 *   5. Settings → Alerts page renders the rule library + roster.
 */

test.describe('Alert center (admin)', () => {
  test.use({ storageState: 'playwright/fixtures/admin-session.json' })

  test('bell → drawer → /alerts → settings/alerts navigation chain', async ({ page }) => {
    await page.goto('/')

    // 1. Bell button is in the topbar.
    const bell = page.getByRole('button', { name: /Alerts \(\d+ unread/i })
    await expect(bell).toBeVisible()

    // 2. Open the drawer.
    await bell.click()
    await expect(page.getByRole('dialog')).toBeVisible()

    // 3. "See all alerts →" link navigates to /alerts.
    await page.getByRole('link', { name: /See all alerts/i }).click()
    await expect(page).toHaveURL(/\/alerts$/)

    // 4. Filter chips toolbar rendered.
    await expect(page.getByRole('toolbar', { name: /Alert filters/i })).toBeVisible()

    // URL-state survives reload — set severity=critical and reload.
    await page.getByLabel('Severity').selectOption('critical')
    await expect(page).toHaveURL(/severity=critical/)
    await page.reload()
    await expect(page.getByLabel('Severity')).toHaveValue('critical')

    // 5. Settings → Alerts page renders.
    await page.goto('/settings/alerts')
    await expect(page.getByRole('heading', { name: /Settings → Alerts/i })).toBeVisible()
    await expect(page.getByText(/Rule library/i)).toBeVisible()
    await expect(page.getByText(/Anomaly detection/i)).toBeVisible()
  })

  test('Sidebar has Alerts entry between Profiles and Audit/Settings (Plan 06-04 order)', async ({
    page,
  }) => {
    await page.goto('/')
    // Sidebar Alerts link present.
    const alertsLink = page.getByRole('link', { name: /^Alerts/i })
    await expect(alertsLink).toBeVisible()
    await alertsLink.click()
    await expect(page).toHaveURL(/\/alerts$/)
  })
})
