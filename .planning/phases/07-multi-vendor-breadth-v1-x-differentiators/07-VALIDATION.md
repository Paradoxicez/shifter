---
phase: 07
slug: multi-vendor-breadth-v1-x-differentiators
status: complete
nyquist_compliant: true
wave_0_complete: true
created: 2026-05-12
updated: 2026-05-13
---

# Phase 07 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (backend) + vitest (frontend) + Playwright (E2E) |
| **Config file** | `go.mod` (backend) / `web/vitest.config.ts` (frontend) / `web/playwright.config.ts` (E2E) |
| **Quick run command** | `go test ./internal/profile/... ./internal/codec/... ./internal/alert/... ./internal/doctor/...` |
| **Full suite command** | `just test` (or `go test ./... && cd web && pnpm test && pnpm test:e2e`) |
| **Estimated runtime** | ~90 seconds (backend ~30s, frontend ~20s, E2E ~40s) |

---

## Sampling Rate

- **After every task commit:** Run quick command (scoped to changed package)
- **After every plan wave:** Run full suite command
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 90 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 07-01 T1 | 07-01 | 0 | V2-VEND-01 | — | Catalog embed.FS loaded at boot | unit | `go test ./internal/codec/catalog/... -count=1` | `internal/codec/catalog/` | ✅ green |
| 07-01 T2 | 07-01 | 0 | V2-VEND-01 | — | Wave 0 test skeletons created | unit | `go test ./internal/... ./web/... -run _skeleton` | `web/playwright/specs/phase-07-vendor-catalog.spec.ts` | ✅ green |
| 07-02 T1 | 07-02 | 1 | V2-VEND-01 | T-07-02-01 | Migration adds vendor_catalog_entry table | migration | `go test ./internal/db/... -run Migration` | `internal/db/migrations/0044_vendor_catalog.up.sql` | ✅ green |
| 07-02 T2 | 07-02 | 1 | V2-VEND-01 | — | Catalog JSON validated against schema | unit | `go test ./internal/codec/catalog/... -run TestCatalogValid` | `internal/codec/catalog/catalog_test.go` | ✅ green |
| 07-03 T1 | 07-03 | 2 | V2-VEND-01 | — | CatalogLoader loads embedded JSON | unit | `go test ./internal/catalog/... -run TestCatalogLoader` | `internal/catalog/loader.go` | ✅ green |
| 07-03 T2 | 07-03 | 2 | V2-VEND-01 | — | DriftDetect compares DB vs embedded versions | unit | `go test ./internal/catalog/... -run TestDrift` | `internal/catalog/drift_test.go` | ✅ green |
| 07-04 T1 | 07-04 | 3 | V2-VEND-01 | T-07-04-01 | GET /catalog returns JSON list filtered by vendor | unit+integration | `go test ./internal/api/... -run TestCatalogList` | `internal/api/catalog_handler_test.go` | ✅ green |
| 07-04 T2 | 07-04 | 3 | V2-VEND-01 | T-07-04-02 | POST /catalog/import prefills device profile | unit+integration | `go test ./internal/api/... -run TestCatalogImport` | `internal/api/catalog_handler_test.go` | ✅ green |
| 07-05 T1 | 07-05 | 4 | V2-VEND-01 | — | VendorCatalogCard renders catalog tab in settings | unit | `pnpm --dir web test:run -- VendorCatalogCard` | `web/src/routes/settings/VendorCatalogCard.test.tsx` | ✅ green |
| 07-05 T2 | 07-05 | 4 | V2-VEND-01 | — | Catalog list search filter works | unit | `pnpm --dir web test:run -- VendorCatalogCard` | `web/src/routes/settings/VendorCatalogCard.test.tsx` | ✅ green |
| 07-06 T1 | 07-06 | 5 | V2-VEND-01 | — | importFromCatalog + applyCatalogUpdate API mutations | unit | `pnpm --dir web test:run -- catalog` | `web/src/lib/api/catalog.test.ts` | ✅ green |
| 07-06 T2 | 07-06 | 5 | V2-VEND-01 | — | ImportFromCatalogDialog 3-step flow | unit | `pnpm --dir web test:run -- ImportFromCatalogDialog` | `web/src/components/catalog/ImportFromCatalogDialog.test.tsx` | ✅ green |
| 07-06 T3 | 07-06 | 5 | V2-VEND-01 | — | CatalogUpdateModal per-field diff toggles | unit | `pnpm --dir web test:run -- CatalogUpdateModal` | `web/src/components/catalog/CatalogUpdateModal.test.tsx` | ✅ green |
| 07-06 T4 | 07-06 | 5 | V2-VEND-01 | — | E2E import + update flows | E2E | `pnpm --dir web exec playwright test phase-07` | `web/playwright/specs/phase-07-vendor-catalog.spec.ts` | ✅ green (operator-approved 2026-05-13) |
| 07-07 T1 | 07-07 | 6 | V2-VEND-02 | T-07-07-01 | goja sandbox runner decodes Axioma W1 hex | unit | `go test ./internal/codec/... -run TestCodecRunner` | `internal/codec/runner_test.go` | ✅ green |
| 07-07 T2 | 07-07 | 6 | V2-VEND-02 | T-07-07-01 | Runner enforces 50ms timeout + per-run isolation | unit | `go test ./internal/codec/... -run TestCodecRunner_Timeout` | `internal/codec/runner_test.go` | ✅ green |
| 07-07 T3 | 07-07 | 6 | V2-VEND-02 | — | POST /profiles/:id/test-codec HTTP handler | unit | `go test ./internal/api/... -run TestCodecTest` | `internal/api/codec_test_handler_test.go` | ✅ green |
| 07-08 T1 | 07-08 | 7 | V2-VEND-02 | — | CodecTestRunner UI hex input + tab output | unit | `pnpm --dir web test:run -- CodecTestRunner` | `web/src/routes/profiles/CodecTestRunner.test.tsx` | ✅ green |
| 07-08 T2 | 07-08 | 7 | V2-VEND-02 | — | Decoded JSON + Canonical Mapping tabs render | unit | `pnpm --dir web test:run -- CodecTestRunner` | `web/src/routes/profiles/CodecTestRunner.test.tsx` | ✅ green |
| 07-08 T3 | 07-08 | 7 | V2-VEND-02 | — | E2E codec test runner flow | E2E | `pnpm --dir web exec playwright test phase-07` | `web/playwright/specs/phase-07-vendor-catalog.spec.ts` | ✅ green (operator-approved 2026-05-13) |
| 07-09a T1 | 07-09a | 8 | ALERT-04 | T-07-09-01 | Battery curve normalization per profile | unit | `go test ./internal/alert/... -run TestBatteryCurve` | `internal/alert/battery_curve_test.go` | ✅ green |
| 07-09a T2 | 07-09a | 8 | ALERT-04 | — | Normalize anomaly thresholds by profile type | unit | `go test ./internal/alert/... -run TestNormalize` | `internal/alert/normalize_test.go` | ✅ green |
| 07-09b T1 | 07-09b | 8 | ALERT-04 | T-07-09-02 | Profile-aware anomaly worker respects battery curve | unit | `go test ./internal/alert/worker/... -run TestAnomalyWorker_ProfileAware` | `internal/alert/worker/anomaly_worker_test.go` | ✅ green |
| 07-09b T2 | 07-09b | 8 | ALERT-04 | — | Warmup suppression for new meters (<24h) | unit | `go test ./internal/alert/worker/... -run TestWarmup` | `internal/alert/worker/anomaly_worker_test.go` | ✅ green |
| 07-10 T1 | 07-10 | 9 | ALERT-04 | — | Anomaly backtest runs over historical window | unit | `go test ./internal/alert/backtest/... -run TestBacktest` | `internal/alert/backtest/backtest_test.go` | ✅ green |
| 07-10 T2 | 07-10 | 9 | ALERT-04 | — | Backtest result UI card renders fire count | unit | `pnpm --dir web test:run -- BacktestCard` | `web/src/components/alert/BacktestCard.test.tsx` | ✅ green |
| 07-11a T1 | 07-11a | 10 | V2-VEND-03 | — | SaveTemplate CRUD handler + DB schema | unit | `go test ./internal/report/... -run TestSaveTemplate` | `internal/report/template_handler_test.go` | ✅ green |
| 07-11a T2 | 07-11a | 10 | V2-VEND-03 | — | ApplyTemplate prefills report form | unit | `go test ./internal/report/... -run TestApplyTemplate` | `internal/report/template_handler_test.go` | ✅ green |
| 07-11b T1 | 07-11b | 10 | V2-VEND-03 | — | SaveTemplateDialog UI form | unit | `pnpm --dir web test:run -- SaveTemplateDialog` | `web/src/routes/reports/SaveTemplateDialog.test.tsx` | ✅ green |
| 07-11b T2 | 07-11b | 10 | V2-VEND-03 | — | Template picker in report form | unit | `pnpm --dir web test:run -- TemplatePicker` | `web/src/routes/reports/TemplatePicker.test.tsx` | ✅ green |
| 07-12 T1 | 07-12 | 11 | V2-VEND-03 | — | CompareView backend two-meter/two-site handler | unit | `go test ./internal/compare/... -run TestCompareView` | `internal/compare/compare_handler_test.go` | ✅ green |
| 07-12 T2 | 07-12 | 11 | V2-VEND-03 | — | CompareView frontend side-by-side chart | unit | `pnpm --dir web test:run -- CompareView` | `web/src/routes/reports/CompareView.test.tsx` | ✅ green |
| 07-13 T1 | 07-13 | 12 | UX-POWER | — | BulkGatewayImport CSV parse + idempotent upsert | unit | `go test ./internal/gateway/... -run TestBulkGatewayImport` | `internal/gateway/bulk_import_test.go` | ✅ green |
| 07-13 T2 | 07-13 | 12 | UX-POWER | — | BulkGatewayImportDialog dry-run + commit UI | unit | `pnpm --dir web test:run -- BulkGatewayImportDialog` | `web/src/routes/gateways/BulkGatewayImportDialog.test.tsx` | ✅ green |
| 07-14 T1 | 07-14 | 13 | INST-HARDEN | T-07-14-03 | ProbeChirpStack never leaks API key (sentinel test) | unit | `go test ./internal/doctor/... ./internal/install/probe/... -count=1` | `internal/doctor/probes.go` + `internal/install/probe/chirpstack_test.go` | ✅ green |
| 07-14 T2 | 07-14 | 13 | INST-HARDEN | T-07-14-02 | /health/detailed probe_results block present | unit | `go test ./internal/http/... -run TestHealthDetailed -count=1` | `internal/http/health.go` + `internal/http/health_test.go` | ✅ green |
| 07-14 T3 | 07-14 | 13 | V2-VEND-01/02/03 | — | Phase 7 Playwright E2E specs filled in (3 flows) | E2E | `pnpm --dir web exec playwright test phase-07-vendor-catalog.spec.ts` | `web/playwright/specs/phase-07-vendor-catalog.spec.ts` | ✅ green |
| 07-14 T4 | 07-14 | 13 | all Phase 7 | — | REQUIREMENTS + VALIDATION + RETROSPECTIVE reconciled | documentation | `grep -q "ROADMAP SC#5" .planning/RETROSPECTIVE.md` | `.planning/RETROSPECTIVE.md` | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] `go.mod` adds `github.com/dop251/goja` dependency (D-26 test-runner runtime)
- [x] `internal/codec/catalog/` package with `embed.FS` for `*.json` (D-01 catalog storage)
- [x] `internal/codec/catalog_test.go` skeleton — `TestCatalogValid` (D-22 schema validation)
- [x] `internal/profile/codecs/itron_kinmy_lora_test.go` skeleton — fixture-based decode tests against goja runtime
- [x] `web/src/routes/settings/VendorCatalogCard.test.tsx` skeleton (D-23 catalog tab)
- [x] `web/src/components/codec-test-runner/CodecTestRunner.test.tsx` skeleton (D-05..D-08)
- [x] `web/src/routes/reports/CompareView.test.tsx` skeleton (D-37, D-38)
- [x] `web/playwright/specs/phase-07-vendor-catalog.spec.ts` skeleton (E2E for Import + Update flows)

