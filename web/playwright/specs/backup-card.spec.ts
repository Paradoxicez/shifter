import { test, expect } from '@playwright/test'

/**
 * Plan 06-10 — Backup status card E2E (SETT-05).
 *
 * Validates the BackupStatusCard on /settings:
 *   1. Admin sees backup card with freshness dot
 *   2. Admin sees threshold form (inputs + Save button)
 *   3. Admin can update thresholds (PATCH /api/settings/backup/thresholds)
 *   4. Viewer sees backup card but no threshold form
 *   5. Restore guidance card is visible to both roles
 *
 * The spec tolerates a never-run state — when no backups have been created,
 * the never_run indicator proves the surface renders correctly.
 */

test.describe('Backup status card (admin)', () => {
  test.use({ storageState: 'playwright/fixtures/admin-session.json' })

  test('backup card renders with freshness dot and threshold form', async ({ page }) => {
    await page.goto('/settings')

    // Backup status card must be present.
    const card = page.getByTestId('backup-status-card')
    await expect(card).toBeVisible()

    // Freshness dot must be present (color depends on DB state — any status is valid).
    const dot = page.getByTestId('backup-freshness-dot')
    await expect(dot).toBeVisible()
    await expect(dot).toHaveAttribute('data-status', /ok|warn|crit/)

    // Age text must be present.
    await expect(page.getByTestId('backup-age-text')).toBeVisible()
  })

  test('admin sees threshold form with warn/crit inputs', async ({ page }) => {
    await page.goto('/settings')

    // Threshold form visible for admin.
    const form = page.getByTestId('backup-threshold-form')
    await expect(form).toBeVisible()

    // Warn and crit inputs must be populated.
    const warnInput = page.getByTestId('input-warn-hours')
    const critInput = page.getByTestId('input-crit-hours')
    await expect(warnInput).toBeVisible()
    await expect(critInput).toBeVisible()

    // Both inputs must have positive numeric values.
    const warnVal = await warnInput.inputValue()
    const critVal = await critInput.inputValue()
    expect(Number(warnVal)).toBeGreaterThan(0)
    expect(Number(critVal)).toBeGreaterThan(0)
    expect(Number(warnVal)).toBeLessThan(Number(critVal))
  })

  test('admin can update thresholds and see success feedback', async ({ page }) => {
    await page.goto('/settings')

    const form = page.getByTestId('backup-threshold-form')
    await expect(form).toBeVisible()

    // Set warn=12, crit=72 (valid: warn < crit, both > 0, both <= 8760).
    await page.getByTestId('input-warn-hours').fill('12')
    await page.getByTestId('input-crit-hours').fill('72')

    // Save button should now be enabled (form is dirty).
    const saveBtn = page.getByTestId('save-thresholds-btn')
    await expect(saveBtn).toBeEnabled()
    await saveBtn.click()

    // Toast: "Thresholds updated" must appear.
    await expect(page.getByText('Thresholds updated')).toBeVisible({ timeout: 5000 })

    // Restore defaults to avoid test pollution.
    await page.getByTestId('input-warn-hours').fill('24')
    await page.getByTestId('input-crit-hours').fill('168')
    await page.getByTestId('save-thresholds-btn').click()
    await expect(page.getByText('Thresholds updated')).toBeVisible({ timeout: 5000 })
  })
})

test.describe('Backup status card (viewer)', () => {
  test.use({ storageState: 'playwright/fixtures/viewer-session.json' })

  test('viewer sees backup card but no threshold form', async ({ page }) => {
    await page.goto('/settings')

    // Backup card must render for viewer too.
    await expect(page.getByTestId('backup-status-card')).toBeVisible()
    await expect(page.getByTestId('backup-freshness-dot')).toBeVisible()

    // Threshold form must NOT be present.
    await expect(page.getByTestId('backup-threshold-form')).not.toBeVisible()
  })
})

test.describe('Restore guidance card', () => {
  test.use({ storageState: 'playwright/fixtures/admin-session.json' })

  test('restore guidance card visible with external link', async ({ page }) => {
    await page.goto('/settings')

    const card = page.getByTestId('restore-guidance-card')
    await expect(card).toBeVisible()

    const link = page.getByTestId('restore-docs-link')
    await expect(link).toBeVisible()
    await expect(link).toHaveAttribute('target', '_blank')
  })
})
