---
phase: 4
slug: realtime-dashboard
status: complete
nyquist_compliant: true
wave_0_complete: true
created: 2026-05-11
closed: 2026-05-11
---

# Phase 4 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework (backend)** | `go test` (Go 1.24+, std `testing` + `testcontainers-go` for Postgres/TimescaleDB) |
| **Framework (frontend)** | `vitest` (unit/component) + `@playwright/test` (E2E) |
| **Config file (backend)** | none — uses default `go test ./...` |
| **Config file (frontend)** | `web/vitest.config.ts`, `web/playwright.config.ts` |
| **Quick run command** | Backend: `go test ./internal/events/... ./internal/dashboard/... ./internal/meteringpoint/... -count=1` · Frontend: `cd web && pnpm test -- --run` |
| **Full suite command** | `make test` (runs `go test ./...` + `cd web && pnpm test -- --run && pnpm exec playwright test`) |
| **Estimated runtime** | quick ~20s · full ~3–5min (Playwright dominates) |

---

## Sampling Rate

- **After every task commit:** Run the quick command relevant to the changed file (Go package quick test OR `pnpm test -- --run path/to/file.test.tsx`)
- **After every plan wave:** Run the full suite command
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 30 seconds (quick) · 5 minutes (full)

---

## Per-Task Verification Map

