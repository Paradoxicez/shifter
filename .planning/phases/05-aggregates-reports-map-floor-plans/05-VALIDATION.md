---
phase: 5
slug: aggregates-reports-map-floor-plans
status: draft
nyquist_compliant: false
wave_0_complete: true
created: 2026-05-12
---

# Phase 5 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
> Phase 5 spans four testable subsystems: CAGGs (DATA-11..13), Reports (REPT-01..07), Map (MAP-01..04), and Floor Plans (SITE-02..06). Each subsystem has its own test ladder.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Backend framework** | Go 1.26 `go test` + `testify` + `testcontainers-go/modules/postgres` (TimescaleDB image) |
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

> Populated by plan 05-01 Task 3 (Wave 0 hand-off). One row per task across plans 02–12.
> Automated Command sourced verbatim from each plan's `<verify><automated>` block.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 05-02-1 | 02 | 1 | DATA-11, DATA-12 | T-05-02-01 | CAGG hierarchy emits monotonic deltas per MP; LAG across bucket boundaries; no Pitfall #1 | integration | `go test ./internal/aggregate/... -race -count=1 -short=false -run "TestCAGGHierarchy\|TestRefreshPolicyParams\|TestRetentionPolicy\|TestCAGGChain_DeltaCorrectness" && go test ./internal/db/... -race -count=1 -short=false -run "TestRunMigrations" && grep -q "WITH NO DATA" internal/db/migrations/0025_cagg_hourly.up.sql && grep -q "WITH NO DATA" internal/db/migrations/0026_cagg_daily.up.sql && grep -q "WITH NO DATA" internal/db/migrations/0027_cagg_monthly.up.sql && grep -q "WITH NO DATA" internal/db/migrations/0028_cagg_yearly.up.sql` | ✅ (skeleton in 05-01) | ⬜ pending |
| 05-02-2 | 02 | 1 | DATA-13 | — | retention_config seeded with D-09 defaults inside FinishSetup Serializable txn | integration | `go test ./internal/install/... -race -count=1 -short=false -run "TestFinishSetup" && go test ./internal/db/... -race -count=1 -short=false -run "TestRunMigrations" && grep -q "INSERT INTO retention_config" internal/install/finish.go` | ✅ (skeleton in 05-01) | ⬜ pending |
| 05-03-1 | 03 | 2 | REPT-01, REPT-02, REPT-07 | T-05-03-01 | Period deltas computed with YoY silent fallback; scope+group routing hits correct CAGG level | integration | `just sqlc && go test ./internal/report/... -race -count=1 -short=false -run "TestPeriodDelta\|TestReportScopeGrouping" && go test ./internal/db/... -race -count=1 -short=false -run "TestRunMigrations"` | ✅ (skeleton in 05-01) | ⬜ pending |
| 05-03-2 | 03 | 2 | REPT-03 | T-05-03-02 | CSV UTF-8 BOM + ISO timestamps + comma separator + timezone header | unit | `go test ./internal/report/... -race -count=1 -short -run TestCSVFormat` | ✅ (skeleton in 05-01) | ⬜ pending |
| 05-03-3 | 03 | 2 | REPT-04 | — | Excel 3-sheet structure (Summary/Period Detail/Meter Detail); bold totals row | unit | `go test ./internal/report/... -race -count=1 -short -run "TestExcelFormat\|TestGenerateHandler"` | ✅ (skeleton in 05-01) | ⬜ pending |
| 05-04-1 | 04 | 2 | MAP-01, MAP-02, MAP-03, MAP-04 | — | Map data endpoint returns sites+gateways with lat/lng; OSM tile URL in response; no paid API key | unit | `just sqlc && go test ./internal/map/... -race -count=1 -short -run "TestMapData\|TestOSMTileURL"` | ✅ (skeleton in 05-01) | ⬜ pending |
| 05-05-1 | 05 | 2 | SITE-02, SITE-03 | — | Floor plan + placement migrations round-trip; FK cascade from site | integration | `just sqlc && go test ./internal/db/... -race -count=1 -short=false -run "TestRunMigrations_(Clean\|FloorPlanCascadeFromSite\|PlacementFractionalBounds)"` | ❌ W0 | ⬜ pending |
| 05-05-2 | 05 | 2 | SITE-03 | T-05-05-01 | Image upload validates MIME + dimensions BEFORE write; 10MB cap; PNG+JPG only | integration | `go test ./internal/floorplan/... -race -count=1 -short -run "TestImageUpload\|TestMultiFloor\|TestReplaceKeepsPins" && grep -q "floor_plans:/var/lib/shifter/floor-plans" compose/bundled.yml && grep -q "floor_plans:/var/lib/shifter/floor-plans" compose/external.yml` | ✅ (skeleton in 05-01) | ⬜ pending |
| 05-06-1 | 06 | 3 | REPT-05 | — | PDF page 'N of M' footer on every page; logo + display_name + address header | unit | `go test ./internal/report/... -race -count=1 -short -run "TestPDFBranding"` | ✅ (skeleton in 05-01) | ⬜ pending |
| 05-06-2 | 06 | 3 | REPT-06 | T-05-06-02, T-05-06-03, T-05-06-04 | Worker enqueue atomic with report record; StatusHandler returns pdf_status; compose volumes declared | integration | `go test ./internal/report/... -race -count=1 -short=false -run "TestPDFJob\|TestEnqueuePDF_AtomicWithReport\|TestCleanupWorker\|TestGenerateHandler_EnqueuesPDFJob\|TestStatusHandler_" && grep -q "reports_cache:/var/lib/shifter/reports" compose/bundled.yml && grep -q "reports_cache:/var/lib/shifter/reports" compose/external.yml && grep -q "river.NewClient" cmd/serve/serve.go && grep -q 'r.Get("/api/reports/{id}"' internal/report/handlers.go` | ✅ (skeleton in 05-01) | ⬜ pending |
| 05-07-1 | 07 | 3 | SITE-04 | T-05-07-01 | Placement x_frac/y_frac clamped to [0,1]; same-site integrity guard; CRUD via PATCH | integration | `just sqlc && go test ./internal/floorplan/... -race -count=1 -short=false -run "TestPlacementCRUD\|TestPlacement_Rejects\|TestPlacement_Upsert\|TestListPlacements"` | ✅ (skeleton in 05-01) | ⬜ pending |
| 05-07-2 | 07 | 3 | SITE-04, SITE-05 | T-05-07-02 | Auth-gated image serve; UUID validated; path traversal mitigated; decommission DELETEs placement in same tx | integration | `go test ./internal/floorplan/... ./internal/device/... ./internal/http/... -race -count=1 -short=false -run "TestFloorPlanImageAuthGated\|TestDecommission"` | ✅ (skeleton in 05-01) | ⬜ pending |
| 05-08-1 | 08 | 4 | MAP-01, MAP-02, MAP-03, MAP-04 | — | MapView renders OSM tiles; auto-fit bounds; Bangkok fallback; cluster above ~50; sidebar nav | unit + e2e | `pnpm --dir web test:run --reporter=basic web/src/components/map web/src/routes/map.test.tsx web/src/components/shell/sidebar.test.tsx && pnpm --dir web build && pnpm --dir web exec playwright test --list map-drill-down.spec.ts 2>&1 | grep -c "›"` | ✅ (skeleton in 05-01) | ⬜ pending |
| 05-08-2 | 08 | 4 | MAP-01 (GW-04) | — | MapPicker dialog wired into gateway create/edit; lat/lng populated from map click | unit | `pnpm --dir web test:run --reporter=basic web/src/components/map/MapView.test.tsx web/src/components/gateways/ && pnpm --dir web build` | ✅ (skeleton in 05-01) | ⬜ pending |
| 05-09-1 | 09 | 5 | REPT-01, REPT-02, REPT-03, REPT-04, REPT-05, REPT-06, REPT-07 | T-05-09-01 | useReportGenerate hook + config panel + URL state; result panel renders 3 download tiles | unit | `pnpm --dir web test:run --reporter=basic web/src/routes/reports/ && pnpm --dir web build && grep -q "export function useReportGenerate" web/src/routes/reports/useReportGenerate.ts && grep -q "useReportGenerate(" web/src/routes/reports/index.tsx` | ✅ (skeleton in 05-01) | ⬜ pending |
| 05-09-2 | 09 | 5 | REPT-06, REPT-07 | T-05-09-01 | PDF spinner→download; toast on ready; navigate-away loses result panel | unit + e2e | `pnpm --dir web test:run --reporter=basic web/src/routes/reports/ && pnpm --dir web build && pnpm --dir web exec playwright test --list reports-generate.spec.ts 2>&1 | grep -c "›"` | ✅ (skeleton in 05-01) | ⬜ pending |
| 05-10-1 | 10 | 5 | SITE-03, SITE-04 | — | pdfToPng 150-DPI canvas; UploadFloorPlanDialog; ReplaceImageDialog with pin-keep confirmation | unit | `pnpm --dir web test:run --reporter=basic web/src/lib/pdfToPng.test.ts web/src/components/floor-plan/ && pnpm --dir web build` | ✅ (skeleton in 05-01) | ⬜ pending |
| 05-10-2 | 10 | 5 | SITE-04, SITE-05, SITE-06 | — | FloorPlanCanvas click→frac coords; DevicePin state colors; SSE health updates | unit + e2e | `pnpm --dir web test:run --reporter=basic web/src/components/floor-plan/ web/src/lib/pdfToPng.test.ts && pnpm --dir web build && pnpm --dir web exec playwright test --list floor-plan-pinning.spec.ts floor-plan-health.spec.ts site-drill-through.spec.ts 2>&1 | grep -c "›"` | ✅ (skeleton in 05-01) | ⬜ pending |
| 05-11-1 | 11 | 5 | DATA-13 | T-05-11-01, T-05-11-02 | PATCH retention reconciles policies in-tx; pgx-only (no lib/pq); range-guarded per UI-SPEC | integration | `just sqlc && go test ./internal/settings/... -race -count=1 -short=false -run "TestRetentionConfig" && test "$(grep -r 'lib/pq' internal/settings/ \| wc -l \| tr -d ' ')" = "0"` | ✅ (skeleton in 05-01) | ⬜ pending |
| 05-11-2 | 11 | 5 | DATA-13, SETT-04 | T-05-11-01 | Frontend Data Retention card + EditRetentionDialog; REQUIREMENTS.md updated | unit + e2e | `pnpm --dir web test:run --reporter=basic web/src/routes/settings.test.tsx web/src/components/settings/ && pnpm --dir web build && grep -qE "^\| SETT-04 \| Phase 5" .planning/REQUIREMENTS.md && pnpm --dir web exec playwright test --list retention-settings.spec.ts 2>&1 | grep -c "›"` | ✅ (skeleton in 05-01) | ⬜ pending |
| 05-12-1 | 12 | 6 | SITE-02..06, MAP-01..04, REPT-01..07, DATA-11..13 | — | All 6 E2E spec bodies implemented; no test.skip remains in playwright/specs/; short suite green | e2e | `pnpm --dir web exec playwright test --list 2>&1 \| grep -E "reports-generate\|map-drill-down\|floor-plan-pinning\|site-drill-through\|floor-plan-health\|retention-settings" \| wc -l && ! grep -rn "test\.skip" web/playwright/specs/ && go test ./... -race -count=1 -short && pnpm --dir web test:run` | ✅ (skeleton in 05-01) | ⬜ pending |
| 05-12-2 | 12 | 6 | SITE-02..06, MAP-01..04, REPT-01..07, DATA-11..13, SETT-04 | — | REQUIREMENTS.md + ROADMAP.md + VALIDATION.md + RESEARCH.md reconciled; nyquist_compliant flipped | documentation | `grep -cE "\| (SITE-0[2-6]\|MAP-0[1-4]\|REPT-0[1-7]\|DATA-1[1-3]\|SETT-04) \| Phase 5 \| Complete" .planning/REQUIREMENTS.md && grep -q "Plans: 12 plans" .planning/ROADMAP.md && grep -q "nyquist_compliant: true" .planning/phases/05-aggregates-reports-map-floor-plans/05-VALIDATION.md && grep -q "Open Questions (RESOLVED)" .planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md` | ❌ W0 | ⬜ pending |
| 05-12-3 | 12 | 6 | SITE-02..06, MAP-01..04, REPT-01..07 | — | UX-03 vocabulary audit: no ChirpStack terms leak into Phase 5 UI components | e2e | `! rg -i --type tsx --type ts "tenant\|app_eui\|join_eui\|network_server\|gateway_bridge" web/src/routes/reports/ web/src/components/map/ web/src/components/floor-plan/ web/src/components/settings/DataRetentionCard.tsx web/src/components/settings/EditRetentionDialog.tsx && test -f .planning/phases/05-aggregates-reports-map-floor-plans/05-VOCAB-AUDIT.md && go test ./... -race -count=1 -short && pnpm --dir web test:run` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Subsystem Validation Ladders

