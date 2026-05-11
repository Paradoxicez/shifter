import { test, expect } from '@playwright/test'

/**
 * Plan 03-10 Task 2 — Gateway CRUD E2E (GW-01, GW-02, D-29..D-32).
 *
 * Admin-only happy path: create → list → edit → decommission → restore.
 *
 * Runs against the bundled compose stack (or a locally-running dev server
 * with seeded admin@example.com / changeme account). Fixture storage state
 * pre-authenticates the admin session.
 *
 * Skip-guard: the fixture in playwright/fixtures/admin-session.json is a
 * placeholder cookie. The operator regenerates it via
 *   pnpm exec playwright open --save-storage=playwright/fixtures/admin-session.json
 *   http://localhost:8080
 * before this spec passes against a real server. The spec STRUCTURE — locator
 * targets, click sequence, assertion shapes — is the contract; the cookie
 * material is environmental.
 */

test.describe('Gateway CRUD (admin)', () => {
  test.use({ storageState: 'playwright/fixtures/admin-session.json' })

  test('create → edit → decommission → restore', async ({ page }) => {
    await page.goto('/gateways')
    await expect(page.getByRole('heading', { name: /Gateways/i })).toBeVisible()

    // --- Create ---
    await page.getByRole('button', { name: /Add gateway/i }).click()
    await expect(page.getByText(/Add gateway/i)).toBeVisible()
    // Gateway ID is a stable EUI-64 hex string.
    const gatewayId = 'ac1f09fffe' + Date.now().toString(16).padStart(6, '0').slice(-6)
    await page.getByLabel(/Gateway ID/i).fill(gatewayId)
    await page.getByLabel(/Name/i).fill('Rooftop A — Playwright')
    await page.getByRole('button', { name: /Create gateway|Add gateway/i }).last().click()
    await expect(page.getByText('Rooftop A — Playwright')).toBeVisible()

    // --- Edit ---
    await page.getByText('Rooftop A — Playwright').click()
    await page.getByRole('button', { name: /Edit gateway/i }).click()
    await page.getByLabel(/Name/i).fill('Rooftop A — Playwright (renamed)')
    await page.getByRole('button', { name: /Save|Update/i }).last().click()
    await expect(page.getByText('Rooftop A — Playwright (renamed)')).toBeVisible()

    // --- Decommission ---
    await page.getByRole('button', { name: /Decommission/i }).click()
    await page
      .getByRole('button', { name: /Decommission gateway/i })
      .last()
      .click()
    // Archived banner appears.
    await expect(page.getByText(/archived/i)).toBeVisible()

    // --- Restore (via list + Show archived toggle) ---
    await page.goto('/gateways')
    await page.getByRole('switch', { name: /Show archived/i }).click()
    const archivedRow = page.getByText('Rooftop A — Playwright (renamed)')
    await expect(archivedRow).toBeVisible()
    await archivedRow.click()
    await page.getByRole('button', { name: /Restore/i }).click()
    // Archived banner gone.
    await expect(page.getByText(/archived/i)).toHaveCount(0)
  })
})
