---
phase: 5
slug: aggregates-reports-map-floor-plans
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-05-12
---

# Phase 5 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
> Phase 5 spans four testable subsystems: CAGGs (DATA-11..13), Reports (REPT-01..07), Map (MAP-01..04), and Floor Plans (SITE-02..06). Each subsystem has its own test ladder.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Backend framework** | Go 1.25 `go test` + `testify` + `testcontainers-go/modules/postgres` (TimescaleDB image) |
| **Frontend framework** | vitest 4.x (unit) + Playwright 1.59 (E2E, established Phase 3/4) |
| **Config files** | `go.mod`, `web/vitest.config.ts`, `web/playwright.config.ts` (Phase 1 + 4 baselines) |
| **Quick run (backend)** | `go test ./... -race -count=1 -short` |
| **Quick run (frontend unit)** | `pnpm --dir web test:run` |
| **Full suite (backend)** | `go test ./... -race -count=1` (no `-short`; runs testcontainer integration tests) |
| **Full suite (frontend)** | `pnpm --dir web test:run && pnpm --dir web build` |
| **Full suite (E2E)** | `pnpm --dir web test:e2e` (requires `shifter serve` + Caddy via `just compose-smoke-bundled`) |
| **Estimated runtime** | Quick ~30s · Full backend ~3-5min (testcontainers) · Full frontend ~1min · E2E ~2-4min |

---

## Sampling Rate

- **After every task commit:** Run quick backend (`go test ./... -race -short`) for Go tasks; quick frontend (`pnpm test:run`) for web tasks
- **After every plan wave:** Run full backend suite + full frontend suite (no `-short`)
- **Before `/gsd-verify-work`:** Full backend + full frontend + E2E must all be green
- **Max feedback latency:** 30 seconds for quick run; 5 minutes for full

---

## Per-Task Verification Map

> Populated by the planner during Wave 0 / task definition. Each task lands a row mapping its `<automated>` block to the requirement(s) it covers.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 05-XX-NN | XX | N | REQ-{XX} | T-05-XX / — | {expected secure behavior or "N/A"} | unit/integration/e2e | `{command}` | ✅ / ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Subsystem Validation Ladders

### CAGGs (DATA-11, DATA-12, DATA-13)

| Layer | Test type | Where | Verifies |
|-------|-----------|-------|----------|
| 1 | Migration round-trip | `internal/db/migrations_test.go` | Each `0026_cagg_hourly` / `0027_cagg_daily` / `0028_cagg_monthly` / `0029_cagg_yearly` up+down clean against Timescale image |
| 2 | Refresh-policy assertion | `internal/db/cagg_policy_test.go` | `start_offset ≤ raw_retention` per DATA-12; `end_offset ≥ 2 × max(expected_interval_s)` |
| 3 | CAGG-over-CAGG ingest | `internal/db/cagg_chain_test.go` | Insert N raw rows → hourly populates → daily populates from hourly → monthly populates from daily → query end-to-end sums match |
| 4 | Retention config CRUD | `internal/settings/retention_test.go` | Setting updates apply to the right policy; default seed at install matches D-09 |

### Reports (REPT-01..07)

| Layer | Test type | Where | Verifies |
|-------|-----------|-------|----------|
| 1 | CSV format | `internal/reports/csv_test.go` | UTF-8 BOM present; ISO-8601 timestamps in install timezone; timezone label in header; comma separator |
| 2 | Excel format | `internal/reports/xlsx_test.go` (excelize) | Multi-sheet (Summary / Per-period / Per-meter); formatted dates; bold totals row |
| 3 | PDF generation | `internal/reports/pdf_test.go` (maroto) | Header (logo + display_name + address); footer ("Generated <ts> <tz> — page X of Y"); summary + detail sections render |
| 4 | Period-delta math | `internal/reports/delta_test.go` | vs previous period always present; YoY present when ≥1 measurement in prior-year window; silent fallback otherwise |
| 5 | Scope/group HTTP | `internal/reports/handlers_test.go` | All 3 scope radios (all/site/single); group-by utility class hides absent capabilities |
| 6 | Background job | `internal/reports/pdf_worker_test.go` (River) | Job enqueued in same tx as audit row; worker produces file at expected path; toast wire-up |
| 7 | Generate-once flow | `web/src/routes/reports/*.test.tsx` | Result panel renders CSV/Excel/PDF tiles; CSV+Excel immediate; PDF spinner→download |
| 8 | E2E | `web/playwright/specs/reports-generate.spec.ts` | Login → /reports → configure → Generate → download CSV + Excel → PDF tile resolves to downloadable |

### Map (MAP-01..04)

