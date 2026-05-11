---
phase: 4
slug: realtime-dashboard
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-05-11
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
| **Quick run command** | Backend: `go test ./internal/events/... ./internal/handlers/... -count=1` · Frontend: `cd web && pnpm test -- --run` |
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

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| _Populated by planner from PLAN.md `<automated_verify>` blocks_ | — | — | — | — | — | — | — | — | ⬜ pending |

> **Planner instruction:** When writing PLAN.md files, every task with code changes must declare an `<automated_verify>` block. Update this table during execution; each row maps a task ID to the command that proves the task's acceptance criteria.

---

## Wave 0 Requirements

Greenfield for Phase 4 — these test scaffolds must land before functional code:

### Backend (Go)
- [ ] `internal/events/listener_test.go` — LISTEN connection lifecycle, reconnect on disconnect, NOTIFY → channel fan-out
- [ ] `internal/events/trigger_test.go` — **Integration test pinning TimescaleDB AFTER INSERT trigger propagation to chunks** (load-bearing — see RESEARCH §Pitfalls)
- [ ] `internal/handlers/sse_test.go` — heartbeat cadence, `Cache-Control: no-cache`, `Content-Type: text/event-stream`, context cancellation flushes cleanly
- [ ] `internal/handlers/dashboard_test.go` — KPI endpoint shape (water-only / electricity-only / mixed), capability filter applied
- [ ] `internal/handlers/meteringpoint_test.go` — detail endpoint (normal vs JSONB extra), uplink-log pagination
- [ ] `internal/db/migrations/0021_measurement_trigger_test.go` — migration up/down idempotent
- [ ] `internal/db/migrations/0022_install_capabilities_test.go` — migration up/down idempotent
- [ ] `internal/db/migrations/0023_device_profile_interval_test.go` — migration up/down idempotent

### Frontend (Vitest)
- [ ] `web/src/hooks/useSSE.test.ts` — exponential backoff math (`min(30s, 0.5s * 2^n) + rand(0, 1s)`), reconnect triggers snapshot refetch, cookie-credentialed connection
- [ ] `web/src/routes/dashboard.test.tsx` — renders KPIs by capability, hides absent capability sections
- [ ] `web/src/components/dashboard/KpiCard.test.tsx` — formatting, online/offline pill states
- [ ] `web/src/components/dashboard/ConsumptionChart.test.tsx` — date-range picker triggers refetch with new bucket size (hour vs day rule)
- [ ] `web/src/components/metering-point/NormalView.test.tsx` — cumulative / instantaneous / last-update / alarms render
- [ ] `web/src/components/metering-point/AdvancedView.test.tsx` — walks JSONB `extra` keys, renders definition list
- [ ] `web/src/components/metering-point/UplinkLog.test.tsx` — virtualized list, 500-row cap, decoded vs raw toggle

### Frontend (Playwright E2E)
- [ ] `web/e2e/dashboard-loads.spec.ts` — login → dashboard renders, KPIs visible
- [ ] `web/e2e/dashboard-live-update.spec.ts` — inject synthetic measurement, KPIs change without page refresh
- [ ] `web/e2e/dashboard-capability-filter.spec.ts` — water-only install hides electricity sections
- [ ] `web/e2e/dashboard-date-range.spec.ts` — pick range, chart re-renders
- [ ] `web/e2e/metering-point-detail.spec.ts` — normal view shows; advanced toggle reveals JSONB fields
- [ ] `web/e2e/dashboard-mobile.spec.ts` — viewport 375×667, KPI cards stack, charts retain width

### Frameworks to install (if absent)
- [ ] `testcontainers-go` — confirm available in `go.mod` (Phase 3 likely added it; verify in planning)
- [ ] `@playwright/test` — confirm in `web/package.json`; install browsers via `pnpm exec playwright install` on first run

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Dashboard remains usable on a real mobile device (not just emulated viewport) | DASH-03 | Touch-target sizing, real Safari/Chrome mobile behavior with SSE reconnection during tab backgrounding cannot be reliably exercised via Playwright | Open dashboard on iPhone Safari and Android Chrome over LTE; background the tab for 60s; foreground; confirm KPIs resume updating within 30s |
| Caddy/nginx reverse-proxy buffering does not break SSE in production deploy | DASH-02 | Requires the actual production reverse-proxy config; integration test runs without it | Deploy to a staging instance; `curl -N https://staging/api/events/dashboard` and confirm events stream without buffering |
| Visual polish of KPI cards, sparklines, and chart axes meets UI-SPEC.md | DASH-01..06, DETL-01..03 | Pixel-level visual quality is a `/gsd-ui-review` concern, not unit-testable | Run `/gsd-ui-review 4` after execution |

---

## Validation Sign-Off

- [ ] All tasks have `<automated_verify>` blocks or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references (listener, trigger, SSE handler, dashboard handler, metering-point handler, three migrations, useSSE hook, dashboard/metering-point components, six E2E specs)
- [ ] No watch-mode flags in any test command (CI must complete and exit)
- [ ] Feedback latency < 30s for quick suite, < 5min for full suite
- [ ] `nyquist_compliant: true` set in frontmatter once all checkboxes pass

**Approval:** pending
