---
phase: 3
slug: provisioning-gateways-devices-bulk-import
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-05-11
---

# Phase 3 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework (Go)** | `go test` (stdlib) + `testcontainers-go` for Postgres + ChirpStack mock |
| **Framework (Web)** | `vitest` (unit/component) + `@playwright/test` (E2E) |
| **Config file (Go)** | `internal/**/testdata/`, table-driven tests; no central config |
| **Config file (Web)** | `web/vitest.config.ts`, `web/playwright.config.ts` |
| **Quick run command** | `go test ./internal/... -short && (cd web && pnpm test -- --run)` |
| **Full suite command** | `go test ./... && (cd web && pnpm test -- --run && pnpm exec playwright test)` |
| **Estimated runtime** | ~90s quick · ~6–8min full (testcontainers ChirpStack adds ~30s startup) |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/{touched_pkg}/... -run {Touched}` plus `pnpm test -- --run {touched}`
- **After every plan wave:** Run the **Quick run command** above
- **Before `/gsd-verify-work`:** **Full suite command** must be green (incl. Playwright)
- **Max feedback latency:** quick ≤90s · full ≤8min

---

## Per-Task Verification Map

> Filled by planner during plan creation. Each task in a `*-PLAN.md` MUST list an `<automated>` block referencing one of the test files below, or declare an explicit Wave 0 dependency.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 3-00-01 | 00 | 0 | infra | — | testcontainers boots Postgres+CS mock | infra | `go test ./internal/testsupport/...` | ❌ W0 | ⬜ pending |
| 3-XX-YY | XX | N | REQ-{XX} | T-3-{N} / — | {expected secure behavior or "N/A"} | unit/integration/e2e | `{command}` | ✅ / ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/testsupport/chirpstack_mock.go` — in-process gRPC mock for `GatewayService`, `DeviceService`, `ApplicationService` (extends Phase 2 mock if present)
- [ ] `internal/testsupport/postgres_container.go` — testcontainers Postgres+Timescale bootstrap + migrations applied (reuse if exists from Phase 2)
- [ ] `internal/testsupport/eui_fixtures.go` — canonical valid/invalid DevEUI/Key fixtures (16-hex / 32-hex / 8-hex DevAddr, plus malformed cases)
- [ ] `internal/testsupport/xlsx_fixtures.go` — pre-built XLSX byte slices for: happy path 5 rows, mixed valid/invalid 10 rows, intra-file duplicate, already-exists fixture, malformed-EUI fixture
- [ ] `web/tests/fixtures/import-template.xlsx` — golden file (must match the generator output for round-trip test)
- [ ] `web/playwright/fixtures/admin-session.json` — pre-authenticated admin session storage state (reuse Phase 2 fixture if present)
- [ ] `web/playwright/fixtures/viewer-session.json` — viewer session for negative tests on `device.reveal_secrets`

*If Phase 2 already produced any of these, reuse and update — do not duplicate.*

---

## Required Test Files (Phase 3 — emit Wave 0 or Wave-specific)

### Backend (`go test`)

- [ ] `internal/chirpstack/gateway_test.go` — `CreateGateway`, `UpdateGateway`, `DeleteGateway`, `GetGateway`, `ListGateways`, `GetMetrics` wrapper tests against the in-process mock (GW-01, GW-02, GW-03)
- [ ] `internal/chirpstack/gateway_metrics_cache_test.go` — TTL cache + single-flight refresh; verifies "50+ gateway fleet doesn't fan-out per list refresh" (D-02)
- [ ] `internal/api/gateways_handler_test.go` — chi HTTP handlers for `/api/gateways` CRUD + region default from install config (D-03)
- [ ] `internal/import/euikeys_test.go` — DevEUI / AppEUI / AppKey / DevAddr / NwkSKey / AppSKey normalization (strip `0x`, `:`, `-`, lowercase, length validation) — table-driven (D-05)
- [ ] `internal/import/parser_xlsx_test.go` — excelize/v2 read, BOM handling for inline CSV fallback, Thai-encoded rejection (D-04a)
- [ ] `internal/import/parser_csv_test.go` — UTF-8-with-BOM accept, non-UTF-8 reject with operator-readable error (D-04a)
- [ ] `internal/import/dryrun_test.go` — validate-only path: format errors, intra-file duplicate dev_eui, pre-existing dev_eui → `already_exists`, missing site, missing device_profile (D-06, D-08)
- [ ] `internal/import/commit_test.go` — partial commit (valid rows created, invalid rows reported), per-row outcome log, idempotent re-run (D-06, D-07, D-10, D-11)
- [ ] `internal/import/template_test.go` — generated XLSX template round-trips through `parser_xlsx`; `NumFmt=49` text format applied to DevEUI columns
- [ ] `internal/import/job_ttl_test.go` — 1h TTL between dry-run and commit; expired → `expired` status (D-11)
- [ ] `internal/auth/can_phase3_test.go` — new actions admit admin, deny viewer: `gateway.create/update/archive/restore`, `device.bulk_import`, `device.reveal_secrets` (D-26)
- [ ] `internal/api/devices_reveal_test.go` — POST `/api/devices/:eui/keys` returns OTAA keys or ABP activation; viewer 403; missing device 404; CS unreachable 502; not-activated 409; audit row written with `action='device.reveal_secrets'`, NO secret material in `diff` (D-27, D-28)
- [ ] `internal/api/devices_list_test.go` — server-side filters (site multi-select, status, last_seen window, text q), offset pagination, sortable columns (D-12, D-13, D-14)
- [ ] `internal/api/devices_bulk_decommission_test.go` — N rows → atomic CS+PG per row, partial-success report (D-17)
- [ ] `internal/api/devices_add_test.go` — OTAA flow (`CreateDevice` + `CreateDeviceKeys`), ABP flow (`CreateDevice` + `ActivateDevice`), atomic CS+PG with best-effort CS rollback (D-19..D-25)
- [ ] `internal/api/gateways_decommission_test.go` — soft-delete + CS `DeleteGateway` atomic; 24h-uplink warning surfacing; restore path (D-30, D-31, D-32) — **awaiting Open Q #1 resolution on whether CS DeleteGateway is part of the contract**
- [ ] `internal/audit/bulk_import_envelope_test.go` — 1 envelope row (`action='device.bulk_import'`) + N per-device rows, all sharing `request_id = import_job.job_id` (D-33, D-34)
- [ ] `internal/db/migrations_test.go` — `0018+_gateway`, `0019+_import_jobs`, `0020+_audit_log_vocabulary` apply + roll back cleanly (research recommends 0020 extends CHECK constraints for new action / entity_type values)

