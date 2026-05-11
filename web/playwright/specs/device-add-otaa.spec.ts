import { test, expect } from '@playwright/test'

/**
 * Plan 03-10 Task 2 — Add Device OTAA flow E2E (DEV-02, DEV-04, D-19..D-21).
 *
 * Walks the 5-step Add Device dialog as an admin, picks OTAA, submits, and
 * verifies the success state shows keys + Copy keys writes to clipboard.
 *
 * Clipboard inspection: Playwright grants permission and reads the platform
 * clipboard via the `clipboard-read` permission API.
 */

test.describe('Add Device — OTAA (admin)', () => {
  test.use({ storageState: 'playwright/fixtures/admin-session.json' })

  test('5-step OTAA → success state → Copy keys writes clipboard', async ({
    page,
    context,
  }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'], {
      origin: 'http://localhost:8080',
    })

    await page.goto('/devices')
    await page.getByRole('button', { name: /Add device/i }).click()

    // --- Step 1 Identity ---
    const devEUI = '0011223344' + Date.now().toString(16).padStart(6, '0').slice(-6)
    await page.getByLabel(/Paste from sticker|DevEUI/i).fill(devEUI)
    await page.getByLabel(/^Name/i).fill('Playwright OTAA meter')
    await page.getByRole('button', { name: /Next/i }).click()

    // --- Step 2 Bind --- (skip — optional)
    await page.getByRole('button', { name: /Next|Skip/i }).click()

    // --- Step 3 Activation ---
    // OTAA radio is default-selected; just pick a profile and Next.
    await page.getByLabel(/Device profile/i).click()
    await page
      .getByRole('option', { name: /Axioma|water|electric/i })
      .first()
      .click()
    await page.getByRole('button', { name: /Next/i }).click()

    // --- Step 4 Keys (OTAA) ---
    await page.getByLabel(/AppKey/i).fill('00112233445566778899aabbccddeeff')
    await page.getByLabel(/Join EUI/i).fill('0000000000000000')
    await page.getByRole('button', { name: /Next/i }).click()

    // --- Step 5 Review → Add device ---
    await page.getByRole('button', { name: /Add device/i }).click()

    // --- Success state ---
    await expect(page.getByText(/AppKey/i)).toBeVisible()
    await page.getByRole('button', { name: /Copy keys/i }).click()
    const clipboard = await page.evaluate(() => navigator.clipboard.readText())
    expect(clipboard).toContain('OTAA')
    expect(clipboard).toContain('00112233445566778899aabbccddeeff')

    await page.getByRole('button', { name: /Done/i }).click()

    // Row appears in the devices list.
    await expect(page.getByText('Playwright OTAA meter')).toBeVisible()
  })
})
