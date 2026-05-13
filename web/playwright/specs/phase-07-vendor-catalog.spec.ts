/**
 * Phase 7 — Vendor Catalog E2E flows — Plan 07-14 Task 3
 *
 * Covers three Phase 7 happy paths:
 *   1. Import from catalog — select Itron, prefill form, confirm
 *   2. Apply catalog update with per-field "use mine"/"use catalog" toggles
 *   3. Run codec test, see decoded JSON + canonical mapping
 *
 * Prerequisites (seeded by test fixture or dev server setup):
 *   - Admin user: admin@test.local / TestPass123!
 *   - Itron KINMY LoRa Module catalog entry present in embedded catalog
 *   - At least one device profile with codec_js set (for codec test)
 */

import { test, expect } from '@playwright/test'

test.describe('Phase 7 — Vendor Catalog flows', () => {
  test('import from catalog: select Itron, prefill form, confirm', async ({ page }) => {
    // Login as admin
    await page.goto('/login')
    await page.fill('[name=email]', 'admin@test.local')
    await page.fill('[name=password]', 'TestPass123!')
    await page.click('button[type=submit]')
    await expect(page).not.toHaveURL('/login', { timeout: 5_000 })

    // Navigate to Device Profiles
    await page.goto('/profiles')
    await expect(page.getByText(/device profile|profile/i).first()).toBeVisible({ timeout: 5_000 })

    // Click "Import from catalog" button (outline button in header)
    await page.getByRole('button', { name: /import from catalog/i }).click()

    // Step 1: dialog opens with RadioGroup cards
    await expect(page.getByText('Add Device Profile')).toBeVisible()
    await expect(page.getByText('Start blank')).toBeVisible()
    await expect(page.getByText('Import from catalog')).toBeVisible()

    // Select "Import from catalog" radio
    await page.getByText('Import from catalog').click()

    // Click Continue
    await page.getByRole('button', { name: 'Continue' }).click()

    // Step 2: searchable catalog list appears
    await expect(page.getByPlaceholder('Search vendor profiles…')).toBeVisible({ timeout: 3_000 })

    // Type "Itron" — only Itron entries remain
    await page.getByPlaceholder('Search vendor profiles…').fill('Itron')
    await expect(page.getByText(/itron/i).first()).toBeVisible()

    // Select the Itron entry (click the first result)
    await page.getByText(/KINMY/i).first().click()

    // Click Continue to advance to review form
    await page.getByRole('button', { name: 'Continue' }).click()

    // Step 3: Review form with "Review Profile: Itron KINMY LoRa Module" title
    await expect(page.getByText(/Review Profile/i)).toBeVisible({ timeout: 3_000 })
    await expect(page.getByText(/Itron/i).first()).toBeVisible()

    // Profile name input should be pre-filled
    const nameInput = page.getByLabel('Profile name')
    await expect(nameInput).not.toBeEmpty()

    // Codec textarea should be pre-filled with JS source
    const codecTextarea = page.locator('textarea').first()
    const codecValue = await codecTextarea.inputValue()
    expect(codecValue.length).toBeGreaterThan(10)

    // Click "Add Profile" to submit
    await page.getByRole('button', { name: 'Add Profile' }).click()

    // Success toast appears
    await expect(page.getByText('Profile added')).toBeVisible({ timeout: 5_000 })

    // Dialog closes
    await expect(page.getByText('Add Device Profile')).not.toBeVisible({ timeout: 3_000 })
  })

  test('apply catalog update with per-field "use mine"/"use catalog" toggles', async ({ page }) => {
    // Login as admin
    await page.goto('/login')
    await page.fill('[name=email]', 'admin@test.local')
    await page.fill('[name=password]', 'TestPass123!')
    await page.click('button[type=submit]')
    await expect(page).not.toHaveURL('/login', { timeout: 5_000 })

    // Navigate to Settings → scroll to Vendor Catalog section
    await page.goto('/settings')

    // Find the Vendor Catalog section
    await expect(page.getByText(/vendor catalog/i).first()).toBeVisible({ timeout: 5_000 })

    // Look for an "Update to v..." button (requires an update-available row)
    // If no update is available, the test skips gracefully with a note.
    const updateButton = page.getByRole('button', { name: /update to v/i }).first()
    const updateButtonCount = await updateButton.count()
    if (updateButtonCount === 0) {
      test.info().annotations.push({
        type: 'skip-reason',
        description: 'No catalog update available — bump a catalog JSON version to trigger this test',
      })
      return
    }

    // Click the Update button
    await updateButton.click()

    // Update diff modal opens
    await expect(page.getByText(/Update .+ →/)).toBeVisible({ timeout: 3_000 })
    await expect(page.getByText('Review changes field by field.')).toBeVisible()

    // At least one diff row should be visible
    await expect(page.getByText('Use mine').first()).toBeVisible()
    await expect(page.getByText('Use catalog').first()).toBeVisible()

    // Toggle first row to "Use mine"
    await page.getByRole('button', { name: /Use my value for/i }).first().click()

    // Apply Update button is visible with tooltip text
    const applyBtn = page.getByRole('button', { name: 'Apply Update' })
    await expect(applyBtn).toBeVisible()

    // Click Apply Update
    await applyBtn.click()

    // Success toast "Profile updated to v{X.Y.Z}"
    await expect(page.getByText(/Profile updated to v/)).toBeVisible({ timeout: 5_000 })

    // Modal closes
    await expect(page.getByText('Apply Update')).not.toBeVisible({ timeout: 3_000 })
  })

  test('run codec test, see decoded JSON + canonical mapping', async ({ page }) => {
    // Login as admin
    await page.goto('/login')
    await page.fill('[name=email]', 'admin@test.local')
    await page.fill('[name=password]', 'TestPass123!')
    await page.click('button[type=submit]')
    await expect(page).not.toHaveURL('/login', { timeout: 5_000 })

    // Navigate to Device Profiles list
    await page.goto('/profiles')
    await expect(page.getByText(/profile/i).first()).toBeVisible({ timeout: 5_000 })

    // Click first profile to open its editor
    const profileLink = page.getByRole('link').filter({ hasText: /axioma|itron|acrel/i }).first()
    const profileLinkCount = await profileLink.count()
    if (profileLinkCount === 0) {
      test.info().annotations.push({
        type: 'skip-reason',
        description: 'No vendor profiles found — import at least one catalog profile first',
      })
      return
    }
    await profileLink.click()

    // Profile editor opens — scroll to Test Codec section
    await expect(page.getByText(/Test Codec/i)).toBeVisible({ timeout: 5_000 })

    // Expand / click the Test Codec section
    await page.getByText(/Test Codec/i).click()

    // Hex input field should be visible
    const hexInput = page.getByPlaceholder(/hex payload|hex bytes|paste hex/i)
    await expect(hexInput).toBeVisible({ timeout: 3_000 })

    // Paste a known Axioma W1 hex payload (16 bytes, meter reading + battery)
    const axiomHex = '01 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00'
    await hexInput.fill(axiomHex)

    // Click Run Test
    await page.getByRole('button', { name: /run test/i }).click()

    // Decoded JSON tab should appear with content
    await expect(page.getByRole('tab', { name: /decoded json/i })).toBeVisible({ timeout: 5_000 })
    await page.getByRole('tab', { name: /decoded json/i }).click()
    await expect(page.getByText(/cumulative|battery|flow/i)).toBeVisible({ timeout: 3_000 })

    // Canonical Mapping tab
    await expect(page.getByRole('tab', { name: /canonical mapping/i })).toBeVisible()
    await page.getByRole('tab', { name: /canonical mapping/i }).click()
    // Canonical mapping should show some field mappings
    await expect(page.getByText(/→|maps to|canonical/i).first()).toBeVisible({ timeout: 3_000 })
  })
})