*Wave 0 ensures the test files exist before any production code lands, satisfying Nyquist sampling continuity.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Itron+KINMY codec produces expected canonical mapping in production | V2-VEND-01 / D-48 | Real LoRa uplinks require physical hardware; covered by hex-fixture unit tests for now | After Phase 7 ship, capture 3+ real uplinks from a customer's KINMY module, validate decode matches expected canonical row |
| ChirpStack v4.10+ codec push succeeds for new catalog vendors | D-19 / D-36 | Integration with live ChirpStack required; covered by `internal/chirpstack/device_profile_test.go` mocked tests | On first customer install with new vendor, observe `codec_js_synced_at` populates in `device_profile` row |
| `shifter doctor probe-region` matches gateway region against install identity | D-12 | Requires real gateway + ChirpStack instance with region config | After Phase 7 doctor subcommands ship, run on bundled-compose install and validate output |
| Anomaly threshold defaults for Itron+KINMY don't spam alerts on real 24h staggered uplinks | D-09 / D-42 / D-43 | Requires 60+ days of real Itron+KINMY uplink data | Phase 7 customer beta: monitor `alert` table for `anomaly_*` rule fires per Itron MP; tune defaults if >2/week false-positive |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references (goja, codec catalog package, test skeletons)
- [x] No watch-mode flags (use single-run `go test`, `vitest run`, `playwright test`)
- [x] Feedback latency < 90s
- [x] `nyquist_compliant: true` set in frontmatter after planner completes

**Approval:** Complete — 2026-05-13 (Plan 07-14 Task 4)
