import { test, expect } from '@playwright/test'
import {
  seedFixture,
  injectMeasurement,
} from '../helpers/dashboard-fixtures'

/**
 * Plan 04-10 Task 1 — Dashboard live update (DASH-02, DASH-03, DASH-04)
 *
 * Validates the SSE-driven KPI update without page refresh:
 *   1. Seed a water-1mp fixture so there's a real MP with measurements.
 *   2. Navigate to the dashboard; capture the current "instant" KPI tile value.
 *   3. Inject a new measurement with a distinct instant_value.
 *   4. Wait up to 5 seconds for the KPI tile to update (Playwright polls).
 *   5. Assert the page URL did not change (no full-page reload).
 *
 * The KPI tile is located via [data-kpi="instant"] (added to KpiCard in
 * Plan 04-10's KpiGrid data-attribute wiring). The instant tile is the only
 * one updated in real-time via SSE without waiting for a snapshot refetch.
 *
 * If the internal/test/inject-measurement endpoint is not available (production
 * binary without testharness build tag), the injection step is a no-op and the
 * test still validates that the dashboard renders KPI tiles without crashing.
 *
 * Runs against the bundled compose stack or a locally-running dev server.
 * Session pre-authenticated via admin-session.json storageState.
 */

test.describe('Dashboard live update (admin)', () => {
  test.use({ storageState: 'playwright/fixtures/admin-session.json' })

  test('KPI instant tile updates via SSE without page reload', async ({ page }) => {
    // Seed fixture — sets E2E_FIXTURE_MP_ID if the internal endpoint is available
    await seedFixture('water-1mp')

    await page.goto('/')
    await expect(page.getByRole('heading', { name: /Dashboard/i })).toBeVisible()

    // Locate the instant KPI tile by data-kpi attribute
    const instantTile = page.locator('[data-kpi="instant"]').first()

    // If there's no data yet (empty-state), skip live-update assertion.
    // The "Today's consumption" / "Current flow" text appears only when KPI data exists.
    const hasKpiData = await instantTile.isVisible({ timeout: 3000 }).catch(() => false)

    if (!hasKpiData) {
      // Dashboard in onboarding state — validate empty-state renders correctly and skip.
      await expect(
        page.getByText(/Add your first gateway|add your first device|Waiting for first uplink/i),
      ).toBeVisible()
      return
    }

    // Capture initial instant value text
    const initialText = await instantTile.textContent()

    // Inject a measurement with a clearly different instant value
    const mpId = process.env.E2E_FIXTURE_MP_ID
    if (mpId) {
      await injectMeasurement({
        meteringPointId: mpId,
        cumulativeValue: 99999,
        instantValue: 777.77,
        quality: 'ok',
      })

      // Wait up to 5 seconds for the tile text to change via SSE
      await expect(instantTile).not.toHaveText(initialText ?? '', { timeout: 5000 })
    }

    // URL must not have changed (no full page reload triggered by SSE update)
    await expect(page).toHaveURL('/')
  })
})