> Populated during Plan 04-10 Task 2 (phase-closure reconciliation). Each row was run and exited 0 during its respective plan execution.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 04-01-T1 | 01 | 1 | DASH-02 | T-04-01-01 | NOTIFY payload restricted to 7 fields; raw_payload/decoded_object excluded | grep | `grep -q "pg_notify('measurement_inserted'" internal/db/migrations/0021_measurement_inserted_trigger.up.sql && grep -q "AFTER INSERT ON measurement" internal/db/migrations/0021_measurement_inserted_trigger.up.sql && ! grep -qE "raw_payload\|decoded_object" internal/db/migrations/0021_measurement_inserted_trigger.up.sql` | yes | green |
| 04-01-T2 | 01 | 1 | DASH-01 | T-04-01-02 | capabilities CHECK restricts to water/electricity/both | grep | `grep -q "ADD COLUMN capabilities TEXT NOT NULL DEFAULT 'both'" internal/db/migrations/0022_install_capabilities.up.sql && grep -q "CHECK (capabilities IN ('water','electricity','both'))" internal/db/migrations/0022_install_capabilities.up.sql` | yes | green |
| 04-01-T3 | 01 | 1 | DASH-02 | — | expected_interval_s CHECK > 0 prevents zero intervals | grep | `grep -q "ADD COLUMN expected_interval_s INTEGER NOT NULL DEFAULT 3600" internal/db/migrations/0023_device_profile_expected_interval.up.sql && grep -q "expected_interval_s > 0" internal/db/migrations/0023_device_profile_expected_interval.up.sql` | yes | green |
| 04-01-T4 | 01 | 1 | DASH-02 | — | Seed profiles carry correct expected_interval_s values | go test | `go test ./internal/profile/... -run "TestSeed" -count=1 -timeout 30s` | yes | green |
| 04-01-T5 | 01 | 1 | DASH-01, DASH-02 | — | NOTIFY propagates to TimescaleDB chunks; sqlc compiles | go test | `sqlc generate && go build ./... && go test ./internal/db/... -run TestRunMigrations_RoundTrip -count=1 -timeout 120s` | yes | green |
| 04-02-T1 | 02 | 2 | DASH-02 | T-04-02-01 | Non-blocking send prevents slow subscriber blocking listener | go test | `go test ./internal/events/ -run "TestHub" -count=1 -timeout 30s -race` | yes | green |
| 04-02-T2 | 02 | 2 | DASH-03 | T-04-02-01 | LISTEN measurement_inserted reconnects on disconnect | go test | `grep -q "LISTEN measurement_inserted" internal/events/listener.go && go test ./internal/events/ -run "TestListener\|TestHub" -count=1 -timeout 30s -race` | yes | green |
| 04-02-T3 | 02 | 2 | DASH-02 | — | AFTER INSERT trigger propagates NOTIFY to TimescaleDB chunks | integration | `SHIFTER_INTEGRATION_TESTS=1 go test ./internal/events/ -run "TestMeasurementTrigger" -count=1 -timeout 180s` | yes | green |
| 04-03-T1 | 03 | 2 | DASH-03 | T-04-03-01 | SSE handler validates topics ≤64; connID internal-only (not on wire) | go test | `go test ./internal/events/ -run "TestSSE\|TestParseTopics" -count=1 -timeout 30s -race && ! grep -q "handleSubscribe\|handleUnsubscribe" internal/events/handler.go` | yes | green |
| 04-03-T2 | 03 | 2 | DASH-03 | T-04-03-02 | EventsDeps nil-guard prevents router panic without events | grep+build | `grep -q "EventsDeps \*events.Deps" internal/http/router.go && grep -q "events.RegisterRoutes" internal/http/router.go && ! grep -q "/api/events/subscribe" internal/events/routes.go && go build ./...` | yes | green |
| 04-03-T3 | 03 | 2 | DASH-03 | — | Hub.Run goroutine wired in serve.go at boot | grep+test | `grep -q "events.NewHub" cmd/shifter/serve.go && grep -q "eventsHub.Run" cmd/shifter/serve.go && go build ./... && go test ./cmd/shifter/... -count=1 -timeout 120s` | yes | green |
| 04-03-T4 | 03 | 2 | DASH-03 | — | Caddy @sse matcher extended to /api/events with flush_interval -1 | grep | `grep -E '^\s*@sse\s+path\s+.*\/api\/events' Caddyfile && awk '/@sse/,/^\s*}\s*$/' Caddyfile \| grep -q 'flush_interval -1'` | yes | green |
| 04-04-T1 | 04 | 3 | DASH-02, DASH-05 | T-04-04-04 | D-12 bucket schedule; online rule uses server-side timezone | grep+build | `grep -q "time_bucket" internal/db/queries/timeseries.sql && grep -q "2 \* dp.expected_interval_s \* INTERVAL" internal/db/queries/dashboard.sql && sqlc generate && go build ./...` | yes | green |
| 04-04-T2 | 04 | 3 | DASH-01, DASH-02, DASH-04, DASH-05 | T-04-04-03 | KPI endpoints: capability gating; DoS cap 365d; water/elec absent-key pattern | go test | `go test ./internal/dashboard/ -count=1 -timeout 120s -race && go build ./...` | yes | green |
| 04-05-T1 | 05 | 3 | DETL-01, DETL-02 | T-04-05-01 | MP detail SQL; uplinks cursor pagination cap ≤500 | grep+build | `grep -q "2 \* dp.expected_interval_s \* INTERVAL" internal/db/queries/mp_detail.sql && grep -q "time < COALESCE" internal/db/queries/uplinks.sql && sqlc generate && go build ./...` | yes | green |
| 04-05-T2 | 05 | 3 | DETL-01, DETL-02, DETL-03 | — | MP detail + uplinks + signal-history handlers pass tests | go test | `go test ./internal/meteringpoint/ -count=1 -timeout 120s -race && go build ./...` | yes | green |
| 04-06-T1 | 06 | 3 | DASH-03, DASH-04 | T-04-06-01 | useSSE: no /api/events/subscribe path; exponential backoff math verified | vitest | `cd web && pnpm test -- --run src/hooks/useSSE.test.ts && ! grep -q "conn_id\|/api/events/subscribe" src/hooks/useSSE.ts` | yes | green |
| 04-06-T2 | 06 | 3 | DASH-01 | — | useDashboardScope wraps /api/dashboard/scope | vitest | `cd web && pnpm test -- --run src/hooks/useDashboardScope.test.ts` | yes | green |
| 04-07-T1 | 07 | 4 | DASH-01, DASH-02 | T-04-07-02 | KpiCard + KpiGrid + LiveChannelBanner + EmptyStateOnboarding unit tests | vitest | `cd web && pnpm test -- --run src/components/dashboard/` | yes | green |
| 04-07-T2 | 07 | 4 | DASH-01, DASH-03, DASH-06 | T-04-07-01 | Dashboard route auth-required; Sidebar nav wired; build clean | vitest+build | `cd web && pnpm test -- --run src/routes/dashboard.test.tsx src/components/shell/ && pnpm build` | yes | green |
| 04-08-T1 | 08 | 4 | DASH-05 | T-04-08-01 | computeRangeWindow validated; zod.catch('today') fallback; custom >1y blocked | vitest | `cd web && pnpm test -- --run src/lib/dateRange.test.ts src/components/dashboard/DateRangePicker.test.tsx` | yes | green |
| 04-08-T2 | 08 | 4 | DASH-04, DASH-05 | T-04-08-02 | ConsumptionChart + CumulativeChartCard wired to timeseries; build clean | vitest+build | `cd web && pnpm test -- --run src/components/dashboard/ConsumptionChart.test.tsx src/components/dashboard/CumulativeChartCard.test.tsx src/routes/dashboard.test.tsx && pnpm build` | yes | green |
| 04-09-T1 | 09 | 4 | DETL-01, DETL-03 | T-04-09-02, T-04-09-03 | JsonTree uses React text nodes (no dangerouslySetInnerHTML); MAX_DEPTH=32 | vitest | `cd web && pnpm test -- --run src/components/metering-point/JsonTree src/components/metering-point/QualityBadge src/components/metering-point/SparklineTriplet` | yes | green |
| 04-09-T2 | 09 | 4 | DETL-01, DETL-02 | T-04-09-04 | UplinksLogTab 500-row cap; pendingPayload forensic-safety (no auto-swap) | vitest | `cd web && pnpm test -- --run src/components/metering-point/NormalTab src/components/metering-point/AdvancedTab src/components/metering-point/UplinksLogTab && grep -q "@tanstack/react-virtual" web/package.json && grep -E "pendingPayload.*MeasurementDelta" web/src/components/metering-point/AdvancedTab.tsx && grep -q "clearPending" web/src/components/metering-point/AdvancedTab.tsx` | yes | green |
| 04-09-T3 | 09 | 4 | DETL-01, DETL-02, DETL-03 | T-04-09-01 | $id.tsx route; Admin CTAs gated by role; build clean | vitest+build | `cd web && pnpm test -- --run src/routes/metering-points/ src/components/metering-point/ && pnpm build` | yes | green |
| 04-10-T1 | 10 | 4 | DASH-01..06, DETL-01..03 | T-04-10-01 | 6 E2E specs structural contracts; data-kpi attrs wired; no manual migrate step | playwright | `cd web && pnpm exec playwright test --reporter=line dashboard-loads.spec.ts dashboard-live-update.spec.ts dashboard-capability-filter.spec.ts dashboard-date-range.spec.ts metering-point-detail.spec.ts dashboard-mobile.spec.ts && ! grep -q "shifter migrate" playwright.config.ts playwright/helpers/dashboard-fixtures.ts` | yes | green |
| 04-10-T2 | 10 | 4 | DASH-01..06, DETL-01..03 | — | REQUIREMENTS.md reconciled; VALIDATION.md table populated | grep | `grep -c "\| DASH-0. \| Phase 4 \| Complete \|" .planning/REQUIREMENTS.md \| awk '{ if ($1 < 6) exit 1 }' && grep -q "DETL-01 \| Phase 4 \| Complete" .planning/REQUIREMENTS.md && grep -q "nyquist_compliant: true" .planning/phases/04-realtime-dashboard/04-VALIDATION.md` | yes | green |

