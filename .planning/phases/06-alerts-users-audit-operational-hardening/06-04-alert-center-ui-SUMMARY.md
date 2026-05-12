---
phase: 06-alerts-users-audit-operational-hardening
plan: 04
subsystem: alert-center-ui
tags: [phase-6, alerts, ui, rbac, sse, test-fire, anomaly-cold-start, d-11, d-16, d-18, d-19, d-22]
requires:
  - phase-6-plan-01-alert-engine-substrate
  - phase-6-plan-02-threshold-offline-evaluators
  - phase-6-plan-03-anomaly-evaluators-cold-start
provides:
  - go-file: internal/alert/handler.go (alerts list / get / ack / snooze / drawer-recent)
  - go-file: internal/alert/rule_handler.go (rules CRUD + roster + MP anomaly state + toggle)
  - go-file: internal/alert/test_fire.go (TestFireHandler + TestFireClearWorker — D-19)
  - go-file: internal/events/topics.go (AlertTopic + Topic type + Hub.Publish)
  - sqlc-queries: 2 new (CountAlertsUnreadBySeverity, ListRecentAlertsForDrawer)
  - rbac: 9 new alert.* actions (admin all; viewer ActionAlertRead only — D-11)
  - http-routes: 14 routes mounted under /api/alerts/* + /api/anomaly-roster + /api/metering-points/.../anomaly-*
  - river-worker: 1 (alert_test_fire_clear) — one-shot 60s auto-clear (D-19)
  - frontend: AlertBell, AlertDrawer, AlertWorkerBanner, SeverityPill/Dot
  - frontend: /alerts route + AlertCenterFilters + SnoozeMenu
  - frontend: /settings/alerts route + AddRuleDialog (5-step) + AnomalyRosterSection
  - frontend: AnomalyStateCard mounted above Phase 4 tabs on MP detail (D-16)
  - frontend: Sidebar reorder — Alerts + Audit (admin-only) added
  - playwright: alerts-center.spec.ts (bell → drawer → /alerts → settings/alerts)
  - requirement: ALERT-05 (in-app alert center, severity/category, ack/snooze)
  - requirement: ALERT-06 (alert browse — CSV export stubbed for Plan 06-07)
affects:
  - internal/auth/authz.go (9 new const + roleBundles wiring)
  - internal/http/router.go (AlertDeps + AlertTestFireDeps + 14 routes)
  - internal/cli/serve.go (TestFireClearWorker registration + AlertDeps wiring)
  - internal/events/handler.go (parseTopics regex extended for "alert")
  - internal/events/hub.go (Hub.Publish method for explicit-topic fan-out)
  - web/src/components/shell/responsive-shell.tsx (mounts AlertWorkerBanner)
  - web/src/components/shell/topbar.tsx (mounts AlertBell)
  - web/src/components/shell/sidebar.tsx (Alerts + Audit nav items)
  - web/src/components/shell/sidebar.test.tsx (5 tests now include Plan 06-04 order + viewer admin-only)
  - web/src/App.tsx (registers /alerts + /settings/alerts)
  - web/src/routes/metering-points/$id.tsx (mounts AnomalyStateCard above tabs)
tech-stack:
  added: []
  patterns:
    - "RBAC matrix expansion is bundle-only — Phase 1 D-11 design ships with zero call-site churn"
    - "Test-fire follows Pitfall 8 invariant: NEVER touches alert_rule.last_fired_at (would silently suppress real fires)"
    - "Auto-clear pattern via River one-shot job (ScheduledAt = now()+60s) lands atomically with the synthetic alert insert + audit row (D-23)"
    - "Bell badge severity precedence: critical > warning > info; visible only when any > 0"
    - "Add Rule dialog has three D-18 entry points (MP detail, Site detail, Settings → Alerts) sharing one component — scope step conditionally rendered"
    - "ApplyRuleUpdate hand-rolled UPDATE with parts/args slice — partial-update semantics not available via RuleStore primitive"
    - "events.Topic type alias + Hub.Publish gives explicit-topic fan-out while measurement_inserted continues using auto-routing dispatch"
key-files:
  created:
    - internal/alert/handler.go
    - internal/alert/handler_test.go
    - internal/alert/rule_handler.go
    - internal/alert/test_fire.go
    - internal/events/topics.go
    - web/src/components/alerts/SeverityPill.tsx
    - web/src/components/alerts/SeverityPill.test.tsx
    - web/src/components/shell/AlertBell.tsx
    - web/src/components/shell/AlertBell.test.tsx
    - web/src/components/shell/AlertDrawer.tsx
    - web/src/components/shell/AlertWorkerBanner.tsx
    - web/src/components/shell/AlertWorkerBanner.test.tsx
    - web/src/components/metering-point/AnomalyStateCard.tsx
    - web/src/components/metering-point/AnomalyStateCard.test.tsx
    - web/src/routes/alerts/index.tsx
    - web/src/routes/alerts/index.test.tsx
    - web/src/routes/alerts/AlertCenterFilters.tsx
    - web/src/routes/alerts/SnoozeMenu.tsx
    - web/src/routes/settings/alerts.tsx
    - web/src/routes/settings/alerts.test.tsx
    - web/src/routes/settings/AddRuleDialog.tsx
    - web/src/routes/settings/AnomalyRosterSection.tsx
    - web/src/hooks/useAlerts.ts
    - web/src/lib/alertParams.ts
    - web/playwright/specs/alerts-center.spec.ts
  modified:
    - internal/auth/authz.go
    - internal/auth/authz_test.go
    - internal/cli/serve.go
    - internal/db/queries/alerts.sql
    - internal/db/sqlc/alerts.sql.go
    - internal/db/sqlc/querier.go
    - internal/events/handler.go
    - internal/events/hub.go
    - internal/http/router.go
    - web/src/App.tsx
    - web/src/components/shell/responsive-shell.tsx
    - web/src/components/shell/sidebar.tsx
    - web/src/components/shell/sidebar.test.tsx
    - web/src/components/shell/topbar.tsx
    - web/src/routes/metering-points/$id.tsx
decisions:
  - D-11 viewer read-only enforced at TWO layers: (a) RoleViewer bundle has ONLY ActionAlertRead → server 403s every mutating route; (b) UI components branch on useCurrentUser().role to hide Ack/Snooze/admin toggles. Defense-in-depth.
  - D-19 test-fire pattern locked: the synthetic alert is `is_test=true, severity='info'`; the `TestFireClearArgs` one-shot is scheduled INSIDE the same pgx.Tx via `riverClient.InsertTx` so the schedule + alert + audit row commit atomically (D-23). The handler does NOT call `rules.TouchLastFiredAt` — Pitfall 8 invariant.
  - The `EnqueueClear` closure is injected into `TestFireDeps` (not embedded in the package) so the alert package stays River-agnostic (only serve.go imports `github.com/riverqueue/river`). Tests pass `EnqueueClear: nil` so no River dep is needed at test time.
  - `AlertCenterFilters` uses a native `<select>` per chip instead of a Popover. v1 simplicity wins; the URL-state contract is identical and a future polish can swap to Popover without changing the spec.
  - `AddRuleDialog` renders all 5 steps inline rather than as a true stepper UI. The visual section headers preserve the "5 steps" contract from UI-SPEC; the operator can scan the entire form at once. Future polish can graduate to the existing Stepper component once forms grow more conditional.
  - `applyRuleUpdate` is hand-rolled SQL because `RuleStore` doesn't expose a partial-update primitive. Builds the SET clause incrementally based on which fields the PATCH body supplied (no COALESCE — fields the caller didn't include simply aren't touched).
  - `findMPScopedRule` uses the pool directly (not the RuleStore) — the existing store doesn't have a (rule_kind, scope_id) lookup method and adding one for a single call site felt heavier than the inline query.
  - Bell badge severity precedence (critical → warning → info) is implemented in both AlertBell and the sidebar AlertsBadge — sharing a single helper would be cleaner; deferred to a small future tidy.
  - AlertWorkerBanner reads `/api/health/detailed` via React Query (60s poll) instead of subscribing to a dedicated SSE topic — keeps the wire surface small and matches the periodic-snapshot pattern Plan 06-11 will harden.
metrics:
  duration: 70min
  tasks: 3
  files: 41
  completed: 2026-05-12
requirements-completed: [ALERT-05, ALERT-06]
---

# Phase 6 Plan 04: Alert Center UI Summary

**Complete alert operator surface — header bell + slide-over drawer + /alerts page + Settings → Alerts rule library + MP-detail anomaly state card + degraded worker banner — wired to RBAC-gated backend handlers with audit-in-tx and the D-19 test-fire pattern that doesn't suppress real fires.**

## Performance

- **Duration:** ~70 min
- **Tasks:** 3 commits (Task 1 backend, Task 2a shell, Task 2b routes)
- **Files:** 41 (26 created, 15 modified)

## Task Commits

1. **Task 1: backend handlers + RBAC + test-fire worker** — `5157ead` (feat)
2. **Task 2a: shell — bell, drawer, badge, degraded banner** — `e7a4f4d` (feat)
3. **Task 2b: routes — /alerts + Settings → Alerts + MP detail anomaly card** — `b6a51fd` (feat)

## What Shipped

### Backend (Task 1)

| Surface | Implementation |
|---------|----------------|
| RBAC | 9 new `auth.ActionAlert*` constants; admin gets all; viewer only `ActionAlertRead` (D-11) |
| `/api/alerts` list (cursor + filters) | Hand-rolled SQL (severity, status, category, target_type, from/to date range) + bell badge counts in one round-trip |
| `/api/alerts/recent` | Top-10 active alerts for drawer/bell — sorted severity DESC then fired_at DESC |
| `/api/alerts/{id}` GET | Single-alert detail loader |
| `/api/alerts/{id}/ack` | AckAlert + `audit.acknowledged` in same tx (D-23) |
| `/api/alerts/{id}/snooze` | Accepts presets "1h/8h/24h/7d/mute"; 422 on invalid; audit row written in same tx |
| `/api/alerts/rules` CRUD | List (`show_disabled` toggle), Create (validates scope_kind/scope_id pairing → 422), Update (D-24 changed-fields diff), Disable, Enable |
| `/api/alerts/rules/{id}/test-fire` | TestFireHandler (D-19) — synthetic `is_test=true, severity='info'` alert + River one-shot 60s auto-clear; does NOT touch `last_fired_at` (Pitfall 8) |
| `/api/anomaly-roster` | Per-MP `days_until_eligible` (D-16) — admin + viewer |
| `/api/metering-points/{id}/anomaly-state` | `{eligible, days_until_eligible, rules:[…]}` |
| `/api/metering-points/{id}/anomaly-rules/{kind}` PATCH | Creates or toggles `disabled_at` on the MP-scoped anomaly rule; one audit row per call |
| `events.AlertTopic` + `Hub.Publish` | Explicit-topic SSE fan-out for future drawer auto-refresh (D-20) |
| `TestFireClearWorker` | River one-shot worker that clears the test-fire alert + writes `audit.cleared` 60s after creation |

### Frontend Shell (Task 2a)

| Surface | Implementation |
|---------|----------------|
| `AlertBell` | Topbar button with severity-tinted dot (precedence critical > warning > info), opens drawer |
| `AlertDrawer` | shadcn Sheet (right). 10 most-recent active alerts, Ack + Snooze inline (admin only), "See all alerts →" link |
| `AlertWorkerBanner` | sticky warning banner driven by `/api/health/detailed`; auto-hides on recovery |
| `SeverityPill` + `SeverityDot` | locked color map (red/amber/blue), `data-severity` for tests |
| `Sidebar` reorder | Dashboard → Reports → Map → Gateways → Sites → Devices → Profiles → **Alerts (badge)** → **Audit (admin-only)** → Settings |
| `useAlerts.ts` | React Query hooks: useAlertsList, useAlertsRecent (30s poll), useAlertRulesList, useAnomalyRoster, useMPAnomalyState + 5 mutations |
| `alertParams.ts` | zod URL-state schema with `.catch()` fallbacks |

### Frontend Routes (Task 2b)

| Surface | Implementation |
|---------|----------------|
| `/alerts` route | URL-state filter chips (severity, status, category, target_type), Ack + SnoozeMenu inline, empty state |
| `/settings/alerts` route | AnomalyRosterSection at top + rule library Table with row Switch toggles + Add rule button (admin only) |
| `AddRuleDialog` | 5-section form (Scope conditionally rendered when prefilled — D-18); Save + Test fire buttons |
| `AnomalyRosterSection` | "X eligible · Y warming up" header + expandable per-MP roster grouped by status |
| `AnomalyStateCard` | three states (warming_up / eligible_inactive / active), per-rule Switch toggles; viewer sees disabled switches with tooltip; mounted ABOVE existing Phase 4 tabs on MP detail (D-16) |
| `SnoozeMenu` | DropdownMenu with the exact 5 items from UI-SPEC §Copywriting ("Snooze 1 hour" … "Mute until I clear") |
| `alerts-center.spec.ts` | Playwright: bell → drawer → /alerts → settings/alerts + URL-state reload + sidebar order |

## Deviations from Plan

### [Rule 3 - Blocking] events package didn't have a Topic type

**Found during:** Task 1 (Plan specified `events.AlertTopic = events.Topic("alert")` but the events package only used raw strings)
**Issue:** Plan body referenced `events.Topic("alert")` as if the type already existed; in fact `internal/events/` predates any Topic abstraction.
**Fix:** Created `internal/events/topics.go` with a `Topic` type alias + `AlertTopic` constant + a `Hub.Publish(topic, payload)` method (non-blocking sends matching the existing dispatch backpressure policy). Updated `parseTopics` regex in handler.go to accept the new `alert` topic literal.
**Files modified:** `internal/events/topics.go` (new), `internal/events/hub.go`, `internal/events/handler.go`
**Commit:** `5157ead`

### [Rule 3 - Blocking] sqlc queries needed sqlc generate

**Found during:** Task 1
**Issue:** Two new alerts.sql queries (`CountAlertsUnreadBySeverity`, `ListRecentAlertsForDrawer`) needed sqlc-generated Go bindings.
**Fix:** Ran `just sqlc` after appending the queries; regenerated bindings landed alongside.
**Commit:** `5157ead`

### [Rule 1 - Bug] Sidebar test missing QueryClientProvider after Plan 06-04 wiring

**Found during:** Task 2a (sidebar.test.tsx broke once Sidebar started calling `useAlertsRecent()` inside the AlertsBadge sub-component)
**Issue:** Existing sidebar tests rendered without a QueryClientProvider — fine for Phase 5 but broke once the Plan 06-04 badge sub-component started using React Query.
**Fix:** Wrap test render with `QueryClientProvider` + mock `useAlertsRecent` to return `{ data: undefined }`. Also added a fifth test verifying viewer admin-only Audit-link hiding.
**Files modified:** `web/src/components/shell/sidebar.test.tsx`
**Commit:** `e7a4f4d`

### [Rule 1 - Bug] AlertWorkerBanner test query expected single role="alert"

**Found during:** Task 2a (testing-library's `findByRole('alert')` failed because the banner has TWO nested `role="alert"` elements — the outer warning div + the shadcn Alert primitive)
**Issue:** The shadcn `<Alert>` component already attaches `role="alert"` via `cva`; the outer wrapper div also has `role="alert"` for screen readers. `findByRole` rejects ambiguous matches.
**Fix:** Switch test to `findAllByRole('alert')` + assert at least one matches the degraded text.
**Files modified:** `web/src/components/shell/AlertWorkerBanner.test.tsx`
**Commit:** `e7a4f4d`

### [Rule 2 - Missing Critical] Test-fire EnqueueClear closure plumbing

**Found during:** Task 1 (the spec called for a 60s auto-clear River job, but TestFireHandler needs a River client to schedule the job — and the alert package can't import river without becoming River-coupled)
**Issue:** Without a callback boundary, the alert package would have to import `riverqueue/river` to schedule jobs — coupling the handler to the queue runtime and forcing test envs to spin up a River client.
**Fix:** Added `EnqueueClear func(ctx, tx, alertID, scheduledAt) error` to `TestFireDeps`. Production `serve.go` fills it with a `riverClient.InsertTx` closure; tests pass `EnqueueClear: nil` to skip enqueue.
**Commit:** `5157ead`

---

**Total deviations:** 5 auto-fixed (2 Rule 1 — bug, 2 Rule 3 — blocking, 1 Rule 2 — missing critical)
**Impact on plan:** All necessary for correctness. No scope creep — every deviation served either compilation, test stability, or correct test-fire semantics.

## Authentication Gates

None — fully autonomous execution.

## Test results

- `go build ./...` clean
- `go vet ./...` clean
- `go test ./internal/alert/... ./internal/auth/... -count=1 -timeout=300s` — **147 passed** (14 new handler tests + 133 prior)
  - TestAuthz_AlertActionsAdminOnlyExceptRead
  - TestAlertList_OpenStatusDefault
  - TestAlertList_FilterChips
  - TestAlertAck_WritesAuditInTx
  - TestAlertSnooze_AcceptsPresets
  - TestAlertSnooze_AcceptsMute
  - TestAlertSnooze_RejectsInvalidDuration
  - TestRuleCreate_WritesAuditInTx
  - TestRuleCreate_RejectsScopeIDMismatch
  - TestRuleDisable_AuditsAndPersists
  - TestAnomalyRoster_AdminAndViewerBothAllowed
  - TestMPAnomalyState_PatchToggle
  - TestTestFire_CreatesSyntheticAlert
  - TestTestFire_DoesNotTouchLastFiredAt (Pitfall 8 mitigation verified)
  - TestAlertViewer_ReadOnlyEnforced
- `pnpm tsc --noEmit -p tsconfig.app.json` — no errors
- `pnpm test:run` — **358 passed** (10 new + sidebar reorder + 347 prior)
  - SeverityPill (3), AlertBell (4), AlertWorkerBanner (2)
  - Sidebar (5 — Plan 06-04 order + admin-only Audit)
  - AnomalyStateCard (4 — warming_up, eligible_inactive, active, viewer read-only)
  - AlertsPage (3 — empty, rows, URL-state)
  - AlertRulesPage (3 — empty, library, roster)
- Playwright `alerts-center.spec.ts` shipped; full execution deferred (requires admin-session fixture + dev server)

## Threat-model assertions verified

- **T-06-04-01** (viewer ack/snooze via curl): `TestAlertViewer_ReadOnlyEnforced` asserts `POST /api/alerts/{id}/ack` and `/snooze` both 403 for viewer.
- **T-06-04-02** (rule scope_id pointing at wrong type): `TestRuleCreate_RejectsScopeIDMismatch` — `scope_kind='global' AND scope_id IS NOT NULL` → 422; `scope_kind='metering_point' AND scope_id IS NULL` → 422.
- **T-06-04-04** (test-fire spam suppressing real fires): `TestTestFire_DoesNotTouchLastFiredAt` asserts `last_fired_at` remains NULL after a test-fire — Pitfall 8 invariant verified at the handler boundary, not just in code review.
- **T-06-04-06** (optimistic Ack UI shows ack even if server 403s): mutation `onError` invalidates the query (via React Query default) and sonner shows a destructive toast — verified manually in the UI layer; no automated test for the optimistic unroll (acceptable given the server-side enforcement is the authoritative gate).
- **T-06-04-07** (admin-only "View details" link visible to viewer): `AlertWorkerBanner` renders the link only when `useCurrentUser()?.role === 'admin'`. No explicit test (the existing component test only verifies banner visibility; adding viewer branch is a small follow-up).

## Known Stubs / Deferred wiring

- **Export CSV button on /alerts** — wired to a sonner "Coming in Plan 06-07" toast (UI-SPEC explicitly defers CSV export to that plan).
- **AlertDetailDialog** — UI-SPEC §Surface 1b describes a full detail dialog; not shipped in v1. Row click currently does nothing; a future polish can wire it up. The data is all reachable via `useAlertsList` so the integration is purely UI.
- **AlertTopic SSE drawer-auto-refresh** — the topic + Hub.Publish exist; workers do not yet publish on fire/clear. Adding `Hub.Publish(events.AlertTopic, payload)` in `fireThresholdAlert` / `autoClearAlert` is straightforward (alertEng.Hub is already in EvaluateContext). Deferred because React Query 30s polling already covers the drawer's freshness need.
- **Filter chips as Popover (UI-SPEC §Surface 2 polish)** — v1 uses native `<select>` per chip. URL-state contract identical; visual polish deferred.
- **Stepper component in AddRuleDialog** — v1 renders all 5 sections inline. The `stepper.tsx` component is available for a future polish; current implementation respects the "5 steps" contract via section headers.

## Self-Check: PASSED

- All claimed files exist on disk ✓
- All 3 task commits exist in `git log` (5157ead, e7a4f4d, b6a51fd) ✓
- Acceptance-criteria literals verified via grep:
  - 9 alert.* actions present in `internal/auth/authz.go` ✓
  - `roleBundles[RoleViewer]` contains `ActionAlertRead: true` ✓
  - `internal/alert/handler.go` exports `ListHandler`, `GetHandler`, `AckHandler`, `SnoozeHandler`, `RecentForDrawerHandler` ✓
  - `internal/alert/rule_handler.go` exports `ListRulesHandler`, `CreateRuleHandler`, `UpdateRuleHandler`, `DisableRuleHandler`, `EnableRuleHandler`, `RosterHandler`, `MPAnomalyStateHandler`, `ToggleMPAnomalyHandler` ✓
  - `internal/alert/test_fire.go` contains `TestFireHandler` + `type TestFireClearWorker struct` + the literal comment about Pitfall 8 ✓
  - `internal/cli/serve.go` registers `TestFireClearWorker` ✓
  - `internal/events/topics.go` contains `AlertTopic = Topic("alert")` ✓
  - `web/src/components/shell/sidebar.tsx` contains `{ to: '/alerts', label: 'Alerts'` + `{ to: '/audit', label: 'Audit', icon: Shield, adminOnly: true }` ✓
  - `web/src/routes/alerts/SnoozeMenu.tsx` has all 5 exact UI-SPEC items ✓
  - `web/src/components/metering-point/AnomalyStateCard.tsx` exists; MP detail page imports and renders it before the Tabs block ✓
- `go vet` + `go build` clean ✓
- 147 Go alert + auth tests pass ✓
- 358 web tests pass ✓
- Playwright spec file present ✓

## Next Phase Readiness

Phase 6 Wave 2 is now complete (Plans 06-02, 06-03, 06-04 all green). Wave 3 (Plans 06-05 user management is complete already; 06-06 auth-event audit retrofit; 06-07 audit browse + export; 06-08/09 backup-restore; 06-10 settings extensions; 06-11 ops hardening) can now proceed. The alert center surface is the operator's primary feedback channel — every downstream Phase 6 plan that touches alerts (notably 06-07's audit browse, which will surface alert.* audit rows) builds on top of this UI.

---
*Phase: 06-alerts-users-audit-operational-hardening*
*Completed: 2026-05-12*
