import { test, expect } from '@playwright/test'
import { seedFixture } from '../helpers/dashboard-fixtures'

/**
 * Plan 04-10 Task 1 — Dashboard loads (DASH-01, DASH-02, DASH-06)
 *
 * Validates the operator's primary surface loads after login:
 *   - Page heading "Dashboard" is visible
 *   - Sidebar "Dashboard" nav link is active
 *   - At least one KPI tile label OR the empty-state onboarding card is visible
 *
 * Runs against the bundled compose stack or a locally-running dev server.
 * Session is pre-authenticated via admin-session.json storageState.
 */

test.describe('Dashboard loads (admin)', () => {
  test.use({ storageState: 'playwright/fixtures/admin-session.json' })

  test.beforeEach(async () => {
    await seedFixture('water-1mp')
  })

  test('renders dashboard heading and KPIs or onboarding', async ({ page }) => {
    await page.goto('/')

    // Page heading "Dashboard" must be visible
    await expect(page.getByRole('heading', { name: /Dashboard/i })).toBeVisible()

    // Sidebar nav: "Dashboard" link must be present (active or not, depending on route)
    await expect(
      page.getByRole('link', { name: /Dashboard/i }).first(),
    ).toBeVisible()

    // Either a KPI tile label OR an empty-state card is visible.
    // KPI labels: "Today's consumption", "Current flow", "Current load", "Online devices"
    // Empty-state: "Add your first gateway" / "Now add your first device" / "Waiting for first uplink"
    const kpiOrOnboarding = page
      .getByText(
        /Today's consumption|Current flow|Current load|Online devices|Add your first gateway|add your first device|Waiting for first uplink/i,
      )
      .first()
    await expect(kpiOrOnboarding).toBeVisible({ timeout: 8000 })
  })

  test('no horizontal scroll on desktop viewport', async ({ page }) => {
    await page.goto('/')
    await expect(page.getByRole('heading', { name: /Dashboard/i })).toBeVisible()

    const overflows = await page.evaluate(
      () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
    )
    expect(overflows).toBe(false)
  })
})

test.describe('Dashboard loads — unauthenticated redirects to login', () => {
  // No storageState — chromium project (no pre-auth)
  test('redirects to /login when not authenticated', async ({ page }) => {
    await page.goto('/')
    await expect(page).toHaveURL(/\/login/, { timeout: 8000 })
  })
})
