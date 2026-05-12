---
phase: 07
slug: multi-vendor-breadth-v1-x-differentiators
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-05-12
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

To be filled by planner — every task in PLAN.md files needs a row here mapping task → test type → automated command → file-exists status.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| _TBD_ | _TBD_ | _TBD_ | V2-VEND-01..03 / ALERT-04 | _TBD_ | _TBD_ | _TBD_ | _TBD_ | _TBD_ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `go.mod` adds `github.com/dop251/goja` dependency (D-26 test-runner runtime)
- [ ] `internal/codec/catalog/` package with `embed.FS` for `*.json` (D-01 catalog storage)
- [ ] `internal/codec/catalog_test.go` skeleton — `TestCatalogValid` (D-22 schema validation)
- [ ] `internal/profile/codecs/itron_kinmy_lora_test.go` skeleton — fixture-based decode tests against goja runtime
- [ ] `web/src/routes/settings/VendorCatalogCard.test.tsx` skeleton (D-23 catalog tab)
- [ ] `web/src/components/codec-test-runner/CodecTestRunner.test.tsx` skeleton (D-05..D-08)
- [ ] `web/src/routes/reports/CompareView.test.tsx` skeleton (D-37, D-38)
- [ ] `web/playwright/specs/phase-07-vendor-catalog.spec.ts` skeleton (E2E for Import + Update flows)

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

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references (goja, codec catalog package, test skeletons)
- [ ] No watch-mode flags (use single-run `go test`, `vitest run`, `playwright test`)
- [ ] Feedback latency < 90s
- [ ] `nyquist_compliant: true` set in frontmatter after planner completes

**Approval:** pending
