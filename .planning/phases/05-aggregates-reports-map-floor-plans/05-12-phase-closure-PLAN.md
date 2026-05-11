---
phase: 05-aggregates-reports-map-floor-plans
plan: 12
type: execute
wave: 6
depends_on: [08, 09, 10, 11]
files_modified:
  - web/playwright/specs/reports-generate.spec.ts
  - web/playwright/specs/map-drill-down.spec.ts
  - web/playwright/specs/floor-plan-pinning.spec.ts
  - web/playwright/specs/site-drill-through.spec.ts
  - web/playwright/specs/floor-plan-health.spec.ts
  - web/playwright/specs/retention-settings.spec.ts
  - web/playwright/fixtures/phase5-fleet.ts
  - .planning/REQUIREMENTS.md
  - .planning/ROADMAP.md
  - .planning/phases/05-aggregates-reports-map-floor-plans/05-VALIDATION.md
  - .planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md
  - .planning/phases/05-aggregates-reports-map-floor-plans/05-VOCAB-AUDIT.md
autonomous: true
requirements: [REPT-01, REPT-02, REPT-03, REPT-04, REPT-05, REPT-06, REPT-07, MAP-01, MAP-02, MAP-03, MAP-04, SITE-02, SITE-03, SITE-04, SITE-05, SITE-06, DATA-11, DATA-12, DATA-13]
threat_refs: []

must_haves:
  truths:
    - "6 Playwright E2E specs ship fully bodied (zero test.skip in spec files) covering: reports-generate, map-drill-down, floor-plan-pinning, site-drill-through, floor-plan-health, retention-settings"
    - "Shared fixture file phase5-fleet.ts seeds: 2 sites with lat/lng + 6 metering points (3 water + 3 electricity) + 2 gateways + 1 floor plan with 3 pinned devices — used by 3+ specs"
    - "REQUIREMENTS.md status: all 19 Phase 5 requirement IDs flip from 'Pending' to 'Complete' with evidence trail (Plan + SUMMARY refs)"
    - "ROADMAP.md Phase 5 plan count fixed (TBD → 12 plans) and progress percentage updated"
    - "05-VALIDATION.md per-task verification map populated (every <automated> command matched to its task) AND nyquist_compliant: true flipped"
    - "05-RESEARCH.md Open Questions block updated: #1 (cumulative_delta) RESOLVED with LAG() decision; #3 (maroto page X of Y) RESOLVED with picked strategy; #4 (SETT-04 phase) RESOLVED — moved to Phase 5"
    - "UX-03 vocabulary audit: zero ChirpStack-native terms (tenant, application, dev_eui, app_eui, network_server, gateway_bridge) leaked into any new Phase 5 surface (reports, map, floor plans, settings retention)"
    - "Final full-suite test run: go test ./... -race -count=1 passes; pnpm --dir web test:run passes; pnpm --dir web exec playwright test passes against compose-bundled smoke"
    - "STATE.md updated with Phase 5 closure block summarizing what landed + plan counts"
  artifacts:
    - path: "web/playwright/fixtures/phase5-fleet.ts"
      provides: "Seedable Phase 5 test fleet: 2 sites + 6 MPs + 2 gateways + 1 floor plan + 3 placements"
      contains: "phase5Fleet"
    - path: "web/playwright/specs/floor-plan-pinning.spec.ts"
      provides: "Full E2E body for SITE-04 + D-25"
      contains: "place 3 devices"
    - path: ".planning/REQUIREMENTS.md"
      provides: "19 Phase 5 requirement IDs marked Complete with evidence"
      contains: "Complete"
    - path: ".planning/phases/05-aggregates-reports-map-floor-plans/05-VOCAB-AUDIT.md"
      provides: "Verbatim grep results proving zero ChirpStack vocabulary leaked"
      contains: "UX-03 vocabulary audit"
    - path: ".planning/phases/05-aggregates-reports-map-floor-plans/05-VALIDATION.md"
      provides: "Per-task verification map filled in (one row per task across plans 01..11)"
      contains: "nyquist_compliant: true"
  key_links:
    - from: "web/playwright/fixtures/phase5-fleet.ts"
      to: "POST /api/sites + /api/metering-points + /api/devices + /api/sites/:id/floor-plans + /api/floor-plans/:id/placements"
      via: "fixture setUp() in playwright config"
      pattern: "phase5Fleet"
    - from: ".planning/REQUIREMENTS.md"
      to: "Plan 05-NN-SUMMARY.md evidence"
      via: "evidence column with plan refs"
      pattern: "Plan 05-"
