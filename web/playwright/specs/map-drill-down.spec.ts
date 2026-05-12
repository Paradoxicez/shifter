import { expect, test } from '@playwright/test'

test.describe('Map — drill-down (MAP-03)', () => {
  test.use({ storageState: './playwright/fixtures/admin-session.json' })

  test('click site marker → popup → "View site" → /sites/:id', async ({ page }) => {
    await page.goto('/map')

    // Wait for the Leaflet map container to render
    await page.waitForSelector('[aria-label="Fleet map"]', { timeout: 15_000 })

    // Click the first site marker (divIcon, rendered as a div inside the map)
    // Site markers have aria-label="Site marker" from the divIcon HTML
    const firstMarker = page.locator('[aria-label="Site marker"]').first()
    await firstMarker.click()

    // Wait for the popup to appear (Leaflet popup container)
    const popup = page.locator('.leaflet-popup-content')
    await expect(popup).toBeVisible({ timeout: 5_000 })

    // Click "View site" inside the popup
    const viewSiteLink = popup.getByRole('link', { name: /View site/i })
    await viewSiteLink.click()

    // Assert URL changed to /sites/<id>
    await expect(page).toHaveURL(/\/sites\/[0-9a-f-]+/, { timeout: 5_000 })
  })

  test('click cluster → zoom to cluster bounds (D-14)', async ({ page }) => {
    await page.goto('/map')

    await page.waitForSelector('[aria-label="Fleet map"]', { timeout: 15_000 })

    // If a cluster is present (>50 markers), click it and assert map zoomed in
    const cluster = page.locator('.marker-cluster').first()
    if (await cluster.isVisible()) {
      await cluster.click()
      // After clicking a cluster, Leaflet zooms in — the cluster should eventually
      // disappear or new individual markers appear
      await expect(cluster).not.toBeVisible({ timeout: 5_000 })
    } else {
      // Fewer than ~50 markers — no cluster to test; pass trivially
      test.info().annotations.push({ type: 'info', description: 'No clusters present (< ~50 markers)' })
    }
  })
})
