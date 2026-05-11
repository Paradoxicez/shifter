import { test, expect } from '@playwright/test'

/**
 * Plan 03-10 Task 2 — Add Device ABP flow E2E (DEV-04, D-19, D-20).
 *
 * Same 5-step dialog but selects ABP at step 3. Asserts DevAddr / NwkSKey /
 * AppSKey / FCnt Up / FCnt Down fields are present on step 4 and the
 * success-state KeysPanel renders the FCnt counters.
 */

test.describe('Add Device — ABP (admin)', () => {
  test.use({ storageState: 'playwright/fixtures/admin-session.json' })

  test('5-step ABP → success state shows ABP keys + FCnt counters', async ({
    page,
  }) => {
    await page.goto('/devices')
    await page.getByRole('button', { name: /Add device/i }).click()

    // --- Step 1 Identity ---
    const devEUI = '99aabbccdd' + Date.now().toString(16).padStart(6, '0').slice(-6)
    await page.getByLabel(/Paste from sticker|DevEUI/i).fill(devEUI)
    await page.getByLabel(/^Name/i).fill('Playwright ABP meter')
    await page.getByRole('button', { name: /Next/i }).click()

    // --- Step 2 Bind --- (skip)
    await page.getByRole('button', { name: /Next|Skip/i }).click()

    // --- Step 3 Activation: switch to ABP ---
    await page.getByLabel(/Device profile/i).click()
    await page
      .getByRole('option', { name: /Axioma|water|electric/i })
      .first()
      .click()
    // Pick ABP radio (label includes "not recommended").
    await page.getByRole('radio', { name: /ABP/i }).check()
    await page.getByRole('button', { name: /Next/i }).click()

    // --- Step 4 Keys (ABP) ---
    await page.getByLabel(/Dev Addr/i).fill('01020304')
    await page.getByLabel(/Network Session Key|NwkSKey/i).fill('aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')
    await page.getByLabel(/AppSKey|Session Key/i).first().fill('bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb')
    await page.getByLabel(/FCnt Up/i).fill('12')
    await page.getByLabel(/FCnt Down/i).fill('8')
    await page.getByRole('button', { name: /Next/i }).click()

    // --- Step 5 Review → Add device ---
    await page.getByRole('button', { name: /Add device/i }).click()

    // --- Success state ---
    await expect(page.getByText(/Dev Addr/i)).toBeVisible()
    await expect(page.getByText(/NwkSKey/i)).toBeVisible()
    await expect(page.getByText(/AppSKey/i)).toBeVisible()
    await expect(page.getByText('12')).toBeVisible() // FCnt Up
    await expect(page.getByText('8')).toBeVisible() // FCnt Down

    await page.getByRole('button', { name: /Done/i }).click()
    await expect(page.getByText('Playwright ABP meter')).toBeVisible()
  })
})