### CAGGs (DATA-11, DATA-12, DATA-13)

| Layer | Test type | Where | Verifies |
|-------|-----------|-------|----------|
| 1 | Migration round-trip | `internal/db/migrations_test.go` | Each `0025_cagg_hourly` / `0026_cagg_daily` / `0027_cagg_monthly` / `0028_cagg_yearly` up+down clean against Timescale image |
| 2 | Refresh-policy assertion | `internal/aggregate/aggregate_test.go` | `start_offset ≤ raw_retention` per DATA-12; `end_offset ≥ 2 × max(expected_interval_s)` |
| 3 | CAGG-over-CAGG ingest | `internal/aggregate/aggregate_test.go` | Insert N raw rows → hourly populates → daily populates from hourly → monthly populates from daily → query end-to-end sums match |
| 4 | Retention config CRUD | `internal/settings/retention_test.go` | Setting updates apply to the right policy; default seed at install matches D-09 |

### Reports (REPT-01..07)

| Layer | Test type | Where | Verifies |
|-------|-----------|-------|----------|
| 1 | CSV format | `internal/report/csv_test.go` | UTF-8 BOM present; ISO-8601 timestamps in install timezone; timezone label in header; comma separator |
| 2 | Excel format | `internal/report/excel_test.go` (excelize) | Multi-sheet (Summary / Per-period / Per-meter); formatted dates; bold totals row |
| 3 | PDF generation | `internal/report/pdf_test.go` (maroto) | Header (logo + display_name + address); footer ("Generated <ts> <tz> — page X of Y"); summary + detail sections render |
| 4 | Period-delta math | `internal/report/delta_test.go` | vs previous period always present; YoY present when ≥1 measurement in prior-year window; silent fallback otherwise |
| 5 | Scope/group HTTP | `internal/report/handlers_test.go` | All 3 scope radios (all/site/single); group-by utility class hides absent capabilities |
| 6 | Background job | `internal/report/pdf_worker_test.go` (River) | Job enqueued in same tx as audit row; worker produces file at expected path; toast wire-up |
| 7 | Generate-once flow | `web/src/routes/reports/index.test.tsx` | Result panel renders CSV/Excel/PDF tiles; CSV+Excel immediate; PDF spinner→download |
| 8 | E2E | `web/playwright/specs/reports-generate.spec.ts` | Login → /reports → configure → Generate → download CSV + Excel → PDF tile resolves to downloadable |