---

## Wave 0 Requirements

All Wave 0 scaffolds landed across Plans 01–09. Checkboxes reflect shipped state.

### Backend (Go)
- [x] `internal/events/listener_test.go` — LISTEN connection lifecycle, reconnect on disconnect, NOTIFY → channel fan-out (Plan 02)
- [x] `internal/events/trigger_test.go` — **Integration test pinning TimescaleDB AFTER INSERT trigger propagation to chunks** (Plan 02 — 2 tests: TestMeasurementTrigger_PropagatesToChunks + _AcrossChunks)
- [x] `internal/events/handler_test.go` — heartbeat cadence, `Cache-Control: no-cache`, `Content-Type: text/event-stream`, context cancellation flushes cleanly (Plan 03)
- [x] `internal/dashboard/snapshot_handler_test.go` — KPI endpoint shape (water-only / electricity-only / mixed), capability filter applied (Plan 04)
- [x] `internal/meteringpoint/handlers_test.go` — detail endpoint (normal vs JSONB extra), uplink-log pagination (Plan 05)
- [x] `internal/db/migrations_test.go` — migrations 0021/0022/0023 up/down idempotent (Plan 01 — covered by TestRunMigrations_Clean + _RoundTrip)

### Frontend (Vitest)
- [x] `web/src/hooks/useSSE.test.ts` — exponential backoff math, reconnect triggers snapshot refetch, cookie-credentialed connection (Plan 06)
- [x] `web/src/routes/dashboard.test.tsx` — renders KPIs by capability, hides absent capability sections (Plan 07)
- [x] `web/src/components/dashboard/KpiCard.test.tsx` — formatting, online/offline pill states (Plan 07)
- [x] `web/src/components/dashboard/ConsumptionChart.test.tsx` — date-range picker triggers refetch with correct bucket size (Plan 08)
- [x] `web/src/components/metering-point/NormalTab.test.tsx` — cumulative / instantaneous / last-update render (Plan 09; file shipped as NormalTab not NormalView)
- [x] `web/src/components/metering-point/AdvancedTab.test.tsx` — walks JSONB extra keys, JsonTree renders (Plan 09; file shipped as AdvancedTab not AdvancedView)
- [x] `web/src/components/metering-point/UplinksLogTab.test.tsx` — virtualized list, 500-row cap, quality filter chips (Plan 09; file shipped as UplinksLogTab not UplinkLog)

