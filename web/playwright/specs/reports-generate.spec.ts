/**
 * Reports — generate-once flow — Plan 05-09 Task 2
 *
 * E2E spec: login → /reports → configure single-meter monthly → Generate
 * → CSV + Excel tiles immediately available → PDF tile polls to ready
 * → Sonner toast fires.
 */

import { test, expect } from '@playwright/test'

test.describe('Reports — generate-once flow', () => {
  test('login → /reports → configure single-meter monthly → Generate → download CSV + Excel + PDF', async ({ page }) => {
    await page.goto('/login')
    await page.fill('[name=email]', 'admin@test.local')
    await page.fill('[name=password]', 'TestPass123!')
    await page.click('button[type=submit]')

    await page.goto('/reports')
    await expect(page.getByText('Configure report')).toBeVisible()

    // Select single-meter scope
    await page.click('label:has-text("Single meter")')

    // Open combobox and select first meter
    await page.getByRole('combobox').click()
    await page.waitForSelector('[role=option]')
    await page.getByRole('option').first().click()

    // Select monthly range
    await page.getByRole('button', { name: 'Monthly' }).click()

    // Generate
    await page.getByRole('button', { name: 'Generate report' }).click()
    await expect(page.getByText('Report ready')).toBeVisible({ timeout: 10_000 })

    // CSV + Excel tiles enabled immediately
    await expect(page.getByRole('link', { name: /Download CSV/ })).toBeVisible()
    await expect(page.getByRole('link', { name: /Download Excel/ })).toBeVisible()

    // PDF tile starts as Generating…
    await expect(page.getByText('Generating PDF…')).toBeVisible()

    // Wait for PDF to become ready (poll runs every 2s, budget 30s)
    await expect(page.getByRole('link', { name: /Download PDF/ })).toBeVisible({ timeout: 30_000 })

    // Toast fires with the correct copy
    await expect(page.getByText('Your PDF is ready')).toBeVisible()
  })

  test('Configure another button returns to config panel (D-07)', async ({ page }) => {
    await page.goto('/login')
    await page.fill('[name=email]', 'admin@test.local')
    await page.fill('[name=password]', 'TestPass123!')
    await page.click('button[type=submit]')

    await page.goto('/reports')
    await expect(page.getByText('Configure report')).toBeVisible()

    // Quick generate with default settings (all meters monthly)
    await page.getByRole('button', { name: 'Generate report' }).click()

    // Wait for result panel
    await expect(page.getByText('Report ready')).toBeVisible({ timeout: 10_000 })

    // Go back to config — D-07: result panel is lost
    await page.getByRole('button', { name: 'Configure another' }).click()
    await expect(page.getByText('Configure report')).toBeVisible()
  })
})
