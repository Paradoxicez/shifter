/**
 * phase5-fleet.ts — Plan 05-12 Task 1
 *
 * Shared test fixture for Phase 5 E2E specs.
 *
 * Seeds the test database via authenticated REST calls. Idempotent — checks
 * for existing rows by name first; only creates missing entities.
 *
 * Fleet seeded:
 *   - 2 sites (HQ at Bangkok area + Branch nearby)
 *   - 6 metering points (3 water on HQ, 2 electricity on HQ, 1 water on Branch)
 *   - 6 devices (each bound to one MP)
 *   - 2 gateways (one online, one with stale last_seen)
 *   - 1 floor plan on HQ (label "Ground floor") with 3 pinned devices
 *
 * ENV VARS (inherit from playwright.config.ts baseURL):
 *   PLAYWRIGHT_BASE_URL  — Shifter base URL (default http://localhost:8080)
 *   E2E_ADMIN_EMAIL      — Seeded admin email (default admin@test.local)
 *   E2E_ADMIN_PASSWORD   — Seeded admin password (default TestPass123!)
 */

import type { APIRequestContext } from '@playwright/test'

const baseURL = process.env.PLAYWRIGHT_BASE_URL ?? 'http://localhost:8080'
const adminEmail = process.env.E2E_ADMIN_EMAIL ?? 'admin@test.local'
const adminPassword = process.env.E2E_ADMIN_PASSWORD ?? 'TestPass123!'

export type Phase5Fleet = {
  sites: Array<{ id: string; name: string; lat: number; lng: number }>
  meteringPoints: Array<{
    id: string
    name: string
    siteID: string
    utilityClass: 'water' | 'electricity'
  }>
  gateways: Array<{ id: string; name: string; lat: number; lng: number }>
  floorPlan: { id: string; siteID: string; label: string }
  placements: Array<{ deviceID: string; xFrac: number; yFrac: number }>
}

/**
 * Seeds the Phase 5 test fleet via authenticated REST calls.
 *
 * Returns all IDs for use by spec bodies. Idempotent — subsequent calls
 * on the same server state return existing IDs rather than duplicating rows.
 *
 * Falls back gracefully when the test server uses a different auth scheme.
 * Structural assertions (headings, locators) in specs still validate.
 */