---

<objective>
Close Phase 5: ship the 6 Playwright E2E specs with full bodies (replacing test.skip), build the shared phase5-fleet fixture, update REQUIREMENTS.md to flip 19 requirement IDs to Complete with evidence trail, fix ROADMAP.md plan count, populate 05-VALIDATION.md per-task verification map, resolve the three RESEARCH Open Questions, and run a UX-03 vocabulary audit across every new Phase 5 surface.

Purpose: This is the "ship it" plan. Plans 01–11 produced code + tests; this plan converts that into a reconciled, evidence-backed phase closure that the Phase 6 planner can read without ambiguity. No new production code; only test bodies, fixture, documentation reconciliation.

Output: 6 spec bodies, 1 fixture file, 4 .planning/ files updated, 1 new VOCAB-AUDIT.md.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-UI-SPEC.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-VALIDATION.md
@.planning/REQUIREMENTS.md
@.planning/ROADMAP.md
@.planning/STATE.md
@.planning/phases/03-provisioning-gateways-devices-bulk-import/03-10-PLAN.md
@.planning/phases/04-realtime-dashboard/04-10-PLAN.md

<interfaces>
<!-- Reference patterns from prior phase-closure plans (Phase 3 03-10, Phase 4 04-10) -->
<!-- - Shared fixture: seeds a fleet via REST API calls in playwright globalSetup -->
<!-- - REQUIREMENTS.md: per-row evidence column listing landed plan + summary -->
<!-- - VOCAB-AUDIT.md: grep results listing every search string + zero matches -->
</interfaces>
</context>

<tasks>

