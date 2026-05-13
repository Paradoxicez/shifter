import { test, expect } from '@playwright/test'

/**
 * BUG (phase-05 audit test 6): FloorPlanTab fetched /api/sites/{id}/devices → 404.
 * Backend exposes the same data at /api/devices/by-site/{siteID}.
 * This spec proves the corrected URL returns 200 (not 404).
 */
test.describe('Floor plan — device sidebar API URL', () => {
  test.use({ storageState: 'playwright/fixtures/admin-session.json' })

  test('/api/devices/by-site/{siteID} returns 200 for a real site', async ({ request }) => {
    // Resolve a real site ID from the sites list.
    const sitesRes = await request.get('/api/sites')
    expect(sitesRes.status()).toBe(200)
    const sites = await sitesRes.json()

    // Skip gracefully if no sites are seeded (CI environments may have 0 sites).
    if (!Array.isArray(sites) || sites.length === 0) {
      test.skip(true, 'No sites seeded — cannot test by-site URL with a real ID')
      return
    }

    const siteID = sites[0].id
    const res = await request.get(`/api/devices/by-site/${siteID}`)
    // Must return 200 (was 404 before the FloorPlanTab.tsx fix).
    expect(res.status()).toBe(200)
    // Response must be a JSON array (even if empty).
    const body = await res.json()
    expect(Array.isArray(body)).toBe(true)
  })
})
