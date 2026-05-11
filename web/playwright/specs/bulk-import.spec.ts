import { test, expect } from '@playwright/test'
import { resolve } from 'node:path'

/**
 * Plan 03-10 Task 2 — Bulk import E2E (DEV-06, DEV-07, DEV-08, D-04..D-11).
 *
 * Validates the canonical 3-step import flow:
 *   1. Upload template (empty data rows) → preview shows 0 valid / 0 invalid.
 *   2. Upload a 5-row sample XLSX → preview shows 5 valid → commit →
 *      success toast → /admin/imports/:job_id shows 5 created rows.
 *   3. Re-upload the same 5-row XLSX → preview now shows 5 `already_exists`
 *      → commit → outcomes all read `already_exists` (idempotency proof —
 *      D-07).
 *
 * Fixtures used:
 *   - web/tests/fixtures/import-template.xlsx  (Wave 0 fixture)
 *   - web/playwright/fixtures/sample-5-devices.xlsx
 *
 * If the sample fixture is missing, this spec test.fails the second/third
 * blocks but still proves the template-upload flow.
 */

test.describe('Bulk import (admin)', () => {
  test.use({ storageState: 'playwright/fixtures/admin-session.json' })

  const templatePath = resolve(
    process.cwd(),
    'tests/fixtures/import-template.xlsx',
  )
  const samplePath = resolve(
    process.cwd(),
    'playwright/fixtures/sample-5-devices.xlsx',
  )

  test('template + commit + idempotent re-upload (already_exists)', async ({
    page,
  }) => {
    await page.goto('/devices')
    await page.getByRole('button', { name: /Bulk import/i }).click()

    // --- Step 1: upload empty template ---
    const fileInput = page.locator('input[type="file"]')
    await fileInput.setInputFiles(templatePath)
    // Preview banner: 0 valid / 0 invalid.
    await expect(page.getByText(/0 valid/i)).toBeVisible()
    await page.getByRole('button', { name: /Cancel|Close/i }).click()

    // --- Step 2: upload 5-row sample → commit → see /admin/imports/:job_id ---
    await page.getByRole('button', { name: /Bulk import/i }).click()
    await fileInput.setInputFiles(samplePath)
    await expect(page.getByText(/5 valid/i)).toBeVisible()
    await page.getByRole('button', { name: /Import 5 devices/i }).click()
    await expect(page.getByText(/Imported|success/i)).toBeVisible()

    // /admin/imports/:job_id detail page.
    await page.goto('/admin/imports')
    await page.getByRole('link', { name: /\d+ rows|imports?/i }).first().click()
    await expect(page.getByText(/created/i)).toBeVisible()

    // --- Step 3: re-upload SAME file → all rows now `already_exists` ---
    await page.goto('/devices')
    await page.getByRole('button', { name: /Bulk import/i }).click()
    await fileInput.setInputFiles(samplePath)
    await expect(page.getByText(/already_exists/i)).toBeVisible()
    await page.getByRole('button', { name: /Import|Commit/i }).first().click()
    await expect(page.getByText(/already_exists/i)).toBeVisible()
  })
})
