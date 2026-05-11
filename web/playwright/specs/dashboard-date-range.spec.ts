import { test, expect } from '@playwright/test'
import { seedFixture } from '../helpers/dashboard-fixtures'

/**
 * Plan 04-10 Task 1 — Dashboard date range (DASH-05)
 *
 * Validates the date-range picker wired to URL state and chart refetches:
 *   1. Seed 7-day fixture (measurements across the past 7 days).
 *   2. Navigate to dashboard (default: ?range=today).
 *   3. Click "7d" preset → assert URL becomes ?range=7d.
 *   4. Assert the timeseries API was refetched with range=7d.
 *   5. Click "Custom" → assert calendar popover opens.
 *
 * The DateRangePicker component (Plan 04-08) controls URL state via
 * useSearchParams + zod.catch('today') fallback (T-04-08-01). Preset buttons
 * have visible labels "Today" / "24h" / "7d" / "30d" / "Custom".
 *
 * The `page.waitForRequest` assertion proves the frontend made a real fetch to
 * the backend when the range changed — not just a URL update.
 *
 * Runs against the bundled compose stack or a locally-running dev server.
 * Session pre-authenticated via admin-session.json storageState.
 */

test.describe('Dashboard date range (admin)', () => {
  test.use({ storageState: 'playwright/fixtures/admin-session.json' })

  test('7d preset updates URL and triggers timeseries refetch', async ({ page }) => {
    await seedFixture('water-7days')
    await page.goto('/')
    await expect(page.getByRole('heading', { name: /Dashboard/i })).toBeVisible()

    // DateRangePicker renders after dashboard content loads
    const sevenDayButton = page.getByRole('button', { name: /^7d$/i })
    await sevenDayButton.waitFor({ timeout: 8000 })

    // Intercept the timeseries API call that fires when range changes
    const timeseriesRequest = page.waitForRequest(
      (req) =>
        req.url().includes('/api/dashboard/timeseries') &&
        req.url().includes('range=7d'),
      { timeout: 8000 },
    )

    await sevenDayButton.click()

    // URL must now have range=7d
    await expect(page).toHaveURL(/range=7d/, { timeout: 5000 })

    // Timeseries API must have been called with range=7d
    await timeseriesRequest
  })

  test('Custom range button opens calendar popover', async ({ page }) => {
    await seedFixture('water-7days')
    await page.goto('/')
    await expect(page.getByRole('heading', { name: /Dashboard/i })).toBeVisible()

    // Wait for the DateRangePicker to render
    const customButton = page.getByRole('button', { name: /Custom/i })
    await customButton.waitFor({ timeout: 8000 })
    await customButton.click()

    // A calendar popover must appear (shadcn Calendar uses role="grid" for the month grid)
    await expect(page.getByRole('grid').first()).toBeVisible({ timeout: 5000 })
  })

  test('today preset is active by default', async ({ page }) => {
    await page.goto('/')
    await expect(page.getByRole('heading', { name: /Dashboard/i })).toBeVisible()

    // DateRangePicker Today button should have data-state="active" when range=today (default)
    const todayButton = page.getByRole('button', { name: /^Today$/i })
    await todayButton.waitFor({ timeout: 8000 })

    // The active preset button carries data-state="active" per DateRangePicker impl
    await expect(todayButton).toHaveAttribute('data-state', 'active')
  })
})
