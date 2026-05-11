import { test, expect } from '@playwright/test'

/**
 * Plan 03-10 Task 2 — Devices URL-state filter deep link (DEV-01, D-12..D-18).
 *
 * Navigates directly to a fully-loaded `/devices?...` URL and verifies:
 *   - Toolbar reflects the URL state (q input value, last_seen pill, page).
 *   - A network request was emitted to /api/devices with the SAME query.
 *   - Pagination footer shows the expected page index.
 *   - Browser back updates URL and filter state in lockstep.
 */

test.describe('Devices filter URL-state (admin)', () => {
  test.use({ storageState: 'playwright/fixtures/admin-session.json' })

  test('deep-link with ?q=&last_seen=&sort=&page=&per_page= round-trips', async ({
    page,
  }) => {
    const apiCalls: string[] = []
    page.on('request', (req) => {
      const u = req.url()
      if (u.includes('/api/devices') && !u.includes('/api/devices/')) {
        apiCalls.push(u)
      }
    })

    await page.goto(
      '/devices?q=meter&last_seen=24h&sort=-last_seen&page=2&per_page=25',
    )

    // Toolbar reflects q value.
    await expect(page.getByPlaceholder(/Search/i)).toHaveValue('meter')

    // API call carries the same params.
    const matched = apiCalls.find(
      (u) =>
        u.includes('q=meter') &&
        u.includes('last_seen=24h') &&
        u.includes('sort=-last_seen') &&
        u.includes('page=2') &&
        u.includes('per_page=25'),
    )
    expect(matched, `Expected an /api/devices call with deep-link params; got ${JSON.stringify(apiCalls)}`).toBeDefined()

    // Pagination footer indicates page 2.
    await expect(page.getByText(/Page\s*2\s*of/i)).toBeVisible()

    // --- Back nav → URL updates → toolbar follows. ---
    await page.goto('/devices')
    await expect(page.getByPlaceholder(/Search/i)).toHaveValue('')
    await page.goBack()
    await expect(page).toHaveURL(/q=meter/)
    await expect(page.getByPlaceholder(/Search/i)).toHaveValue('meter')
  })
})
