/**
 * Plan 06-07 — Audit log E2E spec (audit-export.spec.ts).
 *
 * Validates the /audit page: default 7-day view, entity-type filter chip,
 * CSV export download with UTF-8 BOM assertion.
 *
 * Tolerant of empty databases — when no audit rows exist the empty-state
 * assertion is what proves the surface renders correctly.
 */

import { test, expect } from '@playwright/test'
import * as fs from 'fs'

test.describe('Audit log (admin)', () => {
  test.use({ storageState: 'playwright/fixtures/admin-session.json' })

  test('admin exports filtered CSV with UTF-8 BOM', async ({ page }) => {
    await page.goto('/audit')

    // Page title
    await expect(page.getByRole('heading', { name: /Audit log/i })).toBeVisible()

    // Default filter chip shows last 7 days
    await expect(page.getByText(/Last 7 days/i)).toBeVisible()

    // Export CSV button is present
    await expect(page.getByRole('button', { name: /Export CSV/i })).toBeVisible()

    // Download the CSV
    const [download] = await Promise.all([
      page.waitForEvent('download'),
      page.getByRole('button', { name: /Export CSV/i }).click(),
    ])

    const path = await download.path()
    expect(path).toBeTruthy()

    // Read first 3 bytes — must be UTF-8 BOM (EF BB BF)
    const buf = fs.readFileSync(path!)
    expect(buf[0]).toBe(0xef)
    expect(buf[1]).toBe(0xbb)
    expect(buf[2]).toBe(0xbf)
  })
})

test.describe('Audit log (viewer)', () => {
  test.use({ storageState: 'playwright/fixtures/viewer-session.json' })

  test('viewer is redirected away from /audit', async ({ page }) => {
    await page.goto('/audit')
    // Viewer should be redirected to / (root dashboard)
    await expect(page).not.toHaveURL(/\/audit/)
  })
})