### Map (MAP-01..04)

| Layer | Test type | Where | Verifies |
|-------|-----------|-------|----------|
| 1 | Map data endpoint | `internal/map/handler_test.go` | Returns sites + gateways with lat/lng + summary fields per D-15 popup |
| 2 | Frontend container | `web/src/components/map/MapView.test.tsx` | Auto-fit bounding box logic; Bangkok zoom-5 fallback for zero-data; capability-gated popup fields |
| 3 | Clustering | `web/src/components/map/MapView.test.tsx` | Marker click emits popup; "View site" navigates to /sites/:id; clusters above ~50 |
| 4 | E2E | `web/playwright/specs/map-drill-down.spec.ts` | Map renders with markers, cluster forms above ~50, popup → site detail |

### Floor Plans (SITE-02..06)

| Layer | Test type | Where | Verifies |
|-------|-----------|-------|----------|
| 1 | Migrations | `internal/db/migrations_test.go` | `floor_plan` + `device_floor_plan_placement` round-trip; FK on cascade-delete of site |
| 2 | Floor-plan CRUD | `internal/floorplan/handlers_test.go` | Upload validates MIME + dims (10MB / 8192×8192); list ordered by sort_order; replace keeps pins |
| 3 | Placement CRUD | `internal/floorplan/placement_test.go` | Insert/update by `(x_frac, y_frac)` clamped [0,1]; decommission removes placement in same tx + audit row |
| 4 | Static file serving | `internal/http/floorplan_static_test.go` | Auth-gated `/floor-plans/:id/image`; viewer GETs 200, anonymous 401 |
| 5 | pdf.js client-side | `web/src/lib/pdfToPng.test.ts` | Renders PDF page 1 to 150-DPI canvas, exports PNG blob with expected dims |
| 6 | Pin canvas + drag | `web/src/components/floor-plan/FloorPlanCanvas.test.tsx` | Click coords → fractional coords math; drag updates fractional; right-click triggers remove confirm |
| 7 | Marker state colors | `web/src/components/floor-plan/DevicePin.test.tsx` | Green/yellow/red per D-22 threshold matrix; hover label; click popover |
| 8 | E2E | `web/playwright/specs/floor-plan-pinning.spec.ts` | Upload PNG → place 3 devices → reload → pins persist → decommission one → pin gone |