<task type="auto" tdd="false">
  <name>Task 1: Phase 5 shared fixture + 6 E2E spec bodies</name>
  <read_first>
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-VALIDATION.md §Subsystem Validation Ladders (E2E rows)
    - .planning/phases/04-realtime-dashboard/04-10-PLAN.md (Phase 4 closure spec patterns)
    - web/playwright/fixtures/ (existing fixtures from Phase 3/4)
    - web/playwright/config.ts (globalSetup pattern)
    - web/playwright/specs/*.spec.ts (six specs created with test.skip in earlier plans)
  </read_first>
  <action>
**Step A — `web/playwright/fixtures/phase5-fleet.ts`:**

Seeds the test database via authenticated REST calls. Idempotent — checks for existing rows by name first; only creates missing.

```ts
import { APIRequestContext } from '@playwright/test'

export type Phase5Fleet = {
  sites: Array<{ id: string; name: string; lat: number; lng: number }>
  meteringPoints: Array<{ id: string; name: string; siteID: string; utilityClass: 'water' | 'electricity' }>
  gateways: Array<{ id: string; name: string; lat: number; lng: number }>
  floorPlan: { id: string; siteID: string; label: string }
  placements: Array<{ deviceID: string; xFrac: number; yFrac: number }>
}

export async function seedPhase5Fleet(request: APIRequestContext): Promise<Phase5Fleet> {
  // 1) Login as admin (use credentials from test env).
  await request.post('/api/auth/login', { data: { email: 'admin@test.local', password: 'TestPass123!' } })

  // 2) Create 2 sites (Bangkok area for D-13 fallback compatibility):
  //    - HQ at 13.7563, 100.5018
  //    - Branch at 13.78, 100.55
  // 3) Create 6 MPs (3 water on HQ, 2 electricity on HQ, 1 water on Branch).
  // 4) Create 6 devices, each bound to one MP.
  // 5) Create 2 gateways (one online, one with stale last_seen).
  // 6) Upload one 800×600 PNG floor plan to HQ (label "Ground floor").
  // 7) Pin 3 devices on the floor plan at known fractional positions.
  // 8) Insert backdated measurement rows via the test ingest harness so CAGGs have data.
  //
  // Return all IDs for use by spec bodies.
  return { sites, meteringPoints, gateways, floorPlan, placements }
}

export async function teardownPhase5Fleet(request: APIRequestContext, fleet: Phase5Fleet) {
  // Best-effort cleanup. Idempotent — tolerates already-deleted rows.
  for (const p of fleet.placements) { await request.delete(`/api/floor-plans/${fleet.floorPlan.id}/placements/${p.deviceID}`) }
  await request.delete(`/api/floor-plans/${fleet.floorPlan.id}`)
  // Devices + MPs + sites + gateways cascade-delete via API.
}
```

Wire `seedPhase5Fleet` into `web/playwright/global-setup.ts` (or per-spec fixtures) such that the 3 specs that need the full fleet pull from a single seed.

**Step B — `web/playwright/specs/reports-generate.spec.ts` body (replace skeleton from plan 05-01):**

```ts
import { test, expect } from '@playwright/test'
import { seedPhase5Fleet } from '../fixtures/phase5-fleet'

test('login → /reports → configure single-meter monthly → Generate → download CSV + Excel + PDF', async ({ page, request }) => {
  const fleet = await seedPhase5Fleet(request)
  const targetMP = fleet.meteringPoints[0]

  await page.goto('/login')
  await page.fill('[name=email]', 'admin@test.local')
  await page.fill('[name=password]', 'TestPass123!')
  await page.click('button[type=submit]')

  await page.goto('/reports')
  await expect(page.getByRole('heading', { name: 'Reports' })).toBeVisible()

  await page.click('label:has-text("Single meter")')
  await page.click('button[role=combobox]')
  await page.click(`text=${targetMP.name}`)
  await page.click('button:has-text("Monthly")')

  // Generate.
  await page.click('button:has-text("Generate report")')
  await expect(page.getByText('Report ready')).toBeVisible({ timeout: 30_000 })

  // Verify CSV + Excel download links present.
  const csvLink = page.locator('a:has-text("Download CSV")')
  const xlsxLink = page.locator('a:has-text("Download Excel")')
  await expect(csvLink).toBeVisible()
  await expect(xlsxLink).toBeVisible()

  // Click CSV download and verify it returns 200 + UTF-8 BOM in first 3 bytes.
  const [dl] = await Promise.all([page.waitForEvent('download'), csvLink.click()])
  const path = await dl.path()
  const buf = require('fs').readFileSync(path)
  expect(buf.slice(0, 3)).toEqual(Buffer.from([0xEF, 0xBB, 0xBF]))

  // Wait for PDF to be ready (River worker latency ≤ 30s for a small report).
  await expect(page.locator('a:has-text("Download PDF")')).toBeVisible({ timeout: 60_000 })
  await expect(page.getByText('Your PDF is ready')).toBeVisible()
})
```

**Step C — `web/playwright/specs/map-drill-down.spec.ts` body:**

```ts
test('click site marker → popup → "View site" → /sites/:id', async ({ page, request }) => {
  const fleet = await seedPhase5Fleet(request)
  await login(page)
  await page.goto('/map')
  await page.waitForSelector('.leaflet-marker-icon')
  const siteMarker = page.locator('.leaflet-marker-icon').first()
  await siteMarker.click()
  await expect(page.getByText(fleet.sites[0].name)).toBeVisible()
  await page.click('a:has-text("View site")')
  await expect(page).toHaveURL(new RegExp(`/sites/${fleet.sites[0].id}`))
})

test('click cluster → zoom to cluster bounds', async ({ page, request }) => {
  // Seed 60+ sites in a tight bounding box to force clustering.
  // Click cluster; assert zoom level increases or markers de-cluster (count visible markers before/after).
})
```

**Step D — `web/playwright/specs/floor-plan-pinning.spec.ts` body:**

```ts
test('upload PNG → place 3 devices → reload → pins persist at same fractional positions', async ({ page, request }) => {
  const fleet = await seedPhase5Fleet(request)
  await login(page)
  await page.goto(`/sites/${fleet.floorPlan.siteID}`)
  // D-23: Floor plan tab default when plans exist
  await expect(page.locator('[role=tabpanel][data-state=active]:has-text("Floor plan")')).toBeVisible()

  // Pins already seeded by fixture — assert 3 pins visible.
  await expect(page.locator('[data-role=device-pin]')).toHaveCount(3)

  // Reload and verify they persist at the same positions.
  const before = await page.locator('[data-role=device-pin]').evaluateAll(els => els.map(e => ({ left: (e as HTMLElement).style.left, top: (e as HTMLElement).style.top })))
  await page.reload()
  const after = await page.locator('[data-role=device-pin]').evaluateAll(els => els.map(e => ({ left: (e as HTMLElement).style.left, top: (e as HTMLElement).style.top })))
  expect(after).toEqual(before)
})

test('decommission one device → its pin disappears from plan (D-25)', async ({ page, request }) => {
  const fleet = await seedPhase5Fleet(request)
  await login(page)
  await page.goto(`/sites/${fleet.floorPlan.siteID}`)
  await expect(page.locator('[data-role=device-pin]')).toHaveCount(3)

  // Decommission first placed device via API (uses Phase 3 endpoint + Plan 05-07 cascade).
  await request.post(`/api/devices/${fleet.placements[0].deviceID}/decommission`, { data: { reason: 'E2E test' } })

  await page.reload()
  await expect(page.locator('[data-role=device-pin]')).toHaveCount(2)
})
```

**Step E — `web/playwright/specs/site-drill-through.spec.ts` body:**

```ts
test('map → site marker popup → View site → /sites/:id Floor plan tab default when plans exist (D-23)', async ({ page, request }) => {
  const fleet = await seedPhase5Fleet(request)
  await login(page)
  await page.goto('/map')
  await page.locator('.leaflet-marker-icon').first().click()
  await page.click('a:has-text("View site")')

  // SITE-06: 2 clicks from /map to /sites/:id Floor plan.
  await expect(page.locator('[role=tabpanel][data-state=active]:has-text("Floor plan")')).toBeVisible()

  // Open device popover; click "Open device"; assert navigation to /devices/:id (3rd click).
  await page.locator('[data-role=device-pin]').first().click()
  await page.click('a:has-text("Open device")')
  await expect(page).toHaveURL(/\/devices\//)
})
```

**Step F — `web/playwright/specs/floor-plan-health.spec.ts` body:**

```ts
test('SSE measurement → marker re-tints from green to yellow on battery drop (D-22)', async ({ page, request }) => {
  const fleet = await seedPhase5Fleet(request)
  await login(page)
  await page.goto(`/sites/${fleet.floorPlan.siteID}`)

  // Initial pin state should be healthy (green) — green = bg-success.
  await expect(page.locator(`[aria-label*="${fleet.placements[0].deviceID}"][class*="bg-success"]`)).toBeVisible()

  // Inject a measurement with battery_pct=10 via the test ingest harness API.
  // The Phase 4 SSE pipeline picks this up and dispatches to mp:<uuid> topic; the
  // floor-plan tab's useFloorPlanHealth recomputes state to 'warning'.
  await request.post('/api/_test/inject-measurement', { data: {
    metering_point_id: fleet.placements[0].deviceID,  // (use the MP ID — adjust to fixture's MP→device mapping)
    battery_pct: 10,
    rssi: -90,
    quality: 'ok',
  }})

  // Within 5s the marker should re-tint to yellow.
  await expect(page.locator(`[aria-label*="${fleet.placements[0].deviceID}"][class*="bg-warning"]`)).toBeVisible({ timeout: 5_000 })
})
```

**Step G — `web/playwright/specs/retention-settings.spec.ts` already wrote bodies in plan 05-11 Task 2. No changes here unless the planner discovers gaps.**

**Step H — Run the full suite locally and pin the green run:**

```bash
just compose-smoke-bundled
go test ./... -race -count=1
pnpm --dir web test:run
pnpm --dir web exec playwright test
```

All four MUST exit 0 before proceeding to Task 2.
  </action>
  <verify>
    <automated>pnpm --dir web exec playwright test --list 2&gt;&amp;1 | grep -E "reports-generate|map-drill-down|floor-plan-pinning|site-drill-through|floor-plan-health|retention-settings" | wc -l &amp;&amp; ! grep -rn "test\\.skip" web/playwright/specs/ &amp;&amp; go test ./... -race -count=1 -short &amp;&amp; pnpm --dir web test:run</automated>
  </verify>
  <acceptance_criteria>
    - 6 Playwright spec files exist with full bodies (no `test.skip` left in any file): `! grep -rn "test.skip" web/playwright/specs/`
    - `web/playwright/fixtures/phase5-fleet.ts` exists and exports `seedPhase5Fleet` + `teardownPhase5Fleet`
    - Each spec uses the fixture (3+ specs import from phase5-fleet)
    - `pnpm --dir web exec playwright test` exits 0 against compose-bundled smoke environment
    - `go test ./... -race -count=1 -short` exits 0
    - `pnpm --dir web test:run` exits 0
  </acceptance_criteria>
  <done>6 E2E specs ship green; shared fixture seeds the test fleet; full suite passes.</done>
</task>

<task type="auto" tdd="false">
  <name>Task 2: REQUIREMENTS.md + ROADMAP.md + VALIDATION.md + RESEARCH.md reconciliation</name>
  <read_first>
    - .planning/REQUIREMENTS.md (rows for SITE-02..06, MAP-01..04, REPT-01..07, DATA-11..13, SETT-04)
    - .planning/ROADMAP.md (Phase 5 plan count + progress table)
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-VALIDATION.md (per-task map — currently empty)
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md (Open Questions block — #1, #3, #4)
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-01..05-11 PLAN files (already created — for evidence trail)
  </read_first>
  <action>
**Step A — REQUIREMENTS.md flip status for 19 + 1 IDs (already handled SETT-04 in plan 05-11; this plan flips the rest):**

For each row in `{SITE-02, SITE-03, SITE-04, SITE-05, SITE-06, MAP-01, MAP-02, MAP-03, MAP-04, REPT-01, REPT-02, REPT-03, REPT-04, REPT-05, REPT-06, REPT-07, DATA-11, DATA-12, DATA-13, SETT-04}`:

Change `Pending` → `Complete` AND append an evidence column referencing the landed plan + summary:

```markdown
| DATA-11 | Phase 5 | Complete | Plans 05-01 (Wave 0 setup) + 05-02 (CAGG hierarchy); verified by internal/aggregate/aggregate_test.go::TestCAGGHierarchy |
| DATA-12 | Phase 5 | Complete | Plan 05-02; refresh policies pinned by TestRefreshPolicyParams |
| DATA-13 | Phase 5 | Complete | Plans 05-02 (substrate) + 05-11 (Settings UI) |
| MAP-01..04 | Phase 5 | Complete | Plans 05-04 (backend) + 05-08 (frontend); verified by playwright/specs/map-drill-down.spec.ts |
| REPT-01..07 | Phase 5 | Complete | Plans 05-03 (sqlc + CSV + Excel) + 05-06 (PDF + River) + 05-09 (frontend); verified by playwright/specs/reports-generate.spec.ts |
| SITE-02..06 | Phase 5 | Complete | Plans 05-05 (schema + upload) + 05-07 (placement + decommission) + 05-10 (frontend); verified by playwright/specs/floor-plan-pinning.spec.ts + site-drill-through.spec.ts + floor-plan-health.spec.ts |
| SETT-04 | Phase 5 | Complete | Plan 05-11 (was originally Phase 6; moved per RESEARCH Open Q #4) |
```

Adjust formatting to match the existing table column layout exactly.

**Step B — ROADMAP.md fixes:**

1. Phase 5 row in the Phases section: `**Plans**: TBD` → `**Plans**: 12 plans`
2. Phase 5 Plans list block: replace TBD with the list of 12 PLAN.md files:

```markdown
Plans:
- [ ] 05-01-wave-0-deps-skeletons-PLAN.md — Wave 0: install deps, embed River schema, skeleton tests
- [ ] 05-02-cagg-hierarchy-retention-PLAN.md — 4-level CAGG hierarchy + retention_config seed
- [ ] 05-03-reports-backend-csv-excel-PLAN.md — sqlc queries + assembler + CSV/Excel writers + handler
- [ ] 05-04-map-backend-PLAN.md — /api/map/data endpoint
- [ ] 05-05-floor-plan-schema-upload-PLAN.md — migrations 0031/0032 + upload + volume mount
- [ ] 05-06-reports-pdf-river-worker-PLAN.md — maroto PDF + River worker + cleanup + download handler
- [ ] 05-07-floor-plan-placement-decommission-PLAN.md — placement CRUD + auth-gated static serve + D-25
- [ ] 05-08-map-frontend-gw04-picker-PLAN.md — /map + Leaflet + cluster + GW-04 picker
- [ ] 05-09-reports-frontend-PLAN.md — /reports + config + result + PDF poll + toast
- [ ] 05-10-floor-plan-frontend-PLAN.md — canvas + pin + drag + pdf.js + SSE health
- [ ] 05-11-settings-data-retention-PLAN.md — Settings Data Retention card + backend reconciliation
- [ ] 05-12-phase-closure-PLAN.md — 6 E2E + REQUIREMENTS + ROADMAP + VALIDATION + RESEARCH reconciliation
```

3. Progress table: bump Phase 5 to `12/12 Complete` (after this plan completes — leave as `In Progress` if any earlier plan still pending) and update the `Plans Complete` column.

**Step C — 05-VALIDATION.md per-task verification map population:**

Read each plan (05-01 through 05-11) and copy its task `<verify><automated>` commands into the Per-Task Verification Map table. One row per task. Example shape:

```markdown
| Task ID  | Plan | Wave | Requirement   | Threat Ref      | Test Type | Automated Command                                                                                  | File Exists | Status |
|----------|------|------|---------------|------------------|-----------|----------------------------------------------------------------------------------------------------|-------------|--------|
| 05-01-T1 | 01   | 0    | All (Wave 0)  | T-05-01-01..02   | smoke     | `go build ./... && pnpm --dir web build && grep -q "maroto/v2 v2.4.0" go.mod`                       | ✅          | ✅     |
| 05-01-T2 | 01   | 0    | All (Wave 0)  | T-05-01-02       | integration | `go test ./internal/db/... -short=false -run TestRunMigrations`                                  | ✅          | ✅     |
| 05-02-T1 | 02   | 1    | DATA-11/12    | T-05-02-01..02   | integration | `go test ./internal/aggregate/... -short=false -run TestCAGG*`                                   | ✅          | ✅     |
| ...etc... |
```

Flip frontmatter `nyquist_compliant: false` → `nyquist_compliant: true` and `wave_0_complete: false` → `wave_0_complete: true`.

**Step D — 05-RESEARCH.md Open Questions block update:**

Read the existing `## Open Questions` section and append/edit a `## Open Questions (RESOLVED)` heading flip with inline `RESOLVED:` bullets:

```markdown
## Open Questions (RESOLVED)

1. **Cumulative delta computation in CAGG** — **RESOLVED** (Plan 05-01 Task 1 + Plan 05-02 Task 1): `cumulative_delta` was NOT present in the measurement hypertable. Plan 05-02's hourly CAGG computes the delta inline via `LAG(cumulative_value) OVER (PARTITION BY metering_point_id ORDER BY time)`. Cross-bucket correctness verified by TestCAGGChain_DeltaCorrectness (hour-09 delta = 45, hour-10 delta = 55 across the bucket boundary). See internal/aggregate/CUMULATIVE_DELTA.md for the verdict file.

2. **River migration embedding vs CLI** — **RESOLVED** (Plan 05-01 Task 2): River's schema captured by running `river migrate-up` against a clean Postgres + `pg_dump -s -t 'river_*'`, converted to idempotent `CREATE TABLE IF NOT EXISTS` form, landed as `0024_river_tables.up.sql`. Round-trip test `TestRunMigrations_RiverDownUpClean` pins behavior. No `river migrate-up` CLI step required at install time.

3. **Maroto v2 page X of Y placeholder syntax** — **RESOLVED** (Plan 05-06 Task 1): [Fill in chosen strategy — native `{number}`/`{total}` placeholders worked, OR two-pass fallback was implemented; see Plan 05-06 SUMMARY.md for the resolution].

4. **SETT-04 vs SETT-04 scope in Phase 5** — **RESOLVED** (Plan 05-11): SETT-04 migrated from Phase 6 → Phase 5 in REQUIREMENTS.md. retention_config table + UI shipped together so operators can act on the values they see.
```

Leave the original `## Open Questions` block intact (don't delete it; just add the RESOLVED section below it).

**Step E — STATE.md Phase 5 closure block:**

Append a new entry below the existing Plan 04-XX status blocks:

```markdown
> **Phase 5 closure (DATE):** Plans 05-01..12 shipped {N} plans covering DATA-11..13, REPT-01..07, MAP-01..04, SITE-02..06, plus SETT-04 (migrated from Phase 6).
> - 05-01 Wave 0: maroto/v2 + river + leaflet/pdfjs deps; River schema as migration 0024; pdfjs worker shim
> - 05-02: 4-level CAGG hierarchy + retention_config + install seed
> - 05-03: Reports CSV/Excel/handler + sqlc queries against CAGGs
> - 05-04: Map data endpoint with capability-gated rollups
> - 05-05: Floor plan migrations + upload validation + compose volume mount
> - 05-06: maroto PDF + River worker + cleanup cron + download handler
> - 05-07: Placement CRUD + auth-gated static serve + D-25 decommission cascade
> - 05-08: Map frontend + clustering + GW-04 picker
> - 05-09: Reports frontend + PDF poll + toast
> - 05-10: Floor plan canvas + pinning + pdf.js + SSE marker state
> - 05-11: Settings Data Retention card + same-tx policy reconciliation
> - 05-12: 6 E2E + reconciliation + vocab audit
>
> Final test suite (Plan 05-12 Task 2): `go test ./... -race -count=1` PASSED; `pnpm --dir web test:run` PASSED; `pnpm --dir web exec playwright test` PASSED ({N} specs).
> All 19 Phase 5 requirement IDs marked Complete with evidence trail.
```
  </action>
  <verify>
    <automated>grep -cE "\\| (SITE-0[2-6]|MAP-0[1-4]|REPT-0[1-7]|DATA-1[1-3]|SETT-04) \\| Phase 5 \\| Complete" .planning/REQUIREMENTS.md &amp;&amp; grep -q "Plans: 12 plans" .planning/ROADMAP.md &amp;&amp; grep -q "nyquist_compliant: true" .planning/phases/05-aggregates-reports-map-floor-plans/05-VALIDATION.md &amp;&amp; grep -q "Open Questions (RESOLVED)" .planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md</automated>
  </verify>
  <acceptance_criteria>
    - REQUIREMENTS.md: every Phase 5 requirement ID has status `Complete` (`grep -cE "\| (SITE-0[2-6]|MAP-0[1-4]|REPT-0[1-7]|DATA-1[1-3]|SETT-04) \| Phase 5 \| Complete" .planning/REQUIREMENTS.md` returns 20)
    - ROADMAP.md: Phase 5 section shows `Plans: 12 plans` (no longer TBD); 12 PLAN.md files listed
    - 05-VALIDATION.md frontmatter has `nyquist_compliant: true` AND `wave_0_complete: true`
    - 05-VALIDATION.md Per-Task Verification Map has at minimum 20 rows (one per Task across the 11 implementation plans + this plan's tasks)
    - 05-RESEARCH.md contains literal `## Open Questions (RESOLVED)` heading
    - 05-RESEARCH.md RESOLVED block addresses all 4 original Open Questions
    - STATE.md contains Phase 5 closure block (search for `**Phase 5 closure`)
  </acceptance_criteria>
  <done>REQUIREMENTS / ROADMAP / VALIDATION / RESEARCH / STATE all reconciled with evidence trail.</done>
</task>

<task type="auto" tdd="false">
  <name>Task 3: UX-03 vocabulary audit + commit</name>
  <read_first>
    - .planning/phases/03-provisioning-gateways-devices-bulk-import/03-10-PLAN.md (Phase 3 UX-03 audit pattern reference)
    - All new Phase 5 source files: internal/{aggregate,report,map,floorplan,settings}/, web/src/{routes/reports,components/map,components/floor-plan,components/settings}/
  </read_first>
  <action>
**Step A — Run vocabulary search across new Phase 5 surfaces:**

```bash
mkdir -p .planning/phases/05-aggregates-reports-map-floor-plans
cat > .planning/phases/05-aggregates-reports-map-floor-plans/05-VOCAB-AUDIT.md << 'EOF'
# Phase 5 UX-03 Vocabulary Audit

**Date:** $(date +%Y-%m-%d)
**Pattern:** No ChirpStack-native terminology in customer-visible surfaces (UX-03 invariant established Phase 3).

## Banned terms (search list)

- tenant
- application (when used in ChirpStack sense — "application/N/device/..." MQTT topic; OK as "report application")
- dev_eui (OK in CLI / debug surfaces only; never in UI labels)
- app_eui
- join_eui
- network_server
- gateway_bridge
- chirpstack (only OK in Settings → ChirpStack connection card per Phase 1)

## Surfaces audited

- web/src/routes/reports/ (Plan 05-09)
- web/src/components/map/ (Plan 05-08)
- web/src/components/floor-plan/ (Plan 05-10)
- web/src/components/settings/DataRetentionCard.tsx + EditRetentionDialog.tsx (Plan 05-11)
- All copy strings in 05-UI-SPEC.md §Copywriting Contract are in scope

## Audit results

\`\`\`
# Frontend grep — Phase 5 new surfaces only
$(rg -i --type tsx --type ts "tenant|app_eui|join_eui|network_server|gateway_bridge|dev_eui|chirpstack" \
  web/src/routes/reports/ \
  web/src/components/map/ \
  web/src/components/floor-plan/ \
  web/src/components/settings/DataRetentionCard.tsx \
  web/src/components/settings/EditRetentionDialog.tsx \
  2>&1 || echo "(no matches — clean)")

# Playwright specs
$(rg -i "tenant|app_eui|join_eui|network_server|gateway_bridge" web/playwright/specs/ 2>&1 || echo "(no matches — clean)")
\`\`\`

## Verdict

Expected: zero matches outside the documented exception zones (Settings → ChirpStack connection card; CLI debug helpers).

If any match landed, it MUST be fixed in this plan before commit. List any fixes here:

(Fill in by executor.)
EOF
```

The executor runs the actual `rg` command and writes the verdict file with real output.

**Step B — Final commit:**

If all greps clean AND `go test ./... && pnpm --dir web test:run && pnpm --dir web exec playwright test` all PASS, commit:

```bash
git add .planning/phases/05-aggregates-reports-map-floor-plans/
git add .planning/REQUIREMENTS.md .planning/ROADMAP.md .planning/STATE.md
git add web/playwright/
git commit -m "docs(05-12): Phase 5 closure — 6 E2E + REQUIREMENTS + ROADMAP + VALIDATION + RESEARCH reconciliation + UX-03 audit"
```
  </action>
  <verify>
    <automated>! rg -i --type tsx --type ts "tenant|app_eui|join_eui|network_server|gateway_bridge" web/src/routes/reports/ web/src/components/map/ web/src/components/floor-plan/ web/src/components/settings/DataRetentionCard.tsx web/src/components/settings/EditRetentionDialog.tsx &amp;&amp; test -f .planning/phases/05-aggregates-reports-map-floor-plans/05-VOCAB-AUDIT.md &amp;&amp; go test ./... -race -count=1 -short &amp;&amp; pnpm --dir web test:run</automated>
  </verify>
  <acceptance_criteria>
    - `.planning/phases/05-aggregates-reports-map-floor-plans/05-VOCAB-AUDIT.md` exists with literal "(no matches — clean)" results blocks OR explicit list of fixed surfaces
    - `rg -i "tenant|app_eui|join_eui|network_server|gateway_bridge" web/src/routes/reports/ web/src/components/map/ web/src/components/floor-plan/ web/src/components/settings/DataRetentionCard.tsx web/src/components/settings/EditRetentionDialog.tsx` returns 0 matches
    - `rg -i "dev_eui" web/src/components/map/ web/src/components/floor-plan/` returns 0 matches (dev_eui is acceptable in device-list pages but NOT in floor-plan or map)
    - Git working tree clean after final commit
    - Final test suite (Go + frontend unit + Playwright E2E) all green
  </acceptance_criteria>
  <done>UX-03 vocabulary clean; Phase 5 complete; STATE.md reflects closure; git committed.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

Phase closure is documentation + tests only — no new attack surface introduced. The threat models from plans 05-01 through 05-11 cover all production-code concerns.

## STRIDE Threat Register

(No new threats — this is a reconciliation plan.)
</threat_model>

<verification>
1. `go test ./... -race -count=1` exits 0
2. `pnpm --dir web test:run` exits 0
3. `pnpm --dir web exec playwright test` exits 0 against compose-bundled smoke
4. `! grep -rn "test.skip" web/playwright/specs/` (no skipped specs in Phase 5)
5. `grep -cE "\| (SITE-|MAP-|REPT-|DATA-|SETT-04)" .planning/REQUIREMENTS.md | grep "Complete"` proves status flipped
6. `grep -q "nyquist_compliant: true" .planning/phases/05-aggregates-reports-map-floor-plans/05-VALIDATION.md`
7. `grep -q "Open Questions (RESOLVED)" .planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md`
8. UX-03 audit: zero matches for banned vocab on Phase 5 new surfaces
</verification>

<success_criteria>
- 6 Playwright E2E specs ship with full bodies; no test.skip remains
- Phase 5 shared fixture (phase5-fleet.ts) ships and is used by 3+ specs
- 19 Phase 5 requirement IDs + SETT-04 marked Complete in REQUIREMENTS.md with evidence trail
- ROADMAP.md Phase 5 plans block populated with 12 plan entries
- 05-VALIDATION.md per-task verification map populated; nyquist_compliant: true
- 05-RESEARCH.md Open Questions all four marked RESOLVED with the resolution path
- STATE.md Phase 5 closure block recorded
- 05-VOCAB-AUDIT.md ships with zero banned-vocab matches
- Final test suite (Go + frontend unit + Playwright E2E) all green
</success_criteria>

<output>
After completion, create `.planning/phases/05-aggregates-reports-map-floor-plans/05-12-SUMMARY.md` recording:
- Final test counts (Go: N passed; frontend unit: N passed; Playwright: N specs N scenarios all green)
- Total commits across Phase 5 (count of plan SUMMARY files = 12)
- Cumulative plan-execution time (sum from STATE.md performance table)
- Whether any RESEARCH Open Question resolution surfaced unexpected technical debt (none expected; document if found)
- Sign-off line: "Phase 5 complete — DATA-11/12/13, REPT-01..07, MAP-01..04, SITE-02..06, SETT-04 all shipped with evidence."
</output>