### Frontend (Playwright E2E)
- [x] `web/playwright/specs/dashboard-loads.spec.ts` — login → dashboard renders, KPIs visible (Plan 10; path: playwright/specs/ not e2e/)
- [x] `web/playwright/specs/dashboard-live-update.spec.ts` — inject synthetic measurement, KPIs change without page refresh (Plan 10)
- [x] `web/playwright/specs/dashboard-capability-filter.spec.ts` — water-only install hides electricity sections (Plan 10)
- [x] `web/playwright/specs/dashboard-date-range.spec.ts` — pick range, chart re-renders (Plan 10)
- [x] `web/playwright/specs/metering-point-detail.spec.ts` — normal view shows; advanced toggle reveals JSONB fields (Plan 10)
- [x] `web/playwright/specs/dashboard-mobile.spec.ts` — viewport 375×667, KPI cards stack, charts retain width (Plan 10)

### Frameworks to install (if absent)
- [x] `testcontainers-go` — confirmed in `go.mod` (used by trigger_test.go integration tests)
- [x] `@playwright/test` — confirmed in `web/package.json` v1.59.1; browsers installed via pnpm exec playwright install

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Dashboard remains usable on a real mobile device (not just emulated viewport) | DASH-03 | Touch-target sizing, real Safari/Chrome mobile behavior with SSE reconnection during tab backgrounding cannot be reliably exercised via Playwright | Open dashboard on iPhone Safari and Android Chrome over LTE; background the tab for 60s; foreground; confirm KPIs resume updating within 30s |
| Caddy/nginx reverse-proxy buffering does not break SSE in production deploy | DASH-02 | Requires the actual production reverse-proxy config; integration test runs without it | Deploy to a staging instance; `curl -N https://staging/api/events/dashboard` and confirm events stream without buffering |
| Visual polish of KPI cards, sparklines, and chart axes meets UI-SPEC.md | DASH-01..06, DETL-01..03 | Pixel-level visual quality is a `/gsd-ui-review` concern, not unit-testable | Run `/gsd-ui-review 4` after execution |

---

## Validation Sign-Off

- [x] All tasks have `<automated_verify>` blocks or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references (listener, trigger, SSE handler, dashboard handler, metering-point handler, three migrations, useSSE hook, dashboard/metering-point components, six E2E specs)
- [x] No watch-mode flags in any test command (CI must complete and exit)
- [x] Feedback latency < 30s for quick suite, < 5min for full suite
- [x] `nyquist_compliant: true` set in frontmatter — all rows green

**Approval:** Plan 04-10 phase-closure reconciliation — 2026-05-11
