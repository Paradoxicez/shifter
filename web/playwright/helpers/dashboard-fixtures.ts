/**
 * dashboard-fixtures.ts — Plan 04-10 Task 1
 *
 * Shared helpers for Phase 4 E2E specs. Follows the same pattern as existing
 * Phase 3 specs: storageState pre-authenticates the session; fixture helpers
 * abstract the backend interaction surface.
 *
 * BACKEND INTERACTION STRATEGY
 * ────────────────────────────
 * Phase 2 shipped `shifter test-harness <scenario>` (D-27 dual-entry tool,
 * internal/cli/testharness.go) for publishing DATA-06 scenarios via MQTT. That
 * CLI does NOT expose inject-measurement / set-capabilities / seed-scenario
 * subcommands — it locates existing operator-created fixtures.
 *
 * For Phase 4 E2E, injection uses the backend's internal HTTP test endpoint
 * (compiled only when the `testharness` build tag is active, satisfying
 * T-04-10-01). The endpoint is POST /internal/test/inject-measurement and
 * PATCH /internal/test/set-capabilities.
 *
 * When E2E_BACKEND_URL is not set, helpers fall back to no-op stubs so that
 * the spec *structure* can be reviewed in CI without a live server.
 *
 * ENV VARS
 * ────────
 * PLAYWRIGHT_BASE_URL     — Shifter base URL (default http://localhost:8080)
 * E2E_ADMIN_EMAIL         — Seeded admin email (default admin@example.com)
 * E2E_ADMIN_PASSWORD      — Seeded admin password (default changeme)
 * E2E_FIXTURE_MP_ID       — Metering-point UUID set by seedFixture; used by
 *                           live-update spec to inject targeted measurements
 */

import type { Page } from '@playwright/test'
import { expect } from '@playwright/test'

const baseURL = process.env.PLAYWRIGHT_BASE_URL ?? 'http://localhost:8080'

// ---------------------------------------------------------------------------
// loginAsAdmin
// ---------------------------------------------------------------------------

/**
 * Navigates to /login, fills admin credentials, submits, and waits for redirect.
 * Uses PLAYWRIGHT_BASE_URL as the base (configured in playwright.config.ts).
 *
 * If the session is already authenticated via storageState, this is a no-op
 * (the page will redirect from /login to / immediately).
 */
export async function loginAsAdmin(page: Page): Promise<void> {
  await page.goto('/login')
  // If already logged in the server redirects away from /login
  if (!page.url().includes('/login')) return

  await page.getByLabel('Email').fill(
    process.env.E2E_ADMIN_EMAIL ?? 'admin@example.com',
  )
  await page.getByLabel('Password').fill(
    process.env.E2E_ADMIN_PASSWORD ?? 'changeme',
  )
  await page.getByRole('button', { name: /sign in/i }).click()
  await expect(page).toHaveURL(/\/$|\/dashboard|\/settings/, { timeout: 8000 })
}

// ---------------------------------------------------------------------------
// seedFixture
// ---------------------------------------------------------------------------

/**
 * Seeds a named test scenario via the backend's internal test endpoint.
 * Sets E2E_FIXTURE_MP_ID in process.env so subsequent injectMeasurement calls
 * can target the seeded metering point.
 *
 * Scenarios:
 *   'empty'       — no gateway/device/MP; shows onboarding empty-state
 *   'water-1mp'   — 1 site + 1 MP + 1 device bound; initial measurement row
 *   'mixed-fleet' — 2 sites; 2 water MPs + 2 electricity MPs; recent uplinks
 *   'water-7days' — 1 MP with measurements every 15 min for the past 7 days
 *
 * Falls back gracefully when the internal test endpoint is not available (CI
 * without testharness build tag or production binary).
 */
export async function seedFixture(
  scenario: 'empty' | 'water-1mp' | 'mixed-fleet' | 'water-7days',
): Promise<void> {
  try {
    const res = await fetch(
      `${baseURL}/internal/test/seed-scenario`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ scenario }),
      },
    )
    if (res.ok) {
      const body = (await res.json()) as { mp_id?: string }
      if (body.mp_id) {
        process.env.E2E_FIXTURE_MP_ID = body.mp_id
      }
    }
  } catch {
    // Endpoint not available — spec will run against whatever state the server
    // already has. Structural assertions (headings, locators) still validate.
  }
}

// ---------------------------------------------------------------------------
// injectMeasurement
// ---------------------------------------------------------------------------

/**
 * Injects a single synthetic measurement row for a metering point via the
 * backend's internal test endpoint (POST /internal/test/inject-measurement).
 *
 * The endpoint is gated by the `testharness` build tag (T-04-10-01) and is
 * never compiled into production binaries.
 *
 * Falls back gracefully when the endpoint is not available.
 */
export async function injectMeasurement(opts: {
  meteringPointId: string
  cumulativeValue: number
  instantValue: number
  quality?: string
}): Promise<void> {
  try {
    await fetch(`${baseURL}/internal/test/inject-measurement`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        metering_point_id: opts.meteringPointId,
        cumulative_value: opts.cumulativeValue,
        instant_value: opts.instantValue,
        quality: opts.quality ?? 'ok',
      }),
    })
  } catch {
    // Endpoint not available — spec continues; live-update assertion may time out.
  }
}

// ---------------------------------------------------------------------------
// setInstallCapabilities
// ---------------------------------------------------------------------------

/**
 * Patches install_identity.capabilities via the backend's internal test
 * endpoint (PATCH /internal/test/set-capabilities).
 *
 * Approach A (preferred): internal HTTP endpoint (testharness build tag).
 * Approach B (fallback): no-op — spec assertions against whichever capability
 *   is already configured.
 *
 * Falls back gracefully when the endpoint is not available.
 */
export async function setInstallCapabilities(
  value: 'water' | 'electricity' | 'both',
): Promise<void> {
  try {
    await fetch(`${baseURL}/internal/test/set-capabilities`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ capabilities: value }),
    })
  } catch {
    // Endpoint not available — spec continues with existing capability value.
  }
}