### Frontend (`vitest` + Playwright)

- [ ] `web/src/routes/gateways/__tests__/gateways-list.test.tsx` — vitest: filter/search/region badge, sparkline render, archived toggle
- [ ] `web/src/routes/gateways/__tests__/add-gateway-dialog.test.tsx` — vitest: region pre-selected from install default, "Pick on map" rendered-but-disabled with v5 tooltip (D-01)
- [ ] `web/src/routes/devices/__tests__/devices-list.test.tsx` — vitest: TanStack Table + URL-state filters via react-router-dom v7 `useSearchParams` + zod schema (D-12..D-18, corrected per RESEARCH §URL state)
- [ ] `web/src/routes/devices/__tests__/add-device-dialog.test.tsx` — vitest: 5-step OTAA/ABP toggle (D-19), OTAA keys reveal-after-create + Copy (D-21), ABP optional fcnt fields (D-20)
- [ ] `web/src/routes/devices/__tests__/reveal-secrets.test.tsx` — vitest: POST flow + audit-row-shape stub, viewer fork shows no Reveal button (D-26..D-28)
- [ ] `web/src/routes/admin/imports/__tests__/import-dialog.test.tsx` — vitest: file picker → dry-run preview banner + TanStack Table with expandable per-row reason (D-09)
- [ ] `web/src/routes/admin/imports/__tests__/import-detail.test.tsx` — vitest: `/admin/imports/:job_id` outcomes + download-errors.xlsx (D-07, D-36)
- [ ] `web/playwright/specs/gateway-crud.spec.ts` — E2E: create → list → edit → decommission → restore
- [ ] `web/playwright/specs/device-add-otaa.spec.ts` — E2E: add OTAA device end-to-end via stepped dialog → success state shows keys + Copy
- [ ] `web/playwright/specs/device-add-abp.spec.ts` — E2E: add ABP device with fcnt_up/down (D-20)
- [ ] `web/playwright/specs/bulk-import.spec.ts` — E2E: upload XLSX → preview → commit → re-upload same file shows `already_exists` outcomes (D-06)
- [ ] `web/playwright/specs/devices-filters-deeplink.spec.ts` — E2E: deep-link with `?site=...&q=...` restores filter state on reload (D-15)
- [ ] `web/playwright/specs/reveal-secrets-rbac.spec.ts` — E2E: admin reveals, viewer gets no Reveal button + direct POST → 403 (D-26)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Thai-language XLSX file from Excel-Windows-Thai locale | D-04a | Cannot generate genuine TIS-620/CP874 file deterministically in CI | Operator exports Thai CSV from Excel-TH → upload → expect non-UTF-8 error toast; resave as XLSX → upload → success |
| Real ChirpStack v4.17 server uplink count appearing in 24h sparkline | GW-01, D-02 | Live gRPC against a real CS instance, not the mock | Bring up `docker-compose.bundled.yml`, register a gateway, simulate uplinks via `chirpstack-gateway-bridge` test harness, confirm sparkline non-zero |
| Excel round-trip of generated import template preserves DevEUI text formatting | D-04a | Excel's "scientific notation on 16-hex" trap only triggers in real Excel, not excelize | Download template → open in Excel for Windows → save → reopen → confirm DevEUI column still shows full 16-hex |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 90s (quick) / 8min (full)
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