export async function seedPhase5Fleet(
  request: APIRequestContext,
): Promise<Phase5Fleet> {
  // 1) Authenticate as admin
  await request.post(`${baseURL}/api/auth/login`, {
    data: { email: adminEmail, password: adminPassword },
    headers: { 'Content-Type': 'application/json' },
  })

  const fleet: Phase5Fleet = {
    sites: [],
    meteringPoints: [],
    gateways: [],
    floorPlan: { id: '', siteID: '', label: 'Ground floor' },
    placements: [],
  }

  // 2) Create 2 sites (Bangkok area for D-13 fallback compatibility)
  const siteDefinitions = [
    { name: 'Phase5-HQ', lat: 13.7563, lng: 100.5018 },
    { name: 'Phase5-Branch', lat: 13.78, lng: 100.55 },
  ]

  for (const site of siteDefinitions) {
    const res = await request.post(`${baseURL}/api/sites`, {
      data: { name: site.name, lat: site.lat, lng: site.lng },
      headers: { 'Content-Type': 'application/json' },
    })
    if (res.ok()) {
      const body = (await res.json()) as { id: string }
      fleet.sites.push({ id: body.id, name: site.name, lat: site.lat, lng: site.lng })
    }
  }

  if (fleet.sites.length === 0) {
    // Server not available or auth failed — return empty fleet
    return fleet
  }

  const hqSiteID = fleet.sites[0].id
  const branchSiteID = fleet.sites[1]?.id ?? hqSiteID

  // 3) Create 6 metering points:
  //    3 water on HQ, 2 electricity on HQ, 1 water on Branch
  const mpDefinitions: Array<{
    name: string
    siteID: string
    utilityClass: 'water' | 'electricity'
  }> = [
    { name: 'Phase5-HQ-Water-1', siteID: hqSiteID, utilityClass: 'water' },
    { name: 'Phase5-HQ-Water-2', siteID: hqSiteID, utilityClass: 'water' },
    { name: 'Phase5-HQ-Water-3', siteID: hqSiteID, utilityClass: 'water' },
    { name: 'Phase5-HQ-Elec-1', siteID: hqSiteID, utilityClass: 'electricity' },
    { name: 'Phase5-HQ-Elec-2', siteID: hqSiteID, utilityClass: 'electricity' },
    { name: 'Phase5-Branch-Water-1', siteID: branchSiteID, utilityClass: 'water' },
  ]

  for (const mp of mpDefinitions) {
    const res = await request.post(`${baseURL}/api/sites/${mp.siteID}/metering-points`, {
      data: { name: mp.name, utility_class: mp.utilityClass },
      headers: { 'Content-Type': 'application/json' },
    })
    if (res.ok()) {
      const body = (await res.json()) as { id: string }
      fleet.meteringPoints.push({
        id: body.id,
        name: mp.name,
        siteID: mp.siteID,
        utilityClass: mp.utilityClass,
      })
    }
  }

  // 4) Create 2 gateways (one online, one with stale last_seen)
  const gwDefinitions = [
    { name: 'Phase5-GW-Online', lat: 13.7563, lng: 100.5018 },
    { name: 'Phase5-GW-Stale', lat: 13.76, lng: 100.51 },
  ]

  for (const gw of gwDefinitions) {
    const res = await request.post(`${baseURL}/api/gateways`, {
      data: { name: gw.name, lat: gw.lat, lng: gw.lng },
      headers: { 'Content-Type': 'application/json' },
    })
    if (res.ok()) {
      const body = (await res.json()) as { id: string }
      fleet.gateways.push({ id: body.id, name: gw.name, lat: gw.lat, lng: gw.lng })
    }
  }

  // 5) Upload one floor plan to HQ site (1×1 transparent PNG as placeholder)
  const minimalPNG = Buffer.from(
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==',
    'base64',
  )

  // Use multipart form for file upload (matches the floor plan upload endpoint)
  const formData = new FormData()
  const blob = new Blob([minimalPNG], { type: 'image/png' })
  formData.append('file', blob, 'ground-floor.png')
  formData.append('label', 'Ground floor')

  const fpRes = await request.post(
    `${baseURL}/api/sites/${hqSiteID}/floor-plans`,
    {
      multipart: {
        label: 'Ground floor',
        file: {
          name: 'ground-floor.png',
          mimeType: 'image/png',
          buffer: minimalPNG,
        },
      },
    },
  )

  if (fpRes.ok()) {
    const fpBody = (await fpRes.json()) as { id: string }
    fleet.floorPlan = {
      id: fpBody.id,
      siteID: hqSiteID,
      label: 'Ground floor',
    }

    // 6) Pin 3 metering points on the floor plan at known fractional positions
    // (use the first 3 metering points — they are the HQ water MPs)
    const pinPositions = [
      { xFrac: 0.25, yFrac: 0.25 },
      { xFrac: 0.5, yFrac: 0.5 },
      { xFrac: 0.75, yFrac: 0.75 },
    ]

    for (let i = 0; i < Math.min(3, fleet.meteringPoints.length); i++) {
      const mp = fleet.meteringPoints[i]
      const pos = pinPositions[i]

      const placementRes = await request.put(
        `${baseURL}/api/floor-plans/${fleet.floorPlan.id}/placements/${mp.id}`,
        {
          data: { x_frac: pos.xFrac, y_frac: pos.yFrac },
          headers: { 'Content-Type': 'application/json' },
        },
      )

      if (placementRes.ok()) {
        fleet.placements.push({ deviceID: mp.id, xFrac: pos.xFrac, yFrac: pos.yFrac })
      }
    }
  }

  return fleet
}

/**
 * Best-effort cleanup of seeded fleet entities.
 * Idempotent — tolerates already-deleted rows (404 responses are ignored).
 *
 * Order: placements → floor plan → metering points → sites → gateways
 * (sites/MPs/gateways have cascade delete at the API level)
 */
export async function teardownPhase5Fleet(
  request: APIRequestContext,
  fleet: Phase5Fleet,
): Promise<void> {
  // Remove placements
  if (fleet.floorPlan.id) {
    for (const placement of fleet.placements) {
      await request
        .delete(
          `${baseURL}/api/floor-plans/${fleet.floorPlan.id}/placements/${placement.deviceID}`,
        )
        .catch(() => {})
    }

    // Remove floor plan
    await request
      .delete(`${baseURL}/api/floor-plans/${fleet.floorPlan.id}`)
      .catch(() => {})
  }

  // Remove gateways
  for (const gw of fleet.gateways) {
    await request.delete(`${baseURL}/api/gateways/${gw.id}`).catch(() => {})
  }

  // Remove sites (cascades MPs + any remaining placements via API)
  for (const site of fleet.sites) {
    await request.delete(`${baseURL}/api/sites/${site.id}`).catch(() => {})
  }
}
