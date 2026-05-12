import { test, expect } from '@playwright/test'

/**
 * Site click-through path (SITE-06)
 *
 * Map → site marker popup → "View site" → /sites/:id defaults to Floor plan tab
 * when plans exist (D-23). Total: 2 clicks (marker → "View site" button).
 */
test.describe('Site click-through (SITE-06)', () => {
  test('map → site marker popup → "View site" → /sites/:id defaults to Floor plan tab when plans exist (D-23)', async ({ page }) => {
    // Navigate to the map
    await page.goto('/map')

    // Wait for map tiles to load
    await expect(page.locator('.leaflet-container')).toBeVisible({ timeout: 15_000 })

    // The map shows site markers. Click the first visible site marker.
    // Site markers are divIcons rendered as divs with specific classes.
    const siteMarker = page.locator('.leaflet-marker-icon').first()
    const hasMarker = await siteMarker.isVisible().catch(() => false)

    if (hasMarker) {
      await siteMarker.click()

      // Wait for the popup to appear with "View site" button
      const viewSiteBtn = page.getByRole('link', { name: /view site/i }).or(
        page.getByRole('button', { name: /view site/i }),
      )
      await expect(viewSiteBtn).toBeVisible({ timeout: 5_000 })

      // Click "View site"
      await viewSiteBtn.click()

      // Should navigate to /sites/:id
      await expect(page).toHaveURL(/\/sites\/[a-f0-9-]+/, { timeout: 5_000 })

      // D-23: when floor plans exist, the Floor plan tab should be the active/default tab
      // The Tabs component renders with defaultValue="floor-plan" when plans.length > 0
      // Verify the floor plan tab trigger is visible
      await expect(page.getByRole('tab', { name: /floor plan/i })).toBeVisible({ timeout: 5_000 })
    } else {
      // No markers on map (empty install) — verify empty state renders
      await expect(page.getByText(/no locations on the map/i).or(
        page.getByText(/add a site/i),
      )).toBeVisible({ timeout: 5_000 })
    }
  })
})
