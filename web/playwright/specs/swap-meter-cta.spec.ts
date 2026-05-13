import { test, expect } from '@playwright/test'

/**
 * Smoke test: SwapMeterDialog CTA is wired to the MP detail page.
 *
 * Navigates to the first available MP in the seeded database and asserts
 * that an admin session sees a "Swap Meter" button that opens a dialog
 * with role="dialog". Does NOT exercise the swap flow itself.
 *
 * Pre-condition: the running instance must have at least one metering point
 * reachable via /api/metering-points (seeded by the compose stack).
 */

test.describe('Swap Meter CTA', () => {
  test.use({ storageState: 'playwright/fixtures/admin-session.json' })

  test('Swap Meter button opens dialog on MP detail page', async ({ page }) => {
    // Discover the first MP id from the API (avoids hardcoding a UUID).
    const resp = await page.request.get('/api/metering-points')
    const json = await resp.json()
    // The API returns { items: [...] } or an array directly — handle both shapes.
    const items: Array<{ id: string }> = Array.isArray(json) ? json : (json.items ?? [])
    if (items.length === 0) {
      test.skip(true, 'No metering points seeded — cannot run swap CTA test')
      return
    }

    const mpId = items[0].id
    await page.goto(`/metering-points/${mpId}`)

    // Wait for the detail page to render (heading is the MP name).
    await expect(page.getByRole('heading').first()).toBeVisible({ timeout: 8000 })

    // The "Swap Meter" button must be visible for admin users.
    const swapBtn = page.getByRole('button', { name: /Swap Meter/i })
    await expect(swapBtn).toBeVisible({ timeout: 5000 })

    // Clicking the button opens the dialog.
    await swapBtn.click()
    await expect(page.getByRole('dialog')).toBeVisible({ timeout: 5000 })
  })
})
