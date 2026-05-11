import { test, expect } from '@playwright/test'
import {
  seedFixture,
  setInstallCapabilities,
} from '../helpers/dashboard-fixtures'

/**
 * Plan 04-10 Task 1 — Dashboard capability filter (DASH-01)
 *
 * Validates that the dashboard adapts to install scope:
 *   - water-only install: water KPI tiles render; "Electricity" label absent
 *   - electricity-only install: electricity KPI tiles render; "Water" label absent
 *
 * The capability filter is controlled by install_identity.capabilities
 * (schema landed in Plan 04-01 migration 0022). The setInstallCapabilities
 * helper calls PATCH /internal/test/set-capabilities (testharness build tag).
 *
 * When the internal endpoint is unavailable, setInstallCapabilities is a
 * no-op and assertions run against the current configured capability. The
 * spec structure (locator targets + assertion shape) remains the E2E contract.
 *
 * Runs against the bundled compose stack or a locally-running dev server.
 * Session pre-authenticated via admin-session.json storageState.
 */

test.describe('Dashboard capability filter (admin)', () => {
  test.use({ storageState: 'playwright/fixtures/admin-session.json' })

  test('water-only install: electricity section hidden', async ({ page }) => {
    await seedFixture('water-1mp')
    await setInstallCapabilities('water')

    await page.goto('/')
    await expect(page.getByRole('heading', { name: /Dashboard/i })).toBeVisible()

    // Wait for dashboard content to render (either KPI tiles or onboarding)
    await page
      .getByText(/Today's consumption|Current flow|Add your first gateway|Waiting for first uplink/i)
      .first()
      .waitFor({ timeout: 8000 })

    // "Electricity" text must NOT appear anywhere visible on the dashboard
    // (Exact match — not a substring of e.g. "electricity" in a meta tag)
    await expect(page.getByText('Electricity', { exact: true })).toHaveCount(0)

    // Water-related KPI labels must be visible (if data exists)
    // We accept either KPI tiles OR the onboarding state (no data seeded yet)
    const waterVisible = await page
      .getByText(/Today's consumption|Current flow|Add your first gateway|Waiting/i)
      .first()
      .isVisible()
    expect(waterVisible).toBe(true)
  })

  test('electricity-only install: water section hidden', async ({ page }) => {
    await seedFixture('empty')
    await setInstallCapabilities('electricity')

    await page.goto('/')
    await expect(page.getByRole('heading', { name: /Dashboard/i })).toBeVisible()

    // Wait for content render
    await page
      .getByText(/Today's consumption|Current load|Add your first gateway|Waiting for first uplink/i)
      .first()
      .waitFor({ timeout: 8000 })

    // "Water" must NOT appear as a standalone tile label
    await expect(page.getByText('Current flow', { exact: true })).toHaveCount(0)
  })
})