---

## Wave 0 Requirements

> Plan 05-01 completed Wave 0. All checkboxes now reflect landed state.

- [x] **Frontend deps installed:** `react-leaflet@5.0.0`, `leaflet@1.9.4`, `leaflet.markercluster@1.5.3`, `react-leaflet-cluster@4.1.3`, `pdfjs-dist@5.7.284` (peer-deps verified against React 19)
- [x] **Backend deps installed:** `github.com/johnfercher/maroto/v2 v2.4.0`, `github.com/riverqueue/river v0.36.0`, `github.com/riverqueue/river/riverdriver/riverpgxv5 v0.36.0`
- [x] **Skeleton test files** for every subsystem ladder (22 files; see Per-Task Verification Map File Exists column)
- [x] **River DB migration** embedded in golang-migrate sequence as `0024_river_tables` — round-trip test `TestRunMigrations_RiverDownUpClean` passes
- [x] **TimescaleDB testcontainer image** confirmed reachable (already used by Phase 2/4; `TestRunMigrations_Clean` passes)
- [x] **Playwright spec files** for all 6 Phase 5 E2E behaviors (all use `test.skip`; bodies in plan 05-12)
- [x] **pdfjs worker shim** at `web/src/lib/pdfWorker.ts` (Pitfall #3 mitigation; `import.meta.url` pattern)
- [x] **cumulative_delta verdict** at `internal/aggregate/CUMULATIVE_DELTA.md` (ABSENT; plan 05-02 owns LAG() computation)

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

- [x] All tasks have `<automated>` verify commands in Per-Task Verification Map
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all skeleton test files (see Wave 0 Requirements above)
- [x] No watch-mode flags in any automated command
- [x] Feedback latency < 30s for quick run
- [ ] `nyquist_compliant: true` set in frontmatter (plan 05-12 flips this after all bodies land)

**Wave 0 hand-off:** `wave_0_complete: true` — Wave 1 (plans 02–05) is unblocked.

**Approval:** pending (plan 05-12 flips nyquist_compliant after all spec bodies land)