| Layer | Test type | Where | Verifies |
|-------|-----------|-------|----------|
| 1 | Map data endpoint | `internal/api/map_handler_test.go` | Returns sites + gateways with lat/lng + summary fields per D-15 popup |
| 2 | Frontend container | `web/src/routes/map.test.tsx` | Auto-fit bounding box logic; Bangkok zoom-5 fallback for zero-data; capability-gated popup fields |
| 3 | Clustering | `web/src/components/map/SiteMarker.test.tsx` | Marker click emits popup; "View site" navigates to /sites/:id |
| 4 | E2E | `web/playwright/specs/map.spec.ts` | Map renders with markers, cluster forms above ~50, popup → site detail |

### Floor Plans (SITE-02..06)

| Layer | Test type | Where | Verifies |
|-------|-----------|-------|----------|
| 1 | Migrations | `internal/db/migrations_test.go` | `0024_floor_plan` + `0025_device_floor_plan_placement` round-trip; FK on cascade-delete of site |
| 2 | Floor-plan CRUD | `internal/floorplan/handlers_test.go` | Upload validates MIME + dims (10MB / 8192×8192); list ordered by sort_order; replace keeps pins |
| 3 | Placement CRUD | `internal/floorplan/placement_test.go` | Insert/update by `(x_frac, y_frac)` clamped [0,1]; decommission removes placement in same tx + audit row |
| 4 | Static file serving | `internal/http/floorplan_static_test.go` | Auth-gated `/floor-plans/:id/image`; viewer GETs 200, anonymous 401 |
| 5 | pdf.js client-side | `web/src/lib/pdfToPng.test.ts` | Renders PDF page 1 to 150-DPI canvas, exports PNG blob with expected dims |
| 6 | Pin canvas + drag | `web/src/components/floor-plan/FloorPlanCanvas.test.tsx` | Click coords → fractional coords math; drag updates fractional; right-click triggers remove confirm |
| 7 | Marker state colors | `web/src/components/floor-plan/DevicePin.test.tsx` | Green/yellow/red per D-22 threshold matrix; hover label; click popover |
| 8 | E2E | `web/playwright/specs/floor-plan-pinning.spec.ts` | Upload PNG → place 3 devices → reload → pins persist → decommission one → pin gone |

---

## Wave 0 Requirements

> The planner produces a Wave 0 plan that installs new test dependencies and lands skeleton test files for every requirement. Wave 0 commands populate the `❌ W0` markers above to `✅` as files appear.

- [ ] **Frontend deps installed:** `react-leaflet@^5`, `leaflet@^1.9`, `leaflet.markercluster`, `pdfjs-dist` (peer-deps verified against React 19)
- [ ] **Backend deps installed:** `github.com/johnfercher/maroto/v2`, `github.com/riverqueue/river`, `github.com/riverqueue/river/riverdriver/riverpgxv5`
- [ ] **Skeleton test files** for every subsystem ladder above (8 + 8 + 4 + 8 = 28 entries; the planner may collapse where one file covers multiple)
- [ ] **River DB migration** embedded in golang-migrate sequence (research §River setup) — round-trip test added to `migrations_test.go`
- [ ] **TimescaleDB testcontainer image** confirmed reachable in `internal/db/` test helpers (already used by Phase 2/4)
- [ ] **Playwright fixture** for "site with floor plan + N pinned devices" + "fleet with sites for map" added to `web/playwright/fixtures/`

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| PDF visual branding | REPT-05 | Visual review — layout, font readability, logo placement on a real page can't be asserted programmatically beyond presence checks | Generate a sample monthly report; verify logo top-left, address top-right, footer "Generated … page X of Y" present on every page; print preview matches `05-UI-SPEC.md` mock |
| Map zoom + pan smoothness on mobile | MAP-01..04 | Leaflet touch interaction quality is subjective; automated tests don't capture jank | Open `/map` on a real iPhone/Android browser; pinch-zoom 5 times, pan; markers should remain responsive |
| Floor-plan pin drag feel | SITE-04 | Drag-precision UX is qualitative | Upload sample floor plan, drag a pin around at multiple zoom/pan levels; pin lands where the pointer is released within ±2px |
| TimescaleDB CAGG refresh latency in production | DATA-11..13 | CI testcontainers test backfill only; production refresh latency depends on data volume | Insert 1M raw rows over 30 days; trigger hourly→daily refresh; observe runtime in logs ≤ 30s/run |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s for quick run
- [ ] `nyquist_compliant: true` set in frontmatter (after planner fills per-task map)

**Approval:** pending (planner to fill per-task map and flip nyquist_compliant)
