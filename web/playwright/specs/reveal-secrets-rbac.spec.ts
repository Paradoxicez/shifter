import { test, expect } from '@playwright/test'

/**
 * Plan 03-10 Task 2 — Reveal secrets RBAC E2E (DEV-09, D-22, D-26..D-28).
 *
 * Three projects worth of assertions consolidated into one file (each
 * `test.describe` declares its own `storageState`):
 *
 *   admin   — clicks Reveal keys, sees the key panel, copy works, AND
 *             the response Cache-Control header is `no-store` (T-3-72).
 *   viewer  — DOM check: Reveal keys button is NOT in the device detail page.
 *   viewer  — Network check: a direct POST to /api/devices/:eui/keys returns
 *             403 (server-side RequireAction is authoritative — D-26).
 *
 * Skip-guard: relies on a seeded device row whose dev_eui is exposed via
 * the env var REVEAL_DEV_EUI when running locally; falls back to a hard-coded
 * placeholder otherwise. The PLAN approach is "seed via Wave 0 bundled
 * compose; test reads env var".
 */

const DEV_EUI = process.env.REVEAL_DEV_EUI ?? '0011223344556677'

test.describe('Reveal secrets — admin', () => {
  test.use({ storageState: 'playwright/fixtures/admin-session.json' })

  test('admin sees Reveal keys + Cache-Control: no-store on response', async ({
    page,
    context,
  }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'], {
      origin: 'http://localhost:8080',
    })

    // Capture the reveal response.
    const responsePromise = page.waitForResponse(
      (resp) =>
        resp.url().includes(`/api/devices/${DEV_EUI}/keys`) &&
        resp.request().method() === 'POST',
    )

    // Resolve the row by dev_eui via the seeded UUID API. Simpler:
    // navigate directly to /devices, find the row link, click in.
    await page.goto('/devices')
    await page.getByText(DEV_EUI).first().click()

    await page.getByRole('button', { name: /Reveal keys/i }).click()
    await page
      .getByRole('button', { name: /^Reveal keys$/i })
      .last()
      .click()

    const resp = await responsePromise
    expect(resp.status()).toBe(200)
    const cacheControl = resp.headers()['cache-control'] ?? ''
    expect(cacheControl).toContain('no-store')

    // Key panel rendered.
    await expect(page.getByText(/AppKey|NwkSKey/i).first()).toBeVisible()

    // Copy all keys writes to clipboard.
    await page.getByRole('button', { name: /Copy all keys/i }).click()
    const clipboard = await page.evaluate(() => navigator.clipboard.readText())
    expect(clipboard.length).toBeGreaterThan(10)
  })
})

test.describe('Reveal secrets — viewer (DOM hide)', () => {
  test.use({ storageState: 'playwright/fixtures/viewer-session.json' })

  test('viewer never sees the Reveal keys button on device detail', async ({
    page,
  }) => {
    await page.goto('/devices')
    await page.getByText(DEV_EUI).first().click()
    // Defense-in-depth — Reveal keys button must NOT be in the DOM.
    await expect(
      page.getByRole('button', { name: /Reveal keys/i }),
    ).toHaveCount(0)
  })
})

test.describe('Reveal secrets — viewer (RBAC 403)', () => {
  test.use({ storageState: 'playwright/fixtures/viewer-session.json' })

  test('viewer POST /api/devices/:eui/keys → 403 forbidden', async ({
    request,
  }) => {
    const resp = await request.post(`/api/devices/${DEV_EUI}/keys`, {
      headers: { 'X-Requested-With': 'shifter' },
      data: {},
    })
    expect(resp.status()).toBe(403)
  })
})
